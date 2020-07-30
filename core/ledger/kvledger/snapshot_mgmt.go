/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"math"
	"os"
	"sync"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/pkg/errors"
)

const (
	commitStart         = "commitStart"
	commitDone          = "commitDone"
	requestAdd          = "requestAdd"
	requestCancel       = "requestCancel"
	snapshotDone        = "snapshotDone"
	snapshotMgrShutdown = "snapshotMgrShutdown"
)

var (
	snapshotRequestKeyPrefix        = []byte("s")
	defaultSmallestHeight    uint64 = math.MaxUint64
)

type snapshotMgr struct {
	snapshotRequestBookkeeper *snapshotRequestBookkeeper
	events                    chan *event
	commitProceed             chan struct{}
	requestResponses          chan *requestResponse
	stopped                   bool
	shutdownLock              sync.Mutex
}

type event struct {
	typ    string
	height uint64
}
type requestResponse struct {
	err error
}

// SubmitSnapshotRequest submits a snapshot request for the specified height.
// The request will be stored in the ledger until the ledger's block height is equal to
// the specified height and the snapshot generation is completed.
// When height is 0, it will generate a snapshot at the current block height.
// It returns an error if the specified height is smaller than the ledger's block height
// or the requested height already exists.
func (l *kvLedger) SubmitSnapshotRequest(height uint64) error {
	l.snapshotMgr.events <- &event{requestAdd, height}
	response := <-l.snapshotMgr.requestResponses
	return response.err
}

// CancelSnapshotRequest cancels the previously submitted request.
// It returns an error if such a request does not exist or is under processing.
// It locks snapshotRequestLock to synchronize among concurrent SubmitSnapshotRequest
// and CancelSnapshotRequest calls.
// It also locks snapshotGenerationRWLock to synchronize with commit before getting
// blockchain info so that it will not cancel a under-processing request.
func (l *kvLedger) CancelSnapshotRequest(height uint64) error {
	l.snapshotMgr.events <- &event{requestCancel, height}
	response := <-l.snapshotMgr.requestResponses
	return response.err
}

