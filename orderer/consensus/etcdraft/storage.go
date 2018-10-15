/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"os"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/coreos/etcd/wal/walpb"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

// MemoryStorage is currently backed by etcd/raft.MemoryStorage. This interface is
// defined to expose dependencies of fsm so that it may be swapped in the
// future. TODO(jay) Add other necessary methods to this interface once we need
// them in implementation, e.g. ApplySnapshot.
type MemoryStorage interface {
	raft.Storage
	Append(entries []raftpb.Entry) error
	SetHardState(st raftpb.HardState) error
}

// RaftStorage encapsulates storages needed for etcd/raft data, i.e. memory, wal
type RaftStorage struct {
	ram MemoryStorage
	wal *wal.WAL
}

// Restore attempts to restore a storage state from persited etcd/raft data.
// If no data is found, it returns a fresh storage.
func Restore(lg *flogging.FabricLogger, applied uint64, walDir string, ram MemoryStorage) (*RaftStorage, bool, error) {
	hasWAL := wal.Exist(walDir)
	if !hasWAL && applied != 0 {
		return nil, hasWAL, errors.Errorf("applied index is not zero but no WAL data found")
	}

	if !hasWAL {
		lg.Infof("No WAL data found, creating new WAL at path '%s'", walDir)
		// TODO(jay_guo) add metadata to be persisted with wal once we need it.
		// use case could be data dump and restore on a new node.
		w, err := wal.Create(walDir, nil)
		if err == os.ErrExist {
			lg.Fatalf("programming error, we've just checked that WAL does not exist")
		}

		if err != nil {
			return nil, hasWAL, errors.Errorf("failed to initialize WAL: %s", err)
		}

		if err = w.Close(); err != nil {
			return nil, hasWAL, errors.Errorf("failed to close the WAL just created: %s", err)
		}
	} else {
		lg.Infof("Found WAL data at path '%s', replaying it", walDir)
	}

	w, err := wal.Open(walDir, walpb.Snapshot{})
	if err != nil {
		return nil, hasWAL, errors.Errorf("failed to open existing WAL: %s", err)
	}

	_, st, ents, err := w.ReadAll() // See previous TODO in this func for metadata
	if err != nil {
		return nil, hasWAL, errors.Errorf("failed to read WAL: %s", err)
	}

	lg.Debugf("Setting HardState to {Term: %d, Commit: %d}", st.Term, st.Commit)
	ram.SetHardState(st) // MemoryStorage.SetHardState always returns nil

	lg.Debugf("Appending %d entries to memory storage", len(ents))
	ram.Append(ents) // MemoryStorage.Append always return nil

	return &RaftStorage{ram: ram, wal: w}, hasWAL, nil
}

// Store persists etcd/raft data
func (rs *RaftStorage) Store(entries []raftpb.Entry, hardstate raftpb.HardState) error {
	if err := rs.ram.Append(entries); err != nil {
		return err
	}

	if err := rs.wal.Save(hardstate, entries); err != nil {
		return err
	}

	return nil
}

// Close closes storage
func (rs *RaftStorage) Close() error {
	if err := rs.wal.Close(); err != nil {
		return err
	}

	return nil
}
