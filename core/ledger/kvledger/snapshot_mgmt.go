/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"math"
	"os"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/pkg/errors"
)

var (
	snapshotRequestKeyPrefix        = []byte("s")
	defaultSmallestHeight    uint64 = math.MaxUint64
)

// SubmitSnapshotRequest submits a snapshot request for the specified height.
// The request will be stored in the ledger until the ledger's block height is equal to
// the specified height and the snapshot generation is completed.
// When height is 0, it will generate a snapshot at the current block height.
// It returns an error if the specified height is smaller than the ledger's block height.
// It uses snapshotRequestLock to synchronize among concurrent SubmitSnapshotRequest
// and CancelSnapshotRequest calls and uses snapshotGenerationRWLock to synchronize with commit.
// It starts a go-routine to call generateSnapshot if needed and will keep the snapshotGenerationRWLock
// until generateSnapshot is finished.
func (l *kvLedger) SubmitSnapshotRequest(height uint64) error {
	l.snapshotRequestLock.Lock()
	defer l.snapshotRequestLock.Unlock()

	// before calling GetBlockchainInfo, locks snapshotGenerationRWLock to synchronize with commit.
	// It gets the block height and invokes generateSnapshot if the submitted height matches
	// the block height. The snapshotGenerationRWLock will be held and unlocked inside of generateSnapshot.
	keepLock := false
	l.snapshotGenerationRWLock.RLock()
	defer func() {
		if !keepLock {
			l.snapshotGenerationRWLock.RUnlock()
		}
	}()

	bcInfo, err := l.blockStore.GetBlockchainInfo()
	if err != nil {
		return err
	}

	if height == 0 {
		// use current block height in this case
		height = bcInfo.Height
	}

	if height < bcInfo.Height {
		return errors.Errorf("requested snapshot height %d cannot be less than the current block height %d", height, bcInfo.Height)
	}

	exists, err := l.snapshotRequestBookkeeper.exists(height)
	if err != nil {
		return err
	}
	if exists {
		return errors.Errorf("duplicate snapshot request for height %d", height)
	}

	if err = l.snapshotRequestBookkeeper.add(height); err != nil {
		return err
	}

	if height == bcInfo.Height {
		keepLock = true
		go func() {
			defer l.snapshotGenerationRWLock.RUnlock()
			if err := l.generateSnapshot(); err != nil {
				logger.Errorw("Failed to generate snapshot", "height", height, "error", err)
			}
			if err := l.deleteSnapshotRequest(height); err != nil {
				logger.Errorw("Failed to delete snapshot request", "height", height, "error", err)
			}
		}()
	}

	return nil
}

// CancelSnapshotRequest cancels the previously submitted request.
// It returns an error if such a request does not exist or is under processing.
// It locks snapshotRequestLock to synchronize among concurrent SubmitSnapshotRequest
// and CancelSnapshotRequest calls.
// It also locks snapshotGenerationRWLock to synchronize with commit before getting
// blockchain info so that it will not cancel a under-processing request.
func (l *kvLedger) CancelSnapshotRequest(height uint64) error {
	l.snapshotRequestLock.Lock()
	defer l.snapshotRequestLock.Unlock()

	l.snapshotGenerationRWLock.RLock()
	defer l.snapshotGenerationRWLock.RUnlock()

	exists, err := l.snapshotRequestBookkeeper.exists(height)
	if err != nil {
		return err
	}
	if !exists {
		return errors.Errorf("no pending snapshot request has height %d", height)
	}

	bcInfo, err := l.blockStore.GetBlockchainInfo()
	if err != nil {
		return err
	}
	if height == bcInfo.Height {
		return errors.Errorf("cannot cancel the snapshot request because it is under processing")
	}

	return l.snapshotRequestBookkeeper.delete(height)
}

// PendingSnapshotRequests returns a list of heights for the pending (or under processing) snapshot requests.
func (l *kvLedger) PendingSnapshotRequests() ([]uint64, error) {
	return l.snapshotRequestBookkeeper.list()
}

// ListSnapshots returns the information for available snapshots.
// It returns a list of strings representing the following JSON object:
// type snapshotSignableMetadata struct {
//    ChannelName        string            `json:"channel_name"`
//    ChannelHeight      uint64            `json:"channel_height"`
//    LastBlockHashInHex string            `json:"last_block_hash"`
//    FilesAndHashes     map[string]string `json:"snapshot_files_raw_hashes"`
// }
func (l *kvLedger) ListSnapshots() ([]string, error) {
	return nil, errors.Errorf("not implemented")
}

// DeleteSnapshot deletes the snapshot files except for the metadata file and
// returns an error if no such a snapshot exists
func (l *kvLedger) DeleteSnapshot(height uint64) error {
	return errors.Errorf("not implemented")
}

