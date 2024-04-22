/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badgerdbhelper

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
)

const (
	// internalDBName is used to keep track of data related to internals such as data format
	// _ is used as name because this is not allowed as a channelname
	internalDBName = "_"
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

// Provider enables to use a single badgerdb as multiple logical badgerdbs
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
			DBInfo:         fmt.Sprintf("badgerdb at [%s]", conf.DBPath),
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

// Close closes the underlying badgerdb
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

func (h *DBHandle) GetMaxBatchSize() int64 {
	return h.db.db.MaxBatchSize()
}

func (h *DBHandle) GetMaxBatchCount() int64 {
	return h.db.db.MaxBatchCount()
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
	defer iter.iterator.Close()

	// use badgerdb iterator directly to be more efficient
	dbIter := iter.iterator

	numKeys := 0
	var batchSize int64
	batchSize = 0
	batch := h.db.db.NewWriteBatch()
	defer batch.Cancel()
	for dbIter.Seek([]byte(h.dbName)); dbIter.ValidForPrefix([]byte(h.dbName)); dbIter.Next() {
		key := dbIter.Item().KeyCopy(nil)
		numKeys++
		batchSize = batchSize + int64(len(key))
		batch.Delete(key)
		if batchSize >= h.db.db.MaxBatchSize() || numKeys == int(h.db.db.MaxBatchCount()) {
			if err := h.db.WriteBatch(batch, true); err != nil {
				return err
			}
			logger.Infof("Have removed %d entries for channel %s in badgerdb %s", numKeys, h.dbName, h.db.conf.DBPath)
			batchSize = 0
			numKeys = 0
			batch = h.db.db.NewWriteBatch()
			defer batch.Cancel()
		}
	}
	if err = batch.Error(); err == nil {
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
	defer itr.iterator.Close()

	return !itr.Next(), nil
}

// NewUpdateBatch returns a new UpdateBatch that can be used to update the db
func (h *DBHandle) NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{
		dbName:     h.dbName,
		WriteBatch: h.db.db.NewWriteBatch(),
	}
}

// WriteBatch writes a batch in an atomic way
func (h *DBHandle) WriteBatch(batch *UpdateBatch, sync bool) error {
	if h.db.db.IsClosed() {
		return errors.New("error writing batch to badgerdb")
	}
	if batch == nil || batch.Error() != nil {
		return nil
	}
	if err := h.db.WriteBatch(batch.WriteBatch, sync); err != nil {
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
	if h.db.dbState == closed {
		err := errors.New("internal badgerdb error while obtaining db iterator: badgerdb: closed")
		return nil, err
	}
	itr := h.db.GetIterator(sKey, eKey)
	return &Iterator{h.dbName, itr, true, false, false}, nil
}

// Close closes the DBHandle after its db data have been deleted
func (h *DBHandle) Close() {
	if h.closeFunc != nil {
		h.closeFunc()
	}
}

// UpdateBatch encloses the details of multiple `updates`
type UpdateBatch struct {
	*badger.WriteBatch
	dbName string
}

// Put adds a KV
func (b *UpdateBatch) Put(key []byte, value []byte) {
	if value == nil {
		panic("Nil value not allowed")
	}
	if err := b.Set(constructLevelKey(b.dbName, key), value); err != nil {
		logger.Errorf("Error while setting key [%#v] and value [%#v]: %#v", constructLevelKey(b.dbName, key), value, err)
	}
}

// Delete deletes a Key and associated value
func (b *UpdateBatch) Delete(key []byte) {
	b.WriteBatch.Delete(constructLevelKey(b.dbName, key))
}

// Iterator extends actual badgerdb iterator
type Iterator struct {
	dbName string
	RangeIterator
	justOpened bool
	outOfRange bool
	IgnoreNext bool
}

// Key wraps actual badgerdb iterator method
func (itr *Iterator) Key() []byte {
	key := itr.iterator.Item().KeyCopy(nil)
	return retrieveAppKey(key)
}

// Next wraps Badger's functions to make function similar Leveldb Next
func (itr *Iterator) Next() bool {
	if itr.outOfRange {
		return false
	}
	// Check does iterator need start from startKey
	if itr.justOpened {
		itr.iterator.Seek(itr.startKey)
		itr.justOpened = false
		if !itr.iterator.ValidForPrefix([]byte(itr.dbName)) {
			return false
		}
		if itr.endKey != nil && bytes.Compare(itr.endKey, itr.iterator.Item().Key()) <= 0 {
			itr.outOfRange = true
			return false
		}
		return true
	}
	if !itr.IgnoreNext {
		itr.iterator.Next()
	}
	itr.IgnoreNext = false
	if !itr.iterator.ValidForPrefix([]byte(itr.dbName)) {
		itr.outOfRange = true
		return false
	}
	if itr.endKey != nil && bytes.Compare(itr.endKey, itr.iterator.Item().Key()) <= 0 {
		itr.outOfRange = true
		return false
	}
	if bytes.Equal(itr.endKey, itr.iterator.Item().Key()) {
		itr.outOfRange = true
		return false
	}
	return true
}

// Release wraps Badger's function to make function similar Leveldb
func (itr *Iterator) Release() {
	itr.iterator.Close()
}

// First wraps Badger's function to make function similar Leveldb
func (itr *Iterator) First() bool {
	itr.outOfRange = false
	itr.iterator.Rewind()
	return itr.iterator.Valid()
}

func (itr *Iterator) Valid() bool {
	if itr.outOfRange {
		return false
	}
	if !itr.iterator.ValidForPrefix([]byte(itr.dbName)) ||
		bytes.Equal(itr.endKey, itr.iterator.Item().Key()) {
		itr.outOfRange = true
		return false
	}
	return true
}

func (itr *Iterator) Value() []byte {
	v, err := itr.iterator.Item().ValueCopy(nil)
	if err != nil {
		logger.Errorf("Error while getting ValueCopy: %#v", err)
		return nil
	}
	return v
}

// Seek moves the iterator to the first key/value pair
// whose key is greater than or equal to the given key.
// It returns whether such pair exist.
func (itr *Iterator) Seek(key []byte) bool {
	itr.justOpened = false
	itr.outOfRange = false
	levelKey := constructLevelKey(itr.dbName, key)
	if bytes.Compare(levelKey, itr.startKey) == -1 {
		itr.iterator.Seek(itr.startKey)
		return itr.iterator.ValidForPrefix([]byte(itr.dbName))
	} else if bytes.Compare(levelKey, itr.endKey) == 1 {
		itr.iterator.Seek(itr.endKey)
		itr.outOfRange = true
		return itr.iterator.ValidForPrefix([]byte(itr.dbName))
	}
	itr.iterator.Seek(levelKey)
	return itr.iterator.ValidForPrefix([]byte(itr.dbName))
}

// constructLevelKey is similar to the same function from leveldb_provider
func constructLevelKey(dbName string, key []byte) []byte {
	return append(append([]byte(dbName), dbNameKeySep...), key...)
}

// retrieveAppKey is similar to the same function from leveldb_provider
func retrieveAppKey(levelKey []byte) []byte {
	return bytes.SplitN(levelKey, dbNameKeySep, 2)[1]
}
