/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	blockfilePrefix                   = "blockfile_"
	bootstrappingSnapshotInfoFile     = "bootstrappingSnapshot.info"
	bootstrappingSnapshotInfoTempFile = "bootstrappingSnapshotTemp.info"
)

var blkMgrInfoKey = []byte("blkMgrInfo")

type blockfileMgr struct {
	rootDir                   string
	conf                      *Conf
	db                        *leveldbhelper.DBHandle
	index                     *blockIndex
	blockfilesInfo            *blockfilesInfo
	bootstrappingSnapshotInfo *BootstrappingSnapshotInfo
	blkfilesInfoCond          *sync.Cond
	currentFileWriter         *blockfileWriter
	bcInfo                    atomic.Value
}

/*
Creates a new manager that will manage the files used for block persistence.
This manager manages the file system FS including

	-- the directory where the files are stored
	-- the individual files where the blocks are stored
	-- the blockfilesInfo which tracks the latest file being persisted to
	-- the index which tracks what block and transaction is in what file

When a new blockfile manager is started (i.e. only on start-up), it checks
if this start-up is the first time the system is coming up or is this a restart
of the system.

The blockfile manager stores blocks of data into a file system.  That file
storage is done by creating sequentially numbered files of a configured size
i.e blockfile_000000, blockfile_000001, etc..

Each transaction in a block is stored with information about the number of
bytes in that transaction

	Adding txLoc [fileSuffixNum=0, offset=3, bytesLength=104] for tx [1:0] to index
	Adding txLoc [fileSuffixNum=0, offset=107, bytesLength=104] for tx [1:1] to index

Each block is stored with the total encoded length of that block as well as the
tx location offsets.

Remember that these steps are only done once at start-up of the system.
At start up a new manager:

	  *) Checks if the directory for storing files exists, if not creates the dir
	  *) Checks if the key value database exists, if not creates one
	       (will create a db dir)
	  *) Determines the blockfilesInfo used for storage
			-- Loads from db if exist, if not instantiate a new blockfilesInfo
			-- If blockfilesInfo was loaded from db, compares to FS
			-- If blockfilesInfo and file system are not in sync, syncs blockfilesInfo from FS
	  *) Starts a new file writer
			-- truncates file per blockfilesInfo to remove any excess past last block
	  *) Determines the index information used to find tx and blocks in
	  the file blkstorage
			-- Instantiates a new blockIdxInfo
			-- Loads the index from the db if exists
			-- syncIndex comparing the last block indexed to what is in the FS
			-- If index and file system are not in sync, syncs index from the FS
	  *)  Updates blockchain info used by the APIs
*/
func newBlockfileMgr(id string, conf *Conf, indexConfig *IndexConfig, indexStore *leveldbhelper.DBHandle) (*blockfileMgr, error) {
	logger.Debugf("newBlockfileMgr() initializing file-based block storage for ledger: %s ", id)
	rootDir := conf.getLedgerBlockDir(id)
	_, err := fileutil.CreateDirIfMissing(rootDir)
	if err != nil {
		panic(fmt.Sprintf("Error creating block storage root dir [%s]: %s", rootDir, err))
	}
	mgr := &blockfileMgr{rootDir: rootDir, conf: conf, db: indexStore}

	blockfilesInfo, err := mgr.loadBlkfilesInfo()
	if err != nil {
		panic(fmt.Sprintf("Could not get block file info for current block file from db: %s", err))
	}
	if blockfilesInfo == nil {
		logger.Info(`Getting block information from block storage`)
		if blockfilesInfo, err = constructBlockfilesInfo(rootDir); err != nil {
			panic(fmt.Sprintf("Could not build blockfilesInfo info from block files: %s", err))
		}
		logger.Debugf("Info constructed by scanning the blocks dir = %s", spew.Sdump(blockfilesInfo))
	} else {
		logger.Debug(`Synching block information from block storage (if needed)`)
		syncBlockfilesInfoFromFS(rootDir, blockfilesInfo)
	}
	err = mgr.saveBlkfilesInfo(blockfilesInfo, true)
	if err != nil {
		panic(fmt.Sprintf("Could not save next block file info to db: %s", err))
	}

	currentFileWriter, err := newBlockfileWriter(deriveBlockfilePath(rootDir, blockfilesInfo.latestFileNumber))
	if err != nil {
		panic(fmt.Sprintf("Could not open writer to current file: %s", err))
	}
	err = currentFileWriter.truncateFile(blockfilesInfo.latestFileSize)
	if err != nil {
		panic(fmt.Sprintf("Could not truncate current file to known size in db: %s", err))
	}
	if mgr.index, err = newBlockIndex(indexConfig, indexStore); err != nil {
		panic(fmt.Sprintf("error in block index: %s", err))
	}

	mgr.blockfilesInfo = blockfilesInfo
	bsi, err := loadBootstrappingSnapshotInfo(rootDir)
	if err != nil {
		return nil, err
	}
	mgr.bootstrappingSnapshotInfo = bsi
	mgr.currentFileWriter = currentFileWriter
	mgr.blkfilesInfoCond = sync.NewCond(&sync.Mutex{})

	if err := mgr.syncIndex(); err != nil {
		return nil, err
	}

	bcInfo := &common.BlockchainInfo{}

	if mgr.bootstrappingSnapshotInfo != nil {
		bcInfo.Height = mgr.bootstrappingSnapshotInfo.LastBlockNum + 1
		bcInfo.CurrentBlockHash = mgr.bootstrappingSnapshotInfo.LastBlockHash
		bcInfo.PreviousBlockHash = mgr.bootstrappingSnapshotInfo.PreviousBlockHash
		bcInfo.BootstrappingSnapshotInfo = &common.BootstrappingSnapshotInfo{}
		bcInfo.BootstrappingSnapshotInfo.LastBlockInSnapshot = mgr.bootstrappingSnapshotInfo.LastBlockNum
	}

	if !blockfilesInfo.noBlockFiles {
		lastBlockHeader, err := mgr.retrieveBlockHeaderByNumber(blockfilesInfo.lastPersistedBlock)
		if err != nil {
			panic(fmt.Sprintf("Could not retrieve header of the last block form file: %s", err))
		}
		// update bcInfo with lastPersistedBlock
		bcInfo.Height = blockfilesInfo.lastPersistedBlock + 1
		bcInfo.CurrentBlockHash = protoutil.BlockHeaderHash(lastBlockHeader)
		bcInfo.PreviousBlockHash = lastBlockHeader.PreviousHash
	}
	mgr.bcInfo.Store(bcInfo)
	return mgr, nil
}

