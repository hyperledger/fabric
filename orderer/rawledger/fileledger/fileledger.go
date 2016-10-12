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

package fileledger

import (
	"fmt"
	"io/ioutil"
	"os"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/rawledger"

	"github.com/golang/protobuf/jsonpb"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("rawledger/fileledger")
var closedChan chan struct{}

func init() {
	logging.SetLevel(logging.DEBUG, "")
	closedChan = make(chan struct{})
	close(closedChan)
}

const blockFileFormatString string = "block_%020d.json"

type cursor struct {
	fl          *fileLedger
	blockNumber uint64
}

type fileLedger struct {
	directory      string
	fqFormatString string
	height         uint64
	signal         chan struct{}
	lastHash       []byte
	marshaler      *jsonpb.Marshaler
}

// New creates a new instance of the file ledger
func New(directory string, genesisBlock *ab.Block) rawledger.ReadWriter {
	logger.Debugf("Initializing fileLedger at '%s'", directory)
	if err := os.MkdirAll(directory, 0700); err != nil {
		panic(err)
	}
	fl := &fileLedger{
		directory:      directory,
		fqFormatString: directory + "/" + blockFileFormatString,
		signal:         make(chan struct{}),
		marshaler:      &jsonpb.Marshaler{Indent: "  "},
	}
	if _, err := os.Stat(fl.blockFilename(genesisBlock.Number)); os.IsNotExist(err) {
		fl.writeBlock(genesisBlock)
	}
	fl.initializeBlockHeight()
	logger.Debugf("Initialized to block height %d with hash %x", fl.height-1, fl.lastHash)
	return fl
}

// initializeBlockHeight verifies all blocks exist between 0 and the block height, and populates the lastHash
func (fl *fileLedger) initializeBlockHeight() {
	infos, err := ioutil.ReadDir(fl.directory)
	if err != nil {
		panic(err)
	}
	nextNumber := uint64(0)
	for _, info := range infos {
		if info.IsDir() {
			continue
		}
		var number uint64
		_, err := fmt.Sscanf(info.Name(), blockFileFormatString, &number)
		if err != nil {
			continue
		}
		if number != nextNumber {
			panic(fmt.Errorf("Missing block %d in the chain", nextNumber))
		}
		nextNumber++
	}
	fl.height = nextNumber
	block, found := fl.readBlock(fl.height - 1)
	if !found {
		panic(fmt.Errorf("Block %d was in directory listing but error reading", fl.height-1))
	}
	if block == nil {
		panic(fmt.Errorf("Error reading block %d", fl.height-1))
	}
	fl.lastHash = block.Hash()
}

// blockFilename returns the fully qualified path to where a block of a given number should be stored on disk
func (fl *fileLedger) blockFilename(number uint64) string {
	return fmt.Sprintf(fl.fqFormatString, number)
}

// writeBlock commits a block to disk
func (fl *fileLedger) writeBlock(block *ab.Block) {
	file, err := os.Create(fl.blockFilename(block.Number))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	err = fl.marshaler.Marshal(file, block)
	logger.Debugf("Wrote block %d", block.Number)
	if err != nil {
		panic(err)
	}

}

// readBlock returns the block or nil, and whether the block was found or not, (nil,true) generally indicates an irrecoverable problem
func (fl *fileLedger) readBlock(number uint64) (*ab.Block, bool) {
	file, err := os.Open(fl.blockFilename(number))
	if err == nil {
		defer file.Close()
		block := &ab.Block{}
		err = jsonpb.Unmarshal(file, block)
		if err != nil {
			return nil, true
		}
		logger.Debugf("Read block %d", block.Number)
		return block, true
	}
	return nil, false
}

// Height returns the highest block number in the chain, plus one
func (fl *fileLedger) Height() uint64 {
	return fl.height
}

// Append creates a new block and appends it to the ledger
func (fl *fileLedger) Append(messages []*ab.BroadcastMessage, proof []byte) *ab.Block {
	block := &ab.Block{
		Number:   fl.height,
		PrevHash: fl.lastHash,
		Messages: messages,
		Proof:    proof,
	}
	fl.writeBlock(block)
	fl.height++
	close(fl.signal)
	fl.signal = make(chan struct{})
	return block
}

// Iterator implements the rawledger.Reader definition
func (fl *fileLedger) Iterator(startType ab.SeekInfo_StartType, specified uint64) (rawledger.Iterator, uint64) {
	switch startType {
	case ab.SeekInfo_OLDEST:
		return &cursor{fl: fl, blockNumber: 0}, 0
	case ab.SeekInfo_NEWEST:
		high := fl.height - 1
		return &cursor{fl: fl, blockNumber: high}, high
	case ab.SeekInfo_SPECIFIED:
		if specified > fl.height {
			return &rawledger.NotFoundErrorIterator{}, 0
		}
		return &cursor{fl: fl, blockNumber: specified}, specified
	}

	// This line should be unreachable, but the compiler requires it
	return &rawledger.NotFoundErrorIterator{}, 0
}

// Next blocks until there is a new block available, or returns an error if the next block is no longer retrievable
func (cu *cursor) Next() (*ab.Block, ab.Status) {
	// This only loops once, as signal reading indicates the new block has been written
	for {
		block, found := cu.fl.readBlock(cu.blockNumber)
		if found {
			if block == nil {
				return nil, ab.Status_SERVICE_UNAVAILABLE
			}
			cu.blockNumber++
			return block, ab.Status_SUCCESS
		}
		<-cu.fl.signal
	}
}

// ReadyChan returns a channel that will close when Next is ready to be called without blocking
func (cu *cursor) ReadyChan() <-chan struct{} {
	signal := cu.fl.signal
	if _, err := os.Stat(cu.fl.blockFilename(cu.blockNumber)); os.IsNotExist(err) {
		return signal
	}
	return closedChan
}
