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

package ramledger

import (
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/ramledger")

type cursor struct {
	list *simpleList
}

type simpleList struct {
	next   *simpleList
	signal chan struct{}
	block  *cb.Block
}

type ramLedger struct {
	maxSize int
	size    int
	oldest  *simpleList
	newest  *simpleList
}

// Next blocks until there is a new block available, or returns an error if the
// next block is no longer retrievable
func (cu *cursor) Next() (*cb.Block, cb.Status) {
	// This only loops once, as signal reading indicates non-nil next
	for {
		if cu.list.next != nil {
			cu.list = cu.list.next
			return cu.list.block, cb.Status_SUCCESS
		}
		<-cu.list.signal
	}
}

// ReadyChan supplies a channel which will block until Next will not block
func (cu *cursor) ReadyChan() <-chan struct{} {
	return cu.list.signal
}

// Iterator returns an Iterator, as specified by a cb.SeekInfo message, and its
// starting block number
func (rl *ramLedger) Iterator(startPosition *ab.SeekPosition) (ledger.Iterator, uint64) {
	var list *simpleList
	switch start := startPosition.Type.(type) {
	case *ab.SeekPosition_Oldest:
		oldest := rl.oldest
		list = &simpleList{
			block:  &cb.Block{Header: &cb.BlockHeader{Number: oldest.block.Header.Number - 1}},
			next:   oldest,
			signal: make(chan struct{}),
		}
		close(list.signal)
	case *ab.SeekPosition_Newest:
		newest := rl.newest
		list = &simpleList{
			block:  &cb.Block{Header: &cb.BlockHeader{Number: newest.block.Header.Number - 1}},
			next:   newest,
			signal: make(chan struct{}),
		}
		close(list.signal)
	case *ab.SeekPosition_Specified:
		oldest := rl.oldest
		specified := start.Specified.Number
		logger.Debugf("Attempting to return block %d", specified)

		// Note the two +1's here is to accommodate the 'preGenesis' block of ^uint64(0)
		if specified+1 < oldest.block.Header.Number+1 || specified > rl.newest.block.Header.Number+1 {
			logger.Debugf("Returning error iterator because specified seek was %d with oldest %d and newest %d",
				specified, rl.oldest.block.Header.Number, rl.newest.block.Header.Number)
			return &ledger.NotFoundErrorIterator{}, 0
		}

		if specified == oldest.block.Header.Number {
			list = &simpleList{
				block:  &cb.Block{Header: &cb.BlockHeader{Number: oldest.block.Header.Number - 1}},
				next:   oldest,
				signal: make(chan struct{}),
			}
			close(list.signal)
			break
		}

		list = oldest
		for {
			if list.block.Header.Number == specified-1 {
				break
			}
			list = list.next // No need for nil check, because of range check above
		}
	}
	cursor := &cursor{list: list}
	blockNum := list.block.Header.Number + 1

	// If the cursor is for pre-genesis, skip it, the block number wraps
	if blockNum == ^uint64(0) {
		cursor.Next()
		blockNum++
	}

	return cursor, blockNum
}

// Height returns the number of blocks on the ledger
func (rl *ramLedger) Height() uint64 {
	return rl.newest.block.Header.Number + 1
}

// Append appends a new block to the ledger
func (rl *ramLedger) Append(block *cb.Block) error {
	if block.Header.Number != rl.newest.block.Header.Number+1 {
		return fmt.Errorf("Block number should have been %d but was %d",
			rl.newest.block.Header.Number+1, block.Header.Number)
	}

	if rl.newest.block.Header.Number+1 != 0 { // Skip this check for genesis block insertion
		if !bytes.Equal(block.Header.PreviousHash, rl.newest.block.Header.Hash()) {
			return fmt.Errorf("Block should have had previous hash of %x but was %x",
				rl.newest.block.Header.Hash(), block.Header.PreviousHash)
		}
	}

	rl.appendBlock(block)
	return nil
}

func (rl *ramLedger) appendBlock(block *cb.Block) {
	rl.newest.next = &simpleList{
		signal: make(chan struct{}),
		block:  block,
	}

	lastSignal := rl.newest.signal
	logger.Debugf("Sending signal that block %d has a successor", rl.newest.block.Header.Number)
	rl.newest = rl.newest.next
	close(lastSignal)

	rl.size++

	if rl.size > rl.maxSize {
		logger.Debugf("RAM ledger max size about to be exceeded, removing oldest item: %d",
			rl.oldest.block.Header.Number)
		rl.oldest = rl.oldest.next
		rl.size--
	}
}