func bootstrapFromSnapshottedTxIDs(
	ledgerID string,
	snapshotDir string,
	snapshotInfo *SnapshotInfo,
	conf *Conf,
	indexStore *leveldbhelper.DBHandle,
) error {
	rootDir := conf.getLedgerBlockDir(ledgerID)
	isEmpty, err := fileutil.CreateDirIfMissing(rootDir)
	if err != nil {
		return err
	}
	if !isEmpty {
		return errors.Errorf("dir %s not empty", rootDir)
	}

	bsi := &BootstrappingSnapshotInfo{
		LastBlockNum:      snapshotInfo.LastBlockNum,
		LastBlockHash:     snapshotInfo.LastBlockHash,
		PreviousBlockHash: snapshotInfo.PreviousBlockHash,
	}

	bsiBytes, err := proto.Marshal(bsi)
	if err != nil {
		return err
	}

	if err := fileutil.CreateAndSyncFileAtomically(
		rootDir,
		bootstrappingSnapshotInfoTempFile,
		bootstrappingSnapshotInfoFile,
		bsiBytes,
		0o644,
	); err != nil {
		return err
	}
	if err := fileutil.SyncDir(rootDir); err != nil {
		return err
	}
	if err := importTxIDsFromSnapshot(snapshotDir, snapshotInfo.LastBlockNum, indexStore); err != nil {
		return err
	}
	return nil
}

