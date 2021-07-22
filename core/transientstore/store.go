/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

var logger = flogging.MustGetLogger("transientstore")

var (
	emptyValue = []byte{}
	nilByte    = byte('\x00')
	// ErrStoreEmpty is used to indicate that there are no entries in transient store
	ErrStoreEmpty = errors.New("Transient store is empty")
	// transient system namespace is the name of a db used for storage bookkeeping metadata.
	systemNamespace  = ""
	underDeletionKey = []byte("UNDER_DELETION")
)

//////////////////////////////////////////////
// Interfaces and data types
/////////////////////////////////////////////

// StoreProvider provides an instance of a TransientStore
type StoreProvider interface {
	OpenStore(ledgerID string) (*Store, error)
	Close()
	DeleteStore(ledgerID string) error
}

// RWSetScanner provides an iterator for EndorserPvtSimulationResults
type RWSetScanner interface {
	// Next returns the next EndorserPvtSimulationResults from the RWSetScanner.
	// It may return nil, nil when it has no further data, and also may return an error
	// on failure
	Next() (*EndorserPvtSimulationResults, error)
	// Close frees the resources associated with this RWSetScanner
	Close()
}

// EndorserPvtSimulationResults captures the details of the simulation results specific to an endorser
type EndorserPvtSimulationResults struct {
	ReceivedAtBlockHeight          uint64
	PvtSimulationResultsWithConfig *transientstore.TxPvtReadWriteSetWithConfigInfo
}

//////////////////////////////////////////////
// Implementation
/////////////////////////////////////////////

// storeProvider encapsulates a leveldb provider which is used to store
// private write sets of simulated transactions, and implements TransientStoreProvider
// interface.
type storeProvider struct {
	dbProvider *leveldbhelper.Provider
}

// store holds an instance of a levelDB.
type Store struct {
	db       *leveldbhelper.DBHandle
	ledgerID string
}

// RwsetScanner helps iterating over results
type RwsetScanner struct {
	txid   string
	dbItr  iterator.Iterator
	filter ledger.PvtNsCollFilter
}

// NewStoreProvider instantiates TransientStoreProvider
func NewStoreProvider(path string) (StoreProvider, error) {
	dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: path})
	if err != nil {
		return nil, err
	}

	provider := &storeProvider{dbProvider: dbProvider}

	// purge any databases marked for deletion.  This may occur at the next peer init after a
	// transient storage deletion failed due to a crash or system error.
	if err = provider.processPendingStorageDeletions(); err != nil {
		return nil, errors.WithMessagef(err, "processing pending storage deletions in folder [%s]", path)
	}

	return provider, nil
}

// OpenStore returns a handle to a ledgerId in Store
func (provider *storeProvider) OpenStore(ledgerID string) (*Store, error) {
	dbHandle := provider.dbProvider.GetDBHandle(ledgerID)
	return &Store{db: dbHandle, ledgerID: ledgerID}, nil
}

// Close closes the TransientStoreProvider
func (provider *storeProvider) Close() {
	provider.dbProvider.Close()
}

// DeleteStore deletes the TransientStoreProvider for a given ledger.
func (provider *storeProvider) DeleteStore(ledgerID string) error {
	return provider.dbProvider.Drop(ledgerID)
}

func (provider *storeProvider) markStorageForDelete(ledgerID string) error {
	marked, err := provider.getStorageMarkedForDeletion()
	if err != nil {
		return errors.WithMessage(err, "while listing delete marked storage")
	}

	// don't update if the storage is already marked for deletion.
	for i := 0; i < len(marked); i++ {
		if strings.EqualFold(ledgerID, marked[i]) {
			logger.Infof("transient storage %s was already marked for delete", ledgerID)
			return nil
		}
	}

	marked = append(marked, ledgerID)

	err = provider.markStorageListForDelete(marked)
	if err != nil {
		return errors.WithMessagef(err, "while updating storage list %v for deletion", marked)
	}

	return nil
}

