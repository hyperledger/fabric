/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestRollback(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 50) // 50 blocks persisted in ~5 block files
	blocksPerFile := 50 / 5
	env := newTestEnv(t, NewConf(path, 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	// 1. Store blocks
	for i, b := range blocks {
		assert.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  blocks[49].Header.Hash(),
		PreviousBlockHash: blocks[48].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(49)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.Equal(t, actualCheckpointInfo.latestFileChunkSuffixNum, 4)

	// 4. Check whether all blocks are stored correctly
	blkfileMgrWrapper.testGetBlockByNumber(blocks, 0, nil)
	blkfileMgrWrapper.testGetBlockByHash(blocks, nil)
	blkfileMgrWrapper.testGetBlockByTxID(blocks, nil)

	// 5. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()
	lastBlockNumberInLastFile := uint64(49)
	middleBlockNumberInLastFile := uint64(45)
	firstBlockNumberInLastFile := uint64(41)

	// 7. Rollback to one before the lastBlockNumberInLastFile
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", lastBlockNumberInLastFile-uint64(1), indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, lastBlockNumberInLastFile-uint64(1), 4, indexConfig)

	// 8. Rollback to middleBlockNumberInLastFile
	err = Rollback(path, "testLedger", middleBlockNumberInLastFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, middleBlockNumberInLastFile, 4, indexConfig)

	// 9. Rollback to firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, firstBlockNumberInLastFile, 4, indexConfig)

	// 10. Rollback to one before the firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile-1, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, firstBlockNumberInLastFile-1, 3, indexConfig)

	// 11. In the middle block file (among a range of block files), find the middle block number
	middleBlockNumberInMiddleFile := uint64(25)

	// 12. Rollback to middleBlockNumberInMiddleFile
	err = Rollback(path, "testLedger", middleBlockNumberInMiddleFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, middleBlockNumberInMiddleFile, 2, indexConfig)

	// 13. Rollback to block 5
	err = Rollback(path, "testLedger", 5, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 5, 0, indexConfig)

	// 14. Rollback to block 1
	err = Rollback(path, "testLedger", 1, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 1, 0, indexConfig)
}

// TestRollbackWithOnlyBlockIndexAttributes mimics the scenario when ledger is used for orderer
// i.e., only block is index and transancations are not indexed
func TestRollbackWithOnlyBlockIndexAttributes(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 50) // 50 blocks persisted in ~5 block files
	blocksPerFile := 50 / 5
	onlyBlockNumIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockNum,
	}
	env := newTestEnvSelectiveIndexing(t, NewConf(path, 0), onlyBlockNumIndex, &disabled.Provider{})
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	// 1. Store blocks
	for i, b := range blocks {
		assert.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  blocks[49].Header.Hash(),
		PreviousBlockHash: blocks[48].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(49)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.Equal(t, actualCheckpointInfo.latestFileChunkSuffixNum, 4)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Rollback to block 2
	onlyBlockNumIndexCfg := &blkstorage.IndexConfig{
		AttrsToIndex: onlyBlockNumIndex,
	}
	err = Rollback(path, "testLedger", 2, onlyBlockNumIndexCfg)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 2, 0, onlyBlockNumIndexCfg)
}

func TestRollbackWithNoIndexDir(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 50)
	blocksPerFile := 50 / 5
	conf := NewConf(path, 0)
	env := newTestEnv(t, conf)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	// 1. Store blocks
	for i, b := range blocks {
		assert.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  blocks[49].Header.Hash(),
		PreviousBlockHash: blocks[48].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(49)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.Equal(t, actualCheckpointInfo.latestFileChunkSuffixNum, 4)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Remove the index directory
	indexDir := conf.getIndexDir()
	err = os.RemoveAll(indexDir)
	assert.NoError(t, err)

	// 6. Rollback to block 2
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", 2, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 2, 0, indexConfig)
}

func TestValidateRollbackParams(t *testing.T) {
	path := testPath()
	env := newTestEnv(t, NewConf(path, 1024*24))
	defer env.Cleanup()

	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Create 10 blocks
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Valid inputs
	err := ValidateRollbackParams(path, "testLedger", 5)
	assert.NoError(t, err)

	// 3. ledgerID does not exist
	err = ValidateRollbackParams(path, "noLedger", 5)
	assert.Equal(t, "ledgerID [noLedger] does not exist", err.Error())

	err = ValidateRollbackParams(path, "testLedger", 15)
	assert.Equal(t, "target block number [15] should be less than the biggest block number [9]", err.Error())
}