func syncBlockfilesInfoFromFS(rootDir string, blkfilesInfo *blockfilesInfo) {
	logger.Debugf("Starting blockfilesInfo=%s", blkfilesInfo)
	// Checks if the file suffix of where the last block was written exists
	filePath := deriveBlockfilePath(rootDir, blkfilesInfo.latestFileNumber)
	exists, size, err := fileutil.FileExists(filePath)
	if err != nil {
		panic(fmt.Sprintf("Error in checking whether file [%s] exists: %s", filePath, err))
	}
	logger.Debugf("status of file [%s]: exists=[%t], size=[%d]", filePath, exists, size)
	// Test is !exists because when file number is first used the file does not exist yet
	// checks that the file exists and that the size of the file is what is stored in blockfilesInfo
	// status of file [<blkstorage_location>/blocks/blockfile_000000]: exists=[false], size=[0]
	if !exists || int(size) == blkfilesInfo.latestFileSize {
		// blockfilesInfo is in sync with the file on disk
		return
	}
	// Scan the file system to verify that the blockfilesInfo stored in db is correct
	_, endOffsetLastBlock, numBlocks, err := scanForLastCompleteBlock(
		rootDir, blkfilesInfo.latestFileNumber, int64(blkfilesInfo.latestFileSize))
	if err != nil {
		panic(fmt.Sprintf("Could not open current file for detecting last block in the file: %s", err))
	}
	blkfilesInfo.latestFileSize = int(endOffsetLastBlock)
	if numBlocks == 0 {
		return
	}
	// Updates the blockfilesInfo for the actual last block number stored and it's end location
	if blkfilesInfo.noBlockFiles {
		blkfilesInfo.lastPersistedBlock = uint64(numBlocks - 1)
	} else {
		blkfilesInfo.lastPersistedBlock += uint64(numBlocks)
	}
	blkfilesInfo.noBlockFiles = false
	logger.Debugf("blockfilesInfo after updates by scanning the last file segment:%s", blkfilesInfo)
}

func deriveBlockfilePath(rootDir string, suffixNum int) string {
	return rootDir + "/" + blockfilePrefix + fmt.Sprintf("%06d", suffixNum)
}

func (mgr *blockfileMgr) close() {
	mgr.currentFileWriter.close()
}

func (mgr *blockfileMgr) moveToNextFile() {
	blkfilesInfo := &blockfilesInfo{
		latestFileNumber:   mgr.blockfilesInfo.latestFileNumber + 1,
		latestFileSize:     0,
		lastPersistedBlock: mgr.blockfilesInfo.lastPersistedBlock,
	}

	nextFileWriter, err := newBlockfileWriter(
		deriveBlockfilePath(mgr.rootDir, blkfilesInfo.latestFileNumber))
	if err != nil {
		panic(fmt.Sprintf("Could not open writer to next file: %s", err))
	}
	mgr.currentFileWriter.close()
	err = mgr.saveBlkfilesInfo(blkfilesInfo, true)
	if err != nil {
		panic(fmt.Sprintf("Could not save next block file info to db: %s", err))
	}
	mgr.currentFileWriter = nextFileWriter
	mgr.updateBlockfilesInfo(blkfilesInfo)
}

