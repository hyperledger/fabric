/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestBlockfileMgrBlockReadWrite(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
	blkfileMgrWrapper.testGetBlockByNumber(blocks)
}

func TestAddBlockWithWrongHash(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[0:9])
	lastBlock := blocks[9]
	lastBlock.Header.PreviousHash = []byte("someJunkHash") // set the hash to something unexpected
	err := blkfileMgrWrapper.blockfileMgr.addBlock(lastBlock)
	require.Error(t, err, "An error is expected when adding a block with some unexpected hash")
	require.Contains(t, err.Error(), "unexpected Previous block hash. Expected PreviousHash")
	t.Logf("err = %s", err)
}

func TestBlockfileMgrCrashDuringWriting(t *testing.T) {
	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 10, false)
	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 1, false)
	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 0, false)
	testBlockfileMgrCrashDuringWriting(t, 0, 0, 1000, 10, false)
	testBlockfileMgrCrashDuringWriting(t, 0, 5, 1000, 10, false)

	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 10, true)
	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 1, true)
	testBlockfileMgrCrashDuringWriting(t, 10, 2, 1000, 0, true)
	testBlockfileMgrCrashDuringWriting(t, 0, 0, 1000, 10, true)
	testBlockfileMgrCrashDuringWriting(t, 0, 5, 1000, 10, true)
}

func testBlockfileMgrCrashDuringWriting(t *testing.T, numBlksBeforeSavingBlkfilesInfo int,
	numBlksAfterSavingBlkfilesInfo int, numLastBlockBytes int, numPartialBytesToWrite int,
	deleteBFInfo bool) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)

	// create all necessary blocks
	totalBlocks := numBlksBeforeSavingBlkfilesInfo + numBlksAfterSavingBlkfilesInfo
	allBlocks := []*common.Block{gb}
	allBlocks = append(allBlocks, bg.NextTestBlocks(totalBlocks+1)...)

	// identify the blocks that are to be added beforeCP, afterCP, and after restart
	blocksBeforeSavingBlkfilesInfo := []*common.Block{}
	blocksAfterSavingBlkfilesInfo := []*common.Block{}
	if numBlksBeforeSavingBlkfilesInfo != 0 {
		blocksBeforeSavingBlkfilesInfo = allBlocks[0:numBlksBeforeSavingBlkfilesInfo]
	}
	if numBlksAfterSavingBlkfilesInfo != 0 {
		blocksAfterSavingBlkfilesInfo = allBlocks[numBlksBeforeSavingBlkfilesInfo : numBlksBeforeSavingBlkfilesInfo+numBlksAfterSavingBlkfilesInfo]
	}
	blocksAfterRestart := allBlocks[numBlksBeforeSavingBlkfilesInfo+numBlksAfterSavingBlkfilesInfo:]

	// add blocks before cp
	blkfileMgrWrapper.addBlocks(blocksBeforeSavingBlkfilesInfo)
	currentBlkfilesInfo := blkfileMgrWrapper.blockfileMgr.blockfilesInfo
	blkfilesInfo1 := &blockfilesInfo{
		latestFileNumber:   currentBlkfilesInfo.latestFileNumber,
		latestFileSize:     currentBlkfilesInfo.latestFileSize,
		noBlockFiles:       currentBlkfilesInfo.noBlockFiles,
		lastPersistedBlock: currentBlkfilesInfo.lastPersistedBlock,
	}

	// add blocks after cp
	blkfileMgrWrapper.addBlocks(blocksAfterSavingBlkfilesInfo)
	blkfilesInfo2 := blkfileMgrWrapper.blockfileMgr.blockfilesInfo

	// simulate a crash scenario
	lastBlockBytes := []byte{}
	encodedLen := proto.EncodeVarint(uint64(numLastBlockBytes))
	randomBytes := testutil.ConstructRandomBytes(t, numLastBlockBytes)
	lastBlockBytes = append(lastBlockBytes, encodedLen...)
	lastBlockBytes = append(lastBlockBytes, randomBytes...)
	partialBytes := lastBlockBytes[:numPartialBytesToWrite]
	blkfileMgrWrapper.blockfileMgr.currentFileWriter.append(partialBytes, true)
	if deleteBFInfo {
		err := blkfileMgrWrapper.blockfileMgr.db.Delete(blkMgrInfoKey, true)
		require.NoError(t, err)
	} else {
		blkfileMgrWrapper.blockfileMgr.saveBlkfilesInfo(blkfilesInfo1, true)
	}
	blkfileMgrWrapper.close()

	// simulate a start after a crash
	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	blkfilesInfo3 := blkfileMgrWrapper.blockfileMgr.blockfilesInfo
	require.Equal(t, blkfilesInfo2, blkfilesInfo3)

	// add fresh blocks after restart
	blkfileMgrWrapper.addBlocks(blocksAfterRestart)
	testBlockfileMgrBlockIterator(t, blkfileMgrWrapper.blockfileMgr, 0, len(allBlocks)-1, allBlocks)
}

