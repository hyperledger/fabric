/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package leveldbhelper

import (
	"fmt"
	"sync"
	"syscall"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	goleveldbutil "github.com/syndtr/goleveldb/leveldb/util"
)

var logger = flogging.MustGetLogger("leveldbhelper")

type dbState int32

const (
	closed dbState = iota
	opened
)

// Conf configuration for `DB`
type Conf struct {
	DBPath string
}

// DB - a wrapper on an actual store
type DB struct {
	conf    *Conf
	db      *leveldb.DB
	dbState dbState
	mux     sync.Mutex

	readOpts        *opt.ReadOptions
	writeOptsNoSync *opt.WriteOptions
	writeOptsSync   *opt.WriteOptions
}

// CreateDB constructs a `DB`
func CreateDB(conf *Conf) *DB {
	readOpts := &opt.ReadOptions{}
	writeOptsNoSync := &opt.WriteOptions{}
	writeOptsSync := &opt.WriteOptions{}
	writeOptsSync.Sync = true

	return &DB{
		conf:            conf,
		dbState:         closed,
		readOpts:        readOpts,
		writeOptsNoSync: writeOptsNoSync,
		writeOptsSync:   writeOptsSync}
}

// Open opens the underlying db
func (dbInst *DB) Open() {
	dbInst.mux.Lock()
	defer dbInst.mux.Unlock()
	if dbInst.dbState == opened {
		return
	}
	dbOpts := &opt.Options{}
	dbPath := dbInst.conf.DBPath
	var err error
	var dirEmpty bool
	if dirEmpty, err = util.CreateDirIfMissing(dbPath); err != nil {
		panic(fmt.Sprintf("Error creating dir if missing: %s", err))
	}
	dbOpts.ErrorIfMissing = !dirEmpty
	if dbInst.db, err = leveldb.OpenFile(dbPath, dbOpts); err != nil {
		panic(fmt.Sprintf("Error opening leveldb: %s", err))
	}
	dbInst.dbState = opened
}

// Close closes the underlying db
func (dbInst *DB) Close() {
	dbInst.mux.Lock()
	defer dbInst.mux.Unlock()
	if dbInst.dbState == closed {
		return
	}
	if err := dbInst.db.Close(); err != nil {
		logger.Errorf("Error closing leveldb: %s", err)
	}
	dbInst.dbState = closed
}

// Get returns the value for the given key
func (dbInst *DB) Get(key []byte) ([]byte, error) {
	value, err := dbInst.db.Get(key, dbInst.readOpts)
	if err == leveldb.ErrNotFound {
		value = nil
		err = nil
	}
	if err != nil {
		logger.Errorf("Error retrieving leveldb key [%#v]: %s", key, err)
		return nil, errors.Wrapf(err, "error retrieving leveldb key [%#v]", key)
	}
	return value, nil
}

// Put saves the key/value
func (dbInst *DB) Put(key []byte, value []byte, sync bool) error {
	wo := dbInst.writeOptsNoSync
	if sync {
		wo = dbInst.writeOptsSync
	}
	err := dbInst.db.Put(key, value, wo)
	if err != nil {
		logger.Errorf("Error writing leveldb key [%#v]", key)
		return errors.Wrapf(err, "error writing leveldb key [%#v]", key)
	}
	return nil
}

// Delete deletes the given key
func (dbInst *DB) Delete(key []byte, sync bool) error {
	wo := dbInst.writeOptsNoSync
	if sync {
		wo = dbInst.writeOptsSync
	}
	err := dbInst.db.Delete(key, wo)
	if err != nil {
		logger.Errorf("Error deleting leveldb key [%#v]", key)
		return errors.Wrapf(err, "error deleting leveldb key [%#v]", key)
	}
	return nil
}

// GetIterator returns an iterator over key-value store. The iterator should be released after the use.
// The resultset contains all the keys that are present in the db between the startKey (inclusive) and the endKey (exclusive).
// A nil startKey represents the first available key and a nil endKey represent a logical key after the last available key
func (dbInst *DB) GetIterator(startKey []byte, endKey []byte) iterator.Iterator {
	return dbInst.db.NewIterator(&goleveldbutil.Range{Start: startKey, Limit: endKey}, dbInst.readOpts)
}

// WriteBatch writes a batch
func (dbInst *DB) WriteBatch(batch *leveldb.Batch, sync bool) error {
	wo := dbInst.writeOptsNoSync
	if sync {
		wo = dbInst.writeOptsSync
	}
	if err := dbInst.db.Write(batch, wo); err != nil {
		return errors.Wrap(err, "error writing batch to leveldb")
	}
	return nil
}

// FileLock encapsulate the DB that holds the file lock.
// As the FileLock to be used by a single process/goroutine,
// there is no need for the semaphore to synchronize the
// FileLock usage.
type FileLock struct {
	db       *leveldb.DB
	filePath string
}

// NewFileLock returns a new file based lock manager.
func NewFileLock(filePath string) *FileLock {
	return &FileLock{
		filePath: filePath,
	}
}

// Lock acquire a file lock. We achieve this by opening
// a db for the given filePath. Internally, leveldb acquires a
// file lock while opening a db. If the db is opened again by the same or
// another process, error would be returned. When the db is closed
// or the owner process dies, the lock would be released and hence
// the other process can open the db. We exploit this leveldb
// functionality to acquire and release file lock as the leveldb
// supports this for Windows, Solaris, and Unix.
func (f *FileLock) Lock() error {
	dbOpts := &opt.Options{}
	var err error
	var dirEmpty bool
	if dirEmpty, err = util.CreateDirIfMissing(f.filePath); err != nil {
		panic(fmt.Sprintf("Error creating dir if missing: %s", err))
	}
	dbOpts.ErrorIfMissing = !dirEmpty
	f.db, err = leveldb.OpenFile(f.filePath, dbOpts)
	if err != nil && err == syscall.EAGAIN {
		return errors.Errorf("lock is already acquired on file %s", f.filePath)
	}
	if err != nil {
		panic(fmt.Sprintf("Error acquiring lock on file %s: %s", f.filePath, err))
	}
	return nil
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
