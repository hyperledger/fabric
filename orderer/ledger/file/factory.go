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

package fileledger

import (
	"sync"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
)

type fileLedgerFactory struct {
	blkstorageProvider blkstorage.BlockStoreProvider
	ledgers            map[string]ordererledger.ReadWriter
	mutex              sync.Mutex
}

// GetOrCreate gets an existing ledger (if it exists) or creates it if it does not
func (lf *fileLedgerFactory) GetOrCreate(chainID string) (ordererledger.ReadWriter, error) {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()

	key := chainID
	// check cache
	ledger, ok := lf.ledgers[key]
	if ok {
		return ledger, nil
	}
	// open fresh
	blockStore, err := lf.blkstorageProvider.OpenBlockStore(key)
	if err != nil {
		return nil, err
	}
	ledger = &fileLedger{blockStore: blockStore, signal: make(chan struct{})}
	lf.ledgers[key] = ledger
	return ledger, nil
}

// ChainIDs returns the chain IDs the Factory is aware of
func (lf *fileLedgerFactory) ChainIDs() []string {
	chainIDs, err := lf.blkstorageProvider.List()
	if err != nil {
		panic(err)
	}
	return chainIDs
}

// Close closes the file ledgers served by this factory
func (lf *fileLedgerFactory) Close() {
	lf.blkstorageProvider.Close()
}

// New creates a new ledger factory
func New(directory string) ordererledger.Factory {
	return &fileLedgerFactory{
		blkstorageProvider: fsblkstorage.NewProvider(
			fsblkstorage.NewConf(directory, -1),
			&blkstorage.IndexConfig{
				AttrsToIndex: []blkstorage.IndexableAttr{blkstorage.IndexableAttrBlockNum}},
		),
		ledgers: make(map[string]ordererledger.ReadWriter),
	}
}
