/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	defer itr.Close()
	readyChan := make(chan struct{})
	iteratedChan := make(chan []*common.Block, 1)
	iterErrChan := make(chan error, 1)
	go func() {
		iterated, err := testIterateAndVerify(t, itr, blocks[1:], 4, readyChan)
		iteratedChan <- iterated
		iterErrChan <- err
	}()
	<-readyChan
	testAppendBlocks(blkfileMgrWrapper, blocks[5:7])
	blkfileMgr.moveToNextFile()
	time.Sleep(time.Millisecond * 10)
	testAppendBlocks(blkfileMgrWrapper, blocks[7:])
	require.NoError(t, <-iterErrChan)
	require.Equal(t, blocks[1:], <-iteratedChan)
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
	require.NoError(t, err)

	bh, _ := itr.Next()
	require.NotNil(t, bh)
	itr.Close()

	bh, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, bh)
}

func TestRaceToDeadlock(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blocks := testutil.ConstructTestBlocks(t, 5)
	blkfileMgrWrapper.addBlocks(blocks)

	for range 1000 {
		itr, err := blkfileMgr.retrieveBlocks(5)
		if err != nil {
			panic(err)
		}
		go func() {
			itr.Next()
		}()
		itr.Close()
	}

	for range 1000 {
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
	require.NoError(t, err)
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
	errs := make([]error, 2)
	itr1, err := blkfileMgr.retrieveBlocks(7)
	require.NoError(t, err)
	// itr1 does not retrieve any block because it closes before new blocks are added
	wg.Go(func() {
		errs[0] = iterateInBackground(t, itr1, 9, []uint64{})
	})

	itr2, err := blkfileMgr.retrieveBlocks(8)
	require.NoError(t, err)
	// itr2 retrieves two blocks 8 and 9. Because it started waiting for 8 and quits at 9
	wg.Go(func() {
		errs[1] = iterateInBackground(t, itr2, 9, []uint64{8, 9})
	})

	// sleep for the background iterators to get started
	time.Sleep(2 * time.Second)
	itr1.Close()
	blkfileMgrWrapper.addBlocks(blocks[5:])
	wg.Wait()
	for _, err = range errs {
		require.NoError(t, err)
	}
}

// iterateInBackground is meant to be called from a goroutine other than the one
// running the test, so it reports failures through the returned error instead
// of asserting.
func iterateInBackground(t *testing.T, itr *blocksItr, quitAfterBlkNum uint64, expectedBlockNums []uint64) (err error) {
	var retrievedBlkNums []uint64
	defer func() {
		if err == nil && !slices.Equal(expectedBlockNums, retrievedBlkNums) {
			err = fmt.Errorf("expected block numbers %v, got %v", expectedBlockNums, retrievedBlkNums)
		}
	}()

	for {
		blk, err := itr.Next()
		if err != nil {
			return err
		}
		if blk == nil {
			return nil
		}
		blkNum := blk.(*common.Block).GetHeader().GetNumber()
		retrievedBlkNums = append(retrievedBlkNums, blkNum)
		t.Logf("blk.Num=%d", blk.(*common.Block).GetHeader().GetNumber())
		if blkNum == quitAfterBlkNum {
			return nil
		}
	}
}

// testIterateAndVerify is meant to be called from a goroutine other than the
// one running the test, so it does not assert: it returns the iterator error
// and the blocks it read, leaving the verification to the test goroutine.
func testIterateAndVerify(t *testing.T, itr *blocksItr, blocks []*common.Block, readyAt int, readyChan chan<- struct{}) ([]*common.Block, error) {
	iterated := make([]*common.Block, 0, len(blocks))
	for {
		t.Logf("blocksIterated: %v", len(iterated))
		block, err := itr.Next()
		if err != nil {
			return iterated, err
		}
		iterated = append(iterated, block.(*common.Block))
		if len(iterated) == readyAt {
			close(readyChan)
		}
		if len(iterated) == len(blocks) {
			return iterated, nil
		}
	}
}

func testAppendBlocks(blkfileMgrWrapper *testBlockfileMgrWrapper, blocks []*common.Block) {
	blkfileMgrWrapper.addBlocks(blocks)
}
