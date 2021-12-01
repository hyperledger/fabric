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
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

const (
	// internalDBName is used to keep track of data related to internals such as data format
	// _ is used as name because this is not allowed as a channelname
	internalDBName = "_"
	// maxBatchSize limits the memory usage (1MB) for a batch. It is measured by the total number of bytes
	// of all the keys in a batch.
	maxBatchSize = 1000000
)

var (
	dbNameKeySep     = []byte{0x00}
	lastKeyIndicator = byte(0x01)
	formatVersionKey = []byte{'f'} // a single key in db whose value indicates the version of the data format
)

// closeFunc closes the db handle
type closeFunc func()

// Conf configuration for `Provider`
//
// `ExpectedFormat` is the expected value of the format key in the internal database.
// At the time of opening the db, A check is performed that
// either the db is empty (i.e., opening for the first time) or the value
// of the formatVersionKey is equal to `ExpectedFormat`. Otherwise, an error is returned.
// A nil value for ExpectedFormat indicates that the format is never set and hence there is no such record.
type Conf struct {
	DBPath         string
	ExpectedFormat string
}

// Provider enables to use a single leveldb as multiple logical leveldbs
type Provider struct {
	db *DB

	mux       sync.Mutex
	dbHandles map[string]*DBHandle
}

// DataFormatInfo contains the information about the version of the data format
type DataFormatInfo struct {
	FormatVerison string // version of the data format
	IsDBEmpty     bool   // set to true if the db does not contain any data
}