func TestDuplicateTxIDDuringRollback(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 4)
	maxFileSize := 1024 * 1024 * 4
	env := newTestEnv(t, NewConf(path, maxFileSize))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blocks[3].Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][0] = byte(peer.TxValidationCode_DUPLICATE_TXID)
	testutil.SetTxID(t, blocks[3], 0, "tx0")
	testutil.SetTxID(t, blocks[2], 0, "tx0")

	// 1. Store blocks
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            4,
		CurrentBlockHash:  blocks[3].Header.Hash(),
		PreviousBlockHash: blocks[2].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Retrieve tx
	blkfileMgrWrapper.testGetTransactionByTxID("tx0", blocks[2].Data.Data[0], nil)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Rollback to block 2
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err := Rollback(path, "testLedger", 2, indexConfig)
	assert.NoError(t, err)

	env = newTestEnv(t, NewConf(path, maxFileSize))
	blkfileMgrWrapper = newTestBlockfileWrapper(env, "testLedger")

	// 6. Check the BlockchainInfo
	expectedBlockchainInfo = &common.BlockchainInfo{
		Height:            3,
		CurrentBlockHash:  blocks[2].Header.Hash(),
		PreviousBlockHash: blocks[1].Header.Hash(),
	}
	actualBlockchainInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 8. Retrieve tx (should not have been deleted)
	blkfileMgrWrapper.testGetTransactionByTxID("tx0", blocks[2].Data.Data[0], nil)
}

func TestRollbackTxIDMissingFromIndex(t *testing.T) {
	// Polpulate block store with four blocks
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 4)
	testutil.SetTxID(t, blocks[3], 0, "blk3_tx0")

	env := newTestEnv(t, NewConf(path, 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	blkfileMgrWrapper.addBlocks(blocks)
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            4,
		CurrentBlockHash:  blocks[3].Header.Hash(),
		PreviousBlockHash: blocks[2].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)
	blkfileMgrWrapper.testGetTransactionByTxID("blk3_tx0", blocks[3].Data.Data[0], nil)

	// Delete index entries for a tx
	testutilDeleteTxFromIndex(t, blkfileMgrWrapper.blockfileMgr.db, 3, 0, "blk3_tx0")
	env.provider.Close()
	blkfileMgrWrapper.close()

	// Rollback to block 2
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err := Rollback(path, "testLedger", 2, indexConfig)
	assert.NoError(t, err)

	// Reopen blockstorage and assert basic functionality
	env = newTestEnv(t, NewConf(path, 0))
	blkfileMgrWrapper = newTestBlockfileWrapper(env, "testLedger")
	expectedBlockchainInfo = &common.BlockchainInfo{
		Height:            3,
		CurrentBlockHash:  blocks[2].Header.Hash(),
		PreviousBlockHash: blocks[1].Header.Hash(),
	}
	actualBlockchainInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)
}

func assertBlockStoreRollback(t *testing.T, path, ledgerID string, blocks []*common.Block,
	rollbackedToBlkNum uint64, lastFileSuffixNum int, indexConfig *blkstorage.IndexConfig) {

	env := newTestEnvSelectiveIndexing(t, NewConf(path, 0), indexConfig.AttrsToIndex, &disabled.Provider{})
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerID)

	// 1. Check the BlockchainInfo after the rollback
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            rollbackedToBlkNum + 1,
		CurrentBlockHash:  blocks[rollbackedToBlkNum].Header.Hash(),
		PreviousBlockHash: blocks[rollbackedToBlkNum-1].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 2. Check the checkpointInfo after the rollback
	expectedCheckpointInfoLastBlockNumber := rollbackedToBlkNum
	expectedCheckpointInfoIsChainEmpty := false
	expectedBlockchainInfoLastFileSuffixNum := lastFileSuffixNum
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.Equal(t, expectedBlockchainInfoLastFileSuffixNum, actualCheckpointInfo.latestFileChunkSuffixNum)

	// 3. Check whether all blocks till the target block number are stored correctly
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(blkstorage.IndexableAttrBlockNum) {
		blkfileMgrWrapper.testGetBlockByNumber(blocks[:rollbackedToBlkNum+1], 0, nil)
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(blkstorage.IndexableAttrBlockHash) {
		blkfileMgrWrapper.testGetBlockByHash(blocks[:rollbackedToBlkNum+1], nil)
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(blkstorage.IndexableAttrTxID) {
		blkfileMgrWrapper.testGetBlockByTxID(blocks[:rollbackedToBlkNum+1], nil)
	}

	// 4. Check whether all blocks with number greater than target block number
	// are removed including index entries
	expectedErr := errors.New("Entry not found in index")
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(blkstorage.IndexableAttrBlockHash) {
		blkfileMgrWrapper.testGetBlockByHash(blocks[rollbackedToBlkNum+1:], expectedErr)
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(blkstorage.IndexableAttrTxID) {
		blkfileMgrWrapper.testGetBlockByTxID(blocks[rollbackedToBlkNum+1:], expectedErr)
	}

	// 5. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()
}

func testutilDeleteTxFromIndex(t *testing.T, db *leveldbhelper.DBHandle, blkNum, txNum uint64, txID string) {
	indexKeys := [][]byte{
		constructTxIDKey(txID),
		constructBlockNumTranNumKey(blkNum, txNum),
		constructBlockTxIDKey(txID),
		constructTxValidationCodeIDKey(txID),
	}

	batch := leveldbhelper.NewUpdateBatch()
	for _, key := range indexKeys {
		value, err := db.Get(key)
		assert.NoError(t, err)
		assert.NotNil(t, value)
		batch.Delete(key)
	}
	db.WriteBatch(batch, true)

	for _, key := range indexKeys {
		value, err := db.Get(key)
		assert.NoError(t, err)
		assert.Nil(t, value)
	}
}
