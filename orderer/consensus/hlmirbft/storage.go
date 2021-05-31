/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hyperledger-labs/mirbft/pkg/status"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/etcdserver/api/snap"
	"go.etcd.io/etcd/pkg/fileutil"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"go.etcd.io/etcd/wal"
	"go.etcd.io/etcd/wal/walpb"
)

// MaxSnapshotFiles defines max number of etcd/raft snapshot files to retain
// on filesystem. Snapshot files are read from newest to oldest, until first
// intact file is found. The more snapshot files we keep around, the more we
// mitigate the impact of a corrupted snapshots. This is exported for testing
// purpose. This MUST be greater equal than 1.
var MaxSnapshotFiles = 4

// MemoryStorage is currently backed by etcd/raft.MemoryStorage. This interface is
// defined to expose dependencies of fsm so that it may be swapped in the
// future. TODO(jay) Add other necessary methods to this interface once we need
// them in implementation, e.g. ApplySnapshot.
type MemoryStorage interface {
	raft.Storage
	Append(entries []raftpb.Entry) error
	SetHardState(st raftpb.HardState) error
	CreateSnapshot(i uint64, status *status.StateMachine) (raftpb.Snapshot, error)
	Compact(compactIndex uint64) error
	ApplySnapshot(snap raftpb.Snapshot) error
}

// MirBFTStorage encapsulates storages needed for etcd/raft data, i.e. memory, wal
type MirBFTStorage struct {
	// SnapshotCatchUpEntries uint64

	walDir  string
	snapDir string

	lg *flogging.FabricLogger

	ram  MemoryStorage
	wal  *wal.WAL
	snap *snap.Snapshotter

	// // a queue that keeps track of indices of snapshots on disk
	snapshotIndex []uint64
}

// CreateStorage attempts to create a storage to persist etcd/raft data.
// If data presents in specified disk, they are loaded to reconstruct storage state.
func CreateStorage(
	lg *flogging.FabricLogger,
	walDir string,
	snapDir string,
	ram MemoryStorage,
) (*MirBFTStorage, error) {
	sn, err := createSnapshotter(lg, snapDir)
	if err != nil {
		return nil, err
	}

	snapshot, err := sn.Load()
	if err != nil {
		if err == snap.ErrNoSnapshot {
			lg.Debugf("No snapshot found at %s", snapDir)
		} else {
			return nil, errors.Errorf("failed to load snapshot: %s", err)
		}
	} else {
		// snapshot found
		lg.Debugf("Loaded snapshot at Term %d and Index %d, Nodes: %+v",
			snapshot.Metadata.Term, snapshot.Metadata.Index, snapshot.Metadata.ConfState.Nodes)
	}

	w, st, ents, err := createOrReadWAL(lg, walDir, snapshot)
	if err != nil {
		return nil, errors.Errorf("failed to create or read WAL: %s", err)
	}

	if snapshot != nil {
		lg.Debugf("Applying snapshot to raft MemoryStorage")
		if err := ram.ApplySnapshot(*snapshot); err != nil {
			return nil, errors.Errorf("Failed to apply snapshot to memory: %s", err)
		}
	}

	lg.Debugf("Setting HardState to {Term: %d, Commit: %d}", st.Term, st.Commit)
	ram.SetHardState(st) // MemoryStorage.SetHardState always returns nil

	lg.Debugf("Appending %d entries to memory storage", len(ents))
	ram.Append(ents) // MemoryStorage.Append always return nil

	return &MirBFTStorage{
		lg:            lg,
		ram:           ram,
		wal:           w,
		snap:          sn,
		walDir:        walDir,
		snapDir:       snapDir,
		snapshotIndex: ListSnapshots(lg, snapDir),
	}, nil
}

// ListSnapshots returns a list of RaftIndex of snapshots stored on disk.
// If a file is corrupted, rename the file.
func ListSnapshots(logger *flogging.FabricLogger, snapDir string) []uint64 {
	dir, err := os.Open(snapDir)
	if err != nil {
		logger.Errorf("Failed to open snapshot directory %s: %s", snapDir, err)
		return nil
	}
	defer dir.Close()

	filenames, err := dir.Readdirnames(-1)
	if err != nil {
		logger.Errorf("Failed to read snapshot files: %s", err)
		return nil
	}

	snapfiles := []string{}
	for i := range filenames {
		if strings.HasSuffix(filenames[i], ".snap") {
			snapfiles = append(snapfiles, filenames[i])
		}
	}
	sort.Strings(snapfiles)

	var snapshots []uint64
	for _, snapfile := range snapfiles {
		fpath := filepath.Join(snapDir, snapfile)
		s, err := snap.Read(logger.Zap(), fpath)
		if err != nil {
			logger.Errorf("Snapshot file %s is corrupted: %s", fpath, err)

			broken := fpath + ".broken"
			if err = os.Rename(fpath, broken); err != nil {
				logger.Errorf("Failed to rename corrupted snapshot file %s to %s: %s", fpath, broken, err)
			} else {
				logger.Debugf("Renaming corrupted snapshot file %s to %s", fpath, broken)
			}

			continue
		}

		snapshots = append(snapshots, s.Metadata.Index)
	}

	return snapshots
}

