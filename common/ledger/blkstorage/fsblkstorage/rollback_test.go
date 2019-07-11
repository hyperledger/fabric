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
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestRollback(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 1000)
	// assumption, except the genesis block, the size of all other blocks would be same.
	// we would not get into a scenario where the block size is greater than the
	// maxFileSize.
	maxFileSize := int(0.2 * float64(testutilEstimateTotalSizeOnDisk(t, blocks)))
	// maxFileSize is set such that there would be at least 5 block files.
	env := newTestEnv(t, NewConf(path, maxFileSize))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Store blocks
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            1000,
		CurrentBlockHash:  blocks[999].Header.Hash(),
		PreviousBlockHash: blocks[998].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(999)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.True(t, actualCheckpointInfo.latestFileChunkSuffixNum >= 5)
	// as there is a possibility of leaving out the last few bytes in each file,
	// we might have either 5 or more block files given that we assign 1/5th of the
	// total size of blocks as the maxFileSie.

	// 4. Check whether all blocks are stored correctly
	blkfileMgrWrapper.testGetBlockByNumber(blocks, 0, nil)
	blkfileMgrWrapper.testGetBlockByHash(blocks, nil)
	blkfileMgrWrapper.testGetBlockByTxID(blocks, nil)

	// 5. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 6. In the last block file, find the first, middle, and lastBlockNumber
	ledgerDir := (&Conf{blockStorageDir: path}).getLedgerBlockDir("testLedger")
	_, _, numBlocksInLastFile, err := scanForLastCompleteBlock(ledgerDir, actualCheckpointInfo.latestFileChunkSuffixNum, 0)
	assert.NoError(t, err)
	lastFileSuffixNum := actualCheckpointInfo.latestFileChunkSuffixNum
	lastBlockNumberInLastFile := uint64(999)
	middleBlockNumberInLastFile := uint64(999 - (numBlocksInLastFile / 2))
	firstBlockNumberInLastFile := uint64(999 - numBlocksInLastFile + 1)

	// 7. Rollback to one before the lastBlockNumberInLastFile
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", lastBlockNumberInLastFile-uint64(1), indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, lastBlockNumberInLastFile-uint64(1), lastFileSuffixNum, indexConfig)

	// 8. Rollback to middleBlockNumberInLastFile
	err = Rollback(path, "testLedger", middleBlockNumberInLastFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, middleBlockNumberInLastFile, lastFileSuffixNum, indexConfig)

	// 9. Rollback to firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, firstBlockNumberInLastFile, lastFileSuffixNum, indexConfig)

	// 10. Rollback to one before the firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile-1, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, firstBlockNumberInLastFile-1, lastFileSuffixNum-1, indexConfig)

	// 11. In the middle block file (among a range of block files), find the middle block number
	blockBytes, _, numBlocks, err := scanForLastCompleteBlock(ledgerDir, lastFileSuffixNum/2, 0)
	assert.NoError(t, err)
	blockInfo, err := extractSerializedBlockInfo(blockBytes)
	assert.NoError(t, err)
	middleBlockNumberInMiddleFile := blockInfo.blockHeader.Number - uint64(numBlocks/2)

	// 12. Rollback to middleBlockNumberInMiddleFile
	err = Rollback(path, "testLedger", middleBlockNumberInMiddleFile, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, middleBlockNumberInMiddleFile, lastFileSuffixNum/2, indexConfig)

	// 13. Rollback to block 5
	err = Rollback(path, "testLedger", 5, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, 5, 0, indexConfig)

	// 13. Rollback to block 1
	err = Rollback(path, "testLedger", 1, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, 1, 0, indexConfig)
}

// TestRollbackWithOnlyBlockIndexAttributes mimics the scenario when ledger is used for orderer
// i.e., only block is index and transancations are not indexed
func TestRollbackWithOnlyBlockIndexAttributes(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 100)
	// assumption, except the genesis block, the size of all other blocks would be same.
	// we would not get into a scenario where the block size is greater than the
	// maxFileSize.
	maxFileSize := int(0.2 * float64(testutilEstimateTotalSizeOnDisk(t, blocks)))
	// maxFileSize is set such that there would be at least 5 block files.

	onlyBlockNumIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockNum,
	}

	env := newTestEnvSelectiveIndexing(t, NewConf(path, maxFileSize), onlyBlockNumIndex, &disabled.Provider{})
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Store blocks
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            100,
		CurrentBlockHash:  blocks[99].Header.Hash(),
		PreviousBlockHash: blocks[98].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(99)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.True(t, actualCheckpointInfo.latestFileChunkSuffixNum >= 5)
	// as there is a possibility of leaving out the last few bytes in each file,
	// we might have either 5 or more block files given that we assign 1/5th of the
	// total size of blocks as the maxFileSie.

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Rollback to block 2
	onlyBlockNumIndexCfg := &blkstorage.IndexConfig{
		AttrsToIndex: onlyBlockNumIndex,
	}
	err = Rollback(path, "testLedger", 2, onlyBlockNumIndexCfg)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, 2, 0, onlyBlockNumIndexCfg)
}

func TestRollbackWithNoIndexDir(t *testing.T) {
	path := testPath()
	blocks := testutil.ConstructTestBlocks(t, 100)
	// assumption, except the genesis block, the size of all other blocks would be same.
	// we would not get into a scenario where the block size is greater than the
	// maxFileSize.
	maxFileSize := int(0.2 * float64(testutilEstimateTotalSizeOnDisk(t, blocks)))
	// maxFileSize is set such that there would be at least 5 block files.
	conf := NewConf(path, maxFileSize)
	env := newTestEnv(t, conf)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Store blocks
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            100,
		CurrentBlockHash:  blocks[99].Header.Hash(),
		PreviousBlockHash: blocks[98].Header.Hash(),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the checkpointInfo
	expectedCheckpointInfoLastBlockNumber := uint64(99)
	expectedCheckpointInfoIsChainEmpty := false
	actualCheckpointInfo, err := blkfileMgrWrapper.blockfileMgr.loadCurrentInfo()
	assert.NoError(t, err)
	assert.Equal(t, expectedCheckpointInfoLastBlockNumber, actualCheckpointInfo.lastBlockNumber)
	assert.Equal(t, expectedCheckpointInfoIsChainEmpty, actualCheckpointInfo.isChainEmpty)
	assert.True(t, actualCheckpointInfo.latestFileChunkSuffixNum >= 5)
	// as there is a possibility of leaving out the last few bytes in each file,
	// we might have either 5 or more block files given that we assign 1/5th of the
	// total size of blocks as the maxFileSie.

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Remove the index directory
	indexDir := conf.getIndexDir()
	err = os.RemoveAll(indexDir)
	assert.NoError(t, err)

	// 5. Rollback to block 2
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", 2, indexConfig)
	assert.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", maxFileSize, blocks, 2, 0, indexConfig)
}

func TestValidateRollbackParams(t *testing.T) {
	path := testPath()
	env := newTestEnv(t, NewConf(path, 1024*24))
	defer env.Cleanup()

	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Create 100 blocks
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

func assertBlockStoreRollback(t *testing.T, path, ledgerID string, maxFileSize int, blocks []*common.Block,
	rollbackedToBlkNum uint64, lastFileSuffixNum int, indexConfig *blkstorage.IndexConfig) {

	env := newTestEnvSelectiveIndexing(t, NewConf(path, maxFileSize), indexConfig.AttrsToIndex, &disabled.Provider{})
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
