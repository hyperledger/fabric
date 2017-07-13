/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/golang/protobuf/jsonpb"
	"github.com/hyperledger/fabric/orderer/ledger"
)

type jsonLedgerFactory struct {
	directory string
	ledgers   map[string]ledger.ReadWriter
	mutex     sync.Mutex
}

// GetOrCreate gets an existing ledger (if it exists) or creates it if it does not
func (jlf *jsonLedgerFactory) GetOrCreate(chainID string) (ledger.ReadWriter, error) {
	jlf.mutex.Lock()
	defer jlf.mutex.Unlock()

	key := chainID

	l, ok := jlf.ledgers[key]
	if ok {
		return l, nil
	}

	directory := filepath.Join(jlf.directory, fmt.Sprintf(chainDirectoryFormatString, chainID))

	logger.Debugf("Initializing chain %s at: %s", chainID, directory)

	if err := os.MkdirAll(directory, 0700); err != nil {
		logger.Warningf("Failed initializing chain %s: %s", chainID, err)
		return nil, err
	}

	ch := newChain(directory)
	jlf.ledgers[key] = ch
	return ch, nil
}

// newChain creates a new chain backed by a JSON ledger
func newChain(directory string) ledger.ReadWriter {
	jl := &jsonLedger{
		directory: directory,
		signal:    make(chan struct{}),
		marshaler: &jsonpb.Marshaler{Indent: "  "},
	}
	jl.initializeBlockHeight()
	logger.Debugf("Initialized to block height %d with hash %x", jl.height-1, jl.lastHash)
	return jl
}

// initializeBlockHeight verifies that all blocks exist between 0 and the block
// height, and populates the lastHash
func (jl *jsonLedger) initializeBlockHeight() {
	infos, err := ioutil.ReadDir(jl.directory)
	if err != nil {
		logger.Panic(err)
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
			logger.Panicf("Missing block %d in the chain", nextNumber)
		}
		nextNumber++
	}
	jl.height = nextNumber
	if jl.height == 0 {
		return
	}
	block, found := jl.readBlock(jl.height - 1)
	if !found {
		logger.Panicf("Block %d was in directory listing but error reading", jl.height-1)
	}
	if block == nil {
		logger.Panicf("Error reading block %d", jl.height-1)
	}
	jl.lastHash = block.Header.Hash()
}

// ChainIDs returns the chain IDs the factory is aware of
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

// Close is a no-op for the JSON ledger
func (jlf *jsonLedgerFactory) Close() {
	return // nothing to do
}

// New creates a new ledger factory
func New(directory string) ledger.Factory {
	logger.Debugf("Initializing ledger at: %s", directory)
	if err := os.MkdirAll(directory, 0700); err != nil {
		logger.Panicf("Could not create directory %s: %s", directory, err)
	}

	jlf := &jsonLedgerFactory{
		directory: directory,
		ledgers:   make(map[string]ledger.ReadWriter),
	}

	infos, err := ioutil.ReadDir(jlf.directory)
	if err != nil {
		logger.Panicf("Error reading from directory %s while initializing ledger: %s", jlf.directory, err)
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
		jlf.GetOrCreate(chainID)
	}

	return jlf
}
