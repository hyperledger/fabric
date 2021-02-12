/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"os"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type rollbackMgr struct {
	ledgerID       string
	ledgerDir      string
	indexDir       string
	dbProvider     *leveldbhelper.Provider
	indexStore     *blockIndex
	targetBlockNum uint64
	reusableBatch  *leveldbhelper.UpdateBatch
}

// Rollback reverts changes made to the block store beyond a given block number.
func Rollback(blockStorageDir, ledgerID string, targetBlockNum uint64, indexConfig *IndexConfig) error {
	r, err := newRollbackMgr(blockStorageDir, ledgerID, indexConfig, targetBlockNum)
	if err != nil {
		return err
	}
	defer r.dbProvider.Close()

	if err := recordHeightIfGreaterThanPreviousRecording(r.ledgerDir); err != nil {
		return err
	}

	logger.Infof("Rolling back block index to block number [%d]", targetBlockNum)
	if err := r.rollbackBlockIndex(); err != nil {
		return err
	}

	logger.Infof("Rolling back block files to block number [%d]", targetBlockNum)
	if err := r.rollbackBlockFiles(); err != nil {
		return err
	}

	return nil
}

func newRollbackMgr(blockStorageDir, ledgerID string, indexConfig *IndexConfig, targetBlockNum uint64) (*rollbackMgr, error) {
	r := &rollbackMgr{}

	r.ledgerID = ledgerID
	conf := &Conf{blockStorageDir: blockStorageDir}
	r.ledgerDir = conf.getLedgerBlockDir(ledgerID)
	r.targetBlockNum = targetBlockNum

	r.indexDir = conf.getIndexDir()
	var err error
	r.dbProvider, err = leveldbhelper.NewProvider(
		&leveldbhelper.Conf{
			DBPath:         r.indexDir,
			ExpectedFormat: dataFormatVersion(indexConfig),
		},
	)
	if err != nil {
		return nil, err
	}
	indexDB := r.dbProvider.GetDBHandle(ledgerID)
	r.indexStore, err = newBlockIndex(indexConfig, indexDB)
	r.reusableBatch = r.indexStore.db.NewUpdateBatch()
	return r, err
}

func (r *rollbackMgr) rollbackBlockIndex() error {
	lastBlockNumber, err := r.indexStore.getLastBlockIndexed()
	if err == errIndexSavePointKeyNotPresent {
		return nil
	}
	if err != nil {
		return err
	}

	// we remove index associated with only 10 blocks at a time
	// to avoid overuse of memory occupied by the leveldb batch.
	// If we assume a block size of 2000 transactions and 4 indices
	// per transaction and 2 index per block, the total number of
	// index keys to be added in the batch would be 80020. Even if a
	// key takes 100 bytes (overestimation), the memory utilization
	// of a batch of 10 blocks would be 7 MB only.
	batchLimit := uint64(10)

	// start each iteration of the loop with full range for deletion
	// and shrink the range to batchLimit if the range is greater than batchLimit
	start, end := r.targetBlockNum+1, lastBlockNumber
	for end >= start {
		if end-start >= batchLimit {
			start = end - batchLimit + 1 // 1 is added as range is inclusive
		}
		logger.Infof("Deleting index associated with block number [%d] to [%d]", start, end)
		if err := r.deleteIndexEntriesRange(start, end); err != nil {
			return err
		}
		start, end = r.targetBlockNum+1, start-1
	}
	return nil
}

func (r *rollbackMgr) deleteIndexEntriesRange(startBlkNum, endBlkNum uint64) error {
	// TODO: when more than half of the blocks' indices are to be deleted, it
	// might be efficient to drop the whole index database rather than deleting
	// entries. However, if there is more than more than 1 channel, dropping of
	// index would impact the time taken to recover the peer. We need to analyze
	// a bit before making a decision on rollback vs drop of index. FAB-15672
	r.reusableBatch.Reset()
	lp, err := r.indexStore.getBlockLocByBlockNum(startBlkNum)
	if err != nil {
		return err
	}

	stream, err := newBlockStream(r.ledgerDir, lp.fileSuffixNum, int64(lp.offset), -1)
	if err != nil {
		return err
	}
	defer stream.close()

	numberOfBlocksToRetrieve := endBlkNum - startBlkNum + 1
	for numberOfBlocksToRetrieve > 0 {
		blockBytes, _, err := stream.nextBlockBytesAndPlacementInfo()
		if err != nil {
			return err
		}
		blockInfo, err := extractSerializedBlockInfo(blockBytes)
		if err != nil {
			return err
		}
		addIndexEntriesToBeDeleted(r.reusableBatch, blockInfo, r.indexStore)
		numberOfBlocksToRetrieve--
	}

	r.reusableBatch.Put(indexSavePointKey, encodeBlockNum(startBlkNum-1))
	return r.indexStore.db.WriteBatch(r.reusableBatch, true)
}

