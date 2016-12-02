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
	"sync"

	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
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

type ramLedgerFactory struct {
	maxSize int
	ledgers map[string]rawledger.ReadWriter
	mutex   sync.Mutex
}

func New(maxSize int, systemGenesis *cb.Block) (rawledger.Factory, rawledger.ReadWriter) {
	rlf := &ramLedgerFactory{
		maxSize: maxSize,
		ledgers: make(map[string]rawledger.ReadWriter),
	}
	env := &cb.Envelope{}
	err := proto.Unmarshal(systemGenesis.Data.Data[0], env)
	if err != nil {
		logger.Fatalf("Bad envelope in genesis block: %s", err)
	}

	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	if err != nil {
		logger.Fatalf("Bad payload in genesis block: %s", err)
	}

	rl, _ := rlf.GetOrCreate(payload.Header.ChainHeader.ChainID)
	rl.(*ramLedger).appendBlock(systemGenesis)
	return rlf, rl
}

func (rlf *ramLedgerFactory) GetOrCreate(chainID string) (rawledger.ReadWriter, error) {
	rlf.mutex.Lock()
	defer rlf.mutex.Unlock()

	key := chainID

	// Check a second time with the lock held
	l, ok := rlf.ledgers[key]
	if ok {
		return l, nil
	}

	ch := newChain(rlf.maxSize)
	rlf.ledgers[key] = ch
	return ch, nil
}

func (rlf *ramLedgerFactory) ChainIDs() []string {
	rlf.mutex.Lock()
	defer rlf.mutex.Unlock()
	ids := make([]string, len(rlf.ledgers))

	i := 0
	for key := range rlf.ledgers {
		ids[i] = key
		i++
	}

	return ids
}

// newChain creates a new instance of the ram ledger for a chain
func newChain(maxSize int) rawledger.ReadWriter {
	preGenesis := &cb.Block{
		Header: &cb.BlockHeader{
			Number: ^uint64(0),
		},
	}

	rl := &ramLedger{
		maxSize: maxSize,
		size:    1,
		oldest: &simpleList{
			signal: make(chan struct{}),
			block:  preGenesis,
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
		logger.Debugf("Attempting to return block %d", specified)
		oldest := rl.oldest
		// Note the two +1's here is to accomodate the 'preGenesis' block of ^uint64(0)
		if specified+1 < oldest.block.Header.Number+1 || specified > rl.newest.block.Header.Number+1 {
			logger.Debugf("Returning error iterator because specified seek was %d with oldest %d and newest %d", specified, rl.oldest.block.Header.Number, rl.newest.block.Header.Number)
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
	cursor := &cursor{list: list}
	blockNum := list.block.Header.Number + 1

	// If the cursor is for pre-genesis, skip it, the block number wraps
	if blockNum == ^uint64(0) {
		cursor.Next()
		blockNum++
	}

	return cursor, blockNum
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
func (rl *ramLedger) Append(messages []*cb.Envelope, metadata [][]byte) *cb.Block {
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
			Metadata: metadata,
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
		logger.Debugf("RAM ledger max size about to be exceeded, removing oldest item: %d", rl.oldest.block.Header.Number)
		rl.oldest = rl.oldest.next
		rl.size--
	}
}
