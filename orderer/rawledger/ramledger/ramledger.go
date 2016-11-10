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
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("rawledger/ramledger")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

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

// New creates a new instance of the ram ledger
func New(maxSize int, genesis *cb.Block) rawledger.ReadWriter {
	rl := &ramLedger{
		maxSize: maxSize,
		size:    1,
		oldest: &simpleList{
			signal: make(chan struct{}),
			block:  genesis,
		},
	}
	rl.newest = rl.oldest
	return rl
}

// Height returns the highest block number in the chain, plus one
func (rl *ramLedger) Height() uint64 {
	return rl.newest.block.Header.Number + 1
}

// Iterator implements the rawledger.Reader definition
func (rl *ramLedger) Iterator(startType ab.SeekInfo_StartType, specified uint64) (rawledger.Iterator, uint64) {
	var list *simpleList
	switch startType {
	case ab.SeekInfo_OLDEST:
		oldest := rl.oldest
		list = &simpleList{
			block:  &cb.Block{Header: &cb.BlockHeader{Number: oldest.block.Header.Number - 1}},
			next:   oldest,
			signal: make(chan struct{}),
		}
		close(list.signal)
	case ab.SeekInfo_NEWEST:
		newest := rl.newest
		list = &simpleList{
			block:  &cb.Block{Header: &cb.BlockHeader{Number: newest.block.Header.Number - 1}},
			next:   newest,
			signal: make(chan struct{}),
		}
		close(list.signal)
	case ab.SeekInfo_SPECIFIED:
		oldest := rl.oldest
		if specified < oldest.block.Header.Number || specified > rl.newest.block.Header.Number+1 {
			return &rawledger.NotFoundErrorIterator{}, 0
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
	return &cursor{list: list}, list.block.Header.Number + 1
}

// Next blocks until there is a new block available, or returns an error if the next block is no longer retrievable
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

// ReadyChan returns a channel that will close when Next is ready to be called without blocking
func (cu *cursor) ReadyChan() <-chan struct{} {
	return cu.list.signal
}

// Append creates a new block and appends it to the ledger
func (rl *ramLedger) Append(messages []*cb.Envelope, proof []byte) *cb.Block {
	data := &cb.BlockData{
		Data: make([][]byte, len(messages)),
	}

	var err error
	for i, msg := range messages {
		data.Data[i], err = proto.Marshal(msg)
		if err != nil {
			logger.Fatalf("Error marshaling data which should be a valid proto: %s", err)
		}
	}

	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number:       rl.newest.block.Header.Number + 1,
			PreviousHash: rl.newest.block.Header.Hash(),
			DataHash:     data.Hash(),
		},
		Data: data,
		Metadata: &cb.BlockMetadata{
			Metadata: [][]byte{proof},
		},
	}
	rl.appendBlock(block)
	return block
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
		rl.oldest = rl.oldest.next
		rl.size--
	}
}
