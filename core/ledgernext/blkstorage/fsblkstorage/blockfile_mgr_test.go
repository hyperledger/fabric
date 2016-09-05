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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledgernext/testutil"
	"github.com/hyperledger/fabric/protos"
)

func TestBlockfileMgrBlockReadWrite(t *testing.T) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
	blkfileMgrWrapper.testGetBlockByNumber(blocks, 1)
}

func TestBlockfileMgrBlockIterator(t *testing.T) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	itr, err := blkfileMgrWrapper.blockfileMgr.retrieveBlocks(1, 8)
	defer itr.Close()
	testutil.AssertNoError(t, err, "Error while getting blocks iterator")
	numBlocksItrated := 0
	for ; itr.Next(); numBlocksItrated++ {
		block, err := itr.Get()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error while getting block number [%d] from iterator", numBlocksItrated))
		testutil.AssertEquals(t, block.(*BlockHolder).GetBlock(), blocks[numBlocksItrated])
	}
	testutil.AssertEquals(t, numBlocksItrated, 8)
}

func TestBlockfileMgrBlockchainInfo(t *testing.T) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()

	bcInfo := blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &protos.BlockchainInfo{Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	bcInfo = blkfileMgrWrapper.blockfileMgr.getBlockchainInfo()
	testutil.AssertEquals(t, bcInfo.Height, uint64(10))
}

func TestBlockfileMgrGetTxById(t *testing.T) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	for i, blk := range blocks {
		for j, txBytes := range blk.Transactions {
			// blockNum starts with 1
			txID := constructTxID(uint64(i+1), j)
			txFromFileMgr, err := blkfileMgrWrapper.blockfileMgr.retrieveTransactionByID(txID)
			testutil.AssertNoError(t, err, "Error while retrieving tx from blkfileMgr")
			tx := &protos.Transaction2{}
			err = proto.Unmarshal(txBytes, tx)
			testutil.AssertNoError(t, err, "Error while unmarshalling tx")
			testutil.AssertEquals(t, txFromFileMgr, tx)
		}
	}
}

func TestBlockfileMgrRestart(t *testing.T) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.close()

	blkfileMgrWrapper = newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	testutil.AssertEquals(t, int(blkfileMgrWrapper.blockfileMgr.cpInfo.lastBlockNumber), 10)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
}

func TestBlockfileMgrFileRolling(t *testing.T) {
	env := newTestEnv(t)
	blocks := testutil.ConstructTestBlocks(t, 100)
	size := 0
	for _, block := range blocks {
		serBlock, err := protos.ConstructSerBlock2(block)
		testutil.AssertNoError(t, err, "Error while getting bytes from block")
		by := serBlock.GetBytes()
		blockBytesSize := len(by)
		encodedLen := proto.EncodeVarint(uint64(blockBytesSize))
		size += blockBytesSize + len(encodedLen)
	}

	env.conf.maxBlockfileSize = int(0.75 * float64(size))
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	blkfileMgrWrapper.addBlocks(blocks)
	testutil.AssertEquals(t, blkfileMgrWrapper.blockfileMgr.cpInfo.latestFileChunkSuffixNum, 1)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
	blkfileMgrWrapper.close()
	env.Cleanup()

	env = newTestEnv(t)
	defer env.Cleanup()
	env.conf.maxBlockfileSize = int(0.40 * float64(size))
	blkfileMgrWrapper = newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	blkfileMgrWrapper.addBlocks(blocks)
	testutil.AssertEquals(t, blkfileMgrWrapper.blockfileMgr.cpInfo.latestFileChunkSuffixNum, 2)
	blkfileMgrWrapper.testGetBlockByHash(blocks)
}
