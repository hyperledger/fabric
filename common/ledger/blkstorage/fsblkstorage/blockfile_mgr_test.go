/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fsblkstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	ledgerutil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	putil "github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestBlockfileMgrBlockReadWrite(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.testGetBlockByHash(blocks, nil)
	blkfileMgrWrapper.testGetBlockByNumber(blocks, 0, nil)
}

func TestAddBlockWithWrongHash(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[0:9])
	lastBlock := blocks[9]
	lastBlock.Header.PreviousHash = []byte("someJunkHash") // set the hash to something unexpected
	err := blkfileMgrWrapper.blockfileMgr.addBlock(lastBlock)
	assert.Error(t, err, "An error is expected when adding a block with some unexpected hash")
	assert.Contains(t, err.Error(), "unexpected Previous block hash. Expected PreviousHash")
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

func testBlockfileMgrCrashDuringWriting(t *testing.T, numBlocksBeforeCheckpoint int,
	numBlocksAfterCheckpoint int, numLastBlockBytes int, numPartialBytesToWrite int,
	deleteCPInfo bool) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)

	// create all necessary blocks
	totalBlocks := numBlocksBeforeCheckpoint + numBlocksAfterCheckpoint
	allBlocks := []*common.Block{gb}
	allBlocks = append(allBlocks, bg.NextTestBlocks(totalBlocks+1)...)

	// identify the blocks that are to be added beforeCP, afterCP, and after restart
	blocksBeforeCP := []*common.Block{}
	blocksAfterCP := []*common.Block{}
	if numBlocksBeforeCheckpoint != 0 {
		blocksBeforeCP = allBlocks[0:numBlocksBeforeCheckpoint]
	}
	if numBlocksAfterCheckpoint != 0 {
		blocksAfterCP = allBlocks[numBlocksBeforeCheckpoint : numBlocksBeforeCheckpoint+numBlocksAfterCheckpoint]
	}
	blocksAfterRestart := allBlocks[numBlocksBeforeCheckpoint+numBlocksAfterCheckpoint:]

	// add blocks before cp
	blkfileMgrWrapper.addBlocks(blocksBeforeCP)
	currentCPInfo := blkfileMgrWrapper.blockfileMgr.cpInfo
	cpInfo1 := &checkpointInfo{
		currentCPInfo.latestFileChunkSuffixNum,
		currentCPInfo.latestFileChunksize,
		currentCPInfo.isChainEmpty,
		currentCPInfo.lastBlockNumber}

	// add blocks after cp
	blkfileMgrWrapper.addBlocks(blocksAfterCP)
	cpInfo2 := blkfileMgrWrapper.blockfileMgr.cpInfo

	// simulate a crash scenario
	lastBlockBytes := []byte{}
	encodedLen := proto.EncodeVarint(uint64(numLastBlockBytes))
	randomBytes := testutil.ConstructRandomBytes(t, numLastBlockBytes)
	lastBlockBytes = append(lastBlockBytes, encodedLen...)
	lastBlockBytes = append(lastBlockBytes, randomBytes...)
	partialBytes := lastBlockBytes[:numPartialBytesToWrite]
	blkfileMgrWrapper.blockfileMgr.currentFileWriter.append(partialBytes, true)
	if deleteCPInfo {
		err := blkfileMgrWrapper.blockfileMgr.db.Delete(blkMgrInfoKey, true)
		assert.NoError(t, err)
	} else {
		blkfileMgrWrapper.blockfileMgr.saveCurrentInfo(cpInfo1, true)
	}
	blkfileMgrWrapper.close()

	// simulate a start after a crash
	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	cpInfo3 := blkfileMgrWrapper.blockfileMgr.cpInfo
	assert.Equal(t, cpInfo2, cpInfo3)

	// add fresh blocks after restart
	blkfileMgrWrapper.addBlocks(blocksAfterRestart)
	testBlockfileMgrBlockIterator(t, blkfileMgrWrapper.blockfileMgr, 0, len(allBlocks)-1, allBlocks)
}

func TestBlockfileMgrBlockIterator(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
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
	defer itr.Close()
	assert.NoError(t, err, "Error while getting blocks iterator")
	numBlocksItrated := 0
	for {
		block, err := itr.Next()
		assert.NoError(t, err, "Error while getting block number [%d] from iterator", numBlocksItrated)
		assert.Equal(t, expectedBlocks[numBlocksItrated], block)
		numBlocksItrated++
		if numBlocksItrated == lastBlockNum-firstBlockNum+1 {
			break
		}
	}
	assert.Equal(t, lastBlockNum-firstBlockNum+1, numBlocksItrated)
}

func TestBlockfileMgrBlockchainInfo(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()

	bcInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil}, bcInfo)

	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	bcInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	assert.Equal(t, uint64(10), bcInfo.Height)
}

