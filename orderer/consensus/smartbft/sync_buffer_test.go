/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNewSyncBuffer(t *testing.T) {
	buff := smartbft.NewSyncBuffer(100)
	require.NotNil(t, buff)
}

func TestSyncBuffer_PullBlock(t *testing.T) {
	t.Run("blocks until stopped", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		result := make(chan *common.Block, 1)
		var wg sync.WaitGroup
		wg.Go(func() { result <- buff.PullBlock(1) })

		buff.Stop()
		wg.Wait()
		require.Nil(t, <-result)
	})

	t.Run("blocks until HandleBlock is called", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		blockIn := &common.Block{
			Header: &common.BlockHeader{Number: 2, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}
		result := make(chan *common.Block, 1)
		var wg sync.WaitGroup
		wg.Go(func() { result <- buff.PullBlock(2) })

		err := buff.HandleBlock("mychannel", blockIn)
		require.NoError(t, err)
		wg.Wait()
		blockOut := <-result
		require.NotNil(t, blockOut)
		require.True(t, proto.Equal(blockIn, blockOut))
	})

	t.Run("block number mismatch, request number lower than head, return nil", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		blockIn := &common.Block{
			Header: &common.BlockHeader{Number: 2, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}
		result := make(chan *common.Block, 1)
		var wg sync.WaitGroup
		wg.Go(func() { result <- buff.PullBlock(1) })

		err := buff.HandleBlock("mychannel", blockIn)
		require.NoError(t, err)
		wg.Wait()
		require.Nil(t, <-result)
	})

	t.Run("block number mismatch, requested number higher than head, blocks until inserted", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		blockIn2 := &common.Block{
			Header: &common.BlockHeader{Number: 2, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}
		blockIn3 := &common.Block{
			Header: &common.BlockHeader{Number: 3, PreviousHash: []byte{9, 10, 11, 12}, DataHash: []byte{13, 14, 15, 16}},
		}
		result := make(chan *common.Block, 1)
		var wg sync.WaitGroup
		wg.Go(func() { result <- buff.PullBlock(3) })

		err := buff.HandleBlock("mychannel", blockIn2)
		require.NoError(t, err)
		err = buff.HandleBlock("mychannel", blockIn3)
		require.NoError(t, err)
		wg.Wait()
		blockOut := <-result
		require.NotNil(t, blockOut)
		require.True(t, proto.Equal(blockIn3, blockOut))
	})

	t.Run("continuous operation", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		var wg sync.WaitGroup
		errCh := make(chan error, 1)
		firstBlock := uint64(10)
		lastBlock := uint64(1000)
		wg.Go(func() {
			for j := firstBlock; j < lastBlock; j++ {
				blockOut := buff.PullBlock(j)
				if blockOut == nil {
					errCh <- fmt.Errorf("expected block %d, got nil", j)
					return
				}
				if blockOut.GetHeader().GetNumber() != j {
					errCh <- fmt.Errorf("expected block %d, got %d", j, blockOut.GetHeader().GetNumber())
					return
				}
			}
			errCh <- nil
		})

		for i := firstBlock; i <= lastBlock; i++ {
			blockIn := &common.Block{
				Header: &common.BlockHeader{Number: i, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
			}
			err := buff.HandleBlock("mychannel", blockIn)
			require.NoError(t, err)
		}

		wg.Wait()
		require.NoError(t, <-errCh)
	})

	t.Run("zero capacity is still buffered and does not block", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(0)
		require.NotNil(t, buff)

		blockIn := &common.Block{
			Header: &common.BlockHeader{Number: uint64(10), PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}
		err := buff.HandleBlock("mychannel", blockIn)
		require.NoError(t, err)
	})
}

func TestSyncBuffer_HandleBlock(t *testing.T) {
	t.Run("blocks until stopped", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		errCh := make(chan error, 1)
		var wg sync.WaitGroup
		wg.Go(func() {
			var number uint64 = 1
			for {
				blockIn := &common.Block{
					Header: &common.BlockHeader{Number: number, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
				}
				if err := buff.HandleBlock("mychannel", blockIn); err != nil {
					errCh <- err
					return
				}
				number++
			}
		})

		buff.Stop()
		wg.Wait()
		require.EqualError(t, <-errCh, "SyncBuffer stopping, channel: mychannel")
	})

	t.Run("bad blocks", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		err := buff.HandleBlock("mychannel", nil)
		require.EqualError(t, err, "empty block or block header, channel: mychannel")

		err = buff.HandleBlock("mychannel", &common.Block{})
		require.EqualError(t, err, "empty block or block header, channel: mychannel")
	})
}
