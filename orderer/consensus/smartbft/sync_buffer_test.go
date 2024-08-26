/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
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

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			block := buff.PullBlock(1)
			require.Nil(t, block)
			wg.Done()
		}()

		buff.Stop()
		wg.Wait()
	})

	t.Run("blocks until HandleBlock is called", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		blockIn := &common.Block{
			Header: &common.BlockHeader{Number: 2, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			blockOut := buff.PullBlock(2)
			require.NotNil(t, blockOut)
			require.True(t, proto.Equal(blockIn, blockOut))
			wg.Done()
		}()

		err := buff.HandleBlock("mychannel", blockIn)
		require.NoError(t, err)
		wg.Wait()
	})

	t.Run("block number mismatch, request number lower than head, return nil", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		blockIn := &common.Block{
			Header: &common.BlockHeader{Number: 2, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			blockOut := buff.PullBlock(1)
			require.Nil(t, blockOut)
			wg.Done()
		}()

		err := buff.HandleBlock("mychannel", blockIn)
		require.NoError(t, err)
		wg.Wait()
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

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			blockOut := buff.PullBlock(3)
			require.NotNil(t, blockOut)
			require.True(t, proto.Equal(blockIn3, blockOut))
			wg.Done()
		}()

		err := buff.HandleBlock("mychannel", blockIn2)
		require.NoError(t, err)
		err = buff.HandleBlock("mychannel", blockIn3)
		require.NoError(t, err)

		wg.Wait()
	})

	t.Run("continuous operation", func(t *testing.T) {
		buff := smartbft.NewSyncBuffer(100)
		require.NotNil(t, buff)

		var wg sync.WaitGroup
		wg.Add(1)

		firstBlock := uint64(10)
		lastBlock := uint64(1000)
		go func() {
			j := firstBlock
			for {
				blockOut := buff.PullBlock(j)
				require.NotNil(t, blockOut)
				require.Equal(t, j, blockOut.Header.Number)
				j++
				if j == lastBlock {
					break
				}
			}
			wg.Done()
		}()

		for i := firstBlock; i <= lastBlock; i++ {
			blockIn := &common.Block{
				Header: &common.BlockHeader{Number: i, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
			}
			err := buff.HandleBlock("mychannel", blockIn)
			require.NoError(t, err)
		}

		wg.Wait()
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

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			var number uint64 = 1
			var err error
			for {
				blockIn := &common.Block{
					Header: &common.BlockHeader{Number: number, PreviousHash: []byte{1, 2, 3, 4}, DataHash: []byte{5, 6, 7, 8}},
				}
				err = buff.HandleBlock("mychannel", blockIn)
				if err != nil {
					break
				}
				number++
			}

			require.EqualError(t, err, "SyncBuffer stopping, channel: mychannel")
			wg.Done()
		}()

		buff.Stop()
		wg.Wait()
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
