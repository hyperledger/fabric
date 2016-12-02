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
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
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

type fileLedgerFactory struct {
	directory string
	ledgers   map[string]rawledger.ReadWriter
	mutex     sync.Mutex
}

func New(directory string, systemGenesis *cb.Block) (rawledger.Factory, rawledger.ReadWriter) {
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

	logger.Debugf("Initializing fileLedger at '%s'", directory)
	if err := os.MkdirAll(directory, 0700); err != nil {
		logger.Fatalf("Could not create directory %s: %s", directory, err)
	}

	flf := &fileLedgerFactory{
		directory: directory,
		ledgers:   make(map[string]rawledger.ReadWriter),
	}

	flt, err := flf.GetOrCreate(payload.Header.ChainHeader.ChainID)
	if err != nil {
		logger.Fatalf("Error getting orderer system chain dir: %s", err)
	}

	fl := flt.(*fileLedger)

	if fl.height > 0 {
		block, ok := fl.readBlock(0)
		if !ok {
			logger.Fatalf("Error reading genesis block for chain of height %d", fl.height)
		}
		if !reflect.DeepEqual(block, systemGenesis) {
			logger.Fatalf("Attempted to reconfigure an existing ordering system chain with new genesis block")
		}
	} else {
		fl.writeBlock(systemGenesis)
		fl.height = 1
		fl.lastHash = systemGenesis.Header.Hash()
	}

	return flf, fl
}

func (flf *fileLedgerFactory) ChainIDs() []string {
	flf.mutex.Lock()
	defer flf.mutex.Unlock()
	ids := make([]string, len(flf.ledgers))

	i := 0
	for key := range flf.ledgers {
		ids[i] = key
		i++
	}

	return ids
}

func (flf *fileLedgerFactory) GetOrCreate(chainID string) (rawledger.ReadWriter, error) {
	flf.mutex.Lock()
	defer flf.mutex.Unlock()

	key := chainID

	// Check a second time with the lock held
	l, ok := flf.ledgers[key]
	if ok {
		return l, nil
	}

	directory := fmt.Sprintf("%s/%s", flf.directory, chainID)

	logger.Debugf("Initializing chain at '%s'", directory)

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}

	ch := newChain(directory)
	flf.ledgers[key] = ch
	return ch, nil
}

// newChain creates a new chain backed by a file ledger
func newChain(directory string) rawledger.ReadWriter {
	fl := &fileLedger{
		directory:      directory,
		fqFormatString: directory + "/" + blockFileFormatString,
		signal:         make(chan struct{}),
		marshaler:      &jsonpb.Marshaler{Indent: "  "},
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
	if fl.height == 0 {
		return
	}
	block, found := fl.readBlock(fl.height - 1)
	if !found {
		panic(fmt.Errorf("Block %d was in directory listing but error reading", fl.height-1))
	}
	if block == nil {
		panic(fmt.Errorf("Error reading block %d", fl.height-1))
	}
	fl.lastHash = block.Header.Hash()
}

// blockFilename returns the fully qualified path to where a block of a given number should be stored on disk
func (fl *fileLedger) blockFilename(number uint64) string {
	return fmt.Sprintf(fl.fqFormatString, number)
}

// writeBlock commits a block to disk
func (fl *fileLedger) writeBlock(block *cb.Block) {
	file, err := os.Create(fl.blockFilename(block.Header.Number))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	err = fl.marshaler.Marshal(file, block)
	logger.Debugf("Wrote block %d", block.Header.Number)
	if err != nil {
		panic(err)
	}

}

// readBlock returns the block or nil, and whether the block was found or not, (nil,true) generally indicates an irrecoverable problem
func (fl *fileLedger) readBlock(number uint64) (*cb.Block, bool) {
	file, err := os.Open(fl.blockFilename(number))
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
func (fl *fileLedger) Height() uint64 {
	return fl.height
}

// Append creates a new block and appends it to the ledger
func (fl *fileLedger) Append(messages []*cb.Envelope, metadata [][]byte) *cb.Block {
	data := &cb.BlockData{
		Data: make([][]byte, len(messages)),
	}

	var err error
	for i, msg := range messages {
		data.Data[i], err = proto.Marshal(msg)
		if err != nil {
			logger.Fatalf("Error marshaling what should be a valid proto message: %s", err)
		}
	}

	block := &cb.Block{
		Header: &cb.BlockHeader{
			Number:       fl.height,
			PreviousHash: fl.lastHash,
			DataHash:     data.Hash(),
		},
		Data: data,
		Metadata: &cb.BlockMetadata{
			Metadata: metadata,
		},
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
func (cu *cursor) Next() (*cb.Block, cb.Status) {
	// This only loops once, as signal reading indicates the new block has been written
	for {
		block, found := cu.fl.readBlock(cu.blockNumber)
		if found {
			if block == nil {
				return nil, cb.Status_SERVICE_UNAVAILABLE
			}
			cu.blockNumber++
			return block, cb.Status_SUCCESS
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