func TestBlockfileMgrGetTxById(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 2)
	blkfileMgrWrapper.addBlocks(blocks)
	for _, blk := range blocks {
		for j, txEnvelopeBytes := range blk.Data.Data {
			// blockNum starts with 0
			txID, err := putil.GetOrComputeTxIDFromEnvelope(blk.Data.Data[j])
			assert.NoError(t, err)
			txEnvelopeFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveTransactionByID(txID)
			assert.NoError(t, err, "Error while retrieving tx from blkfileMgr")
			txEnvelope, err := putil.GetEnvelopeFromBlock(txEnvelopeBytes)
			assert.NoError(t, err, "Error while unmarshalling tx")
			assert.Equal(t, txEnvelope, txEnvelopeFromFileMgr)
		}
	}
}

// TestBlockfileMgrGetTxByIdDuplicateTxid tests that a transaction with an existing txid
// (within same block or a different block) should not over-write the index by-txid (FAB-8557)
func TestBlockfileMgrGetTxByIdDuplicateTxid(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkStore, err := env.provider.OpenBlockStore("testLedger")
	assert.NoError(env.t, err)
	blkFileMgr := blkStore.(*fsBlockStore).fileMgr
	bg, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	assert.NoError(t, blkFileMgr.addBlock(gb))

	block1 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-1"),
			[]byte("tx with id=txid-2"),
			[]byte("another tx with existing id=txid-1"),
		},
		[]string{"txid-1", "txid-2", "txid-1"},
	)
	txValidationFlags := ledgerutil.NewTxValidationFlags(3)
	txValidationFlags.SetFlag(0, peer.TxValidationCode_VALID)
	txValidationFlags.SetFlag(1, peer.TxValidationCode_INVALID_OTHER_REASON)
	txValidationFlags.SetFlag(2, peer.TxValidationCode_DUPLICATE_TXID)
	block1.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txValidationFlags
	assert.NoError(t, blkFileMgr.addBlock(block1))

	block2 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-3"),
			[]byte("yet another tx with existing id=txid-1"),
		},
		[]string{"txid-3", "txid-1"},
	)
	txValidationFlags = ledgerutil.NewTxValidationFlags(2)
	txValidationFlags.SetFlag(0, peer.TxValidationCode_VALID)
	txValidationFlags.SetFlag(1, peer.TxValidationCode_DUPLICATE_TXID)
	block2.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txValidationFlags
	assert.NoError(t, blkFileMgr.addBlock(block2))

	txenvp1, err := putil.GetEnvelopeFromBlock(block1.Data.Data[0])
	assert.NoError(t, err)
	txenvp2, err := putil.GetEnvelopeFromBlock(block1.Data.Data[1])
	assert.NoError(t, err)
	txenvp3, err := putil.GetEnvelopeFromBlock(block2.Data.Data[0])
	assert.NoError(t, err)

	indexedTxenvp, _ := blkFileMgr.retrieveTransactionByID("txid-1")
	assert.Equal(t, txenvp1, indexedTxenvp)
	indexedTxenvp, _ = blkFileMgr.retrieveTransactionByID("txid-2")
	assert.Equal(t, txenvp2, indexedTxenvp)
	indexedTxenvp, _ = blkFileMgr.retrieveTransactionByID("txid-3")
	assert.Equal(t, txenvp3, indexedTxenvp)

	blk, _ := blkFileMgr.retrieveBlockByTxID("txid-1")
	assert.Equal(t, block1, blk)
	blk, _ = blkFileMgr.retrieveBlockByTxID("txid-2")
	assert.Equal(t, block1, blk)
	blk, _ = blkFileMgr.retrieveBlockByTxID("txid-3")
	assert.Equal(t, block2, blk)

	validationCode, _ := blkFileMgr.retrieveTxValidationCodeByTxID("txid-1")
	assert.Equal(t, peer.TxValidationCode_VALID, validationCode)
	validationCode, _ = blkFileMgr.retrieveTxValidationCodeByTxID("txid-2")
	assert.Equal(t, peer.TxValidationCode_INVALID_OTHER_REASON, validationCode)
	validationCode, _ = blkFileMgr.retrieveTxValidationCodeByTxID("txid-3")
	assert.Equal(t, peer.TxValidationCode_VALID, validationCode)
}

func TestBlockfileMgrGetTxByBlockNumTranNum(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	for blockIndex, blk := range blocks {
		for tranIndex, txEnvelopeBytes := range blk.Data.Data {
			// blockNum and tranNum both start with 0
			txEnvelopeFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveTransactionByBlockNumTranNum(uint64(blockIndex), uint64(tranIndex))
			assert.NoError(t, err, "Error while retrieving tx from blkfileMgr")
			txEnvelope, err := putil.GetEnvelopeFromBlock(txEnvelopeBytes)
			assert.NoError(t, err, "Error while unmarshalling tx")
			assert.Equal(t, txEnvelope, txEnvelopeFromFileMgr)
		}
	}
}

