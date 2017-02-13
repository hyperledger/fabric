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
	"io/ioutil"
	"os"
	"sync"

	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/jsonpb"
)

var logger = logging.MustGetLogger("ordererledger/jsonledger")
var closedChan chan struct{}

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
	directory      string
	fqFormatString string
	height         uint64
	signal         chan struct{}
	lastHash       []byte
	marshaler      *jsonpb.Marshaler
}

type jsonLedgerFactory struct {
	directory string
	ledgers   map[string]ordererledger.ReadWriter
	mutex     sync.Mutex
}

// New creates a new jsonledger Factory and the ordering system chain specified by the systemGenesis block (if it does not already exist)
func New(directory string) ordererledger.Factory {

	logger.Debugf("Initializing jsonLedger at '%s'", directory)
	if err := os.MkdirAll(directory, 0700); err != nil {
		logger.Fatalf("Could not create directory %s: %s", directory, err)
	}

	jlf := &jsonLedgerFactory{
		directory: directory,
		ledgers:   make(map[string]ordererledger.ReadWriter),
	}

	infos, err := ioutil.ReadDir(jlf.directory)
	if err != nil {
		logger.Panicf("Error reading from directory %s while initializing jsonledger: %s", jlf.directory, err)
	}

	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		var chainID string
		_, err := fmt.Sscanf(info.Name(), chainDirectoryFormatString, &chainID)
		if err != nil {
			continue
		}
		jl, err := jlf.GetOrCreate(chainID)
		if err != nil {
			logger.Warningf("Failed to initialize chain from %s:", err)
			continue
		}
		jlf.ledgers[chainID] = jl
	}

	return jlf
}

func (jlf *jsonLedgerFactory) ChainIDs() []string {
	jlf.mutex.Lock()
	defer jlf.mutex.Unlock()
	ids := make([]string, len(jlf.ledgers))

	i := 0
	for key := range jlf.ledgers {
		ids[i] = key
		i++
	}

	return ids
}

func (jlf *jsonLedgerFactory) GetOrCreate(chainID string) (ordererledger.ReadWriter, error) {
	jlf.mutex.Lock()
	defer jlf.mutex.Unlock()

	key := chainID

	l, ok := jlf.ledgers[key]
	if ok {
		return l, nil
	}

	directory := fmt.Sprintf("%s/"+chainDirectoryFormatString, jlf.directory, chainID)

	logger.Debugf("Initializing chain at '%s'", directory)

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}

	ch := newChain(directory)
	jlf.ledgers[key] = ch
	return ch, nil
}

// Close does nothing for json ledger
func (jlf *jsonLedgerFactory) Close() {
	return // nothing to do
}

// newChain creates a new chain backed by a json ledger
func newChain(directory string) ordererledger.ReadWriter {
	jl := &jsonLedger{
		directory:      directory,
		fqFormatString: directory + "/" + blockFileFormatString,
		signal:         make(chan struct{}),
		marshaler:      &jsonpb.Marshaler{Indent: "  "},
	}
	jl.initializeBlockHeight()
	logger.Debugf("Initialized to block height %d with hash %x", jl.height-1, jl.lastHash)
	return jl
}

// initializeBlockHeight verifies all blocks exist between 0 and the block height, and populates the lastHash
func (jl *jsonLedger) initializeBlockHeight() {
	infos, err := ioutil.ReadDir(jl.directory)
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
	jl.height = nextNumber
	if jl.height == 0 {
		return
	}
	block, found := jl.readBlock(jl.height - 1)
	if !found {
		panic(fmt.Errorf("Block %d was in directory listing but error reading", jl.height-1))
	}
	if block == nil {
		panic(fmt.Errorf("Error reading block %d", jl.height-1))
	}
	jl.lastHash = block.Header.Hash()
}

// blockFilename returns the fully qualified path to where a block of a given number should be stored on disk
func (jl *jsonLedger) blockFilename(number uint64) string {
	return fmt.Sprintf(jl.fqFormatString, number)
}

// writeBlock commits a block to disk
func (jl *jsonLedger) writeBlock(block *cb.Block) {
	file, err := os.Create(jl.blockFilename(block.Header.Number))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	err = jl.marshaler.Marshal(file, block)
	logger.Debugf("Wrote block %d", block.Header.Number)
	if err != nil {
		panic(err)
	}

}

// readBlock returns the block or nil, and whether the block was found or not, (nil,true) generally indicates an irrecoverable problem
func (jl *jsonLedger) readBlock(number uint64) (*cb.Block, bool) {
	file, err := os.Open(jl.blockFilename(number))
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

// Height returns the highest block number in the chain, plus one
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

// Iterator implements the ordererledger.Reader definition
func (jl *jsonLedger) Iterator(startPosition *ab.SeekPosition) (ordererledger.Iterator, uint64) {
	switch start := startPosition.Type.(type) {
	case *ab.SeekPosition_Oldest:
		return &cursor{jl: jl, blockNumber: 0}, 0
	case *ab.SeekPosition_Newest:
		high := jl.height - 1
		return &cursor{jl: jl, blockNumber: high}, high
	case *ab.SeekPosition_Specified:
		if start.Specified.Number > jl.height {
			return &ordererledger.NotFoundErrorIterator{}, 0
		}
		return &cursor{jl: jl, blockNumber: start.Specified.Number}, start.Specified.Number
	}

	// This line should be unreachable, but the compiler requires it
	return &ordererledger.NotFoundErrorIterator{}, 0
}

// Next blocks until there is a new block available, or returns an error if the next block is no longer retrievable
func (cu *cursor) Next() (*cb.Block, cb.Status) {
	// This only loops once, as signal reading indicates the new block has been written
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

// ReadyChan returns a channel that will close when Next is ready to be called without blocking
func (cu *cursor) ReadyChan() <-chan struct{} {
	signal := cu.jl.signal
	if _, err := os.Stat(cu.jl.blockFilename(cu.blockNumber)); os.IsNotExist(err) {
		return signal
	}
	return closedChan
}