func (mgr *blockfileMgr) addBlock(block *common.Block) error {
	bcInfo := mgr.getBlockchainInfo()
	if block.Header.Number != bcInfo.Height {
		return errors.Errorf(
			"block number should have been %d but was %d",
			mgr.getBlockchainInfo().Height, block.Header.Number,
		)
	}

	// Add the previous hash check - Though, not essential but may not be a bad idea to
	// verify the field `block.Header.PreviousHash` present in the block.
	// This check is a simple bytes comparison and hence does not cause any observable performance penalty
	// and may help in detecting a rare scenario if there is any bug in the ordering service.
	if !bytes.Equal(block.Header.PreviousHash, bcInfo.CurrentBlockHash) {
		return errors.Errorf(
			"unexpected Previous block hash. Expected PreviousHash = [%x], PreviousHash referred in the latest block= [%x]",
			bcInfo.CurrentBlockHash, block.Header.PreviousHash,
		)
	}
	blockBytes, info, err := serializeBlock(block)
	if err != nil {
		return errors.WithMessage(err, "error serializing block")
	}
	blockHash := protoutil.BlockHeaderHash(block.Header)
	// Get the location / offset where each transaction starts in the block and where the block ends
	txOffsets := info.txOffsets
	currentOffset := mgr.blockfilesInfo.latestFileSize

	blockBytesLen := len(blockBytes)
	blockBytesEncodedLen := proto.EncodeVarint(uint64(blockBytesLen))
	totalBytesToAppend := blockBytesLen + len(blockBytesEncodedLen)

	// Determine if we need to start a new file since the size of this block
	// exceeds the amount of space left in the current file
	if currentOffset+totalBytesToAppend > mgr.conf.maxBlockfileSize {
		mgr.moveToNextFile()
		currentOffset = 0
	}
	// append blockBytesEncodedLen to the file
	err = mgr.currentFileWriter.append(blockBytesEncodedLen, false)
	if err == nil {
		// append the actual block bytes to the file
		err = mgr.currentFileWriter.append(blockBytes, true)
	}
	if err != nil {
		truncateErr := mgr.currentFileWriter.truncateFile(mgr.blockfilesInfo.latestFileSize)
		if truncateErr != nil {
			panic(fmt.Sprintf("Could not truncate current file to known size after an error during block append: %s", err))
		}
		return errors.WithMessage(err, "error appending block to file")
	}

	// Update the blockfilesInfo with the results of adding the new block
	currentBlkfilesInfo := mgr.blockfilesInfo
	newBlkfilesInfo := &blockfilesInfo{
		latestFileNumber:   currentBlkfilesInfo.latestFileNumber,
		latestFileSize:     currentBlkfilesInfo.latestFileSize + totalBytesToAppend,
		noBlockFiles:       false,
		lastPersistedBlock: block.Header.Number,
	}
	// save the blockfilesInfo in the database
	if err = mgr.saveBlkfilesInfo(newBlkfilesInfo, false); err != nil {
		truncateErr := mgr.currentFileWriter.truncateFile(currentBlkfilesInfo.latestFileSize)
		if truncateErr != nil {
			panic(fmt.Sprintf("Error in truncating current file to known size after an error in saving blockfiles info: %s", err))
		}
		return errors.WithMessage(err, "error saving blockfiles file info to db")
	}

	// Index block file location pointer updated with file suffex and offset for the new block
	blockFLP := &fileLocPointer{fileSuffixNum: newBlkfilesInfo.latestFileNumber}
	blockFLP.offset = currentOffset
	// shift the txoffset because we prepend length of bytes before block bytes
	for _, txOffset := range txOffsets {
		txOffset.loc.offset += len(blockBytesEncodedLen)
	}
	// save the index in the database
	if err = mgr.index.indexBlock(&blockIdxInfo{
		blockNum: block.Header.Number, blockHash: blockHash,
		flp: blockFLP, txOffsets: txOffsets, metadata: block.Metadata,
	}); err != nil {
		return err
	}

	// update the blockfilesInfo (for storage) and the blockchain info (for APIs) in the manager
	mgr.updateBlockfilesInfo(newBlkfilesInfo)
	mgr.updateBlockchainInfo(blockHash, block)
	return nil
}