// recoverMissingSnapshot is called during peer startup. It generates a snapshot
// if the block height matches the smallest request height and such a snapshot does not exist.
// It starts a go-routine to call generateSnapshot if needed and will keep the snapshotGenerationRWLock
// until generateSnapshot is finished.
func (l *kvLedger) recoverMissingSnapshot() error {
	bcInfo, err := l.blockStore.GetBlockchainInfo()
	if err != nil {
		return err
	}

	if l.snapshotRequestBookkeeper.smallestRequestHeight == bcInfo.Height {
		exists, err := l.snapshotExists(bcInfo.Height)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		l.snapshotGenerationRWLock.RLock()
		go func() {
			defer l.snapshotGenerationRWLock.RUnlock()
			if err := l.generateSnapshot(); err != nil {
				logger.Errorw("Failed to generate snapshot", "height", bcInfo.Height, "error", err)
			}
			if err := l.deleteSnapshotRequest(bcInfo.Height); err != nil {
				logger.Errorw("Failed to delete snapshot request", "height", bcInfo.Height, "error", err)
			}
		}()
	}

	return nil
}

// snapshotExists checks if the snapshot for the given height exists
func (l *kvLedger) snapshotExists(height uint64) (bool, error) {
	snapshotDir := SnapshotDirForLedgerHeight(l.config.SnapshotsConfig.RootDir, l.ledgerID, height)
	stat, err := os.Stat(snapshotDir)
	if err != nil {
		return false, err
	}
	return stat != nil, nil
}

// deleteSnapshotRequest deletes the snapshot request for the given height
func (l *kvLedger) deleteSnapshotRequest(height uint64) error {
	l.snapshotRequestLock.Lock()
	defer l.snapshotRequestLock.Unlock()
	return l.snapshotRequestBookkeeper.delete(height)
}

// snapshotRequestBookkeeper stores snapshot requests in a leveldb and maintains smallest height for pending snapshot requests
type snapshotRequestBookkeeper struct {
	dbHandle              *leveldbhelper.DBHandle
	smallestRequestHeight uint64
}

func newSnapshotRequestBookkeeper(dbHandle *leveldbhelper.DBHandle) (*snapshotRequestBookkeeper, error) {
	// read db to get smallest request height
	itr, err := dbHandle.GetIterator(nil, nil)
	if err != nil {
		return nil, err
	}
	defer itr.Release()

	if err = itr.Error(); err != nil {
		return nil, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
	}

	smallestRequestHeight := defaultSmallestHeight
	if itr.Next() {
		if smallestRequestHeight, _, err = decodeSnapshotRequestKey(itr.Key()); err != nil {
			return nil, err
		}
	}

	return &snapshotRequestBookkeeper{
		dbHandle:              dbHandle,
		smallestRequestHeight: smallestRequestHeight,
	}, nil
}

func (k *snapshotRequestBookkeeper) exists(height uint64) (bool, error) {
	val, err := k.dbHandle.Get(encodeSnapshotRequestKey(height))
	if err != nil {
		return false, err
	}
	exists := val != nil
	return exists, nil
}

func (k *snapshotRequestBookkeeper) add(height uint64) error {
	key := encodeSnapshotRequestKey(height)
	if err := k.dbHandle.Put(key, []byte{}, true); err != nil {
		return err
	}

	if height < k.smallestRequestHeight {
		k.smallestRequestHeight = height
	}

	return nil
}

func (k *snapshotRequestBookkeeper) delete(height uint64) error {
	// first, get next smallest request height in order to prevent any inconsistence in case that
	// an error happens when getting smallest request height from db after deletion
	var err error
	nextSmallestRequestHeight := defaultSmallestHeight

	if k.smallestRequestHeight == height {
		nextSmallestRequestHeight, err = k.nextSmallestRequestHeight()
		if err != nil {
			return err
		}
	}

	if err = k.dbHandle.Delete(encodeSnapshotRequestKey(height), true); err != nil {
		return err
	}

	if k.smallestRequestHeight == height {
		k.smallestRequestHeight = nextSmallestRequestHeight
	}

	return nil
}

func (k *snapshotRequestBookkeeper) list() ([]uint64, error) {
	requestedHeights := []uint64{}
	itr, err := k.dbHandle.GetIterator(nil, nil)
	if err != nil {
		return nil, err
	}
	defer itr.Release()

	if err = itr.Error(); err != nil {
		return nil, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
	}

	for itr.Next() {
		height, _, err := decodeSnapshotRequestKey(itr.Key())
		if err != nil {
			return nil, err
		}
		requestedHeights = append(requestedHeights, height)
		if err = itr.Error(); err != nil {
			return nil, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
		}
	}

	return requestedHeights, nil
}

// nextSmallestRequestHeight returns the next smallest request height
func (k *snapshotRequestBookkeeper) nextSmallestRequestHeight() (uint64, error) {
	itr, err := k.dbHandle.GetIterator(nil, nil)
	if err != nil {
		return 0, err
	}
	defer itr.Release()

	// iterate to the second item (i.e., next smallest height)
	for i := 0; i < 2; i++ {
		if err = itr.Error(); err != nil {
			return 0, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
		}
		if !itr.Next() {
			return defaultSmallestHeight, nil
		}
	}

	nextSmallestHeight, _, err := decodeSnapshotRequestKey(itr.Key())
	if err != nil {
		return 0, err
	}
	return nextSmallestHeight, nil
}

func encodeSnapshotRequestKey(height uint64) []byte {
	return append(snapshotRequestKeyPrefix, util.EncodeOrderPreservingVarUint64(height)...)
}

func decodeSnapshotRequestKey(key []byte) (uint64, int, error) {
	return util.DecodeOrderPreservingVarUint64(key[len(snapshotRequestKeyPrefix):])
}