func TestBlockfileMgrBlockIterator(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	testBlockfileMgrBlockIterator(t, blkfileMgrWrapper.blockfileMgr, 0, 7, blocks[0:8])
}

func testBlockfileMgrBlockIterator(t *testing.T, blockfileMgr *blockfileMgr,
	firstBlockNum int, lastBlockNum int, expectedBlocks []*common.Block) {
	itr, err := blockfileMgr.retrieveBlocks(uint64(firstBlockNum))
	require.NoError(t, err, "Error while getting blocks iterator")
	defer itr.Close()
	numBlocksItrated := 0
	for {
		block, err := itr.Next()
		require.NoError(t, err, "Error while getting block number [%d] from iterator", numBlocksItrated)
		require.Equal(t, expectedBlocks[numBlocksItrated], block)
		numBlocksItrated++
		if numBlocksItrated == lastBlockNum-firstBlockNum+1 {
			break
		}
	}
	require.Equal(t, lastBlockNum-firstBlockNum+1, numBlocksItrated)
}

func TestBlockfileMgrBlockchainInfo(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()

	bcInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil}, bcInfo)

	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	bcInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	require.Equal(t, uint64(10), bcInfo.Height)
}

func TestTxIDExists(t *testing.T) {
	t.Run("green-path", func(t *testing.T) {
		env := newTestEnv(t, NewConf(t.TempDir(), 0))
		defer env.Cleanup()

		blkStore, err := env.provider.Open("testLedger")
		require.NoError(t, err)
		defer blkStore.Shutdown()

		blocks := testutil.ConstructTestBlocks(t, 2)
		for _, blk := range blocks {
			require.NoError(t, blkStore.AddBlock(blk))
		}

		for _, blk := range blocks {
			for i := range blk.Data.Data {
				txID, err := protoutil.GetOrComputeTxIDFromEnvelope(blk.Data.Data[i])
				require.NoError(t, err)
				exists, err := blkStore.TxIDExists(txID)
				require.NoError(t, err)
				require.True(t, exists)
			}
		}
		exists, err := blkStore.TxIDExists("non-existent-txid")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("error-path", func(t *testing.T) {
		env := newTestEnv(t, NewConf(t.TempDir(), 0))
		defer env.Cleanup()

		blkStore, err := env.provider.Open("testLedger")
		require.NoError(t, err)
		defer blkStore.Shutdown()

		env.provider.Close()
		exists, err := blkStore.TxIDExists("random")
		require.EqualError(t, err, "error while trying to check the presence of TXID [random]: internal leveldb error while obtaining db iterator: leveldb: closed")
		require.False(t, exists)
	})
}

func TestBlockfileMgrGetTxById(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 2)
	blkfileMgrWrapper.addBlocks(blocks)
	for _, blk := range blocks {
		for j, txEnvelopeBytes := range blk.Data.Data {
			// blockNum starts with 0
			txID, err := protoutil.GetOrComputeTxIDFromEnvelope(blk.Data.Data[j])
			require.NoError(t, err)
			txEnvelopeFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveTransactionByID(txID)
			require.NoError(t, err, "Error while retrieving tx from blkfileMgr")
			txEnvelope, err := protoutil.GetEnvelopeFromBlock(txEnvelopeBytes)
			require.NoError(t, err, "Error while unmarshalling tx")
			require.Equal(t, txEnvelope, txEnvelopeFromFileMgr)
		}
	}
}

// TestBlockfileMgrGetTxByIdDuplicateTxid tests that a transaction with an existing txid
// (within same block or a different block) should not over-write the index by-txid (FAB-8557)
func TestBlockfileMgrGetTxByIdDuplicateTxid(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkStore, err := env.provider.Open("testLedger")
	require.NoError(env.t, err)
	blkFileMgr := blkStore.fileMgr
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	require.NoError(t, blkFileMgr.addBlock(gb))

	block1 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-1"),
			[]byte("tx with id=txid-2"),
			[]byte("another tx with existing id=txid-1"),
		},
		[]string{"txid-1", "txid-2", "txid-1"},
	)
	txValidationFlags := txflags.New(3)
	txValidationFlags.SetFlag(0, peer.TxValidationCode_VALID)
	txValidationFlags.SetFlag(1, peer.TxValidationCode_INVALID_OTHER_REASON)
	txValidationFlags.SetFlag(2, peer.TxValidationCode_DUPLICATE_TXID)
	block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txValidationFlags
	require.NoError(t, blkFileMgr.addBlock(block1))

	block2 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-3"),
			[]byte("yet another tx with existing id=txid-1"),
		},
		[]string{"txid-3", "txid-1"},
	)
	txValidationFlags = txflags.New(2)
	txValidationFlags.SetFlag(0, peer.TxValidationCode_VALID)
	txValidationFlags.SetFlag(1, peer.TxValidationCode_DUPLICATE_TXID)
	block2.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txValidationFlags
	require.NoError(t, blkFileMgr.addBlock(block2))

	txenvp1, err := protoutil.GetEnvelopeFromBlock(block1.Data.Data[0])
	require.NoError(t, err)
	txenvp2, err := protoutil.GetEnvelopeFromBlock(block1.Data.Data[1])
	require.NoError(t, err)
	txenvp3, err := protoutil.GetEnvelopeFromBlock(block2.Data.Data[0])
	require.NoError(t, err)

	indexedTxenvp, _ := blkFileMgr.retrieveTransactionByID("txid-1")
	require.Equal(t, txenvp1, indexedTxenvp)
	indexedTxenvp, _ = blkFileMgr.retrieveTransactionByID("txid-2")
	require.Equal(t, txenvp2, indexedTxenvp)
	indexedTxenvp, _ = blkFileMgr.retrieveTransactionByID("txid-3")
	require.Equal(t, txenvp3, indexedTxenvp)

	blk, _ := blkFileMgr.retrieveBlockByTxID("txid-1")
	require.Equal(t, block1, blk)
	blk, _ = blkFileMgr.retrieveBlockByTxID("txid-2")
	require.Equal(t, block1, blk)
	blk, _ = blkFileMgr.retrieveBlockByTxID("txid-3")
	require.Equal(t, block2, blk)

	validationCode, blkNum, _ := blkFileMgr.retrieveTxValidationCodeByTxID("txid-1")
	require.Equal(t, peer.TxValidationCode_VALID, validationCode)
	require.Equal(t, uint64(1), blkNum)
	validationCode, blkNum, _ = blkFileMgr.retrieveTxValidationCodeByTxID("txid-2")
	require.Equal(t, peer.TxValidationCode_INVALID_OTHER_REASON, validationCode)
	require.Equal(t, uint64(1), blkNum)
	validationCode, blkNum, _ = blkFileMgr.retrieveTxValidationCodeByTxID("txid-3")
	require.Equal(t, peer.TxValidationCode_VALID, validationCode)
	require.Equal(t, uint64(2), blkNum)

	// though we do not expose an API for retrieving all the txs by same id but we may in future
	// and the data is persisted to support this. below code tests this behavior internally
	w := &testBlockfileMgrWrapper{
		t:            t,
		blockfileMgr: blkFileMgr,
	}
	w.testGetMultipleDataByTxID(
		"txid-1",
		[]*expectedBlkTxValidationCode{
			{
				blk:            block1,
				txEnv:          protoutil.ExtractEnvelopeOrPanic(block1, 0),
				validationCode: peer.TxValidationCode_VALID,
			},
			{
				blk:            block1,
				txEnv:          protoutil.ExtractEnvelopeOrPanic(block1, 2),
				validationCode: peer.TxValidationCode_DUPLICATE_TXID,
			},
			{
				blk:            block2,
				txEnv:          protoutil.ExtractEnvelopeOrPanic(block2, 1),
				validationCode: peer.TxValidationCode_DUPLICATE_TXID,
			},
		},
	)

	w.testGetMultipleDataByTxID(
		"txid-2",
		[]*expectedBlkTxValidationCode{
			{
				blk:            block1,
				txEnv:          protoutil.ExtractEnvelopeOrPanic(block1, 1),
				validationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
			},
		},
	)

	w.testGetMultipleDataByTxID(
		"txid-3",
		[]*expectedBlkTxValidationCode{
			{
				blk:            block2,
				txEnv:          protoutil.ExtractEnvelopeOrPanic(block2, 0),
				validationCode: peer.TxValidationCode_VALID,
			},
		},
	)
}