// Write the UNDER_DELETE ledger set as a JSON array in the transient system catalog.
func (provider *storeProvider) markStorageListForDelete(storageList []string) error {
	json, err := json.Marshal(storageList)
	if err != nil {
		return errors.WithMessagef(err, "marshalling delete list")
	}

	db := provider.dbProvider.GetDBHandle(systemNamespace)
	defer db.Close()

	if err = db.Put(underDeletionKey, json, true); err != nil {
		return errors.WithMessagef(err, "writing delete list to system storage")
	}

	return nil
}

// Find the set of ledgers tagged as UNDER_DELETE.
func (provider *storeProvider) getStorageMarkedForDeletion() ([]string, error) {
	var deleteList []string

	db := provider.dbProvider.GetDBHandle(systemNamespace)
	defer db.Close()

	val, err := db.Get(underDeletionKey)
	if err != nil {
		return nil, errors.WithMessagef(err, "retrieving storage marked for deletion")
	}

	// no storage previously marked as delete
	if val == nil {
		return deleteList, nil
	}

	if err = json.Unmarshal(val, &deleteList); err != nil {
		return nil, errors.WithMessagef(err, "unmarshalling json delete list: %s", string(val))
	}

	return deleteList, nil
}

// Remove a ledger ID from the list of transient storages currently marked for delete.
func (provider *storeProvider) clearStorageDeletionStatus(ledgerID string) error {
	dl, err := provider.getStorageMarkedForDeletion()
	if err != nil {
		return errors.WithMessagef(err, "clearing storage flag for ledger [%s]", ledgerID)
	}

	var updatedList []string

	// retain all entries other than the one to be cleared.
	for i := 0; i < len(dl); i++ {
		if !strings.EqualFold(ledgerID, dl[i]) {
			updatedList = append(updatedList, dl[i])
		}
	}

	// Nothing to do: ledgerID was not in the current delete list
	if len(dl) == len(updatedList) {
		return nil
	}

	if err = provider.markStorageListForDelete(updatedList); err != nil {
		return errors.WithMessagef(err, "subtracting [%s] from delete tag list", ledgerID)
	}

	return nil
}

// Delete any transient storages that are marked as UNDER_DELETION.  This routine may be called
// at provider construction to purge any partially deleted transient stores.
func (provider *storeProvider) processPendingStorageDeletions() error {
	dl, err := provider.getStorageMarkedForDeletion()
	if err != nil {
		return errors.WithMessage(err, "processing pending deletion list")
	}

	for i := 0; i < len(dl); i++ {
		err = provider.DeleteStore(dl[i])
		if err != nil {
			return errors.WithMessagef(err, "processing delete for storage [%s]", dl[i])
		}

		err = provider.clearStorageDeletionStatus(dl[i])
		if err != nil {
			return errors.WithMessagef(err, "clearing deletion status for storage [%s]", dl[i])
		}
	}

	return nil
}

