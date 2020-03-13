/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package leveldbhelper

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// internalDBName is used to keep track of data related to internals such as data format
// _ is used as name because this is not allowed as a channelname
const internalDBName = "_"

var (
	dbNameKeySep     = []byte{0x00}
	lastKeyIndicator = byte(0x01)
	formatVersionKey = []byte{'f'} // a single key in db whose value indicates the version of the data format
)

// Conf configuration for `Provider`
//
// `ExpectedFormatVersion` is the expected value of the format key in the internal database.
// At the time of opening the db, A check is performed that
// either the db is empty (i.e., opening for the first time) or the value
// of the formatVersionKey is equal to `ExpectedFormatVersion`. Otherwise, an error is returned.
// A nil value for ExpectedFormatVersion indicates that the format is never set and hence there is no such record
type Conf struct {
	DBPath                string
	ExpectedFormatVersion string
}

// Provider enables to use a single leveldb as multiple logical leveldbs
type Provider struct {
	db *DB

	mux       sync.Mutex
	dbHandles map[string]*DBHandle
}

// NewProvider constructs a Provider
func NewProvider(conf *Conf) (*Provider, error) {
	db, err := openDBAndCheckFormat(conf)
	if err != nil {
		return nil, err
	}
	return &Provider{
		db:        db,
		dbHandles: make(map[string]*DBHandle),
	}, nil
}

func openDBAndCheckFormat(conf *Conf) (d *DB, e error) {
	db := CreateDB(conf)
	db.Open()

	defer func() {
		if e != nil {
			db.Close()
		}
	}()

	internalDB := &DBHandle{
		db:     db,
		dbName: internalDBName,
	}

	dbEmpty, err := db.IsEmpty()
	if err != nil {
		return nil, err
	}

	if dbEmpty && conf.ExpectedFormatVersion != "" {
		logger.Infof("DB is empty Setting db format as %s", conf.ExpectedFormatVersion)
		if err := internalDB.Put(formatVersionKey, []byte(conf.ExpectedFormatVersion), true); err != nil {
			return nil, err
		}
		return db, nil
	}

	formatVersion, err := internalDB.Get(formatVersionKey)
	if err != nil {
		return nil, err
	}
	logger.Debugf("Checking for db format at path [%s]", conf.DBPath)

	if !bytes.Equal(formatVersion, []byte(conf.ExpectedFormatVersion)) {
		logger.Errorf("The db at path [%s] contains data in unexpected format. expected data format = [%s] (%#v), data format = [%s] (%#v).",
			conf.DBPath, conf.ExpectedFormatVersion, []byte(conf.ExpectedFormatVersion), formatVersion, formatVersion)
		return nil, &dataformat.ErrVersionMismatch{
			ExpectedVersion: conf.ExpectedFormatVersion,
			Version:         string(formatVersion),
			DBInfo:          fmt.Sprintf("leveldb at [%s]", conf.DBPath),
		}
	}
	logger.Debug("format is latest, nothing to do")
	return db, nil
}

// GetDataFormat returns the format of the data
func (p *Provider) GetDataFormat() (string, error) {
	f, err := p.GetDBHandle(internalDBName).Get(formatVersionKey)
	return string(f), err
}

// GetDBHandle returns a handle to a named db
func (p *Provider) GetDBHandle(dbName string) *DBHandle {
	p.mux.Lock()
	defer p.mux.Unlock()
	dbHandle := p.dbHandles[dbName]
	if dbHandle == nil {
		dbHandle = &DBHandle{dbName, p.db}
		p.dbHandles[dbName] = dbHandle
	}
	return dbHandle
}

// Close closes the underlying leveldb
func (p *Provider) Close() {
	p.db.Close()
}

// DBHandle is an handle to a named db
type DBHandle struct {
	dbName string
	db     *DB
}

// Get returns the value for the given key
func (h *DBHandle) Get(key []byte) ([]byte, error) {
	return h.db.Get(constructLevelKey(h.dbName, key))
}

// Put saves the key/value
func (h *DBHandle) Put(key []byte, value []byte, sync bool) error {
	return h.db.Put(constructLevelKey(h.dbName, key), value, sync)
}

// Delete deletes the given key
func (h *DBHandle) Delete(key []byte, sync bool) error {
	return h.db.Delete(constructLevelKey(h.dbName, key), sync)
}

// WriteBatch writes a batch in an atomic way
func (h *DBHandle) WriteBatch(batch *UpdateBatch, sync bool) error {
	if len(batch.KVs) == 0 {
		return nil
	}
	levelBatch := &leveldb.Batch{}
	for k, v := range batch.KVs {
		key := constructLevelKey(h.dbName, []byte(k))
		if v == nil {
			levelBatch.Delete(key)
		} else {
			levelBatch.Put(key, v)
		}
	}
	if err := h.db.WriteBatch(levelBatch, sync); err != nil {
		return err
	}
	return nil
}

// GetIterator gets an handle to iterator. The iterator should be released after the use.
// The resultset contains all the keys that are present in the db between the startKey (inclusive) and the endKey (exclusive).
// A nil startKey represents the first available key and a nil endKey represent a logical key after the last available key
func (h *DBHandle) GetIterator(startKey []byte, endKey []byte) *Iterator {
	sKey := constructLevelKey(h.dbName, startKey)
	eKey := constructLevelKey(h.dbName, endKey)
	if endKey == nil {
		// replace the last byte 'dbNameKeySep' by 'lastKeyIndicator'
		eKey[len(eKey)-1] = lastKeyIndicator
	}
	logger.Debugf("Getting iterator for range [%#v] - [%#v]", sKey, eKey)
	return &Iterator{h.db.GetIterator(sKey, eKey)}
}

// UpdateBatch encloses the details of multiple `updates`
type UpdateBatch struct {
	KVs map[string][]byte
}

// NewUpdateBatch constructs an instance of a Batch
func NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{make(map[string][]byte)}
}

// Put adds a KV
func (batch *UpdateBatch) Put(key []byte, value []byte) {
	if value == nil {
		panic("Nil value not allowed")
	}
	batch.KVs[string(key)] = value
}

// Delete deletes a Key and associated value
func (batch *UpdateBatch) Delete(key []byte) {
	batch.KVs[string(key)] = nil
}

// Len returns the number of entries in the batch
func (batch *UpdateBatch) Len() int {
	return len(batch.KVs)
}

// Iterator extends actual leveldb iterator
type Iterator struct {
	iterator.Iterator
}

// Key wraps actual leveldb iterator method
func (itr *Iterator) Key() []byte {
	return retrieveAppKey(itr.Iterator.Key())
}

func constructLevelKey(dbName string, key []byte) []byte {
	return append(append([]byte(dbName), dbNameKeySep...), key...)
}

func retrieveAppKey(levelKey []byte) []byte {
	return bytes.SplitN(levelKey, dbNameKeySep, 2)[1]
}
