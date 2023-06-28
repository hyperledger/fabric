/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
)

const (
	estimatedBlockSize = 512 * 1024
)

type cache struct {
	cacheLock    sync.RWMutex
	disabled     bool
	cache        map[uint64]cachedBlock
	sizeBytes    int
	maxSeq       uint64
	maxSizeBytes int
}

type cachedBlock struct {
	block     *common.Block
	blockSize int
}

func newCache(maxSizeBytes int) *cache {
	isCacheDisabled := maxSizeBytes == 0

	return &cache{
		disabled:     isCacheDisabled,
		cache:        make(map[uint64]cachedBlock, maxSizeBytes/estimatedBlockSize),
		maxSizeBytes: maxSizeBytes,
	}
}

func (c *cache) get(seq uint64) (*common.Block, bool) {
	if c.disabled {
		return nil, false
	}

	c.cacheLock.RLock()
	defer c.cacheLock.RUnlock()

	cachedBlock, exists := c.cache[seq]
	return cachedBlock.block, exists
}

func (c *cache) put(block *common.Block, blockSize int) {
	if c.disabled {
		return
	}

	seq := block.Header.Number

	if c.maxSeq > seq {
		return
	}

	if c.maxSeq+1 < seq && c.maxSeq != 0 {
		panic(fmt.Sprintf("detected out of order block insertion: attempted to insert block number %d but highest block is %d",
			seq, c.maxSeq))
	}

	if c.maxSeq == seq && c.maxSeq != 0 {
		panic(fmt.Sprintf("detected insertion of the same block (%d) twice", seq))
	}

	// Insert the block to the cache
	c.maxSeq = seq

	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	c.sizeBytes += blockSize

	c.cache[seq] = cachedBlock{block: block, blockSize: blockSize}

	// If our cache is too big, evict the oldest block
	for c.sizeBytes > c.maxSizeBytes {
		c.evictOldestCachedBlock()
	}
}

func (c *cache) evictOldestCachedBlock() {
	cachedItemCount := len(c.cache)

	// Given a series of k > 0 consecutive elements: {i, i+1, i+2, ... , i+k-1}
	// If the max sequence is j then j=i+k-1, and then the lowest element i is j-k+1
	evictedIndex := c.maxSeq - uint64(cachedItemCount) + 1
	evictedBlock, exists := c.cache[evictedIndex]
	if !exists {
		panic(fmt.Sprintf("programming error: last stored block sequence is %d and cached block count"+
			" is %d but index to be evicted %d was not found", c.maxSeq, cachedItemCount, evictedIndex))
	}
	delete(c.cache, evictedIndex) // Delete minimum entry
	c.sizeBytes -= evictedBlock.blockSize
}
