/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb/historyleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	// ErrLedgerIDExists is thrown by a CreateLedger call if a ledger with the given id already exists
	ErrLedgerIDExists = errors.New("LedgerID already exists")
	// ErrNonExistingLedgerID is thrown by a OpenLedger call if a ledger with the given id does not exist
	ErrNonExistingLedgerID = errors.New("LedgerID does not exist")
	// ErrLedgerNotOpened is thrown by a CloseLedger call if a ledger with the given id has not been opened
	ErrLedgerNotOpened = errors.New("ledger is not opened yet")

	underConstructionLedgerKey = []byte("underConstructionLedgerKey")
	ledgerKeyPrefix            = []byte("l")
)

// Provider implements interface ledger.PeerLedgerProvider
type Provider struct {
	idStore             *idStore
	ledgerStoreProvider *ledgerstorage.Provider
	vdbProvider         privacyenabledstate.DBProvider
	historydbProvider   historydb.HistoryDBProvider
	configHistoryMgr    confighistory.Mgr
	stateListeners      []ledger.StateListener
	bookkeepingProvider bookkeeping.Provider
	initializer         *ledger.Initializer
}

// NewProvider instantiates a new Provider.
// This is not thread-safe and assumed to be synchronized be the caller
func NewProvider() (ledger.PeerLedgerProvider, error) {
	logger.Info("Initializing ledger provider")
	// Initialize the ID store (inventory of chainIds/ledgerIds)
	idStore := openIDStore(ledgerconfig.GetLedgerProviderPath())
	ledgerStoreProvider := ledgerstorage.NewProvider()
	bookkeepingProvider := bookkeeping.NewProvider()
	// Initialize the versioned database (state database)
	vdbProvider, err := privacyenabledstate.NewCommonStorageDBProvider(bookkeepingProvider)
	if err != nil {
		return nil, err
	}
	// Initialize the history database (index for history of values by key)
	historydbProvider := historyleveldb.NewHistoryDBProvider()
	logger.Info("ledger provider Initialized")
	provider := &Provider{idStore, ledgerStoreProvider,
		vdbProvider, historydbProvider, nil, nil, bookkeepingProvider, nil}
	return provider, nil
}

// Initialize implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Initialize(initializer *ledger.Initializer) {
	provider.initializer = initializer
	provider.configHistoryMgr = confighistory.NewMgr()
	provider.stateListeners = initializer.StateListeners
	provider.recoverUnderConstructionLedger()
}

// Create implements the corresponding method from interface ledger.PeerLedgerProvider
// This functions sets a under construction flag before doing any thing related to ledger creation and
// upon a successful ledger creation with the committed genesis block, removes the flag and add entry into
// created ledgers list (atomically). If a crash happens in between, the 'recoverUnderConstructionLedger'
// function is invoked before declaring the provider to be usable
func (provider *Provider) Create(genesisBlock *common.Block) (ledger.PeerLedger, error) {
	ledgerID, err := utils.GetChainIDFromBlock(genesisBlock)
	if err != nil {
		return nil, err
	}
	exists, err := provider.idStore.ledgerIDExists(ledgerID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrLedgerIDExists
	}
	if err = provider.idStore.setUnderConstructionFlag(ledgerID); err != nil {
		return nil, err
	}
	lgr, err := provider.openInternal(ledgerID)
	if err != nil {
		logger.Errorf("Error opening a new empty ledger. Unsetting under construction flag. Error: %+v", err)
		panicOnErr(provider.runCleanup(ledgerID), "Error running cleanup for ledger id [%s]", ledgerID)
		panicOnErr(provider.idStore.unsetUnderConstructionFlag(), "Error while unsetting under construction flag")
		return nil, err
	}
	if err := lgr.CommitWithPvtData(&ledger.BlockAndPvtData{
		Block: genesisBlock,
	}); err != nil {
		lgr.Close()
		return nil, err
	}
	panicOnErr(provider.idStore.createLedgerID(ledgerID, genesisBlock), "Error while marking ledger as created")
	return lgr, nil
}

// Open implements the corresponding method from interface ledger.PeerLedgerProvider
func (provider *Provider) Open(ledgerID string) (ledger.PeerLedger, error) {
	logger.Debugf("Open() opening kvledger: %s", ledgerID)
	// Check the ID store to ensure that the chainId/ledgerId exists
	exists, err := provider.idStore.ledgerIDExists(ledgerID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNonExistingLedgerID
	}
	return provider.openInternal(ledgerID)
}

func (provider *Provider) openInternal(ledgerID string) (ledger.PeerLedger, error) {
	// Get the block store for a chain/ledger
	blockStore, err := provider.ledgerStoreProvider.Open(ledgerID)
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
	l, err := newKVLedger(ledgerID, blockStore, vDB, historyDB, provider.configHistoryMgr,
		provider.stateListeners, provider.bookkeepingProvider, provider.initializer.DeployedChaincodeInfoProvider)
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
	provider.ledgerStoreProvider.Close()
	provider.vdbProvider.Close()
	provider.historydbProvider.Close()
	provider.bookkeepingProvider.Close()
	provider.configHistoryMgr.Close()
}