func (mgr *blockfileMgr) syncIndex() error {
	nextIndexableBlock := uint64(0)
	lastBlockIndexed, err := mgr.index.getLastBlockIndexed()
	if err != nil {
		if err != errIndexSavePointKeyNotPresent {
			return err
		}
	} else {
		nextIndexableBlock = lastBlockIndexed + 1
	}

	if nextIndexableBlock == 0 && mgr.bootstrappedFromSnapshot() {
		// This condition can happen only if there was a peer crash or failure during
		// bootstrapping the ledger from a snapshot or the index is dropped/corrupted afterward
		return errors.Errorf(
			"cannot sync index with block files. blockstore is bootstrapped from a snapshot and first available block=[%d]",
			mgr.firstPossibleBlockNumberInBlockFiles(),
		)
	}

	if mgr.blockfilesInfo.noBlockFiles {
		logger.Debug("No block files present. This happens when there has not been any blocks added to the ledger yet")
		return nil
	}

	if nextIndexableBlock == mgr.blockfilesInfo.lastPersistedBlock+1 {
		logger.Debug("Both the block files and indices are in sync.")
		return nil
	}

	startFileNum := 0
	startOffset := 0
	skipFirstBlock := false
	endFileNum := mgr.blockfilesInfo.latestFileNumber

	firstAvailableBlkNum, err := retrieveFirstBlockNumFromFile(mgr.rootDir, 0)
	if err != nil {
		return err
	}

	if nextIndexableBlock > firstAvailableBlkNum {
		logger.Debugf("Last block indexed [%d], Last block present in block files [%d]", lastBlockIndexed, mgr.blockfilesInfo.lastPersistedBlock)
		var flp *fileLocPointer
		if flp, err = mgr.index.getBlockLocByBlockNum(lastBlockIndexed); err != nil {
			return err
		}
		startFileNum = flp.fileSuffixNum
		startOffset = flp.locPointer.offset
		skipFirstBlock = true
	}

	logger.Infof("Start building index from block [%d] to last block [%d]", nextIndexableBlock, mgr.blockfilesInfo.lastPersistedBlock)

	// open a blockstream to the file location that was stored in the index
	var stream *blockStream
	if stream, err = newBlockStream(mgr.rootDir, startFileNum, int64(startOffset), endFileNum); err != nil {
		return err
	}
	var blockBytes []byte
	var blockPlacementInfo *blockPlacementInfo

	if skipFirstBlock {
		if blockBytes, _, err = stream.nextBlockBytesAndPlacementInfo(); err != nil {
			return err
		}
		if blockBytes == nil {
			return errors.Errorf("block bytes for block num = [%d] should not be nil here. The indexes for the block are already present",
				lastBlockIndexed)
		}
	}

	// Should be at the last block already, but go ahead and loop looking for next blockBytes.
	// If there is another block, add it to the index.
	// This will ensure block indexes are correct, for example if peer had crashed before indexes got updated.
	blockIdxInfo := &blockIdxInfo{}
	for {
		if blockBytes, blockPlacementInfo, err = stream.nextBlockBytesAndPlacementInfo(); err != nil {
			return err
		}
		if blockBytes == nil {
			break
		}
		info, err := extractSerializedBlockInfo(blockBytes)
		if err != nil {
			return err
		}

		// The blockStartOffset will get applied to the txOffsets prior to indexing within indexBlock(),
		// therefore just shift by the difference between blockBytesOffset and blockStartOffset
		numBytesToShift := int(blockPlacementInfo.blockBytesOffset - blockPlacementInfo.blockStartOffset)
		for _, offset := range info.txOffsets {
			offset.loc.offset += numBytesToShift
		}

		// Update the blockIndexInfo with what was actually stored in file system
		blockIdxInfo.blockHash = protoutil.BlockHeaderHash(info.blockHeader)
		blockIdxInfo.blockNum = info.blockHeader.Number
		blockIdxInfo.flp = &fileLocPointer{
			fileSuffixNum: blockPlacementInfo.fileNum,
			locPointer:    locPointer{offset: int(blockPlacementInfo.blockStartOffset)},
		}
		blockIdxInfo.txOffsets = info.txOffsets
		blockIdxInfo.metadata = info.metadata

		logger.Debugf("syncIndex() indexing block [%d]", blockIdxInfo.blockNum)
		if err = mgr.index.indexBlock(blockIdxInfo); err != nil {
			return err
		}
		if blockIdxInfo.blockNum%10000 == 0 {
			logger.Infof("Indexed block number [%d]", blockIdxInfo.blockNum)
		}
	}
	logger.Infof("Finished building index. Last block indexed [%d]", blockIdxInfo.blockNum)
	return nil
}

func (mgr *blockfileMgr) getBlockchainInfo() *common.BlockchainInfo {
	return mgr.bcInfo.Load().(*common.BlockchainInfo)
}

