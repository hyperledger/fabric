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

package stateleveldb

import (
	"bytes"

	"sync"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	logging "github.com/op/go-logging"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

var logger = logging.MustGetLogger("stateleveldb")

var compositeKeySep = []byte{0x00}
var lastKeyIndicator = byte(0x01)
var savePointKey = []byte{0x00}

// VersionedDBProvider implements interface VersionedDBProvider
type VersionedDBProvider struct {
	db         *db.DB
	databases  map[string]*VersionedDB
	mux        sync.Mutex
	openCounts uint64
}

// NewVersionedDBProvider instantiates VersionedDBProvider
func NewVersionedDBProvider() *VersionedDBProvider {
	dbPath := getDBPath()
	logger.Debugf("constructing VersionedDBProvider dbPath=%s", dbPath)
	db := db.CreateDB(&db.Conf{DBPath: dbPath})
	db.Open()
	logger.Debugf("Opened db dbPath=%s", dbPath)
	return &VersionedDBProvider{db, make(map[string]*VersionedDB), sync.Mutex{}, 0}
}

// GetDBHandle gets the handle to a named database
func (provider *VersionedDBProvider) GetDBHandle(dbName string) (statedb.VersionedDB, error) {
	provider.mux.Lock()
	defer provider.mux.Unlock()
	vdb := provider.databases[dbName]
	if vdb == nil {
		vdb = newVersionedDB(provider.db, dbName)
		provider.databases[dbName] = vdb
	}
	return vdb, nil
}

// Close closes the underlying db
func (provider *VersionedDBProvider) Close() {
	provider.db.Close()
}

// VersionedDB implements VersionedDB interface
type VersionedDB struct {
	db     *db.DB
	dbName string
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(db *db.DB, dbName string) *VersionedDB {
	return &VersionedDB{db, dbName}
}

// Open implements method in VersionedDB interface
func (vdb *VersionedDB) Open() error {
	// do nothing because shared db is used
	return nil
}

// Close implements method in VersionedDB interface
func (vdb *VersionedDB) Close() {
	// do nothing because shared db is used
}

// GetState implements method in VersionedDB interface
func (vdb *VersionedDB) GetState(namespace string, key string) (*statedb.VersionedValue, error) {
	logger.Debugf("GetState(). ns=%s, key=%s", namespace, key)
	compositeKey := constructCompositeKey(vdb.dbName, namespace, key)
	dbVal, err := vdb.db.Get(compositeKey)
	if err != nil {
		return nil, err
	}
	if dbVal == nil {
		return nil, nil
	}
	val, ver := decodeValue(dbVal)
	return &statedb.VersionedValue{Value: val, Version: ver}, nil
}

// GetStateMultipleKeys implements method in VersionedDB interface
func (vdb *VersionedDB) GetStateMultipleKeys(namespace string, keys []string) ([]*statedb.VersionedValue, error) {
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
func (vdb *VersionedDB) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (statedb.ResultsIterator, error) {
	compositeStartKey := constructCompositeKey(vdb.dbName, namespace, startKey)
	compositeEndKey := constructCompositeKey(vdb.dbName, namespace, endKey)
	if endKey == "" {
		compositeEndKey[len(compositeEndKey)-1] = lastKeyIndicator
	}
	dbItr := vdb.db.GetIterator(compositeStartKey, compositeEndKey)
	return newKVScanner(namespace, dbItr), nil
}

// ExecuteQuery implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQuery(query string) (statedb.ResultsIterator, error) {
	panic("Method not supported for leveldb")
}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *VersionedDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {
	levelBatch := &leveldb.Batch{}
	for ck, vv := range batch.KVs {
		compositeKey := constructCompositeKey(vdb.dbName, ck.Namespace, ck.Key)
		logger.Debugf("applying key=%#v, versionedValue=%#v", ck, vv)
		if vv.Value == nil {
			levelBatch.Delete(compositeKey)
		} else {
			levelBatch.Put(compositeKey, encodeValue(vv.Value, vv.Version))
		}
	}
	levelBatch.Put(constructSavepointKey(vdb.dbName), height.ToBytes())
	if err := vdb.db.WriteBatch(levelBatch, false); err != nil {
		return err
	}
	return nil
}

// GetLatestSavePoint implements method in VersionedDB interface
func (vdb *VersionedDB) GetLatestSavePoint() (*version.Height, error) {
	versionBytes, err := vdb.db.Get(constructSavepointKey(vdb.dbName))
	if err != nil {
		return nil, err
	}
	version, _ := version.NewHeightFromBytes(versionBytes)
	return version, nil
}

func encodeValue(value []byte, version *version.Height) []byte {
	encodedValue := version.ToBytes()
	if value != nil {
		encodedValue = append(encodedValue, value...)
	}
	return encodedValue
}

func decodeValue(encodedValue []byte) ([]byte, *version.Height) {
	version, n := version.NewHeightFromBytes(encodedValue)
	value := encodedValue[n:]
	return value, version
}

func constructCompositeKey(dbName string, ns string, key string) []byte {
	compositeKey := []byte(dbName)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, []byte(ns)...)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, []byte(key)...)
	return compositeKey
}

func splitCompositeKey(compositeKey []byte) (string, string, string) {
	split := bytes.SplitN(compositeKey, compositeKeySep, 3)
	return string(split[0]), string(split[1]), string(split[2])
}

func constructSavepointKey(dbName string) []byte {
	key := savePointKey
	return append(key, []byte(dbName)...)
}

type kvScanner struct {
	namespace string
	dbItr     iterator.Iterator
}

func newKVScanner(namespace string, dbItr iterator.Iterator) *kvScanner {
	return &kvScanner{namespace, dbItr}
}

func (scanner *kvScanner) Next() (statedb.QueryResult, error) {
	if !scanner.dbItr.Next() {
		return nil, nil
	}
	_, _, key := splitCompositeKey(scanner.dbItr.Key())
	value, version := decodeValue(scanner.dbItr.Value())
	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		VersionedValue: statedb.VersionedValue{Value: value, Version: version}}, nil
}

func (scanner *kvScanner) Close() {
	scanner.dbItr.Release()
}
