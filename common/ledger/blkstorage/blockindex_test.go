/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util"
	commonledgerutil "github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
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

func TestTxIDKeyEncodingDecoding(t *testing.T) {
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
		encodedTxIDKey := constructTxIDKey(testcase.txid, testcase.blkNum, testcase.txNum)
		t.Run(fmt.Sprintf(" %d", i),
			func(t *testing.T) {
				txID, err := retrieveTxID(encodedTxIDKey)
				require.NoError(t, err)
				require.Equal(t, testcase.txid, txID)
				verifyTxIDKeyDecodable(t,
					encodedTxIDKey,
					testcase.txid, testcase.blkNum, testcase.txNum,
				)
			})
	}
}

func TestTxIDKeyDecodingInvalidInputs(t *testing.T) {
	prefix := []byte{txIDIdxKeyPrefix}
	txIDLen := util.EncodeOrderPreservingVarUint64(uint64(len("mytxid")))
	txID := []byte("mytxid")

	// empty byte
	_, err := retrieveTxID([]byte{})
	require.EqualError(t, err, "invalid txIDKey - zero-length slice")

	// invalid prefix
	invalidPrefix := []byte{txIDIdxKeyPrefix + 1}
	_, err = retrieveTxID(invalidPrefix)
	require.EqualError(t, err, fmt.Sprintf("invalid txIDKey {%x} - unexpected prefix", invalidPrefix))

	// invalid key - only prefix
	_, err = retrieveTxID(prefix)
	require.EqualError(t, err, fmt.Sprintf("invalid txIDKey {%x}: number of consumed bytes from DecodeVarint is invalid, expected 1, but got 0", prefix))

	// invalid key - incomplete length
	incompleteLength := appendAllAndTrimLastByte(prefix, txIDLen)
	_, err = retrieveTxID(incompleteLength)
	require.EqualError(t, err, fmt.Sprintf("invalid txIDKey {%x}: decoded size (1) from DecodeVarint is more than available bytes (0)", incompleteLength))

	// invalid key - incomplete txid
	incompleteTxID := appendAllAndTrimLastByte(prefix, txIDLen, txID)
	_, err = retrieveTxID(incompleteTxID)
	require.EqualError(t, err, fmt.Sprintf("invalid txIDKey {%x}, fewer bytes present", incompleteTxID))
}

func TestExportUniqueTxIDs(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testledger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	bg, gb := testutil.NewBlockGenerator(t, "myChannel", false)
	blkfileMgr.addBlock(gb)

	// add genesis block and test the exported bytes
	configTxID, err := protoutil.GetOrComputeTxIDFromEnvelope(gb.Data.Data[0])
	require.NoError(t, err)
	testWriter := bytes.NewBuffer(nil)
	err = blkfileMgr.index.exportUniqueTxIDs(testWriter)
	require.NoError(t, err)
	verifyExportedTxIDs(t, testWriter.Bytes(), configTxID)

	// add block-1 and test the exported bytes
	block1 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-3"),
			[]byte("tx with id=txid-1"),
			[]byte("tx with id=txid-2"),
			[]byte("another tx with existing id=txid-1"),
		},
		[]string{"txid-3", "txid-1", "txid-2", "txid-1"},
	)
	err = blkfileMgr.addBlock(block1)
	require.NoError(t, err)
	testWriter.Reset()
	err = blkfileMgr.index.exportUniqueTxIDs(testWriter)
	require.NoError(t, err)
	verifyExportedTxIDs(t, testWriter.Bytes(), "txid-1", "txid-2", "txid-3", configTxID) //"txid-1" appears once, Txids appear in radix sort order

	// add block-2 and test the exported bytes
	block2 := bg.NextBlockWithTxid(
		[][]byte{
			[]byte("tx with id=txid-0000000"),
			[]byte("tx with id=txid-3"),
			[]byte("tx with id=txid-4"),
		},
		[]string{"txid-0000000", "txid-3", "txid-4"},
	)
	blkfileMgr.addBlock(block2)
	require.NoError(t, err)
	testWriter.Reset()
	err = blkfileMgr.index.exportUniqueTxIDs(testWriter)
	require.NoError(t, err)
	verifyExportedTxIDs(t, testWriter.Bytes(), "txid-1", "txid-2", "txid-3", "txid-4", "txid-0000000", configTxID) // "txid-1", and "txid-3 appears once and Txids appear in radix sort order
}

func TestExportUniqueTxIDsWhenTxIDsNotIndexed(t *testing.T) {
	env := newTestEnvSelectiveIndexing(t, NewConf(testPath(), 0), []IndexableAttr{IndexableAttrBlockNum}, &disabled.Provider{})
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testledger")
	defer blkfileMgrWrapper.close()

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	err := blkfileMgrWrapper.blockfileMgr.index.exportUniqueTxIDs(bytes.NewBuffer(nil))
	require.Equal(t, err, ErrAttrNotIndexed)
}

func TestExportUniqueTxIDsErrorCases(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testledger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	defer blkfileMgrWrapper.close()

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)
	blockfileMgr := blkfileMgrWrapper.blockfileMgr
	index := blockfileMgr.index

	err := index.exportUniqueTxIDs(&errorThrowingWriter{errors.New("always return error")})
	require.EqualError(t, err, "always return error")

	index.db.Put([]byte{txIDIdxKeyPrefix}, []byte("some junk value"), true)
	err = index.exportUniqueTxIDs(bytes.NewBuffer(nil))
	require.EqualError(t, err, "invalid txIDKey {74}: number of consumed bytes from DecodeVarint is invalid, expected 1, but got 0")

	env.provider.leveldbProvider.Close()
	err = index.exportUniqueTxIDs(bytes.NewBuffer(nil))
	require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
}

func verifyExportedTxIDs(t *testing.T, actual []byte, expectedTxIDs ...string) {
	expectedBytes := []byte{exportFileFormatVersion}
	buf := make([]byte, binary.MaxVarintLen64)
	for _, txID := range expectedTxIDs {
		n := binary.PutUvarint(buf, uint64(len(txID)))
		expectedBytes = append(expectedBytes, buf[:n]...)
		expectedBytes = append(expectedBytes, []byte(txID)...)
	}
	require.Equal(t, expectedBytes, actual)
}

func appendAllAndTrimLastByte(input ...[]byte) []byte {
	r := []byte{}
	for _, i := range input {
		r = append(r, i...)
	}
	return r[:len(r)-1]
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

type errorThrowingWriter struct {
	err error
}

func (w *errorThrowingWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}
