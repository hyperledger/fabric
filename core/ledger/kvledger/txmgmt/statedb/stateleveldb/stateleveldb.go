/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"bytes"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

var logger = flogging.MustGetLogger("stateleveldb")

var (
	dataKeyPrefix    = []byte{'d'}
	nsKeySep         = []byte{0x00}
	lastKeyIndicator = byte(0x01)
	savePointKey     = []byte{'s'}
)

// VersionedDBProvider implements interface VersionedDBProvider
type VersionedDBProvider struct {
	dbProvider *leveldbhelper.Provider
}

// NewVersionedDBProvider instantiates VersionedDBProvider
func NewVersionedDBProvider(dbPath string) (*VersionedDBProvider, error) {
	logger.Debugf("constructing VersionedDBProvider dbPath=%s", dbPath)
	dbProvider, err := leveldbhelper.NewProvider(
		&leveldbhelper.Conf{
			DBPath:                dbPath,
			ExpectedFormatVersion: dataformat.Version20,
		})
	if err != nil {
		return nil, err
	}
	return &VersionedDBProvider{dbProvider}, nil
}

// GetDBHandle gets the handle to a named database
func (provider *VersionedDBProvider) GetDBHandle(dbName string) (statedb.VersionedDB, error) {
	return newVersionedDB(provider.dbProvider.GetDBHandle(dbName), dbName), nil
}

// Close closes the underlying db
func (provider *VersionedDBProvider) Close() {
	provider.dbProvider.Close()
}

