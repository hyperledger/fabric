/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"math"

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
// It returns an error if the specified height is smaller than the ledger's block height
// or the requested height already exists.
func (l *kvLedger) SubmitSnapshotRequest(height uint64) error {
	return errors.New("not implemented")
}

// CancelSnapshotRequest cancels the previously submitted request.
// It returns an error if such a request does not exist or is under processing.
// It locks snapshotRequestLock to synchronize among concurrent SubmitSnapshotRequest
// and CancelSnapshotRequest calls.
// It also locks snapshotGenerationRWLock to synchronize with commit before getting
// blockchain info so that it will not cancel a under-processing request.
func (l *kvLedger) CancelSnapshotRequest(height uint64) error {
	return errors.New("not implemented")
}

// PendingSnapshotRequests returns a list of heights for the pending (or under processing) snapshot requests.
func (l *kvLedger) PendingSnapshotRequests() ([]uint64, error) {
	return nil, errors.New("not implemented")

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

// snapshotRequestBookkeeper manages snapshot requests in a leveldb and maintains smallest height for pending snapshot requests
type snapshotRequestBookkeeper struct {
	dbHandle              *leveldbhelper.DBHandle
	smallestRequestHeight uint64
}

func newSnapshotRequestBookkeeper(dbHandle *leveldbhelper.DBHandle) (*snapshotRequestBookkeeper, error) {
	bk := &snapshotRequestBookkeeper{dbHandle: dbHandle}

	var err error
	if bk.smallestRequestHeight, err = bk.smallestRequest(); err != nil {
		return nil, err
	}

	return bk, nil
}

// add adds the given height to the bookkeeper db and returns an error if the height already exists
func (k *snapshotRequestBookkeeper) add(height uint64) error {
	key := encodeSnapshotRequestKey(height)

	exists, err := k.exist(height)
	if err != nil {
		return err
	}
	if exists {
		return errors.Errorf("duplicate snapshot request for height %d", height)
	}

	if err := k.dbHandle.Put(key, []byte{}, true); err != nil {
		return err
	}

	if height < k.smallestRequestHeight {
		k.smallestRequestHeight = height
	}

	return nil
}

// delete deletes the given height from the bookkeeper db and returns an error if the height does not exist
func (k *snapshotRequestBookkeeper) delete(height uint64) error {
	exists, err := k.exist(height)
	if err != nil {
		return err
	}
	if !exists {
		return errors.Errorf("no snapshot request exists for height %d", height)
	}

	if err = k.dbHandle.Delete(encodeSnapshotRequestKey(height), true); err != nil {
		return err
	}

	if k.smallestRequestHeight != height {
		return nil
	}

	if k.smallestRequestHeight, err = k.smallestRequest(); err != nil {
		return err
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

	for {
		hasMore := itr.Next()
		if err = itr.Error(); err != nil {
			return nil, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
		}
		if !hasMore {
			break
		}
		height, _, err := decodeSnapshotRequestKey(itr.Key())
		if err != nil {
			return nil, err
		}
		requestedHeights = append(requestedHeights, height)
	}

	return requestedHeights, nil
}

func (k *snapshotRequestBookkeeper) exist(height uint64) (bool, error) {
	val, err := k.dbHandle.Get(encodeSnapshotRequestKey(height))
	if err != nil {
		return false, err
	}
	exists := val != nil
	return exists, nil
}

func (k *snapshotRequestBookkeeper) smallestRequest() (uint64, error) {
	itr, err := k.dbHandle.GetIterator(nil, nil)
	if err != nil {
		return 0, err
	}
	defer itr.Release()

	hasMore := itr.Next()
	if err = itr.Error(); err != nil {
		return 0, errors.Wrapf(err, "internal leveldb error while iterating for snapshot requests")
	}
	if !hasMore {
		return defaultSmallestHeight, nil
	}
	smallestRequestHeight, _, err := decodeSnapshotRequestKey(itr.Key())
	if err != nil {
		return 0, err
	}
	return smallestRequestHeight, nil
}

func encodeSnapshotRequestKey(height uint64) []byte {
	return append(snapshotRequestKeyPrefix, util.EncodeOrderPreservingVarUint64(height)...)
}

func decodeSnapshotRequestKey(key []byte) (uint64, int, error) {
	return util.DecodeOrderPreservingVarUint64(key[len(snapshotRequestKeyPrefix):])
}