func addIndexEntriesToBeDeleted(batch *leveldbhelper.UpdateBatch, blockInfo *serializedBlockInfo, indexStore *blockIndex) error {
	if indexStore.isAttributeIndexed(IndexableAttrBlockHash) {
		batch.Delete(constructBlockHashKey(protoutil.BlockHeaderHash(blockInfo.blockHeader)))
	}

	if indexStore.isAttributeIndexed(IndexableAttrBlockNum) {
		batch.Delete(constructBlockNumKey(blockInfo.blockHeader.Number))
	}

	if indexStore.isAttributeIndexed(IndexableAttrBlockNumTranNum) {
		for txIndex := range blockInfo.txOffsets {
			batch.Delete(constructBlockNumTranNumKey(blockInfo.blockHeader.Number, uint64(txIndex)))
		}
	}

	if indexStore.isAttributeIndexed(IndexableAttrTxID) {
		for i, txOffset := range blockInfo.txOffsets {
			batch.Delete(constructTxIDKey(txOffset.txID, blockInfo.blockHeader.Number, uint64(i)))
		}
	}
	return nil
}

func (r *rollbackMgr) rollbackBlockFiles() error {
	logger.Infof("Deleting blockfilesInfo")
	if err := r.indexStore.db.Delete(blkMgrInfoKey, true); err != nil {
		return err
	}
	// must not use index for block location search since the index can be behind the target block
	targetFileNum, err := binarySearchFileNumForBlock(r.ledgerDir, r.targetBlockNum)
	if err != nil {
		return err
	}
	lastFileNum, err := retrieveLastFileSuffix(r.ledgerDir)
	if err != nil {
		return err
	}

	logger.Infof("Removing all block files with suffixNum in the range [%d] to [%d]",
		targetFileNum+1, lastFileNum)

	for n := lastFileNum; n >= targetFileNum+1; n-- {
		filepath := deriveBlockfilePath(r.ledgerDir, n)
		if err := os.Remove(filepath); err != nil {
			return errors.Wrapf(err, "error removing the block file [%s]", filepath)
		}
	}

	logger.Infof("Truncating block file [%d] to the end boundary of block number [%d]", targetFileNum, r.targetBlockNum)
	endOffset, err := calculateEndOffSet(r.ledgerDir, targetFileNum, r.targetBlockNum)
	if err != nil {
		return err
	}

	filePath := deriveBlockfilePath(r.ledgerDir, targetFileNum)
	if err := os.Truncate(filePath, endOffset); err != nil {
		return errors.Wrapf(err, "error trucating the block file [%s]", filePath)
	}

	return nil
}

func calculateEndOffSet(ledgerDir string, targetBlkFileNum int, blockNum uint64) (int64, error) {
	stream, err := newBlockfileStream(ledgerDir, targetBlkFileNum, 0)
	if err != nil {
		return 0, err
	}
	defer stream.close()
	for {
		blockBytes, err := stream.nextBlockBytes()
		if err != nil {
			return 0, err
		}
		blockInfo, err := extractSerializedBlockInfo(blockBytes)
		if err != nil {
			return 0, err
		}
		if blockInfo.blockHeader.Number == blockNum {
			break
		}
	}
	return stream.currentOffset, nil
}

// ValidateRollbackParams performs necessary validation on the input given for
// the rollback operation.
func ValidateRollbackParams(blockStorageDir, ledgerID string, targetBlockNum uint64) error {
	logger.Infof("Validating the rollback parameters: ledgerID [%s], block number [%d]",
		ledgerID, targetBlockNum)
	conf := &Conf{blockStorageDir: blockStorageDir}
	ledgerDir := conf.getLedgerBlockDir(ledgerID)
	if err := validateLedgerID(ledgerDir, ledgerID); err != nil {
		return err
	}
	if err := validateTargetBlkNum(ledgerDir, targetBlockNum); err != nil {
		return err
	}
	return nil
}

func validateLedgerID(ledgerDir, ledgerID string) error {
	logger.Debugf("Validating the existence of ledgerID [%s]", ledgerID)
	exists, err := fileutil.DirExists(ledgerDir)
	if err != nil {
		return err
	}
	if !exists {
		return errors.Errorf("ledgerID [%s] does not exist", ledgerID)
	}
	return nil
}

func validateTargetBlkNum(ledgerDir string, targetBlockNum uint64) error {
	logger.Debugf("Validating the given block number [%d] against the ledger block height", targetBlockNum)
	blkfilesInfo, err := constructBlockfilesInfo(ledgerDir)
	if err != nil {
		return err
	}
	if blkfilesInfo.lastPersistedBlock <= targetBlockNum {
		return errors.Errorf("target block number [%d] should be less than the biggest block number [%d]",
			targetBlockNum, blkfilesInfo.lastPersistedBlock)
	}
	return nil
}