// VersionedDB implements VersionedDB interface
type versionedDB struct {
	db     *leveldbhelper.DBHandle
	dbName string
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(db *leveldbhelper.DBHandle, dbName string) *versionedDB {
	return &versionedDB{db, dbName}
}

// Open implements method in VersionedDB interface
func (vdb *versionedDB) Open() error {
	// do nothing because shared db is used
	return nil
}

// Close implements method in VersionedDB interface
func (vdb *versionedDB) Close() {
	// do nothing because shared db is used
}

// ValidateKeyValue implements method in VersionedDB interface
func (vdb *versionedDB) ValidateKeyValue(key string, value []byte) error {
	return nil
}

// BytesKeySupported implements method in VersionedDB interface
func (vdb *versionedDB) BytesKeySupported() bool {
	return true
}

// GetState implements method in VersionedDB interface
func (vdb *versionedDB) GetState(namespace string, key string) (*statedb.VersionedValue, error) {
	logger.Debugf("GetState(). ns=%s, key=%s", namespace, key)
	dbVal, err := vdb.db.Get(encodeDataKey(namespace, key))
	if err != nil {
		return nil, err
	}
	if dbVal == nil {
		return nil, nil
	}
	return decodeValue(dbVal)
}

// GetVersion implements method in VersionedDB interface
func (vdb *versionedDB) GetVersion(namespace string, key string) (*version.Height, error) {
	versionedValue, err := vdb.GetState(namespace, key)
	if err != nil {
		return nil, err
	}
	if versionedValue == nil {
		return nil, nil
	}
	return versionedValue.Version, nil
}

// GetStateMultipleKeys implements method in VersionedDB interface
func (vdb *versionedDB) GetStateMultipleKeys(namespace string, keys []string) ([]*statedb.VersionedValue, error) {
	vals := make([]*statedb.VersionedValue, len(keys))
	for i, key := range keys {
		val, err := vdb.GetState(namespace, key)
		if err != nil {
			return nil, err
		}
		vals[i] = val
	}
	return vals, nil
}

// GetStateRangeScanIterator implements method in VersionedDB interface
// startKey is inclusive
// endKey is exclusive
func (vdb *versionedDB) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (statedb.ResultsIterator, error) {
	return vdb.GetStateRangeScanIteratorWithMetadata(namespace, startKey, endKey, nil)
}

const optionLimit = "limit"

// GetStateRangeScanIteratorWithMetadata implements method in VersionedDB interface
func (vdb *versionedDB) GetStateRangeScanIteratorWithMetadata(namespace string, startKey string, endKey string, metadata map[string]interface{}) (statedb.QueryResultsIterator, error) {

	requestedLimit := int32(0)
	// if metadata is provided, validate and apply options
	if metadata != nil {
		//validate the metadata
		err := statedb.ValidateRangeMetadata(metadata)
		if err != nil {
			return nil, err
		}
		if limitOption, ok := metadata[optionLimit]; ok {
			requestedLimit = limitOption.(int32)
		}
	}

	// Note:  metadata is not used for the goleveldb implementation of the range query
	dataStartKey := encodeDataKey(namespace, startKey)
	dataEndKey := encodeDataKey(namespace, endKey)
	if endKey == "" {
		dataEndKey[len(dataEndKey)-1] = lastKeyIndicator
	}
	dbItr := vdb.db.GetIterator(dataStartKey, dataEndKey)
	return newKVScanner(namespace, dbItr, requestedLimit), nil
}

// ExecuteQuery implements method in VersionedDB interface
func (vdb *versionedDB) ExecuteQuery(namespace, query string) (statedb.ResultsIterator, error) {
	return nil, errors.New("ExecuteQuery not supported for leveldb")
}

// ExecuteQueryWithMetadata implements method in VersionedDB interface
func (vdb *versionedDB) ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (statedb.QueryResultsIterator, error) {
	return nil, errors.New("ExecuteQueryWithMetadata not supported for leveldb")
}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *versionedDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {
	dbBatch := leveldbhelper.NewUpdateBatch()
	namespaces := batch.GetUpdatedNamespaces()
	for _, ns := range namespaces {
		updates := batch.GetUpdates(ns)
		for k, vv := range updates {
			dataKey := encodeDataKey(ns, k)
			logger.Debugf("Channel [%s]: Applying key(string)=[%s] key(bytes)=[%#v]", vdb.dbName, string(dataKey), dataKey)

			if vv.Value == nil {
				dbBatch.Delete(dataKey)
			} else {
				encodedVal, err := encodeValue(vv)
				if err != nil {
					return err
				}
				dbBatch.Put(dataKey, encodedVal)
			}
		}
	}
	// Record a savepoint at a given height
	// If a given height is nil, it denotes that we are committing pvt data of old blocks.
	// In this case, we should not store a savepoint for recovery. The lastUpdatedOldBlockList
	// in the pvtstore acts as a savepoint for pvt data.
	if height != nil {
		dbBatch.Put(savePointKey, height.ToBytes())
	}
	// Setting snyc to true as a precaution, false may be an ok optimization after further testing.
	if err := vdb.db.WriteBatch(dbBatch, true); err != nil {
		return err
	}
	return nil
}

// GetLatestSavePoint implements method in VersionedDB interface
func (vdb *versionedDB) GetLatestSavePoint() (*version.Height, error) {
	versionBytes, err := vdb.db.Get(savePointKey)
	if err != nil {
		return nil, err
	}
	if versionBytes == nil {
		return nil, nil
	}
	version, _, err := version.NewHeightFromBytes(versionBytes)
	if err != nil {
		return nil, err
	}
	return version, nil
}

func encodeDataKey(ns, key string) []byte {
	k := append(dataKeyPrefix, []byte(ns)...)
	k = append(k, nsKeySep...)
	return append(k, []byte(key)...)
}

func decodeDataKey(encodedDataKey []byte) (string, string) {
	split := bytes.SplitN(encodedDataKey, nsKeySep, 2)
	return string(split[0][1:]), string(split[1])
}

type kvScanner struct {
	namespace            string
	dbItr                iterator.Iterator
	requestedLimit       int32
	totalRecordsReturned int32
}

func newKVScanner(namespace string, dbItr iterator.Iterator, requestedLimit int32) *kvScanner {
	return &kvScanner{namespace, dbItr, requestedLimit, 0}
}

func (scanner *kvScanner) Next() (statedb.QueryResult, error) {
	if scanner.requestedLimit > 0 && scanner.totalRecordsReturned >= scanner.requestedLimit {
		return nil, nil
	}
	if !scanner.dbItr.Next() {
		return nil, nil
	}

	dbKey := scanner.dbItr.Key()
	dbVal := scanner.dbItr.Value()
	dbValCopy := make([]byte, len(dbVal))
	copy(dbValCopy, dbVal)
	_, key := decodeDataKey(dbKey)
	vv, err := decodeValue(dbValCopy)
	if err != nil {
		return nil, err
	}

	scanner.totalRecordsReturned++
	return &statedb.VersionedKV{
		CompositeKey: statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		// TODO remove dereferrencing below by changing the type of the field
		// `VersionedValue` in `statedb.VersionedKV` to a pointer
		VersionedValue: *vv}, nil
}

func (scanner *kvScanner) Close() {
	scanner.dbItr.Release()
}

func (scanner *kvScanner) GetBookmarkAndClose() string {
	retval := ""
	if scanner.dbItr.Next() {
		dbKey := scanner.dbItr.Key()
		_, key := decodeDataKey(dbKey)
		retval = key
	}
	scanner.Close()
	return retval
}