// Drop removes a transient storage associated with an input channel/ledger.
// This function must be invoked while the peer is shut down.  To recover from partial deletion
// due to a crash, the storage will be marked with an UNDER_DELETION status in the a system
// namespace db.  At the next peer startup, transient storage marked with UNDER_DELETION will
// be scrubbed from the system.
func Drop(providerPath, ledgerID string) error {
	logger.Infof("Dropping ledger [%s] from transient storage path [%s]", ledgerID, providerPath)

	// Ensure the routine is invoked while the peer is down.
	lockPath := filepath.Join(providerPath, "fileLock")
	fileLock := leveldbhelper.NewFileLock(lockPath)
	if err := fileLock.Lock(); err != nil {
		return errors.WithMessage(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	// Set up a StoreProvider and type assert as a storeProvider
	sp, err := NewStoreProvider(providerPath)
	if err != nil {
		return errors.WithMessagef(err, "constructing provider from path [%s]", providerPath)
	}
	provider := sp.(*storeProvider)
	defer provider.Close()

	// Mark the storage as UNDER_DELETION so that it can be purged if an error occurred during the drop
	err = provider.markStorageForDelete(ledgerID)
	if err != nil {
		return errors.WithMessagef(err, "marking storage [%s] for deletion", ledgerID)
	}

	// actually delete the storage.
	if err = provider.DeleteStore(ledgerID); err != nil {
		return errors.WithMessagef(err, "dropping ledger [%s] from transient storage", ledgerID)
	}

	// reset the deletion flag
	if err = provider.clearStorageDeletionStatus(ledgerID); err != nil {
		return errors.WithMessagef(err, "clearing deletion state for transient storage [%s]", ledgerID)
	}

	logger.Infof("Successfully dropped ledger [%s] from transient storage", ledgerID)

	return nil
}

// Persist stores the private write set of a transaction along with the collection config
// in the transient store based on txid and the block height the private data was received at
func (s *Store) Persist(txid string, blockHeight uint64,
	privateSimulationResultsWithConfig *transientstore.TxPvtReadWriteSetWithConfigInfo) error {
	logger.Debugf("Persisting private data to transient store for txid [%s] at block height [%d]", txid, blockHeight)

	dbBatch := s.db.NewUpdateBatch()

	// Create compositeKey with appropriate prefix, txid, uuid and blockHeight
	// Due to the fact that the txid may have multiple private write sets persisted from different
	// endorsers (via Gossip), we postfix an uuid with the txid to avoid collision.
	uuid := util.GenerateUUID()
	compositeKeyPvtRWSet := createCompositeKeyForPvtRWSet(txid, uuid, blockHeight)
	privateSimulationResultsWithConfigBytes, err := proto.Marshal(privateSimulationResultsWithConfig)
	if err != nil {
		return err
	}

	// Note that some rwset.TxPvtReadWriteSet may exist in the transient store immediately after
	// upgrading the peer to v1.2. In order to differentiate between new proto and old proto while
	// retrieving, a nil byte is prepended to the new proto, i.e., privateSimulationResultsWithConfigBytes,
	// as a marshaled message can never start with a nil byte. In v1.3, we can avoid prepending the
	// nil byte.
	value := append([]byte{nilByte}, privateSimulationResultsWithConfigBytes...)
	dbBatch.Put(compositeKeyPvtRWSet, value)

	// Create two index: (i) by txid, and (ii) by height

	// Create compositeKey for purge index by height with appropriate prefix, blockHeight,
	// txid, uuid and store the compositeKey (purge index) with a nil byte as value. Note that
	// the purge index is used to remove orphan entries in the transient store (which are not removed
	// by PurgeTxids()) using BTL policy by PurgeBelowHeight(). Note that orphan entries are due to transaction
	// that gets endorsed but not submitted by the client for commit)
	compositeKeyPurgeIndexByHeight := createCompositeKeyForPurgeIndexByHeight(blockHeight, txid, uuid)
	dbBatch.Put(compositeKeyPurgeIndexByHeight, emptyValue)

	// Create compositeKey for purge index by txid with appropriate prefix, txid, uuid,
	// blockHeight and store the compositeKey (purge index) with a nil byte as value.
	// Though compositeKeyPvtRWSet itself can be used to purge private write set by txid,
	// we create a separate composite key with a nil byte as value. The reason is that
	// if we use compositeKeyPvtRWSet, we unnecessarily read (potentially large) private write
	// set associated with the key from db. Note that this purge index is used to remove non-orphan
	// entries in the transient store and is used by PurgeTxids()
	// Note: We can create compositeKeyPurgeIndexByTxid by just replacing the prefix of compositeKeyPvtRWSet
	// with purgeIndexByTxidPrefix. For code readability and to be expressive, we use a
	// createCompositeKeyForPurgeIndexByTxid() instead.
	compositeKeyPurgeIndexByTxid := createCompositeKeyForPurgeIndexByTxid(txid, uuid, blockHeight)
	dbBatch.Put(compositeKeyPurgeIndexByTxid, emptyValue)

	return s.db.WriteBatch(dbBatch, true)
}

// GetTxPvtRWSetByTxid returns an iterator due to the fact that the txid may have multiple private
// write sets persisted from different endorsers.
func (s *Store) GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (RWSetScanner, error) {
	logger.Debugf("Getting private data from transient store for transaction %s", txid)

	// Construct startKey and endKey to do an range query
	startKey := createTxidRangeStartKey(txid)
	endKey := createTxidRangeEndKey(txid)

	iter, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &RwsetScanner{txid, iter, filter}, nil
}

// PurgeByTxids removes private write sets of a given set of transactions from the
// transient store. PurgeByTxids() is expected to be called by coordinator after
// committing a block to ledger.
func (s *Store) PurgeByTxids(txids []string) error {
	logger.Debug("Purging private data from transient store for committed txids")

	dbBatch := s.db.NewUpdateBatch()

	for _, txid := range txids {
		// Construct startKey and endKey to do an range query
		startKey := createPurgeIndexByTxidRangeStartKey(txid)
		endKey := createPurgeIndexByTxidRangeEndKey(txid)

		iter, err := s.db.GetIterator(startKey, endKey)
		if err != nil {
			return err
		}

		// Get all txid and uuid from above result and remove it from transient store (both
		// write set and the corresponding indexes.
		for iter.Next() {
			// For each entry, remove the private read-write set and corresponding indexes

			// Remove private write set
			compositeKeyPurgeIndexByTxid := iter.Key()
			// Note: We can create compositeKeyPvtRWSet by just replacing the prefix of compositeKeyPurgeIndexByTxid
			// with  prwsetPrefix. For code readability and to be expressive, we split and create again.
			uuid, blockHeight, err := splitCompositeKeyOfPurgeIndexByTxid(compositeKeyPurgeIndexByTxid)
			if err != nil {
				return err
			}
			compositeKeyPvtRWSet := createCompositeKeyForPvtRWSet(txid, uuid, blockHeight)
			dbBatch.Delete(compositeKeyPvtRWSet)

			// Remove purge index -- purgeIndexByHeight
			compositeKeyPurgeIndexByHeight := createCompositeKeyForPurgeIndexByHeight(blockHeight, txid, uuid)
			dbBatch.Delete(compositeKeyPurgeIndexByHeight)

			// Remove purge index -- purgeIndexByTxid
			dbBatch.Delete(compositeKeyPurgeIndexByTxid)
		}
		iter.Release()
	}
	// If peer fails before/while writing the batch to golevelDB, these entries will be
	// removed as per BTL policy later by PurgeBelowHeight()
	return s.db.WriteBatch(dbBatch, true)
}

// PurgeBelowHeight removes private write sets at block height lesser than
// a given maxBlockNumToRetain. In other words, Purge only retains private write sets
// that were persisted at block height of maxBlockNumToRetain or higher. Though the private
// write sets stored in transient store is removed by coordinator using PurgebyTxids()
// after successful block commit, PurgeBelowHeight() is still required to remove orphan entries (as
// transaction that gets endorsed may not be submitted by the client for commit)
func (s *Store) PurgeBelowHeight(maxBlockNumToRetain uint64) error {
	logger.Debugf("Purging orphaned private data from transient store received prior to block [%d]", maxBlockNumToRetain)

	// Do a range query with 0 as startKey and maxBlockNumToRetain-1 as endKey
	startKey := createPurgeIndexByHeightRangeStartKey(0)
	endKey := createPurgeIndexByHeightRangeEndKey(maxBlockNumToRetain - 1)
	iter, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return err
	}

	dbBatch := s.db.NewUpdateBatch()

	// Get all txid and uuid from above result and remove it from transient store (both
	// write set and the corresponding index.
	for iter.Next() {
		// For each entry, remove the private read-write set and corresponding indexes

		// Remove private write set
		compositeKeyPurgeIndexByHeight := iter.Key()
		txid, uuid, blockHeight, err := splitCompositeKeyOfPurgeIndexByHeight(compositeKeyPurgeIndexByHeight)
		if err != nil {
			return err
		}
		logger.Debugf("Purging from transient store private data simulated at block [%d]: txid [%s] uuid [%s]", blockHeight, txid, uuid)

		compositeKeyPvtRWSet := createCompositeKeyForPvtRWSet(txid, uuid, blockHeight)
		dbBatch.Delete(compositeKeyPvtRWSet)

		// Remove purge index -- purgeIndexByTxid
		compositeKeyPurgeIndexByTxid := createCompositeKeyForPurgeIndexByTxid(txid, uuid, blockHeight)
		dbBatch.Delete(compositeKeyPurgeIndexByTxid)

		// Remove purge index -- purgeIndexByHeight
		dbBatch.Delete(compositeKeyPurgeIndexByHeight)
	}
	iter.Release()

	return s.db.WriteBatch(dbBatch, true)
}

// GetMinTransientBlkHt returns the lowest block height remaining in transient store
func (s *Store) GetMinTransientBlkHt() (uint64, error) {
	// Current approach performs a range query on purgeIndex with startKey
	// as 0 (i.e., blockHeight) and returns the first key which denotes
	// the lowest block height remaining in transient store. An alternative approach
	// is to explicitly store the minBlockHeight in the transientStore.
	startKey := createPurgeIndexByHeightRangeStartKey(0)
	iter, err := s.db.GetIterator(startKey, nil)
	if err != nil {
		return 0, err
	}
	defer iter.Release()
	// Fetch the minimum transient block height
	if iter.Next() {
		dbKey := iter.Key()
		_, _, blockHeight, err := splitCompositeKeyOfPurgeIndexByHeight(dbKey)
		return blockHeight, err
	}
	// Returning an error may not be the right thing to do here. May be
	// return a bool. -1 is not possible due to unsigned int as first
	// return value
	return 0, ErrStoreEmpty
}

func (s *Store) Shutdown() {
	// do nothing because shared db is used
}

// Next moves the iterator to the next key/value pair.
// It returns <nil, nil> when the iterator is exhausted.
func (scanner *RwsetScanner) Next() (*EndorserPvtSimulationResults, error) {
	if !scanner.dbItr.Next() {
		return nil, nil
	}
	dbKey := scanner.dbItr.Key()
	dbVal := scanner.dbItr.Value()
	_, blockHeight, err := splitCompositeKeyOfPvtRWSet(dbKey)
	if err != nil {
		return nil, err
	}

	txPvtRWSet := &rwset.TxPvtReadWriteSet{}
	txPvtRWSetWithConfig := &transientstore.TxPvtReadWriteSetWithConfigInfo{}

	var filteredTxPvtRWSet *rwset.TxPvtReadWriteSet
	if dbVal[0] == nilByte {
		// new proto, i.e., TxPvtReadWriteSetWithConfigInfo
		if err := proto.Unmarshal(dbVal[1:], txPvtRWSetWithConfig); err != nil {
			return nil, err
		}

		// trim the tx rwset based on the current collection filter,
		// nil will be returned to filteredTxPvtRWSet if the transient store txid entry does not contain the data for the collection
		filteredTxPvtRWSet = trimPvtWSet(txPvtRWSetWithConfig.GetPvtRwset(), scanner.filter)
		configs, err := trimPvtCollectionConfigs(txPvtRWSetWithConfig.CollectionConfigs, scanner.filter)
		if err != nil {
			return nil, err
		}
		txPvtRWSetWithConfig.CollectionConfigs = configs
	} else {
		// old proto, i.e., TxPvtReadWriteSet
		if err := proto.Unmarshal(dbVal, txPvtRWSet); err != nil {
			return nil, err
		}
		filteredTxPvtRWSet = trimPvtWSet(txPvtRWSet, scanner.filter)
	}

	txPvtRWSetWithConfig.PvtRwset = filteredTxPvtRWSet

	return &EndorserPvtSimulationResults{
		ReceivedAtBlockHeight:          blockHeight,
		PvtSimulationResultsWithConfig: txPvtRWSetWithConfig,
	}, nil
}

// Close releases resource held by the iterator
func (scanner *RwsetScanner) Close() {
	scanner.dbItr.Release()
}
