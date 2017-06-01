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

package jsonledger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	ledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/jsonpb"
)

var logger = logging.MustGetLogger("orderer/jsonledger")
var closedChan chan struct{}
var fileLock sync.Mutex

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

const (
	blockFileFormatString      = "block_%020d.json"
	chainDirectoryFormatString = "chain_%s"
)

type cursor struct {
	jl          *jsonLedger
	blockNumber uint64
}

type jsonLedger struct {
	directory string
	height    uint64
	signal    chan struct{}
	lastHash  []byte
	marshaler *jsonpb.Marshaler
}

// readBlock returns the block or nil, and whether the block was found or not, (nil,true) generally indicates an irrecoverable problem
func (jl *jsonLedger) readBlock(number uint64) (*cb.Block, bool) {
	name := jl.blockFilename(number)

	// In case of ongoing write, reading the block file may result in `unexpected EOF` error.
	// Therefore, we use file mutex here to prevent this race condition.
	fileLock.Lock()
	defer fileLock.Unlock()

	file, err := os.Open(name)
	if err == nil {
		defer file.Close()
		block := &cb.Block{}
		err = jsonpb.Unmarshal(file, block)
		if err != nil {
			return nil, true
		}
		logger.Debugf("Read block %d", block.Header.Number)
		return block, true
	}
	return nil, false
}

// Next blocks until there is a new block available, or returns an error if the
// next block is no longer retrievable
func (cu *cursor) Next() (*cb.Block, cb.Status) {
	// This only loops once, as signal reading
	// indicates the new block has been written
	for {
		block, found := cu.jl.readBlock(cu.blockNumber)
		if found {
			if block == nil {
				return nil, cb.Status_SERVICE_UNAVAILABLE
			}
			cu.blockNumber++
			return block, cb.Status_SUCCESS
		}
		<-cu.jl.signal
	}
}

// ReadyChan supplies a channel which will block until Next will not block
func (cu *cursor) ReadyChan() <-chan struct{} {
	signal := cu.jl.signal
	if _, err := os.Stat(cu.jl.blockFilename(cu.blockNumber)); os.IsNotExist(err) {
		return signal
	}
	return closedChan
}

// Iterator returns an Iterator, as specified by a cb.SeekInfo message, and its
// starting block number
func (jl *jsonLedger) Iterator(startPosition *ab.SeekPosition) (ledger.Iterator, uint64) {
	switch start := startPosition.Type.(type) {
	case *ab.SeekPosition_Oldest:
		return &cursor{jl: jl, blockNumber: 0}, 0
	case *ab.SeekPosition_Newest:
		high := jl.height - 1
		return &cursor{jl: jl, blockNumber: high}, high
	case *ab.SeekPosition_Specified:
		if start.Specified.Number > jl.height {
			return &ledger.NotFoundErrorIterator{}, 0
		}
		return &cursor{jl: jl, blockNumber: start.Specified.Number}, start.Specified.Number
	default:
		return &ledger.NotFoundErrorIterator{}, 0
	}
}

// Height returns the number of blocks on the ledger
func (jl *jsonLedger) Height() uint64 {
	return jl.height
}

// Append appends a new block to the ledger
func (jl *jsonLedger) Append(block *cb.Block) error {
	if block.Header.Number != jl.height {
		return fmt.Errorf("Block number should have been %d but was %d", jl.height, block.Header.Number)
	}

	if !bytes.Equal(block.Header.PreviousHash, jl.lastHash) {
		return fmt.Errorf("Block should have had previous hash of %x but was %x", jl.lastHash, block.Header.PreviousHash)
	}

	jl.writeBlock(block)
	jl.lastHash = block.Header.Hash()
	jl.height++
	close(jl.signal)
	jl.signal = make(chan struct{})
	return nil
}

// writeBlock commits a block to disk
func (jl *jsonLedger) writeBlock(block *cb.Block) {
	name := jl.blockFilename(block.Header.Number)

	fileLock.Lock()
	defer fileLock.Unlock()

	file, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	err = jl.marshaler.Marshal(file, block)
	logger.Debugf("Wrote block %d", block.Header.Number)
	if err != nil {
		logger.Panic(err)
	}
}

// blockFilename returns the fully qualified path to where a block
// of a given number should be stored on disk
func (jl *jsonLedger) blockFilename(number uint64) string {
	return filepath.Join(jl.directory, fmt.Sprintf(blockFileFormatString, number))
}