func (mgr *blockfileMgr) updateBlockfilesInfo(blkfilesInfo *blockfilesInfo) {
	mgr.blkfilesInfoCond.L.Lock()
	defer mgr.blkfilesInfoCond.L.Unlock()
	mgr.blockfilesInfo = blkfilesInfo
	logger.Debugf("Broadcasting about update blockfilesInfo: %s", blkfilesInfo)
	mgr.blkfilesInfoCond.Broadcast()
}

func (mgr *blockfileMgr) updateBlockchainInfo(latestBlockHash []byte, latestBlock *common.Block) {
	currentBCInfo := mgr.getBlockchainInfo()
	newBCInfo := &common.BlockchainInfo{
		Height:                    currentBCInfo.Height + 1,
		CurrentBlockHash:          latestBlockHash,
		PreviousBlockHash:         latestBlock.Header.PreviousHash,
		BootstrappingSnapshotInfo: currentBCInfo.BootstrappingSnapshotInfo,
	}

	mgr.bcInfo.Store(newBCInfo)
}

func (mgr *blockfileMgr) retrieveBlockByHash(blockHash []byte) (*common.Block, error) {
	logger.Debugf("retrieveBlockByHash() - blockHash = [%#v]", blockHash)
	loc, err := mgr.index.getBlockLocByHash(blockHash)
	if err != nil {
		return nil, err
	}
	return mgr.fetchBlock(loc)
}

func (mgr *blockfileMgr) retrieveBlockByNumber(blockNum uint64) (*common.Block, error) {
	logger.Debugf("retrieveBlockByNumber() - blockNum = [%d]", blockNum)

	// interpret math.MaxUint64 as a request for last block
	if blockNum == math.MaxUint64 {
		blockNum = mgr.getBlockchainInfo().Height - 1
	}
	if blockNum < mgr.firstPossibleBlockNumberInBlockFiles() {
		return nil, errors.Errorf(
			"cannot serve block [%d]. The ledger is bootstrapped from a snapshot. First available block = [%d]",
			blockNum, mgr.firstPossibleBlockNumberInBlockFiles(),
		)
	}
	loc, err := mgr.index.getBlockLocByBlockNum(blockNum)
	if err != nil {
		return nil, err
	}
	return mgr.fetchBlock(loc)
}

func (mgr *blockfileMgr) retrieveBlockByTxID(txID string) (*common.Block, error) {
	logger.Debugf("retrieveBlockByTxID() - txID = [%s]", txID)
	loc, err := mgr.index.getBlockLocByTxID(txID)
	if err == errNilValue {
		return nil, errors.Errorf(
			"details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
			txID, mgr.firstPossibleBlockNumberInBlockFiles())
	}
	if err != nil {
		return nil, err
	}
	return mgr.fetchBlock(loc)
}

func (mgr *blockfileMgr) retrieveTxValidationCodeByTxID(txID string) (peer.TxValidationCode, uint64, error) {
	logger.Debugf("retrieveTxValidationCodeByTxID() - txID = [%s]", txID)
	validationCode, blkNum, err := mgr.index.getTxValidationCodeByTxID(txID)
	if err == errNilValue {
		return peer.TxValidationCode(-1), 0, errors.Errorf(
			"details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
			txID, mgr.firstPossibleBlockNumberInBlockFiles())
	}
	return validationCode, blkNum, err
}

func (mgr *blockfileMgr) retrieveBlockHeaderByNumber(blockNum uint64) (*common.BlockHeader, error) {
	logger.Debugf("retrieveBlockHeaderByNumber() - blockNum = [%d]", blockNum)
	if blockNum < mgr.firstPossibleBlockNumberInBlockFiles() {
		return nil, errors.Errorf(
			"cannot serve block [%d]. The ledger is bootstrapped from a snapshot. First available block = [%d]",
			blockNum, mgr.firstPossibleBlockNumberInBlockFiles(),
		)
	}
	loc, err := mgr.index.getBlockLocByBlockNum(blockNum)
	if err != nil {
		return nil, err
	}
	blockBytes, err := mgr.fetchBlockBytes(loc)
	if err != nil {
		return nil, err
	}
	info, err := extractSerializedBlockInfo(blockBytes)
	if err != nil {
		return nil, err
	}
	return info.blockHeader, nil
}

