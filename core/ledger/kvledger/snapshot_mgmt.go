/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"math"

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
