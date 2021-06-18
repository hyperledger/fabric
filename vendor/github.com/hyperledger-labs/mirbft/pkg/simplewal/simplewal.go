/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package simplewal is a basic WAL implementation meant to be the first 'real' WAL
// option for mirbft.  More sophisticated WALs with checksums, byte alignments, etc.
// may be produced in the future, but this is just a simple place to start.
package simplewal

import (
	"sync"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"

	"github.com/pkg/errors"
	"github.com/tidwall/wal"
	"google.golang.org/protobuf/proto"
)

type WAL struct {
	mutex sync.Mutex
	log   *wal.Log
}

func Open(path string) (*WAL, error) {
	log, err := wal.Open(path, &wal.Options{
		NoSync: true,
		NoCopy: true,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not open WAL")
	}

	return &WAL{
		log: log,
	}, nil
}

func (w *WAL) IsEmpty() (bool, error) {
	firstIndex, err := w.log.FirstIndex()
	if err != nil {
		return false, errors.WithMessage(err, "could not read first index")
	}

	return firstIndex == 0, nil
}

func (w *WAL) LoadAll(forEach func(index uint64, p *msgs.Persistent)) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	firstIndex, err := w.log.FirstIndex()
	if err != nil {
		return errors.WithMessage(err, "could not read first index")
	}

	if firstIndex == 0 {
		// WAL is empty
		return nil
	}

	lastIndex, err := w.log.LastIndex()
	if err != nil {
		return errors.WithMessage(err, "could not read first index")
	}

	for i := firstIndex; i <= lastIndex; i++ {
		data, err := w.log.Read(i)
		if err != nil {
			return errors.WithMessagef(err, "could not read index %d", i)
		}

		result := &msgs.Persistent{}
		err = proto.Unmarshal(data, result)
		if err != nil {
			return errors.WithMessage(err, "error decoding to proto, is the WAL corrupt?")
		}

		forEach(i, result)
	}

	return nil
}

func (w *WAL) Write(index uint64, p *msgs.Persistent) error {
	data, err := proto.Marshal(p)
	if err != nil {
		return errors.WithMessage(err, "could not marshal")
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.log.Write(index, data)
}

func (w *WAL) Truncate(index uint64) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.log.TruncateFront(index)
}

func (w *WAL) Sync() error {
	return w.log.Sync()
}

func (w *WAL) Close() error {
	return w.log.Close()
}