func TestBlockfileMgrGetTxByBlockNumTranNum(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	for blockIndex, blk := range blocks {
		for tranIndex, txEnvelopeBytes := range blk.Data.Data {
			// blockNum and tranNum both start with 0
			txEnvelopeFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveTransactionByBlockNumTranNum(uint64(blockIndex), uint64(tranIndex))
			require.NoError(t, err, "Error while retrieving tx from blkfileMgr")
			txEnvelope, err := protoutil.GetEnvelopeFromBlock(txEnvelopeBytes)
			require.NoError(t, err, "Error while unmarshalling tx")
			require.Equal(t, txEnvelope, txEnvelopeFromFileMgr)
		}
	}
}

func TestBlockfileMgrRestart(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	expectedHeight := uint64(10)
	require.Equal(t, expectedHeight, blkfileMgrWrapper.blockfileMgr.getBlockchainInfo().Height)
	blkfileMgrWrapper.close()

	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	require.Equal(t, 9, int(blkfileMgrWrapper.blockfileMgr.blockfilesInfo.lastPersistedBlock))
	blkfileMgrWrapper.testGetBlockByHash(blocks)
	require.Equal(t, expectedHeight, blkfileMgrWrapper.blockfileMgr.getBlockchainInfo().Height)
}

func TestBlockfileMgrFileRolling(t *testing.T) {
	blocks := testutil.ConstructTestBlocks(t, 200)
	size := 0
	for _, block := range blocks[:100] {
		by, _, err := serializeBlock(block)
		require.NoError(t, err, "Error while serializing block")
		blockBytesSize := len(by)
		encodedLen := proto.EncodeVarint(uint64(blockBytesSize))
		size += blockBytesSize + len(encodedLen)
	}

	maxFileSie := int(0.75 * float64(size))
	env := newTestEnv(t, NewConf(t.TempDir(), maxFileSie))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	blkfileMgrWrapper.addBlocks(blocks[:100])
	require.Equal(t, 1, blkfileMgrWrapper.blockfileMgr.blockfilesInfo.latestFileNumber)
	blkfileMgrWrapper.testGetBlockByHash(blocks[:100])
	blkfileMgrWrapper.close()

	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	blkfileMgrWrapper.addBlocks(blocks[100:])
	require.Equal(t, 2, blkfileMgrWrapper.blockfileMgr.blockfilesInfo.latestFileNumber)
	blkfileMgrWrapper.testGetBlockByHash(blocks[100:])
}