func createSnapshotter(logger *flogging.FabricLogger, snapDir string) (*snap.Snapshotter, error) {
	if err := os.MkdirAll(snapDir, os.ModePerm); err != nil {
		return nil, errors.Errorf("failed to mkdir '%s' for snapshot: %s", snapDir, err)
	}

	return snap.New(logger.Zap(), snapDir), nil
}

func createOrReadWAL(lg *flogging.FabricLogger, walDir string, snapshot *raftpb.Snapshot) (w *wal.WAL, st raftpb.HardState, ents []raftpb.Entry, err error) {
	if !wal.Exist(walDir) {
		lg.Infof("No WAL data found, creating new WAL at path '%s'", walDir)
		// TODO(jay_guo) add metadata to be persisted with wal once we need it.
		// use case could be data dump and restore on a new node.
		w, err := wal.Create(lg.Zap(), walDir, nil)
		if err == os.ErrExist {
			lg.Fatalf("programming error, we've just checked that WAL does not exist")
		}

		if err != nil {
			return nil, st, nil, errors.Errorf("failed to initialize WAL: %s", err)
		}

		if err = w.Close(); err != nil {
			return nil, st, nil, errors.Errorf("failed to close the WAL just created: %s", err)
		}
	} else {
		lg.Infof("Found WAL data at path '%s', replaying it", walDir)
	}

	walsnap := walpb.Snapshot{}
	if snapshot != nil {
		walsnap.Index, walsnap.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
	}

	lg.Debugf("Loading WAL at Term %d and Index %d", walsnap.Term, walsnap.Index)

	var repaired bool
	for {
		if w, err = wal.Open(lg.Zap(), walDir, walsnap); err != nil {
			return nil, st, nil, errors.Errorf("failed to open WAL: %s", err)
		}

		if _, st, ents, err = w.ReadAll(); err != nil {
			lg.Warnf("Failed to read WAL: %s", err)

			if errc := w.Close(); errc != nil {
				return nil, st, nil, errors.Errorf("failed to close erroneous WAL: %s", errc)
			}

			// only repair UnexpectedEOF and only repair once
			if repaired || err != io.ErrUnexpectedEOF {
				return nil, st, nil, errors.Errorf("failed to read WAL and cannot repair: %s", err)
			}

			if !wal.Repair(lg.Zap(), walDir) {
				return nil, st, nil, errors.Errorf("failed to repair WAL: %s", err)
			}

			repaired = true
			// next loop should be able to open WAL and return
			continue
		}

		// successfully opened WAL and read all entries, break
		break
	}

	return w, st, ents, nil
}

// Snapshot returns the latest snapshot stored in memory
func (rs *MirBFTStorage) Snapshot() raftpb.Snapshot {
	sn, _ := rs.ram.Snapshot() // Snapshot always returns nil error
	return sn
}

// Store persists etcd/raft data
func (rs *MirBFTStorage) Store(entries []raftpb.Entry, hardstate raftpb.HardState, snapshot raftpb.Snapshot) error {
	if err := rs.wal.Save(hardstate, entries); err != nil {
		return err
	}

	if !raft.IsEmptySnap(snapshot) {
		if err := rs.saveSnap(snapshot); err != nil {
			return err
		}

		if err := rs.ram.ApplySnapshot(snapshot); err != nil {
			if err == raft.ErrSnapOutOfDate {
				rs.lg.Warnf("Attempted to apply out-of-date snapshot at Term %d and Index %d",
					snapshot.Metadata.Term, snapshot.Metadata.Index)
			} else {
				rs.lg.Fatalf("Unexpected programming error: %s", err)
			}
		}
	}

	if err := rs.ram.Append(entries); err != nil {
		return err
	}

	return nil
}

func (rs *MirBFTStorage) saveSnap(snap raftpb.Snapshot) error {
	rs.lg.Infof("Persisting snapshot (term: %d, index: %d) to WAL and disk", snap.Metadata.Term, snap.Metadata.Index)

	// must save the snapshot index to the WAL before saving the
	// snapshot to maintain the invariant that we only Open the
	// wal at previously-saved snapshot indexes.
	// walsnap := walpb.Snapshot{
	// 	Index: snap.Metadata.Index,
	// 	Term:  snap.Metadata.Term,
	// }

	// if err := rs.wal.SaveSnapshot(walsnap); err != nil {
	// 	return errors.Errorf("failed to save snapshot to WAL: %s", err)
	// }

	// if err := rs.snap.SaveSnap(snap); err != nil {
	// 	return errors.Errorf("failed to save snapshot to disk: %s", err)
	// }

	rs.lg.Debugf("Releasing lock to wal files prior to %d", snap.Metadata.Index)
	if err := rs.wal.ReleaseLockTo(snap.Metadata.Index); err != nil {
		return err
	}

	return nil
}

