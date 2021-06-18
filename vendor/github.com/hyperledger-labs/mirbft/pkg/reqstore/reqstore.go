/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package reqstore is an implementation of the RequestStore utilized by the samples.
// Depending on your application, it may or may not be appropriate.  In particular, if your
// application wants to retain the requests rather than simply apply and discard them, you may
// wish to write your own or adapt this one.
package reqstore

import (
	"fmt"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	badger "github.com/dgraph-io/badger/v2"
	"github.com/pkg/errors"
)

func reqKey(ack *msgs.RequestAck) []byte {
	return []byte(fmt.Sprintf("req-%d.%d.%x", ack.ClientId, ack.ReqNo, ack.Digest))
}

func allocKey(clientID, reqNo uint64) []byte {
	return []byte(fmt.Sprintf("alloc-%d.%d", clientID, reqNo))
}

type Store struct {
	db *badger.DB
}

func Open(dirPath string) (*Store, error) {
	var badgerOpts badger.Options
	if dirPath == "" {
		badgerOpts = badger.DefaultOptions("").WithInMemory(true)
	} else {
		badgerOpts = badger.DefaultOptions(dirPath).WithSyncWrites(false).WithTruncate(true)
		// TODO, maybe WithDetectConflicts as false?
	}
	db, err := badger.Open(badgerOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "could not open backing db")
	}

	return &Store{
		db: db,
	}, nil
}

func (s *Store) PutAllocation(clientID, reqNo uint64, digest []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(allocKey(clientID, reqNo), digest)
	})
}

func (s *Store) GetAllocation(clientID, reqNo uint64) ([]byte, error) {
	var valCopy []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(allocKey(clientID, reqNo))
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		return err
	})

	if err == badger.ErrKeyNotFound {
		return nil, nil
	}

	return valCopy, err
}

func (s *Store) PutRequest(requestAck *msgs.RequestAck, data []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(reqKey(requestAck), data)
	})
}

func (s *Store) GetRequest(requestAck *msgs.RequestAck) ([]byte, error) {
	var valCopy []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(reqKey(requestAck))
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		return err
	})

	if err == badger.ErrKeyNotFound {
		return nil, nil
	}

	return valCopy, err
}

func (s *Store) Commit(ack *msgs.RequestAck) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(reqKey(ack))
	})
}

func (s *Store) Sync() error {
	return s.db.Sync()
}

func (s *Store) Close() {
	s.db.Close()
}