// RetrieveDataFormatInfo retrieves the DataFormatInfo for the db at the supplied `dbPath`
func RetrieveDataFormatInfo(dbPath string) (*DataFormatInfo, error) {
	db := CreateDB(&Conf{DBPath: dbPath})
	db.Open()
	defer db.Close()

	dbEmpty, err := db.IsEmpty()
	if err != nil {
		return nil, err
	}

	internalDB := &DBHandle{
		db:     db,
		dbName: internalDBName,
	}

	formatVersion, err := internalDB.Get(formatVersionKey)
	if err != nil {
		return nil, err
	}

	return &DataFormatInfo{
		IsDBEmpty:     dbEmpty,
		FormatVerison: string(formatVersion),
	}, nil
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

	if dbEmpty && conf.ExpectedFormat != "" {
		logger.Infof("DB is empty Setting db format as %s", conf.ExpectedFormat)
		if err := internalDB.Put(formatVersionKey, []byte(conf.ExpectedFormat), true); err != nil {
			return nil, err
		}
		return db, nil
	}

	formatVersion, err := internalDB.Get(formatVersionKey)
	if err != nil {
		return nil, err
	}
	logger.Debugf("Checking for db format at path [%s]", conf.DBPath)

	if !bytes.Equal(formatVersion, []byte(conf.ExpectedFormat)) {
		logger.Errorf("The db at path [%s] contains data in unexpected format. expected data format = [%s] (%#v), data format = [%s] (%#v).",
			conf.DBPath, conf.ExpectedFormat, []byte(conf.ExpectedFormat), formatVersion, formatVersion)
		return nil, &dataformat.ErrFormatMismatch{
			ExpectedFormat: conf.ExpectedFormat,
			Format:         string(formatVersion),
			DBInfo:         fmt.Sprintf("leveldb at [%s]", conf.DBPath),
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

func (p *Provider) SetDataFormat(format string) error {
	db := p.GetDBHandle(internalDBName)
	return db.Put(formatVersionKey, []byte(format), true)
}

// GetDBHandle returns a handle to a named db
func (p *Provider) GetDBHandle(dbName string) *DBHandle {
	p.mux.Lock()
	defer p.mux.Unlock()
	dbHandle := p.dbHandles[dbName]
	if dbHandle == nil {
		closeFunc := func() {
			p.mux.Lock()
			defer p.mux.Unlock()
			delete(p.dbHandles, dbName)
		}
		dbHandle = &DBHandle{dbName, p.db, closeFunc}
		p.dbHandles[dbName] = dbHandle
	}
	return dbHandle
}

// Close closes the underlying leveldb
func (p *Provider) Close() {
	p.db.Close()
}

// Drop drops all the data for the given dbName
func (p *Provider) Drop(dbName string) error {
	dbHandle := p.GetDBHandle(dbName)
	defer dbHandle.Close()
	return dbHandle.deleteAll()
}

// DBHandle is an handle to a named db
type DBHandle struct {
	dbName    string
	db        *DB
	closeFunc closeFunc
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

// DeleteAll deletes all the keys that belong to the channel (dbName).
func (h *DBHandle) deleteAll() error {
	iter, err := h.GetIterator(nil, nil)
	if err != nil {
		return err
	}
	defer iter.Release()

	// use leveldb iterator directly to be more efficient
	dbIter := iter.Iterator

	// This is common code shared by all the leveldb instances. Because each leveldb has its own key size pattern,
	// each batch is limited by memory usage instead of number of keys. Once the batch memory usage reaches maxBatchSize,
	// the batch will be committed.
	numKeys := 0
	batchSize := 0
	batch := &leveldb.Batch{}
	for dbIter.Next() {
		if err := dbIter.Error(); err != nil {
			return errors.Wrap(err, "internal leveldb error while retrieving data from db iterator")
		}
		key := dbIter.Key()
		numKeys++
		batchSize = batchSize + len(key)
		batch.Delete(key)
		if batchSize >= maxBatchSize {
			if err := h.db.WriteBatch(batch, true); err != nil {
				return err
			}
			logger.Infof("Have removed %d entries for channel %s in leveldb %s", numKeys, h.dbName, h.db.conf.DBPath)
			batchSize = 0
			batch.Reset()
		}
	}
	if batch.Len() > 0 {
		return h.db.WriteBatch(batch, true)
	}
	return nil
}

// IsEmpty returns true if no data exists for the DBHandle
func (h *DBHandle) IsEmpty() (bool, error) {
	itr, err := h.GetIterator(nil, nil)
	if err != nil {
		return false, err
	}
	defer itr.Release()

	if err := itr.Error(); err != nil {
		return false, errors.WithMessagef(itr.Error(), "internal leveldb error while obtaining next entry from iterator")
	}

	return !itr.Next(), nil
}

// NewUpdateBatch returns a new UpdateBatch that can be used to update the db
func (h *DBHandle) NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{
		dbName:       h.dbName,
		leveldbBatch: &leveldb.Batch{},
	}
}

// WriteBatch writes a batch in an atomic way
func (h *DBHandle) WriteBatch(batch *UpdateBatch, sync bool) error {
	if batch == nil || batch.leveldbBatch.Len() == 0 {
		return nil
	}
	if err := h.db.WriteBatch(batch.leveldbBatch, sync); err != nil {
		return err
	}
	return nil
}

// GetIterator gets an handle to iterator. The iterator should be released after the use.
// The resultset contains all the keys that are present in the db between the startKey (inclusive) and the endKey (exclusive).
// A nil startKey represents the first available key and a nil endKey represent a logical key after the last available key
func (h *DBHandle) GetIterator(startKey []byte, endKey []byte) (*Iterator, error) {
	sKey := constructLevelKey(h.dbName, startKey)
	eKey := constructLevelKey(h.dbName, endKey)
	if endKey == nil {
		// replace the last byte 'dbNameKeySep' by 'lastKeyIndicator'
		eKey[len(eKey)-1] = lastKeyIndicator
	}
	logger.Debugf("Getting iterator for range [%#v] - [%#v]", sKey, eKey)
	itr := h.db.GetIterator(sKey, eKey)
	if err := itr.Error(); err != nil {
		itr.Release()
		return nil, errors.Wrapf(err, "internal leveldb error while obtaining db iterator")
	}
	return &Iterator{h.dbName, itr}, nil
}

// Close closes the DBHandle after its db data have been deleted
func (h *DBHandle) Close() {
	if h.closeFunc != nil {
		h.closeFunc()
	}
}

// UpdateBatch encloses the details of multiple `updates`
type UpdateBatch struct {
	leveldbBatch *leveldb.Batch
	dbName       string
	size         int
}

// Put adds a KV
func (b *UpdateBatch) Put(key []byte, value []byte) {
	if value == nil {
		panic("Nil value not allowed")
	}
	k := constructLevelKey(b.dbName, key)
	b.leveldbBatch.Put(k, value)
	b.size += len(k) + len(value)
}

// Delete deletes a Key and associated value
func (b *UpdateBatch) Delete(key []byte) {
	k := constructLevelKey(b.dbName, key)
	b.size += len(k)
	b.leveldbBatch.Delete(k)
}

// Size returns the current size of the batch
func (b *UpdateBatch) Size() int {
	return b.size
}

// Len returns number of records in the batch
func (b *UpdateBatch) Len() int {
	return b.leveldbBatch.Len()
}

// Reset resets the batch
func (b *UpdateBatch) Reset() {
	b.leveldbBatch.Reset()
	b.size = 0
}

// Iterator extends actual leveldb iterator
type Iterator struct {
	dbName string
	iterator.Iterator
}

// Key wraps actual leveldb iterator method
func (itr *Iterator) Key() []byte {
	return retrieveAppKey(itr.Iterator.Key())
}

// Seek moves the iterator to the first key/value pair
// whose key is greater than or equal to the given key.
// It returns whether such pair exist.
func (itr *Iterator) Seek(key []byte) bool {
	levelKey := constructLevelKey(itr.dbName, key)
	return itr.Iterator.Seek(levelKey)
}

func constructLevelKey(dbName string, key []byte) []byte {
	return append(append([]byte(dbName), dbNameKeySep...), key...)
}

func retrieveAppKey(levelKey []byte) []byte {
	return bytes.SplitN(levelKey, dbNameKeySep, 2)[1]
}