func (mgr *blockfileMgr) retrieveBlocks(startNum uint64) (*blocksItr, error) {
	if startNum < mgr.firstPossibleBlockNumberInBlockFiles() {
		return nil, errors.Errorf(
			"cannot serve block [%d]. The ledger is bootstrapped from a snapshot. First available block = [%d]",
			startNum, mgr.firstPossibleBlockNumberInBlockFiles(),
		)
	}
	return newBlockItr(mgr, startNum), nil
}

func (mgr *blockfileMgr) txIDExists(txID string) (bool, error) {
	return mgr.index.txIDExists(txID)
}

func (mgr *blockfileMgr) retrieveTransactionByID(txID string) (*common.Envelope, error) {
	logger.Debugf("retrieveTransactionByID() - txId = [%s]", txID)
	loc, err := mgr.index.getTxLoc(txID)
	if err == errNilValue {
		return nil, errors.Errorf(
			"details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
			txID, mgr.firstPossibleBlockNumberInBlockFiles())
	}
	if err != nil {
		return nil, err
	}
	return mgr.fetchTransactionEnvelope(loc)
}

func (mgr *blockfileMgr) retrieveTransactionByBlockNumTranNum(blockNum uint64, tranNum uint64) (*common.Envelope, error) {
	logger.Debugf("retrieveTransactionByBlockNumTranNum() - blockNum = [%d], tranNum = [%d]", blockNum, tranNum)
	if blockNum < mgr.firstPossibleBlockNumberInBlockFiles() {
		return nil, errors.Errorf(
			"cannot serve block [%d]. The ledger is bootstrapped from a snapshot. First available block = [%d]",
			blockNum, mgr.firstPossibleBlockNumberInBlockFiles(),
		)
	}
	loc, err := mgr.index.getTXLocByBlockNumTranNum(blockNum, tranNum)
	if err != nil {
		return nil, err
	}
	return mgr.fetchTransactionEnvelope(loc)
}

