/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

// Supported database types.
const (
	GoLevelDB = "goleveldb"
)

// DB is a low-level connection to a physical database.
type DB interface {
	Open()
	Close()
	Get(key []byte) ([]byte, error)
	Put(key, value []byte, sync bool) error
	Delete(key []byte, sync bool) error
	GetIterator(start, end []byte) (Iterator, error)
	IsEmpty() (bool, error)
	NewUpdateBatch() Batch
	WriteBatch(b Batch, sync bool) error
}

// Provider is a multi-tenant factory for logical databases with key prefixing.
type Provider interface {
	GetDBHandle(name string) DBHandle
	Drop(name string) error
	Close()
	GetDataFormat() (string, error)
	SetDataFormat(string) error
}

// DBHandle is a handle to a logical database with key prefixing (dbName+0x00).
type DBHandle interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte, sync bool) error
	Delete(key []byte, sync bool) error
	GetIterator(start, end []byte) (Iterator, error)
	NewUpdateBatch() Batch
	WriteBatch(b Batch, sync bool) error
	IsEmpty() (bool, error)
	Close()
}

// Batch is an atomic write batch.
type Batch interface {
	Put(key, value []byte)
	Delete(key []byte)
	Len() int
	Size() int
	Reset()
}

// FileLock is a cross-process file lock.
type FileLock interface {
	Lock() error
	Unlock()
	IsLocked() bool
}

// Iterator iterates over a range of keys.
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Error() error
	Release()
	Seek(key []byte) bool
	Prev() bool
	First() bool
	Last() bool
}
