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
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	putil "github.com/hyperledger/fabric/protos/utils"
)

type noopIndex struct {
}

func (i *noopIndex) getLastBlockIndexed() (uint64, error) {
	return 0, nil
}
func (i *noopIndex) indexBlock(blockIdxInfo *blockIdxInfo) error {
	return nil
}
func (i *noopIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	return nil, nil
}
func (i *noopIndex) getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error) {
	return nil, nil
}
func (i *noopIndex) getTxLoc(txID string) (*fileLocPointer, error) {
	return nil, nil
}
func (i *noopIndex) getTXLocByBlockNumTranNum(blockNum uint64, tranNum uint64) (*fileLocPointer, error) {
	return nil, nil
}

func (i *noopIndex) getBlockLocByTxID(txID string) (*fileLocPointer, error) {
	return nil, nil
}

func (i *noopIndex) getTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	return peer.TxValidationCode(-1), nil
}

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
		origIndex := blkfileMgr.index
		// construct blocks for testing
		blocks := testutil.ConstructTestBlocks(t, numBlocks)
		// add a few blocks
		blkfileMgrWrapper.addBlocks(blocks[:numBlocksToIndex])

		// Plug-in a noop index and add remaining blocks
		blkfileMgr.index = &noopIndex{}
		blkfileMgrWrapper.addBlocks(blocks[numBlocksToIndex:])

		// Plug-in back the original index
		blkfileMgr.index = origIndex
		// The first set of blocks should be present in the original index
		for i := 0; i < numBlocksToIndex; i++ {
			block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			testutil.AssertNoError(t, err, fmt.Sprintf("block [%d] should have been present in the index", i))
			testutil.AssertEquals(t, block, blocks[i])
		}

		// The last set of blocks should not be present in the original index
		for i := numBlocksToIndex + 1; i <= numBlocks; i++ {
			_, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			testutil.AssertSame(t, err, blkstorage.ErrNotFoundInIndex)
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

		// Now, last set of blocks should also be present in original index
		for i := numBlocksToIndex; i < numBlocks; i++ {
			block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
			testutil.AssertNoError(t, err, fmt.Sprintf("block [%d] should have been present in the index", i))
			testutil.AssertEquals(t, block, blocks[i])
		}
	})
}

func TestBlockIndexSelectiveIndexing(t *testing.T) {
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockHash})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNum})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrTxID})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNumTranNum})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockHash, blkstorage.IndexableAttrBlockNum})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrTxID, blkstorage.IndexableAttrBlockNumTranNum})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockTxID})
	testBlockIndexSelectiveIndexing(t, []blkstorage.IndexableAttr{blkstorage.IndexableAttrTxValidationCode})
}

func testBlockIndexSelectiveIndexing(t *testing.T, indexItems []blkstorage.IndexableAttr) {
	var testName string
	for _, s := range indexItems {
		testName = testName + string(s)
	}
	t.Run(testName, func(t *testing.T) {
		env := newTestEnvSelectiveIndexing(t, NewConf(testPath(), 0), indexItems)
		defer env.Cleanup()
		blkfileMgrWrapper := newTestBlockfileWrapper(env, "testledger")
		defer blkfileMgrWrapper.close()

		blocks := testutil.ConstructTestBlocks(t, 3)
		// add test blocks
		blkfileMgrWrapper.addBlocks(blocks)
		blockfileMgr := blkfileMgrWrapper.blockfileMgr

		// if index has been configured for an indexItem then the item should be indexed else not
		// test 'retrieveBlockByHash'
		block, err := blockfileMgr.retrieveBlockByHash(blocks[0].Header.Hash())
		if testutil.Contains(indexItems, blkstorage.IndexableAttrBlockHash) {
			testutil.AssertNoError(t, err, "Error while retrieving block by hash")
			testutil.AssertEquals(t, block, blocks[0])
		} else {
			testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
		}

		// test 'retrieveBlockByNumber'
		block, err = blockfileMgr.retrieveBlockByNumber(0)
		if testutil.Contains(indexItems, blkstorage.IndexableAttrBlockNum) {
			testutil.AssertNoError(t, err, "Error while retrieving block by number")
			testutil.AssertEquals(t, block, blocks[0])
		} else {
			testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
		}

		// test 'retrieveTransactionByID'
		txid, err := extractTxID(blocks[0].Data.Data[0])
		testutil.AssertNoError(t, err, "")
		txEnvelope, err := blockfileMgr.retrieveTransactionByID(txid)
		if testutil.Contains(indexItems, blkstorage.IndexableAttrTxID) {
			testutil.AssertNoError(t, err, "Error while retrieving tx by id")
			txEnvelopeBytes := blocks[0].Data.Data[0]
			txEnvelopeOrig, err := putil.GetEnvelopeFromBlock(txEnvelopeBytes)
			testutil.AssertNoError(t, err, "")
			testutil.AssertEquals(t, txEnvelope, txEnvelopeOrig)
		} else {
			testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
		}

		//test 'retrieveTrasnactionsByBlockNumTranNum
		txEnvelope2, err := blockfileMgr.retrieveTransactionByBlockNumTranNum(0, 0)
		if testutil.Contains(indexItems, blkstorage.IndexableAttrBlockNumTranNum) {
			testutil.AssertNoError(t, err, "Error while retrieving tx by blockNum and tranNum")
			txEnvelopeBytes2 := blocks[0].Data.Data[0]
			txEnvelopeOrig2, err2 := putil.GetEnvelopeFromBlock(txEnvelopeBytes2)
			testutil.AssertNoError(t, err2, "")
			testutil.AssertEquals(t, txEnvelope2, txEnvelopeOrig2)
		} else {
			testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
		}

		// test 'retrieveBlockByTxID'
		txid, err = extractTxID(blocks[0].Data.Data[0])
		testutil.AssertNoError(t, err, "")
		block, err = blockfileMgr.retrieveBlockByTxID(txid)
		if testutil.Contains(indexItems, blkstorage.IndexableAttrBlockTxID) {
			testutil.AssertNoError(t, err, "Error while retrieving block by txID")
			testutil.AssertEquals(t, blocks[0], block)
		} else {
			testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
		}

		for _, block := range blocks {
			flags := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

			for idx, d := range block.Data.Data {
				txid, err = extractTxID(d)
				testutil.AssertNoError(t, err, "")

				reason, err := blockfileMgr.retrieveTxValidationCodeByTxID(txid)

				if testutil.Contains(indexItems, blkstorage.IndexableAttrTxValidationCode) {
					testutil.AssertNoError(t, err, "Error while retrieving tx validation code by txID")

					reasonFromFlags := flags.Flag(idx)

					testutil.AssertEquals(t, reason, reasonFromFlags)
				} else {
					testutil.AssertSame(t, err, blkstorage.ErrAttrNotIndexed)
				}
			}
		}
	})
}