func (mgr *blockfileMgr) fetchBlock(lp *fileLocPointer) (*common.Block, error) {
	blockBytes, err := mgr.fetchBlockBytes(lp)
	if err != nil {
		return nil, err
	}
	block, err := deserializeBlock(blockBytes)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (mgr *blockfileMgr) fetchTransactionEnvelope(lp *fileLocPointer) (*common.Envelope, error) {
	logger.Debugf("Entering fetchTransactionEnvelope() %v\n", lp)
	var err error
	var txEnvelopeBytes []byte
	if txEnvelopeBytes, err = mgr.fetchRawBytes(lp); err != nil {
		return nil, err
	}
	_, n := proto.DecodeVarint(txEnvelopeBytes)
	return protoutil.GetEnvelopeFromBlock(txEnvelopeBytes[n:])
}

func (mgr *blockfileMgr) fetchBlockBytes(lp *fileLocPointer) ([]byte, error) {
	stream, err := newBlockfileStream(mgr.rootDir, lp.fileSuffixNum, int64(lp.offset))
	if err != nil {
		return nil, err
	}
	defer stream.close()
	b, err := stream.nextBlockBytes()
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (mgr *blockfileMgr) fetchRawBytes(lp *fileLocPointer) ([]byte, error) {
	filePath := deriveBlockfilePath(mgr.rootDir, lp.fileSuffixNum)
	reader, err := newBlockfileReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	b, err := reader.read(lp.offset, lp.bytesLength)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Get the current blockfilesInfo information that is stored in the database
func (mgr *blockfileMgr) loadBlkfilesInfo() (*blockfilesInfo, error) {
	var b []byte
	var err error
	if b, err = mgr.db.Get(blkMgrInfoKey); b == nil || err != nil {
		return nil, err
	}
	i := &blockfilesInfo{}
	if err = i.unmarshal(b); err != nil {
		return nil, err
	}
	logger.Debugf("loaded blockfilesInfo:%s", i)
	return i, nil
}

func (mgr *blockfileMgr) saveBlkfilesInfo(i *blockfilesInfo, sync bool) error {
	b, err := i.marshal()
	if err != nil {
		return err
	}
	if err = mgr.db.Put(blkMgrInfoKey, b, sync); err != nil {
		return err
	}
	return nil
}

func (mgr *blockfileMgr) firstPossibleBlockNumberInBlockFiles() uint64 {
	if mgr.bootstrappingSnapshotInfo == nil {
		return 0
	}
	return mgr.bootstrappingSnapshotInfo.LastBlockNum + 1
}

func (mgr *blockfileMgr) bootstrappedFromSnapshot() bool {
	return mgr.firstPossibleBlockNumberInBlockFiles() > 0
}

// scanForLastCompleteBlock scan a given block file and detects the last offset in the file
// after which there may lie a block partially written (towards the end of the file in a crash scenario).
func scanForLastCompleteBlock(rootDir string, fileNum int, startingOffset int64) ([]byte, int64, int, error) {
	// scan the passed file number suffix starting from the passed offset to find the last completed block
	numBlocks := 0
	var lastBlockBytes []byte
	blockStream, errOpen := newBlockfileStream(rootDir, fileNum, startingOffset)
	if errOpen != nil {
		return nil, 0, 0, errOpen
	}
	defer blockStream.close()
	var errRead error
	var blockBytes []byte
	for {
		blockBytes, errRead = blockStream.nextBlockBytes()
		if blockBytes == nil || errRead != nil {
			break
		}
		lastBlockBytes = blockBytes
		numBlocks++
	}
	if errRead == ErrUnexpectedEndOfBlockfile {
		logger.Debugf(`Error:%s
		The error may happen if a crash has happened during block appending.
		Resetting error to nil and returning current offset as a last complete block's end offset`, errRead)
		errRead = nil
	}
	logger.Debugf("scanForLastCompleteBlock(): last complete block ends at offset=[%d]", blockStream.currentOffset)
	return lastBlockBytes, blockStream.currentOffset, numBlocks, errRead
}

// blockfilesInfo maintains the summary about the blockfiles
type blockfilesInfo struct {
	latestFileNumber   int
	latestFileSize     int
	noBlockFiles       bool
	lastPersistedBlock uint64
}

func (i *blockfilesInfo) marshal() ([]byte, error) {
	buffer := proto.NewBuffer([]byte{})
	var err error
	if err = buffer.EncodeVarint(uint64(i.latestFileNumber)); err != nil {
		return nil, errors.Wrapf(err, "error encoding the latestFileNumber [%d]", i.latestFileNumber)
	}
	if err = buffer.EncodeVarint(uint64(i.latestFileSize)); err != nil {
		return nil, errors.Wrapf(err, "error encoding the latestFileSize [%d]", i.latestFileSize)
	}
	if err = buffer.EncodeVarint(i.lastPersistedBlock); err != nil {
		return nil, errors.Wrapf(err, "error encoding the lastPersistedBlock [%d]", i.lastPersistedBlock)
	}
	var noBlockFilesMarker uint64
	if i.noBlockFiles {
		noBlockFilesMarker = 1
	}
	if err = buffer.EncodeVarint(noBlockFilesMarker); err != nil {
		return nil, errors.Wrapf(err, "error encoding noBlockFiles [%d]", noBlockFilesMarker)
	}
	return buffer.Bytes(), nil
}

func (i *blockfilesInfo) unmarshal(b []byte) error {
	buffer := proto.NewBuffer(b)
	var val uint64
	var noBlockFilesMarker uint64
	var err error

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.latestFileNumber = int(val)

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.latestFileSize = int(val)

	if val, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.lastPersistedBlock = val
	if noBlockFilesMarker, err = buffer.DecodeVarint(); err != nil {
		return err
	}
	i.noBlockFiles = noBlockFilesMarker == 1
	return nil
}

func (i *blockfilesInfo) String() string {
	return fmt.Sprintf("latestFileNumber=[%d], latestFileSize=[%d], noBlockFiles=[%t], lastPersistedBlock=[%d]",
		i.latestFileNumber, i.latestFileSize, i.noBlockFiles, i.lastPersistedBlock)
}
