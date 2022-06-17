/*
	Copyright IBM Corp. All Rights Reserved.

	SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestRollback(t *testing.T) {
	path := t.TempDir()
	blocks := testutil.ConstructTestBlocks(t, 50) // 50 blocks persisted in ~5 block files
	blocksPerFile := 50 / 5
	env := newTestEnv(t, NewConf(path, 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	// 1. Store blocks
	for i, b := range blocks {
		require.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[49].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[48].Header),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the BlockfileInfo
	expectedblkfilesInfoLastBlockNumber := uint64(49)
	expectedBlkfilesInfoIsNoFiles := false
	actualBlkfilesInfo, err := blkfileMgrWrapper.blockfileMgr.loadBlkfilesInfo()
	require.NoError(t, err)
	require.Equal(t, expectedblkfilesInfoLastBlockNumber, actualBlkfilesInfo.lastPersistedBlock)
	require.Equal(t, expectedBlkfilesInfoIsNoFiles, actualBlkfilesInfo.noBlockFiles)
	require.Equal(t, actualBlkfilesInfo.latestFileNumber, 4)

	// 4. Check whether all blocks are stored correctly
	blkfileMgrWrapper.testGetBlockByNumber(blocks)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
	blkfileMgrWrapper.testGetBlockByTxID(blocks)

	// 5. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()
	lastBlockNumberInLastFile := uint64(49)
	middleBlockNumberInLastFile := uint64(45)
	firstBlockNumberInLastFile := uint64(41)

	// 7. Rollback to one before the lastBlockNumberInLastFile
	indexConfig := &IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", lastBlockNumberInLastFile-uint64(1), indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, lastBlockNumberInLastFile-uint64(1), 4, indexConfig)

	// 8. Rollback to middleBlockNumberInLastFile
	err = Rollback(path, "testLedger", middleBlockNumberInLastFile, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, middleBlockNumberInLastFile, 4, indexConfig)

	// 9. Rollback to firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, firstBlockNumberInLastFile, 4, indexConfig)

	// 10. Rollback to one before the firstBlockNumberInLastFile
	err = Rollback(path, "testLedger", firstBlockNumberInLastFile-1, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, firstBlockNumberInLastFile-1, 3, indexConfig)

	// 11. In the middle block file (among a range of block files), find the middle block number
	middleBlockNumberInMiddleFile := uint64(25)

	// 12. Rollback to middleBlockNumberInMiddleFile
	err = Rollback(path, "testLedger", middleBlockNumberInMiddleFile, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, middleBlockNumberInMiddleFile, 2, indexConfig)

	// 13. Rollback to block 5
	err = Rollback(path, "testLedger", 5, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 5, 0, indexConfig)

	// 14. Rollback to block 1
	err = Rollback(path, "testLedger", 1, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 1, 0, indexConfig)
}

// TestRollbackWithOnlyBlockIndexAttributes mimics the scenario when ledger is used for orderer
// i.e., only block is index and transancations are not indexed
func TestRollbackWithOnlyBlockIndexAttributes(t *testing.T) {
	path := t.TempDir()
	blocks := testutil.ConstructTestBlocks(t, 50) // 50 blocks persisted in ~5 block files
	blocksPerFile := 50 / 5
	onlyBlockNumIndex := []IndexableAttr{
		IndexableAttrBlockNum,
	}
	env := newTestEnvSelectiveIndexing(t, NewConf(path, 0), onlyBlockNumIndex, &disabled.Provider{})
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	// 1. Store blocks
	for i, b := range blocks {
		require.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[49].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[48].Header),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the BlockfilesInfo
	expectedBlkfilesInfoLastBlockNumber := uint64(49)
	expectedBlkfilesInfoIsNoBlkFiles := false
	actualBlkfilesInfo, err := blkfileMgrWrapper.blockfileMgr.loadBlkfilesInfo()
	require.NoError(t, err)
	require.Equal(t, expectedBlkfilesInfoLastBlockNumber, actualBlkfilesInfo.lastPersistedBlock)
	require.Equal(t, expectedBlkfilesInfoIsNoBlkFiles, actualBlkfilesInfo.noBlockFiles)
	require.Equal(t, actualBlkfilesInfo.latestFileNumber, 4)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Rollback to block 2
	onlyBlockNumIndexCfg := &IndexConfig{
		AttrsToIndex: onlyBlockNumIndex,
	}
	err = Rollback(path, "testLedger", 2, onlyBlockNumIndexCfg)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 2, 0, onlyBlockNumIndexCfg)
}

func TestRollbackWithNoIndexDir(t *testing.T) {
	path := t.TempDir()
	blocks := testutil.ConstructTestBlocks(t, 50)
	blocksPerFile := 50 / 5
	conf := NewConf(path, 0)
	env := newTestEnv(t, conf)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	// 1. Store blocks
	for i, b := range blocks {
		require.NoError(t, blkfileMgr.addBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 10):file0, (11,20):file1, (21,30):file2, (31, 40):file3, (41,49):file4]
			blkfileMgr.moveToNextFile()
		}
	}

	// 2. Check the BlockchainInfo
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            50,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[49].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[48].Header),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Check the BlockfilesInfo
	expectedBlkfilesInfoLastBlockNumber := uint64(49)
	expectedBlkfilesInfoIsChainEmpty := false
	actualBlkfilesInfo, err := blkfileMgrWrapper.blockfileMgr.loadBlkfilesInfo()
	require.NoError(t, err)
	require.Equal(t, expectedBlkfilesInfoLastBlockNumber, actualBlkfilesInfo.lastPersistedBlock)
	require.Equal(t, expectedBlkfilesInfoIsChainEmpty, actualBlkfilesInfo.noBlockFiles)
	require.Equal(t, actualBlkfilesInfo.latestFileNumber, 4)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Remove the index directory
	indexDir := conf.getIndexDir()
	err = os.RemoveAll(indexDir)
	require.NoError(t, err)

	// 6. Rollback to block 2
	indexConfig := &IndexConfig{AttrsToIndex: attrsToIndex}
	err = Rollback(path, "testLedger", 2, indexConfig)
	require.NoError(t, err)
	assertBlockStoreRollback(t, path, "testLedger", blocks, 2, 0, indexConfig)
}

func TestValidateRollbackParams(t *testing.T) {
	path := t.TempDir()
	env := newTestEnv(t, NewConf(path, 1024*24))
	defer env.Cleanup()

	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")

	// 1. Create 10 blocks
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)

	// 2. Valid inputs
	err := ValidateRollbackParams(path, "testLedger", 5)
	require.NoError(t, err)

	// 3. ledgerID does not exist
	err = ValidateRollbackParams(path, "noLedger", 5)
	require.Equal(t, "ledgerID [noLedger] does not exist", err.Error())

	err = ValidateRollbackParams(path, "testLedger", 15)
	require.Equal(t, "target block number [15] should be less than the biggest block number [9]", err.Error())
}

func TestDuplicateTxIDDuringRollback(t *testing.T) {
	path := t.TempDir()
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
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[3].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[2].Header),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 3. Retrieve tx
	blkfileMgrWrapper.testGetTransactionByTxID("tx0", blocks[2].Data.Data[0], nil)

	// 4. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()

	// 5. Rollback to block 2
	indexConfig := &IndexConfig{AttrsToIndex: attrsToIndex}
	err := Rollback(path, "testLedger", 2, indexConfig)
	require.NoError(t, err)

	env = newTestEnv(t, NewConf(path, maxFileSize))
	blkfileMgrWrapper = newTestBlockfileWrapper(env, "testLedger")

	// 6. Check the BlockchainInfo
	expectedBlockchainInfo = &common.BlockchainInfo{
		Height:            3,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[2].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[1].Header),
	}
	actualBlockchainInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 8. Retrieve tx (should not have been deleted)
	blkfileMgrWrapper.testGetTransactionByTxID("tx0", blocks[2].Data.Data[0], nil)
}

func assertBlockStoreRollback(t *testing.T, path, ledgerID string, blocks []*common.Block,
	rollbackedToBlkNum uint64, lastFileSuffixNum int, indexConfig *IndexConfig) {
	env := newTestEnvSelectiveIndexing(t, NewConf(path, 0), indexConfig.AttrsToIndex, &disabled.Provider{})
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerID)

	// 1. Check the BlockchainInfo after the rollback
	expectedBlockchainInfo := &common.BlockchainInfo{
		Height:            rollbackedToBlkNum + 1,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[rollbackedToBlkNum].Header),
		PreviousBlockHash: protoutil.BlockHeaderHash(blocks[rollbackedToBlkNum-1].Header),
	}
	actualBlockchainInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, expectedBlockchainInfo, actualBlockchainInfo)

	// 2. Check the BlockfilesInfo after the rollback
	expectedBlkfilesInfoLastBlockNumber := rollbackedToBlkNum
	expectedBlkfilesInfoIsNoBlkfiles := false
	expectedBlockchainInfoLastFileSuffixNum := lastFileSuffixNum
	actualBlkfilesInfo, err := blkfileMgrWrapper.blockfileMgr.loadBlkfilesInfo()
	require.NoError(t, err)
	require.Equal(t, expectedBlkfilesInfoLastBlockNumber, actualBlkfilesInfo.lastPersistedBlock)
	require.Equal(t, expectedBlkfilesInfoIsNoBlkfiles, actualBlkfilesInfo.noBlockFiles)
	require.Equal(t, expectedBlockchainInfoLastFileSuffixNum, actualBlkfilesInfo.latestFileNumber)

	// 3. Check whether all blocks till the target block number are stored correctly
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(IndexableAttrBlockNum) {
		blkfileMgrWrapper.testGetBlockByNumber(blocks[:rollbackedToBlkNum+1])
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(IndexableAttrBlockHash) {
		blkfileMgrWrapper.testGetBlockByHash(blocks[:rollbackedToBlkNum+1])
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(IndexableAttrTxID) {
		blkfileMgrWrapper.testGetBlockByTxID(blocks[:rollbackedToBlkNum+1])
	}

	// 4. Check whether all blocks with number greater than target block number
	// are removed including index entries
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(IndexableAttrBlockHash) {
		blkfileMgrWrapper.testGetBlockByHashNotIndexed(blocks[rollbackedToBlkNum+1:])
	}
	if blkfileMgrWrapper.blockfileMgr.index.isAttributeIndexed(IndexableAttrTxID) {
		blkfileMgrWrapper.testGetBlockByTxIDNotIndexed(blocks[rollbackedToBlkNum+1:])
	}

	// 5. Close the blkfileMgrWrapper
	env.provider.Close()
	blkfileMgrWrapper.close()
}