func TestBlockfileMgrRestart(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	expectedHeight := uint64(10)
	assert.Equal(t, expectedHeight, blkfileMgrWrapper.blockfileMgr.getBlockchainInfo().Height)
	blkfileMgrWrapper.close()

	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	assert.Equal(t, 9, int(blkfileMgrWrapper.blockfileMgr.cpInfo.lastBlockNumber))
	blkfileMgrWrapper.testGetBlockByHash(blocks, nil)
	assert.Equal(t, expectedHeight, blkfileMgrWrapper.blockfileMgr.getBlockchainInfo().Height)
}

func TestBlockfileMgrFileRolling(t *testing.T) {
	blocks := testutil.ConstructTestBlocks(t, 200)
	size := 0
	for _, block := range blocks[:100] {
		by, _, err := serializeBlock(block)
		assert.NoError(t, err, "Error while serializing block")
		blockBytesSize := len(by)
		encodedLen := proto.EncodeVarint(uint64(blockBytesSize))
		size += blockBytesSize + len(encodedLen)
	}

	maxFileSie := int(0.75 * float64(size))
	env := newTestEnv(t, NewConf(testPath(), maxFileSie))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	blkfileMgrWrapper.addBlocks(blocks[:100])
	assert.Equal(t, 1, blkfileMgrWrapper.blockfileMgr.cpInfo.latestFileChunkSuffixNum)
	blkfileMgrWrapper.testGetBlockByHash(blocks[:100], nil)
	blkfileMgrWrapper.close()

	blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	blkfileMgrWrapper.addBlocks(blocks[100:])
	assert.Equal(t, 2, blkfileMgrWrapper.blockfileMgr.cpInfo.latestFileChunkSuffixNum)
	blkfileMgrWrapper.testGetBlockByHash(blocks[100:], nil)
}

func TestBlockfileMgrGetBlockByTxID(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	for _, blk := range blocks {
		for j := range blk.Data.Data {
			// blockNum starts with 1
			txID, err := putil.GetOrComputeTxIDFromEnvelope(blk.Data.Data[j])
			assert.NoError(t, err)

			blockFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveBlockByTxID(txID)
			assert.NoError(t, err, "Error while retrieving block from blkfileMgr")
			assert.Equal(t, blk, blockFromFileMgr)
		}
	}
}

func TestBlockfileMgrSimulateCrashAtFirstBlockInFile(t *testing.T) {
	t.Run("CPInfo persisted", func(t *testing.T) {
		testBlockfileMgrSimulateCrashAtFirstBlockInFile(t, false)
	})

	t.Run("CPInfo to be computed from block files", func(t *testing.T) {
		testBlockfileMgrSimulateCrashAtFirstBlockInFile(t, true)
	})
}

func testBlockfileMgrSimulateCrashAtFirstBlockInFile(t *testing.T, deleteCPInfo bool) {
	// open blockfileMgr and add 5 blocks
	env := newTestEnv(t, NewConf(testPath(), 0))
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
	if deleteCPInfo {
		err := blockfileMgr.db.Delete(blkMgrInfoKey, true)
		assert.NoError(t, err)
	}
	blkfileMgrWrapper.close()

	// verify that the block file number 1 has been created with partial bytes as a side-effect of crash
	lastFilePath := blockfileMgr.currentFileWriter.filePath
	lastFileContent, err := ioutil.ReadFile(lastFilePath)
	assert.NoError(t, err)
	assert.Equal(t, lastFileContent, partialBytesForNextBlock)

	// simulate reopen after crash
	blkfileMgrWrapper = newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()

	// last block file (block file number 1) should have been truncated to zero length and concluded as the next file to append to
	assert.Equal(t, 0, testutilGetFileSize(t, lastFilePath))
	assert.Equal(t,
		&checkpointInfo{
			latestFileChunkSuffixNum: 1,
			latestFileChunksize:      0,
			lastBlockNumber:          4,
			isChainEmpty:             false,
		},
		blkfileMgrWrapper.blockfileMgr.cpInfo,
	)

	// Add 5 more blocks and assert that they are added to last file (block file number 1) and full scanning across two files works as expected
	blkfileMgrWrapper.addBlocks(blocks[5:])
	assert.True(t, testutilGetFileSize(t, lastFilePath) > 0)
	assert.Equal(t, firstBlkFileSize, testutilGetFileSize(t, firstFilePath))
	blkfileMgrWrapper.testGetBlockByNumber(blocks, 0, nil)
	testBlockfileMgrBlockIterator(t, blkfileMgrWrapper.blockfileMgr, 0, len(blocks)-1, blocks)
}

func testutilGetFileSize(t *testing.T, path string) int {
	fi, err := os.Stat(path)
	assert.NoError(t, err)
	return int(fi.Size())
}
