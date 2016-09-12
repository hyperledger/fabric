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

package db

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/core/ledgernext/util"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

var logger = logging.MustGetLogger("kvledger.db")

type dbState int32

const (
	defaultCFName         = "default"
	closed        dbState = iota
	opened
)

// Conf configuration for `DB`
type Conf struct {
	DBPath     string
	CFNames    []string
	DisableWAL bool
}

// DB - a rocksDB instance
type DB struct {
	conf         *Conf
	rocksDB      *gorocksdb.DB
	cfHandlesMap map[string]*gorocksdb.ColumnFamilyHandle
	dbState      dbState
	mux          sync.Mutex

	readOpts  *gorocksdb.ReadOptions
	writeOpts *gorocksdb.WriteOptions
}

// CreateDB constructs a `DB`
func CreateDB(conf *Conf) *DB {
	conf.CFNames = append(conf.CFNames, defaultCFName)
	readOpts := gorocksdb.NewDefaultReadOptions()
	writeOpts := gorocksdb.NewDefaultWriteOptions()
	writeOpts.DisableWAL(conf.DisableWAL)
	return &DB{
		conf:         conf,
		cfHandlesMap: make(map[string]*gorocksdb.ColumnFamilyHandle),
		dbState:      closed,
		readOpts:     readOpts,
		writeOpts:    writeOpts}
}

// Open open underlying rocksdb
func (dbInst *DB) Open() {
	dbInst.mux.Lock()
	if dbInst.dbState == opened {
		dbInst.mux.Unlock()
		return
	}

	defer dbInst.mux.Unlock()

	dbPath := dbInst.conf.DBPath
	dirEmpty, err := util.CreateDirIfMissing(dbPath)
	if err != nil {
		panic(fmt.Sprintf("Error while trying to open DB: %s", err))
	}
	opts := gorocksdb.NewDefaultOptions()
	defer opts.Destroy()
	opts.SetCreateIfMissing(dirEmpty)
	opts.SetCreateIfMissingColumnFamilies(true)

	var cfOpts []*gorocksdb.Options
	for range dbInst.conf.CFNames {
		cfOpts = append(cfOpts, opts)
	}
	db, cfHandlers, err := gorocksdb.OpenDbColumnFamilies(opts, dbPath, dbInst.conf.CFNames, cfOpts)
	if err != nil {
		panic(fmt.Sprintf("Error opening DB: %s", err))
	}
	dbInst.rocksDB = db
	for i := 0; i < len(dbInst.conf.CFNames); i++ {
		dbInst.cfHandlesMap[dbInst.conf.CFNames[i]] = cfHandlers[i]
	}
	dbInst.dbState = opened
}

// Close releases all column family handles and closes rocksdb
func (dbInst *DB) Close() {
	dbInst.mux.Lock()
	if dbInst.dbState == closed {
		dbInst.mux.Unlock()
		return
	}

	defer dbInst.mux.Unlock()
	for _, cfHandler := range dbInst.cfHandlesMap {
		cfHandler.Destroy()
	}
	dbInst.rocksDB.Close()
	dbInst.dbState = closed
}

func (dbInst *DB) isOpen() bool {
	dbInst.mux.Lock()
	defer dbInst.mux.Unlock()
	return dbInst.dbState == opened
}

// Get returns the value for the given column family and key
func (dbInst *DB) Get(cfHandle *gorocksdb.ColumnFamilyHandle, key []byte) ([]byte, error) {
	slice, err := dbInst.rocksDB.GetCF(dbInst.readOpts, cfHandle, key)
	if err != nil {
		fmt.Println("Error while trying to retrieve key:", key)
		return nil, err
	}
	defer slice.Free()
	if slice.Data() == nil {
		return nil, nil
	}
	data := makeCopy(slice.Data())
	return data, nil
}

// Put saves the key/value in the given column family
func (dbInst *DB) Put(cfHandle *gorocksdb.ColumnFamilyHandle, key []byte, value []byte) error {
	err := dbInst.rocksDB.PutCF(dbInst.writeOpts, cfHandle, key, value)
	if err != nil {
		fmt.Println("Error while trying to write key:", key)
		return err
	}
	return nil
}

// Delete delets the given key in the specified column family
func (dbInst *DB) Delete(cfHandle *gorocksdb.ColumnFamilyHandle, key []byte) error {
	err := dbInst.rocksDB.DeleteCF(dbInst.writeOpts, cfHandle, key)
	if err != nil {
		fmt.Println("Error while trying to delete key:", key)
		return err
	}
	return nil
}

// WriteBatch writes a batch
func (dbInst *DB) WriteBatch(batch *gorocksdb.WriteBatch) error {
	if err := dbInst.rocksDB.Write(dbInst.writeOpts, batch); err != nil {
		return err
	}
	return nil
}

// GetIterator returns an iterator for the given column family
func (dbInst *DB) GetIterator(cfName string) *gorocksdb.Iterator {
	return dbInst.rocksDB.NewIteratorCF(dbInst.readOpts, dbInst.GetCFHandle(cfName))
}

// GetCFHandle returns handle to a named column family
func (dbInst *DB) GetCFHandle(cfName string) *gorocksdb.ColumnFamilyHandle {
	return dbInst.cfHandlesMap[cfName]
}

// GetDefaultCFHandle returns handle to default column family
func (dbInst *DB) GetDefaultCFHandle() *gorocksdb.ColumnFamilyHandle {
	return dbInst.GetCFHandle(defaultCFName)
}

// Flush flushes rocksDB memory to sst files
func (dbInst *DB) Flush(wait bool) error {
	flushOpts := gorocksdb.NewDefaultFlushOptions()
	defer flushOpts.Destroy()
	flushOpts.SetWait(wait)
	return dbInst.rocksDB.Flush(flushOpts)
}

func makeCopy(src []byte) []byte {
	dest := make([]byte, len(src))
	copy(dest, src)
	return dest
}
