/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badgerdbhelper

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("badgerdbhelper")

type dbState int32

const (
	// ManifestFilename is the filename for the manifest file.
	ManifestFilename = "MANIFEST"

	closed dbState = iota
	opened
)

// DB - a wrapper on an actual store
type DB struct {
	conf    *Conf
	db      *badger.DB
	dbState dbState
	mutex   sync.RWMutex
}

// CreateDB constructs a `DB`
func CreateDB(conf *Conf) *DB {
	return &DB{
		conf:    conf,
		dbState: closed,
	}
}

// Open opens the underlying db
func (dbInst *DB) Open() {
	dbInst.mutex.Lock()
	defer dbInst.mutex.Unlock()
	if dbInst.dbState == opened {
		return
	}
	dbPath := dbInst.conf.DBPath
	var err error
	var dirEmpty bool
	if dirEmpty, err = fileutil.CreateDirIfMissing(dbPath); err != nil {
		panic(fmt.Sprintf("Error creating dir if missing: %s", err))
	}
	if !dirEmpty {
		if _, err := os.Stat(filepath.Join(dbPath, ManifestFilename)); err != nil {
			panic(fmt.Sprintf("Error opening badgerdb: %s", err))
		}
	}
	if dbInst.db, err = badger.Open(badger.DefaultOptions(dbPath)); err != nil {
		panic(fmt.Sprintf("Error opening badgerdb: %s", err))
	}
	dbInst.dbState = opened
}

// IsEmpty returns whether or not a database is empty
func (dbInst *DB) IsEmpty() (bool, error) {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	var hasItems bool
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	err := dbInst.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()
		it.Rewind()
		hasItems = it.Valid()
		return nil
	})
	return !hasItems,
		errors.Wrapf(err, "error while trying to see if the badgerdb at path [%s] is empty", dbInst.conf.DBPath)
}

// Close closes the underlying db
func (dbInst *DB) Close() {
	dbInst.mutex.Lock()
	defer dbInst.mutex.Unlock()
	if dbInst.dbState == closed {
		return
	}
	if err := dbInst.db.Close(); err != nil {
		logger.Errorf("Error closing badgerdb: %s", err)
	}
	dbInst.dbState = closed
}

// Get returns the value for the given key
func (dbInst *DB) Get(key []byte) ([]byte, error) {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	var value []byte
	err := dbInst.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		value = nil
		err = nil
	}
	if err != nil {
		logger.Errorf("Error retrieving badgerdb key [%#v]: %s", key, err)
		return nil, errors.Wrapf(err, "error retrieving badgerdb key [%#v]", key)
	}
	return value, nil
}

// Put saves the key/value. Last arg is sync on/off flag. It is unused as
// all badgerdb writes can survive process crashes or k8s environments
// with sync set to false. Sync is false by default.
func (dbInst *DB) Put(key []byte, value []byte, _ bool) error {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	err := dbInst.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, value)
		return err
	})
	if err != nil {
		logger.Errorf("Error writing badgerdb key [%#v]", key)
		return errors.Wrapf(err, "error writing badgerdb key [%#v]", key)
	}
	return nil
}

// Delete deletes the given key. Last arg is sync on/off flag. It is unused as
// all badgerdb writes can survive process crashes or k8s environments
// with sync set to false. Sync is false by default.
func (dbInst *DB) Delete(key []byte, _ bool) error {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	err := dbInst.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		return err
	})
	if err != nil {
		logger.Errorf("Error deleting badgerdb key [%#v]", key)
		return errors.Wrapf(err, "error deleting badgerdb key [%#v]", key)
	}
	return nil
}

type RangeIterator struct {
	iterator *badger.Iterator
	startKey []byte
	endKey   []byte
}

// Key wraps Badger's functions to make function similar Leveldb Key
func (itr *RangeIterator) Key() []byte {
	return itr.iterator.Item().KeyCopy(nil)
}

// GetIterator returns an iterator over key-value store. The iterator should be released after the use.
// The resultset contains all the keys that are present in the db between the startKey (inclusive) and the endKey (exclusive).
// A nil startKey represents the first available key and a nil endKey represent a logical key after the last available key
func (dbInst *DB) GetIterator(startKey []byte, endKey []byte) RangeIterator {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	txn := dbInst.db.NewTransaction(true)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()
	return RangeIterator{
		iterator: itr,
		startKey: startKey,
		endKey:   endKey,
	}
}

// WriteBatch writes a batch. Last arg is sync on/off flag. It is unused as
// all badgerdb writes can survive process crashes or k8s environments
// with sync set to false. Sync is false by default.
func (dbInst *DB) WriteBatch(batch *badger.WriteBatch, _ bool) error {
	dbInst.mutex.RLock()
	defer dbInst.mutex.RUnlock()
	if err := batch.Flush(); err != nil {
		return errors.Wrap(err, "error writing batch to badgerdb")
	}
	return nil
}

// FileLock encapsulate the DB that holds the file lock.
// As the FileLock to be used by a single process/goroutine,
// there is no need for the semaphore to synchronize the
// FileLock usage.
type FileLock struct {
	db       *badger.DB
	filePath string
}

// NewFileLock returns a new file based lock manager.
func NewFileLock(filePath string) *FileLock {
	return &FileLock{
		filePath: filePath,
	}
}

// Lock acquire a file lock. We achieve this by opening
// a db for the given filePath. Internally, badgerdb acquires a
// file lock while opening a db. If the db is opened again by the same or
// another process not in the read-only mode, error would be returned.
// When the db is closed or the owner process dies, the lock would be
// released and hence the other process can open the db.
func (f *FileLock) Lock() error {
	var err error
	var dirEmpty bool
	if dirEmpty, err = fileutil.CreateDirIfMissing(f.filePath); err != nil {
		panic(fmt.Sprintf("Error creating dir if missing: %s", err))
	}
	if !dirEmpty {
		if _, err := os.Stat(filepath.Join(f.filePath, ManifestFilename)); err != nil {
			panic(fmt.Sprintf("Error opening badgerdb: %s", err))
		}
	}
	db, err := badger.Open(badger.DefaultOptions(f.filePath))
	if fmt.Sprint(err) == fmt.Sprintf("Cannot acquire directory lock on \"%s\".  Another process is using this Badger database. error: resource temporarily unavailable", f.filePath) {
		return errors.Errorf("lock is already acquired on file %s", f.filePath)
	}
	if err != nil {
		panic(fmt.Sprintf("Error acquiring lock on file %s: %s", f.filePath, err))
	}

	// only mutate the lock db reference AFTER validating that the lock was held.
	f.db = db

	return nil
}

// Determine if the lock is currently held open.
func (f *FileLock) IsLocked() bool {
	return f.db != nil
}

// Unlock releases a previously acquired lock. We achieve this by closing
// the previously opened db. FileUnlock can be called multiple times.
func (f *FileLock) Unlock() {
	if f.db == nil {
		return
	}
	if err := f.db.Close(); err != nil {
		logger.Warningf("unable to release the lock on file %s: %s", f.filePath, err)
		return
	}
	f.db = nil
}