// recoverUnderConstructionLedger checks whether the under construction flag is set - this would be the case
// if a crash had happened during creation of ledger and the ledger creation could have been left in intermediate
// state. Recovery checks if the ledger was created and the genesis block was committed successfully then it completes
// the last step of adding the ledger id to the list of created ledgers. Else, it clears the under construction flag
func (provider *Provider) recoverUnderConstructionLedger() {
	logger.Debugf("Recovering under construction ledger")
	ledgerID, err := provider.idStore.getUnderConstructionFlag()
	panicOnErr(err, "Error while checking whether the under construction flag is set")
	if ledgerID == "" {
		logger.Debugf("No under construction ledger found. Quitting recovery")
		return
	}
	logger.Infof("ledger [%s] found as under construction", ledgerID)
	ledger, err := provider.openInternal(ledgerID)
	panicOnErr(err, "Error while opening under construction ledger [%s]", ledgerID)
	bcInfo, err := ledger.GetBlockchainInfo()
	panicOnErr(err, "Error while getting blockchain info for the under construction ledger [%s]", ledgerID)
	ledger.Close()

	switch bcInfo.Height {
	case 0:
		logger.Infof("Genesis block was not committed. Hence, the peer ledger not created. unsetting the under construction flag")
		panicOnErr(provider.runCleanup(ledgerID), "Error while running cleanup for ledger id [%s]", ledgerID)
		panicOnErr(provider.idStore.unsetUnderConstructionFlag(), "Error while unsetting under construction flag")
	case 1:
		logger.Infof("Genesis block was committed. Hence, marking the peer ledger as created")
		genesisBlock, err := ledger.GetBlockByNumber(0)
		panicOnErr(err, "Error while retrieving genesis block from blockchain for ledger [%s]", ledgerID)
		panicOnErr(provider.idStore.createLedgerID(ledgerID, genesisBlock), "Error while adding ledgerID [%s] to created list", ledgerID)
	default:
		panic(errors.Errorf(
			"data inconsistency: under construction flag is set for ledger [%s] while the height of the blockchain is [%d]",
			ledgerID, bcInfo.Height))
	}
	return
}

// runCleanup cleans up blockstorage, statedb, and historydb for what
// may have got created during in-complete ledger creation
func (provider *Provider) runCleanup(ledgerID string) error {
	// TODO - though, not having this is harmless for kv ledger.
	// If we want, following could be done:
	// - blockstorage could remove empty folders
	// - couchdb backed statedb could delete the database if got created
	// - leveldb backed statedb and history db need not perform anything as it uses a single db shared across ledgers
	return nil
}

func panicOnErr(err error, mgsFormat string, args ...interface{}) {
	if err == nil {
		return
	}
	args = append(args, err)
	panic(fmt.Sprintf(mgsFormat+" Error: %s", args...))
}

//////////////////////////////////////////////////////////////////////
// Ledger id persistence related code
///////////////////////////////////////////////////////////////////////
type idStore struct {
	db *leveldbhelper.DB
}

func openIDStore(path string) *idStore {
	db := leveldbhelper.CreateDB(&leveldbhelper.Conf{DBPath: path})
	db.Open()
	return &idStore{db}
}

func (s *idStore) setUnderConstructionFlag(ledgerID string) error {
	return s.db.Put(underConstructionLedgerKey, []byte(ledgerID), true)
}

func (s *idStore) unsetUnderConstructionFlag() error {
	return s.db.Delete(underConstructionLedgerKey, true)
}

func (s *idStore) getUnderConstructionFlag() (string, error) {
	val, err := s.db.Get(underConstructionLedgerKey)
	if err != nil {
		return "", err
	}
	return string(val), nil
}

func (s *idStore) createLedgerID(ledgerID string, gb *common.Block) error {
	key := s.encodeLedgerKey(ledgerID)
	var val []byte
	var err error
	if val, err = s.db.Get(key); err != nil {
		return err
	}
	if val != nil {
		return ErrLedgerIDExists
	}
	if val, err = proto.Marshal(gb); err != nil {
		return err
	}
	batch := &leveldb.Batch{}
	batch.Put(key, val)
	batch.Delete(underConstructionLedgerKey)
	return s.db.WriteBatch(batch, true)
}

func (s *idStore) ledgerIDExists(ledgerID string) (bool, error) {
	key := s.encodeLedgerKey(ledgerID)
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
	defer itr.Release()
	itr.First()
	for itr.Valid() {
		if bytes.Equal(itr.Key(), underConstructionLedgerKey) {
			continue
		}
		id := string(s.decodeLedgerID(itr.Key()))
		ids = append(ids, id)
		itr.Next()
	}
	return ids, nil
}

func (s *idStore) close() {
	s.db.Close()
}

func (s *idStore) encodeLedgerKey(ledgerID string) []byte {
	return append(ledgerKeyPrefix, []byte(ledgerID)...)
}

func (s *idStore) decodeLedgerID(key []byte) string {
	return string(key[len(ledgerKeyPrefix):])
}