// TakeSnapshot takes a snapshot at index i from MemoryStorage, and persists it to wal and disk.
//FLY2-66 Proposed change
//changed the Takesnapshot function
// Modified it to include directory creation,data marshal and writing of the marshaled data
func (rs *MirBFTStorage) TakeSnapshot(i uint64, status status.StateMachine) error {
	if err := os.MkdirAll(rs.snapDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to mkdir '%s' for snapshot: %s", rs.snapDir, err)
	}
	TimeStamp := time.Now().Unix()
	fname := fmt.Sprintf("%016x-%016x%s", TimeStamp, i, ".snap")

	spath := filepath.Join(rs.snapDir, fname)
	//used json to marshal data
	d, err := json.Marshal(status)
	if err != nil {
		return errors.Errorf("Cannot marshal Static Snapshots")
	}
	f, err := os.OpenFile(spath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	n, err := f.Write(d)
	if err == nil && n < len(d) {
		err = io.ErrShortWrite
		return err
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}

// gc collects etcd/raft garbage files, namely wal and snapshot files
func (rs *MirBFTStorage) gc() {
	if len(rs.snapshotIndex) < MaxSnapshotFiles {
		rs.lg.Debugf("Snapshots on disk (%d) < limit (%d), no need to purge wal/snapshot",
			len(rs.snapshotIndex), MaxSnapshotFiles)
		return
	}

	rs.snapshotIndex = rs.snapshotIndex[len(rs.snapshotIndex)-MaxSnapshotFiles:]

	rs.purgeWAL()
	rs.purgeSnap()
}

func (rs *MirBFTStorage) purgeWAL() {
	retain := rs.snapshotIndex[0]

	var files []string
	err := filepath.Walk(rs.walDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".wal") {
			return nil
		}

		var seq, index uint64
		_, f := filepath.Split(path)
		fmt.Sscanf(f, "%016x-%016x.wal", &seq, &index)

		// Only purge WAL with index lower than oldest snapshot.
		// filepath.SkipDir seizes Walk without returning error.
		if index >= retain {
			return filepath.SkipDir
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		rs.lg.Errorf("Failed to read WAL directory %s: %s", rs.walDir, err)
	}

	if len(files) <= 1 {
		// we need to keep one wal segment with index smaller than snapshot.
		// see comment on wal.ReleaseLockTo for the more details.
		return
	}

	rs.purge(files[:len(files)-1])
}

func (rs *MirBFTStorage) purgeSnap() {
	var files []string
	err := filepath.Walk(rs.snapDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".snap") {
			files = append(files, path)
		} else if strings.HasSuffix(path, ".broken") {
			rs.lg.Warnf("Found broken snapshot file %s, it can be removed manually", path)
		}

		return nil
	})
	if err != nil {
		rs.lg.Errorf("Failed to read Snapshot directory %s: %s", rs.snapDir, err)
		return
	}

	l := len(files)
	if l <= MaxSnapshotFiles {
		return
	}

	rs.purge(files[:l-MaxSnapshotFiles]) // retain last MaxSnapshotFiles snapshot files
}

func (rs *MirBFTStorage) purge(files []string) {
	for _, file := range files {
		l, err := fileutil.TryLockFile(file, os.O_WRONLY, fileutil.PrivateFileMode)
		if err != nil {
			rs.lg.Debugf("Failed to lock %s, abort purging", file)
			break
		}

		if err = os.Remove(file); err != nil {
			rs.lg.Errorf("Failed to remove %s: %s", file, err)
		} else {
			rs.lg.Debugf("Purged file %s", file)
		}

		if err = l.Close(); err != nil {
			rs.lg.Errorf("Failed to close file lock %s: %s", l.Name(), err)
		}
	}
}

// ApplySnapshot applies snapshot to local memory storage
func (rs *MirBFTStorage) ApplySnapshot(snap raftpb.Snapshot) {
	if err := rs.ram.ApplySnapshot(snap); err != nil {
		if err == raft.ErrSnapOutOfDate {
			rs.lg.Warnf("Attempted to apply out-of-date snapshot at Term %d and Index %d",
				snap.Metadata.Term, snap.Metadata.Index)
		} else {
			rs.lg.Fatalf("Unexpected programming error: %s", err)
		}
	}
}

// Close closes storage
func (rs *MirBFTStorage) Close() error {
	if err := rs.wal.Close(); err != nil {
		return err
	}

	return nil
}
