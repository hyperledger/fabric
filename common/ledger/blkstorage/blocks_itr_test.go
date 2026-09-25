/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
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
	go func() {
		<-readyChan
		blkfileMgrWrapper.addBlocks(blocks[5:7])
		blkfileMgr.moveToNextFile()
		time.Sleep(time.Millisecond * 10)
		blkfileMgrWrapper.addBlocks(blocks[7:])
		doneChan <- true
	}()

	for i, b := range blocks[1:] {
		t.Logf("blocksIterated: %d", i)
		block, err := itr.Next()
		require.NoError(t, err)
		require.Equal(t, b, block)
		if i == 3 {
			close(readyChan)
		}
	}

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
	itr1, err := blkfileMgr.retrieveBlocks(7)
	require.NoError(t, err)
	// itr1 does not retrieve any block because it closes before new blocks are added
	iterateErrs := make(chan error, 2)
	wg.Go(func() {
		iterateErrs <- iterateInBackground(itr1, 9, []uint64{})
	})

	itr2, err := blkfileMgr.retrieveBlocks(8)
	require.NoError(t, err)
	// itr2 retrieves two blocks 8 and 9. Because it started waiting for 8 and quits at 9
	wg.Go(func() {
		iterateErrs <- iterateInBackground(itr2, 9, []uint64{8, 9})
	})

	// sleep for the background iterators to get started
	time.Sleep(2 * time.Second)
	itr1.Close()
	blkfileMgrWrapper.addBlocks(blocks[5:])
	wg.Wait()
	close(iterateErrs)
	for err = range iterateErrs {
		require.NoError(t, err)
	}
}

func iterateInBackground(itr *blocksItr, quitAfterBlkNum uint64, expectedBlockNums []uint64) error {
	retrievedBlkNums := []uint64{}

	for {
		blk, err := itr.Next()
		if err != nil {
			return err
		}
		if blk == nil {
			break
		}
		blkNum := blk.(*common.Block).GetHeader().GetNumber()
		retrievedBlkNums = append(retrievedBlkNums, blkNum)
		if blkNum == quitAfterBlkNum {
			break
		}
	}
	if len(retrievedBlkNums) != len(expectedBlockNums) {
		return fmt.Errorf("expected block numbers %v, got %v", expectedBlockNums, retrievedBlkNums)
	}
	for i, expected := range expectedBlockNums {
		if retrievedBlkNums[i] != expected {
			return fmt.Errorf("expected block numbers %v, got %v", expectedBlockNums, retrievedBlkNums)
		}
	}
	return nil
}
