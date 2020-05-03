/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	commonledgerutil "github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockIndexSync(t *testing.T) {
	testBlockIndexSync(t, 10, 5, false)
	testBlockIndexSync(t, 10, 5, true)
	testBlockIndexSync(t, 10, 0, true)
	testBlockIndexSync(t, 10, 10, true)
}

func testBlockIndexSync(t *testing.T, numBlocks int, numBlocksToIndex int, syncByRestart bool) {
	testName := fmt.Sprintf("%v/%v/%v", numBlocks, numBlocksToIndex, syncByRestart)
	t.Run(testName, func(t *testing.T) {
		env := newTestEnv(t, NewConf(testPath(), 0))
		defer env.Cleanup()
		ledgerid := "testledger"
		blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
		defer blkfileMgrWrapper.close()
		blkfileMgr := blkfileMgrWrapper.blockfileMgr
		originalIndexStore := blkfileMgr.index.db
		// construct blocks for testing
		blocks := testutil.ConstructTestBlocks(t, numBlocks)
		// add a few blocks
		blkfileMgrWrapper.addBlocks(blocks[:numBlocksToIndex])

		// redirect index writes to some random place and add remaining blocks
		blkfileMgr.index.db = env.provider.leveldbProvider.GetDBHandle("someRandomPlace")
		blkfileMgrWrapper.addBlocks(blocks[numBlocksToIndex:])

		// Plug-in back the original index store
		blkfileMgr.index.db = originalIndexStore
		// Verify that the first set of blocks are indexed in the original index
		for i := 0; i < numBlocksToIndex; i++ {
			block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			assert.NoError(t, err, "block [%d] should have been present in the index", i)
			assert.Equal(t, blocks[i], block)
		}

		// Before, we test for index sync-up, verify that the last set of blocks not indexed in the original index
		for i := numBlocksToIndex + 1; i <= numBlocks; i++ {
			_, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			assert.Exactly(t, ErrNotFoundInIndex, err)
		}

		// perform index sync
		if syncByRestart {
			blkfileMgrWrapper.close()
			blkfileMgrWrapper = newTestBlockfileWrapper(env, ledgerid)
			defer blkfileMgrWrapper.close()
			blkfileMgr = blkfileMgrWrapper.blockfileMgr
		} else {
			blkfileMgr.syncIndex()
		}

		// Now, last set of blocks should also be indexed in the original index
		for i := numBlocksToIndex; i < numBlocks; i++ {
			block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			assert.NoError(t, err, "block [%d] should have been present in the index", i)
			assert.Equal(t, blocks[i], block)
		}
	})
}

func TestBlockIndexSelectiveIndexing(t *testing.T) {
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrBlockHash})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrBlockNum})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrTxID})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrBlockNumTranNum})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrBlockHash, IndexableAttrBlockNum})
	testBlockIndexSelectiveIndexing(t, []IndexableAttr{IndexableAttrTxID, IndexableAttrBlockNumTranNum})
}

