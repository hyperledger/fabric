/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/stretchr/testify/require"
)

func TestBlocksItrBlockingNext(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[:5])

	itr, err := blkfileMgr.retrieveBlocks(1)
	require.NoError(t, err)
	defer itr.Close()
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
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	itr, err := blkfileMgr.retrieveBlocks(1)
	require.NoError(t, err)

	bh, _ := itr.Next()
	require.NotNil(t, bh)
	itr.Close()

	bh, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, bh)
}

func TestRaceToDeadlock(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
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
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	itr, err := blkfileMgr.retrieveBlocks(2)
	require.NoError(t, err)
	itr.Close()
}

func TestCloseMultipleItrsWaitForFutureBlock(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, 10)
	blkfileMgrWrapper.addBlocks(blocks[:5])

	wg := &sync.WaitGroup{}
	wg.Add(2)
	itr1, err := blkfileMgr.retrieveBlocks(7)
	require.NoError(t, err)
	// itr1 does not retrieve any block because it closes before new blocks are added
	go iterateInBackground(t, itr1, 9, wg, []uint64{})

	itr2, err := blkfileMgr.retrieveBlocks(8)
	require.NoError(t, err)
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
	defer func() { require.Equal(t, expectedBlockNums, retrievedBlkNums) }()

	for {
		blk, err := itr.Next()
		require.NoError(t, err)
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
		require.NoError(t, err)
		require.Equal(t, blocks[blocksIterated], block)
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