func TestBlockfileMgrGetBlockByTxID(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	for _, blk := range blocks {
		for j := range blk.Data.Data {
			// blockNum starts with 1
			txID, err := protoutil.GetOrComputeTxIDFromEnvelope(blk.Data.Data[j])
			require.NoError(t, err)

			blockFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveBlockByTxID(txID)
			require.NoError(t, err, "Error while retrieving block from blkfileMgr")
			require.Equal(t, blk, blockFromFileMgr)
		}
	}
}

func TestBlockfileMgrSimulateCrashAtFirstBlockInFile(t *testing.T) {
	t.Run("blockfilesInfo persisted", func(t *testing.T) {
		testBlockfileMgrSimulateCrashAtFirstBlockInFile(t, false)
	})

	t.Run("blockfilesInfo to be computed from block files", func(t *testing.T) {
		testBlockfileMgrSimulateCrashAtFirstBlockInFile(t, true)
	})
}

func testBlockfileMgrSimulateCrashAtFirstBlockInFile(t *testing.T, deleteBlkfilesInfo bool) {
	// open blockfileMgr and add 5 blocks
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()

	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blockfileMgr := blkfileMgrWrapper.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, 10)
	for i := 0; i < 10; i++ {
		fmt.Printf("blocks[i].Header.Number = %d\n", blocks[i].Header.Number)
	}
	blkfileMgrWrapper.addBlocks(blocks[:5])
	firstFilePath := blockfileMgr.currentFileWriter.filePath
	firstBlkFileSize := testutilGetFileSize(t, firstFilePath)

	// move to next file and simulate crash scenario while writing the first block
	blockfileMgr.moveToNextFile()
	partialBytesForNextBlock := append(
		proto.EncodeVarint(uint64(10000)),
		[]byte("partialBytesForNextBlock depicting a crash during first block in file")...,
	)
	blockfileMgr.currentFileWriter.append(partialBytesForNextBlock, true)
	if deleteBlkfilesInfo {
		err := blockfileMgr.db.Delete(blkMgrInfoKey, true)
		require.NoError(t, err)
	}
	blkfileMgrWrapper.close()

	// verify that the block file number 1 has been created with partial bytes as a side-effect of crash
	lastFilePath := blockfileMgr.currentFileWriter.filePath
	lastFileContent, err := ioutil.ReadFile(lastFilePath)
	require.NoError(t, err)
	require.Equal(t, lastFileContent, partialBytesForNextBlock)

	// simulate reopen after crash
	blkfileMgrWrapper = newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()

	// last block file (block file number 1) should have been truncated to zero length and concluded as the next file to append to
	require.Equal(t, 0, testutilGetFileSize(t, lastFilePath))
	require.Equal(t,
		&blockfilesInfo{
			latestFileNumber:   1,
			latestFileSize:     0,
			lastPersistedBlock: 4,
			noBlockFiles:       false,
		},
		blkfileMgrWrapper.blockfileMgr.blockfilesInfo,
	)

	// Add 5 more blocks and assert that they are added to last file (block file number 1) and full scanning across two files works as expected
	blkfileMgrWrapper.addBlocks(blocks[5:])
	require.True(t, testutilGetFileSize(t, lastFilePath) > 0)
	require.Equal(t, firstBlkFileSize, testutilGetFileSize(t, firstFilePath))
	blkfileMgrWrapper.testGetBlockByNumber(blocks)
	testBlockfileMgrBlockIterator(t, blkfileMgrWrapper.blockfileMgr, 0, len(blocks)-1, blocks)
}

func testutilGetFileSize(t *testing.T, path string) int {
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return int(fi.Size())
}