func testBlockIndexSelectiveIndexing(t *testing.T, indexItems []IndexableAttr) {
	var testName string
	for _, s := range indexItems {
		testName = testName + string(s)
	}
	t.Run(testName, func(t *testing.T) {
		env := newTestEnvSelectiveIndexing(t, NewConf(testPath(), 0), indexItems, &disabled.Provider{})
		defer env.Cleanup()
		blkfileMgrWrapper := newTestBlockfileWrapper(env, "testledger")
		defer blkfileMgrWrapper.close()

		blocks := testutil.ConstructTestBlocks(t, 3)
		// add test blocks
		blkfileMgrWrapper.addBlocks(blocks)
		blockfileMgr := blkfileMgrWrapper.blockfileMgr

		// if index has been configured for an indexItem then the item should be indexed else not
		// test 'retrieveBlockByHash'
		block, err := blockfileMgr.retrieveBlockByHash(protoutil.BlockHeaderHash(blocks[0].Header))
		if containsAttr(indexItems, IndexableAttrBlockHash) {
			assert.NoError(t, err, "Error while retrieving block by hash")
			assert.Equal(t, blocks[0], block)
		} else {
			assert.Exactly(t, ErrAttrNotIndexed, err)
		}

		// test 'retrieveBlockByNumber'
		block, err = blockfileMgr.retrieveBlockByNumber(0)
		if containsAttr(indexItems, IndexableAttrBlockNum) {
			assert.NoError(t, err, "Error while retrieving block by number")
			assert.Equal(t, blocks[0], block)
		} else {
			assert.Exactly(t, ErrAttrNotIndexed, err)
		}

		// test 'retrieveTransactionByID'
		txid, err := protoutil.GetOrComputeTxIDFromEnvelope(blocks[0].Data.Data[0])
		assert.NoError(t, err)
		txEnvelope, err := blockfileMgr.retrieveTransactionByID(txid)
		if containsAttr(indexItems, IndexableAttrTxID) {
			assert.NoError(t, err, "Error while retrieving tx by id")
			txEnvelopeBytes := blocks[0].Data.Data[0]
			txEnvelopeOrig, err := protoutil.GetEnvelopeFromBlock(txEnvelopeBytes)
			assert.NoError(t, err)
			assert.Equal(t, txEnvelopeOrig, txEnvelope)
		} else {
			assert.Exactly(t, ErrAttrNotIndexed, err)
		}

		//test 'retrieveTrasnactionsByBlockNumTranNum
		txEnvelope2, err := blockfileMgr.retrieveTransactionByBlockNumTranNum(0, 0)
		if containsAttr(indexItems, IndexableAttrBlockNumTranNum) {
			assert.NoError(t, err, "Error while retrieving tx by blockNum and tranNum")
			txEnvelopeBytes2 := blocks[0].Data.Data[0]
			txEnvelopeOrig2, err2 := protoutil.GetEnvelopeFromBlock(txEnvelopeBytes2)
			assert.NoError(t, err2)
			assert.Equal(t, txEnvelopeOrig2, txEnvelope2)
		} else {
			assert.Exactly(t, ErrAttrNotIndexed, err)
		}

		// test 'retrieveBlockByTxID'
		txid, err = protoutil.GetOrComputeTxIDFromEnvelope(blocks[0].Data.Data[0])
		assert.NoError(t, err)
		block, err = blockfileMgr.retrieveBlockByTxID(txid)
		if containsAttr(indexItems, IndexableAttrTxID) {
			assert.NoError(t, err, "Error while retrieving block by txID")
			assert.Equal(t, block, blocks[0])
		} else {
			assert.Exactly(t, ErrAttrNotIndexed, err)
		}

		for _, block := range blocks {
			flags := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

			for idx, d := range block.Data.Data {
				txid, err = protoutil.GetOrComputeTxIDFromEnvelope(d)
				assert.NoError(t, err)

				reason, err := blockfileMgr.retrieveTxValidationCodeByTxID(txid)

				if containsAttr(indexItems, IndexableAttrTxID) {
					assert.NoError(t, err, "Error while retrieving tx validation code by txID")

					reasonFromFlags := flags.Flag(idx)

					assert.Equal(t, reasonFromFlags, reason)
				} else {
					assert.Exactly(t, ErrAttrNotIndexed, err)
				}
			}
		}
	})
}

func containsAttr(indexItems []IndexableAttr, attr IndexableAttr) bool {
	for _, element := range indexItems {
		if element == attr {
			return true
		}
	}
	return false
}

func TestTxIDKeyEncoding(t *testing.T) {
	testcases := []struct {
		txid   string
		blkNum uint64
		txNum  uint64
	}{
		{"txid1", 0, 0},
		{"", 1, 1},
		{"", 0, 0},
		{"txid1", 100, 100},
	}
	for i, testcase := range testcases {
		t.Run(fmt.Sprintf(" %d", i),
			func(t *testing.T) {
				verifyTxIDKeyDecodable(t,
					constructTxIDKey(testcase.txid, testcase.blkNum, testcase.txNum),
					testcase.txid, testcase.blkNum, testcase.txNum,
				)
			})
	}
}

func verifyTxIDKeyDecodable(t *testing.T, txIDKey []byte, expectedTxID string, expectedBlkNum, expectedTxNum uint64) {
	length, lengthBytes, err := commonledgerutil.DecodeOrderPreservingVarUint64(txIDKey[1:])
	require.NoError(t, err)
	firstIndexTxID := 1 + lengthBytes
	firstIndexBlkNum := firstIndexTxID + int(length)
	require.Equal(t, []byte(expectedTxID), txIDKey[firstIndexTxID:firstIndexBlkNum])

	blkNum, n, err := commonledgerutil.DecodeOrderPreservingVarUint64(txIDKey[firstIndexBlkNum:])
	require.NoError(t, err)
	require.Equal(t, expectedBlkNum, blkNum)

	firstIndexTxNum := firstIndexBlkNum + n
	txNum, n, err := commonledgerutil.DecodeOrderPreservingVarUint64(txIDKey[firstIndexTxNum:])
	require.NoError(t, err)
	require.Equal(t, expectedTxNum, txNum)
	require.Len(t, txIDKey, firstIndexTxNum+n)
}
