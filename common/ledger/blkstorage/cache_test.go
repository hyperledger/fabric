/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

func TestCacheDisabled(t *testing.T) {
	c := newCache(0)

	c.put(&common.Block{Header: &common.BlockHeader{}}, 0)
	block, exists := c.get(0)

	assertNotCached(t, exists, block)
}

func TestNotCachingTooSmallEntry(t *testing.T) {
	c := newCache(10)

	for i := 100; i < 105; i++ {
		block := &common.Block{
			Header: &common.BlockHeader{Number: uint64(i)},
		}
		c.put(block, 1)
	}

	c.put(&common.Block{Header: &common.BlockHeader{Number: 99}}, 1)
	block, exists := c.get(99)

	assertNotCached(t, exists, block)
}

func TestTooBigEntryNotCached(t *testing.T) {
	c := newCache(10)

	c.put(&common.Block{Header: &common.BlockHeader{Number: 100}}, 11)
	block, exists := c.get(100)

	assertNotCached(t, exists, block)
}

func TestOutOfOrderInsertionPanics(t *testing.T) {
	c := newCache(10)

	c.put(&common.Block{Header: &common.BlockHeader{Number: 100}}, 1)
	block, exists := c.get(100)

	assertCached(t, exists, block, 100)

	func() {
		defer func() {
			err := recover()
			assert.Contains(t, err.(string), "detected out of order block insertion: attempted to insert block number 102 but highest block is 100")
		}()

		c.put(&common.Block{Header: &common.BlockHeader{Number: 102}}, 1)
	}()
}

func TestDoubleInsertionPanics(t *testing.T) {
	c := newCache(10)

	c.put(&common.Block{Header: &common.BlockHeader{Number: 100}}, 1)
	block, exists := c.get(100)

	assertCached(t, exists, block, 100)

	func() {
		defer func() {
			err := recover()
			assert.Contains(t, err.(string), "detected insertion of the same block (100) twice")
		}()

		c.put(&common.Block{Header: &common.BlockHeader{Number: 100}}, 1)
	}()
}

func TestTooBigEntryEvictsSmallerEntries(t *testing.T) {
	c := newCache(10)

	c.put(&common.Block{Header: &common.BlockHeader{Number: 1}}, 1)
	c.put(&common.Block{Header: &common.BlockHeader{Number: 2}}, 1)
	c.put(&common.Block{Header: &common.BlockHeader{Number: 3}}, 1)

	block, exists := c.get(1)
	assertCached(t, exists, block, 1)

	block, exists = c.get(2)
	assertCached(t, exists, block, 2)

	block, exists = c.get(3)
	assertCached(t, exists, block, 3)

	c.put(&common.Block{Header: &common.BlockHeader{Number: 4}}, 10)

	block, exists = c.get(1)
	assertNotCached(t, exists, block)

	block, exists = c.get(2)
	assertNotCached(t, exists, block)

	block, exists = c.get(3)
	assertNotCached(t, exists, block)

	block, exists = c.get(4)
	assertCached(t, exists, block, 4)
}

func TestCacheEviction(t *testing.T) {
	c := newCache(10)

	for i := 0; i < 10; i++ {
		block := &common.Block{
			Header: &common.BlockHeader{Number: uint64(i)},
		}
		c.put(block, 1)
	}

	for i := 10; i < 20; i++ {
		// Ensure items 11 blocks in the past are not cached, but evicted
		if uint64(i) > 10 {
			block, exists := c.get(uint64(i) - 11)
			assertNotCached(t, exists, block)
		}
		// Ensure items 10 blocks in the past are still cached
		block, exists := c.get(uint64(i) - 10)
		assertCached(t, exists, block, uint64(i)-10)

		block = &common.Block{
			Header: &common.BlockHeader{Number: uint64(i)},
		}
		c.put(block, 1)
	}

	block, exists := c.get(9)
	assertNotCached(t, exists, block)

	for i := 10; i < 20; i++ {
		block, exists := c.get(uint64(i))
		assertCached(t, exists, block, uint64(i))
	}
}

func assertNotCached(t *testing.T, exists bool, block *common.Block) {
	assertWasCached(t, exists, block, 0, false)
}

func assertCached(t *testing.T, exists bool, block *common.Block, expectedSeq uint64) {
	assertWasCached(t, exists, block, expectedSeq, true)
}

func assertWasCached(t *testing.T, exists bool, block *common.Block, expectedSeq uint64, expectedExists bool) {
	if !expectedExists {
		require.False(t, exists)
		require.Nil(t, block)
		return
	}
	require.True(t, exists)
	require.NotNil(t, block)
	require.Equal(t, expectedSeq, block.Header.Number)
}
