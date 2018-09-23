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
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestBlocksItrBlockingNext(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[:5])

	itr, err := blkfileMgr.retrieveBlocks(1)
	defer itr.Close()
	assert.NoError(t, err)
	readyChan := make(chan struct{})
	doneChan := make(chan bool)
	go testIterateAndVerify(t, itr, blocks[1:], 4, readyChan, doneChan)
	<-readyChan
	testAppendBlocks(blkfileMgrWrapper, blocks[5:7])
	blkfileMgr.moveToNextFile()
	time.Sleep(time.Millisecond * 10)
	testAppendBlocks(blkfileMgrWrapper, blocks[7:])
	<-doneChan
}

func TestBlockItrClose(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	itr, err := blkfileMgr.retrieveBlocks(1)
	assert.NoError(t, err)

	bh, _ := itr.Next()
	assert.NotNil(t, bh)
	itr.Close()

	bh, err = itr.Next()
	assert.NoError(t, err)
	assert.Nil(t, bh)
}

func TestRaceToDeadlock(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	for i := 0; i < 1000; i++ {
		itr, err := blkfileMgr.retrieveBlocks(5)
		if err != nil {
			panic(err)
		}
		go func() {
			itr.Next()
		}()
		itr.Close()
	}

	for i := 0; i < 1000; i++ {
		itr, err := blkfileMgr.retrieveBlocks(5)
		if err != nil {
			panic(err)
		}
		go func() {
			itr.Close()
		}()
		itr.Next()
	}
}

func TestBlockItrCloseWithoutRetrieve(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	itr, err := blkfileMgr.retrieveBlocks(2)
	assert.NoError(t, err)
	itr.Close()
}

func TestCloseMultipleItrsWaitForFutureBlock(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[:5])

	wg := &sync.WaitGroup{}
	wg.Add(2)
	itr1, err := blkfileMgr.retrieveBlocks(7)
	assert.NoError(t, err)
	// itr1 does not retrieve any block because it closes before new blocks are added
	go iterateInBackground(t, itr1, 9, wg, []uint64{})

	itr2, err := blkfileMgr.retrieveBlocks(8)
	assert.NoError(t, err)
	// itr2 retrieves two blocks 8 and 9. Because it started waiting for 8 and quits at 9
	go iterateInBackground(t, itr2, 9, wg, []uint64{8, 9})

	// sleep for the background iterators to get started
	time.Sleep(2 * time.Second)
	itr1.Close()
	blkfileMgrWrapper.addBlocks(blocks[5:])
	wg.Wait()
}

func iterateInBackground(t *testing.T, itr *blocksItr, quitAfterBlkNum uint64, wg *sync.WaitGroup, expectedBlockNums []uint64) {
	defer wg.Done()
	retrievedBlkNums := []uint64{}
	defer func() { assert.Equal(t, expectedBlockNums, retrievedBlkNums) }()

	for {
		blk, err := itr.Next()
		assert.NoError(t, err)
		if blk == nil {
			return
		}
		blkNum := blk.(*common.Block).Header.Number
		retrievedBlkNums = append(retrievedBlkNums, blkNum)
		t.Logf("blk.Num=%d", blk.(*common.Block).Header.Number)
		if blkNum == quitAfterBlkNum {
			return
		}
	}
}

func testIterateAndVerify(t *testing.T, itr *blocksItr, blocks []*common.Block, readyAt int, readyChan chan<- struct{}, doneChan chan bool) {
	blocksIterated := 0
	for {
		t.Logf("blocksIterated: %v", blocksIterated)
		block, err := itr.Next()
		assert.NoError(t, err)
		assert.Equal(t, blocks[blocksIterated], block)
		blocksIterated++
		if blocksIterated == readyAt {
			close(readyChan)
		}
		if blocksIterated == len(blocks) {
			break
		}
	}
	doneChan <- true
}

func testAppendBlocks(blkfileMgrWrapper *testBlockfileMgrWrapper, blocks []*common.Block) {
	blkfileMgrWrapper.addBlocks(blocks)
}