// PendingSnapshotRequests returns a list of heights for the pending (or under processing) snapshot requests.
func (l *kvLedger) PendingSnapshotRequests() ([]uint64, error) {
	return l.snapshotMgr.snapshotRequestBookkeeper.list()
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

// processSnapshotMgmtEvents handles each event in the events channel and performs synchronization acorss
// block commits, snapshot generation, and snapshot request submission/cancellation.
// It should be started in a separate goroutine when the ledger is created/opened.
// There are 3 unbuffered channels and 5 events working together to process events one by one
// and perform synchronization.
// - events: a channel receiving all the events
// - commitProceed: a channel indicating if commit can be proceeded. Commit is blocked if a snapshot generation is in progress.
// - requestResponses: a channel returning the response for snapshot request submission/cancellation.
// The 5 events are:
// - commitStart: sent before committing a block
// - commitDone: sent after a block is committed
// - snapshotDone: sent when a snapshot generation is finished, regardless of success or failure
// - requestAdd: sent when a snapshot request is submitted
// - requestCancel: sent when a snapshot request is cancelled
// In addition, the snapshotMgrShutdown event is sent when snapshotMgr shutdown is called. Upon receiving this event,
// this function will return immediately.
func (l *kvLedger) processSnapshotMgmtEvents(lastCommittedHeight uint64) {
	// toStartCommitHeight is updated when a commitStart event is received
	toStartCommitHeight := lastCommittedHeight
	// committedHeight is updated when a commitDone event is received
	committedHeight := lastCommittedHeight
	// snapshotInProgress is set to true before a generateSnapshot is called when processing commitDone or requestAdd event
	// and set to false when snapshotDone event is received
	snapshotInProgress := false

	events := l.snapshotMgr.events
	commitProceed := l.snapshotMgr.commitProceed
	requestResponses := l.snapshotMgr.requestResponses

	for {
		e := <-events
		logger.Debugw("event received", "type", e.typ, "height", e.height, "snapshotInProgress=", snapshotInProgress)
		switch e.typ {
		case commitStart:
			// toStartCommitHeight is updated to be the new block height before the block is committed;
			// therefore, before commitDone event, toStartCommitHeight is equal to committedHeight + 1.
			// In other words, if toStartCommitHeight is not equal to committedHeight, it means that a commit is in progress.
			toStartCommitHeight = e.height
			if snapshotInProgress {
				logger.Infow("commit waiting on snapshot to be generated", "snapshotHeight", committedHeight)
				continue
			}
			// no in-progress snapshot, let commit proceed
			commitProceed <- struct{}{}

		case commitDone:
			committedHeight = e.height
			if committedHeight != l.snapshotMgr.snapshotRequestBookkeeper.smallestRequestHeight {
				continue
			}
			snapshotInProgress = true
			requestedHeight := committedHeight
			go func() {
				if err := l.generateSnapshot(); err != nil {
					logger.Errorw("Failed to generate snapshot", "height", requestedHeight, "error", err)
				}
				events <- &event{snapshotDone, requestedHeight}
			}()

		case snapshotDone:
			requestedHeight := e.height
			logger.Debugw("snapshot is generated", "height", requestedHeight, "toStartCommitHeight", toStartCommitHeight, "committedHeight", committedHeight)
			if err := l.snapshotMgr.snapshotRequestBookkeeper.delete(requestedHeight); err != nil {
				logger.Errorw("Failed to delete snapshot request, the pending snapshot requests (if any) may not be processed", "height", requestedHeight, "error", err)
			}
			if toStartCommitHeight != committedHeight {
				// there is an in-progress commit, write to commitProceed channel to unblock commit
				commitProceed <- struct{}{}
			}
			snapshotInProgress = false

		case requestAdd:
			requestedHeight := e.height
			if requestedHeight == 0 {
				requestedHeight = toStartCommitHeight
			}
			if requestedHeight < toStartCommitHeight {
				requestResponses <- &requestResponse{errors.Errorf("requested snapshot height %d cannot be less than the current block height %d", requestedHeight, toStartCommitHeight)}
				continue
			}
			if requestedHeight == committedHeight {
				// this is a corner case where no block has been committed since last snapshot was generated.
				exists, err := l.snapshotExists(requestedHeight)
				if err != nil {
					requestResponses <- &requestResponse{err}
					continue
				}
				if exists {
					requestResponses <- &requestResponse{errors.Errorf("snapshot already generated for block height %d", requestedHeight)}
					continue
				}
			}
			if err := l.snapshotMgr.snapshotRequestBookkeeper.add(requestedHeight); err != nil {
				requestResponses <- &requestResponse{err}
				continue
			}
			if toStartCommitHeight == committedHeight && requestedHeight == committedHeight {
				snapshotInProgress = true
				go func() {
					if err := l.generateSnapshot(); err != nil {
						logger.Errorw("Failed to generate snapshot", "height", requestedHeight, "error", err)
					}
					events <- &event{snapshotDone, requestedHeight}
				}()
			}
			requestResponses <- &requestResponse{}

		case requestCancel:
			requestedHeight := e.height
			if snapshotInProgress && requestedHeight == committedHeight {
				requestResponses <- &requestResponse{errors.Errorf("cannot cancel the snapshot request because it is under processing")}
				continue
			}
			requestResponses <- &requestResponse{l.snapshotMgr.snapshotRequestBookkeeper.delete(requestedHeight)}

		case snapshotMgrShutdown:
			return
		}
	}
}

func (l *kvLedger) recoverSnapshot(height uint64) error {
	if height != l.snapshotMgr.snapshotRequestBookkeeper.smallestRequestHeight {
		return nil
	}
	exists, err := l.snapshotExists(height)
	if err != nil {
		return err
	}
	if !exists {
		// send commitDone event to generate the missing snapshot
		l.snapshotMgr.events <- &event{typ: commitDone, height: height}
	}
	return nil
}

// snapshotExists checks if the snapshot for the given height exists
func (l *kvLedger) snapshotExists(height uint64) (bool, error) {
	snapshotDir := SnapshotDirForLedgerHeight(l.config.SnapshotsConfig.RootDir, l.ledgerID, height)
	stat, err := os.Stat(snapshotDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return stat != nil, nil
}

// shutdown sends a snapshotMgrShutdown event and close all the channels, which is called
// when the ledger is closed. For simplicity, this function does not consider in-progress commit
// or snapshot generation. The caller should make sure there is no in-progress commit or
// snapshot generation. Otherwise, it may cause panic because the channels have been closed.
func (m *snapshotMgr) shutdown() {
	m.shutdownLock.Lock()
	defer m.shutdownLock.Unlock()

	if m.stopped {
		return
	}

	m.stopped = true
	m.events <- &event{typ: snapshotMgrShutdown}
	close(m.events)
	close(m.commitProceed)
	close(m.requestResponses)
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
