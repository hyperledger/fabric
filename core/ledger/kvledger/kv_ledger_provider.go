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

package kvledger

import (
	"errors"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb/historyleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

var (
	// ErrLedgerIDExists is thrown by a CreateLedger call if a ledger with the given id already exists
	ErrLedgerIDExists = errors.New("LedgerID already exists")
	// ErrNonExistingLedgerID is thrown by a OpenLedger call if a ledger with the given id does not exist
	ErrNonExistingLedgerID = errors.New("LedgerID does not exist")
	// ErrLedgerNotOpened is thrown by a CloseLedger call if a ledger with the given id has not been opened
	ErrLedgerNotOpened = errors.New("Ledger is not opened yet")
)

// Provider implements interface ledger.PeerLedgerProvider
type Provider struct {
	idStore            *idStore
	blockStoreProvider blkstorage.BlockStoreProvider
	vdbProvider        statedb.VersionedDBProvider
	historydbProvider  historydb.HistoryDBProvider
}

// NewProvider instantiates a new Provider.
// This is not thread-safe and assumed to be synchronized be the caller
func NewProvider() (ledger.PeerLedgerProvider, error) {

	logger.Info("Initializing ledger provider")

	// Initialize the ID store (inventory of chainIds/ledgerIds)
	idStore := openIDStore(ledgerconfig.GetLedgerProviderPath())

	// Initialize the block storage
	attrsToIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
	}
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	blockStoreProvider := fsblkstorage.NewProvider(
		fsblkstorage.NewConf(ledgerconfig.GetBlockStorePath(), ledgerconfig.GetMaxBlockfileSize()),
		indexConfig)

	// Initialize the versioned database (state database)
	var vdbProvider statedb.VersionedDBProvider
	if !ledgerconfig.IsCouchDBEnabled() {
		logger.Debugf("Constructing leveldb VersionedDBProvider")
		vdbProvider = stateleveldb.NewVersionedDBProvider()
	} else {
		logger.Debugf("Constructing CouchDB VersionedDBProvider")
		var err error
		vdbProvider, err = statecouchdb.NewVersionedDBProvider()
		if err != nil {
			return nil, err
		}
	}

	// Initialize the history database (index for history of values by key)
	var historydbProvider historydb.HistoryDBProvider
	historydbProvider = historyleveldb.NewHistoryDBProvider()

	logger.Info("ledger provider Initialized")
	return &Provider{idStore, blockStoreProvider, vdbProvider, historydbProvider}, nil
}

// Create implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Create(ledgerID string) (ledger.PeerLedger, error) {
	exists, err := provider.idStore.ledgerIDExists(ledgerID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrLedgerIDExists
	}
	provider.idStore.createLedgerID(ledgerID)
	return provider.Open(ledgerID)
}

// Open implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Open(ledgerID string) (ledger.PeerLedger, error) {

	// Check the ID store to ensure that the chainId/ledgerId exists
	exists, err := provider.idStore.ledgerIDExists(ledgerID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNonExistingLedgerID
	}

	// Get the block store for a chain/ledger
	blockStore, err := provider.blockStoreProvider.OpenBlockStore(ledgerID)
	if err != nil {
		return nil, err
	}

	// Get the versioned database (state database) for a chain/ledger
	vDB, err := provider.vdbProvider.GetDBHandle(ledgerID)
	if err != nil {
		return nil, err
	}

	// Get the history database (index for history of values by key) for a chain/ledger
	historyDB, err := provider.historydbProvider.GetDBHandle(ledgerID)
	if err != nil {
		return nil, err
	}

	// Create a kvLedger for this chain/ledger, which encasulates the underlying data stores
	// (id store, blockstore, state database, history database)
	l, err := newKVLedger(ledgerID, blockStore, vDB, historyDB)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Exists implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Exists(ledgerID string) (bool, error) {
	return provider.idStore.ledgerIDExists(ledgerID)
}

// List implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) List() ([]string, error) {
	return provider.idStore.getAllLedgerIds()
}

// Close implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Close() {
	provider.idStore.close()
	provider.blockStoreProvider.Close()
	provider.vdbProvider.Close()
	provider.historydbProvider.Close()
}

type idStore struct {
	db *leveldbhelper.DB
}

func openIDStore(path string) *idStore {
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: path})
	db.Open()
	return &idStore{db}
}

func (s *idStore) createLedgerID(ledgerID string) error {
	key := []byte(ledgerID)
	val := []byte{}
	err := error(nil)
	if val, err = s.db.Get(key); err != nil {
		return err
	}
	if val != nil {
		return ErrLedgerIDExists
	}
	return s.db.Put(key, val, true)
}

func (s *idStore) ledgerIDExists(ledgerID string) (bool, error) {
	key := []byte(ledgerID)
	val := []byte{}
	err := error(nil)
	if val, err = s.db.Get(key); err != nil {
		return false, err
	}
	return val != nil, nil
}

func (s *idStore) getAllLedgerIds() ([]string, error) {
	var ids []string
	itr := s.db.GetIterator(nil, nil)
	itr.First()
	for itr.Valid() {
		key := string(itr.Key())
		ids = append(ids, key)
		itr.Next()
	}
	return ids, nil
}

func (s *idStore) close() {
	s.db.Close()
}
