/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"bytes"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
)

// commitState represents our state, as reflected within our log watermarks.
// The mir network state only changes at checkpoint boundaries, and it
// is not possible for two different sets of network configuration state
// to exist within the same state structure.  Once the network state config
// commits, we wait for a strong checkpoint, then perform an epoch change
// and restart the state machine with the new network state as starting state.
// When a checkpoint result returns, it becomes the new activeState, the upperHalf
// of the committed sequences becomes the lowerHalf.
type commitState struct {
	persisted         *persisted
	committingClients map[uint64]*committingClient
	logger            Logger

	lowWatermark      uint64
	lastAppliedCommit uint64
	highestCommit     uint64 // Highest in order commit sequence number. All SNs up to highestCommit are committed.
	stopAtSeqNo       uint64
	activeState       *msgs.NetworkState
	lowerHalfCommits  []*msgs.QEntry
	upperHalfCommits  []*msgs.QEntry
	checkpointPending bool
	transferring      bool
}

func newCommitState(persisted *persisted, logger Logger) *commitState {
	cs := &commitState{
		persisted: persisted,
		logger:    logger,
	}

	return cs
}

func (cs *commitState) reinitialize() *ActionList {
	var lastCEntry, secondToLastCEntry *msgs.CEntry
	var lastTEntry *msgs.TEntry

	cs.persisted.iterate(logIterator{
		onCEntry: func(cEntry *msgs.CEntry) {
			lastCEntry, secondToLastCEntry = cEntry, lastCEntry
		},
		onTEntry: func(tEntry *msgs.TEntry) {
			lastTEntry = tEntry
		},
	})

	if secondToLastCEntry == nil || len(secondToLastCEntry.NetworkState.PendingReconfigurations) == 0 {
		cs.activeState = lastCEntry.NetworkState
		cs.lowWatermark = lastCEntry.SeqNo
	} else {
		cs.activeState = secondToLastCEntry.NetworkState
		cs.lowWatermark = secondToLastCEntry.SeqNo
	}

	actions := &ActionList{}
	actions.StateApplied(cs.lowWatermark, cs.activeState)

	ci := uint64(cs.activeState.Config.CheckpointInterval)
	if len(cs.activeState.PendingReconfigurations) == 0 {
		cs.stopAtSeqNo = lastCEntry.SeqNo + 2*ci
	} else {
		cs.stopAtSeqNo = lastCEntry.SeqNo + ci
	}

	cs.lastAppliedCommit = lastCEntry.SeqNo
	cs.highestCommit = lastCEntry.SeqNo

	cs.lowerHalfCommits = make([]*msgs.QEntry, ci)
	cs.upperHalfCommits = make([]*msgs.QEntry, ci)

	cs.committingClients = map[uint64]*committingClient{}
	for _, clientState := range lastCEntry.NetworkState.Clients {
		cs.committingClients[clientState.Id] = newCommittingClient(lastCEntry.SeqNo, clientState)
	}

	if lastTEntry == nil || lastCEntry.SeqNo >= lastTEntry.SeqNo {
		cs.logger.Log(LevelDebug, "reinitialized commit-state", "low_watermark", cs.lowWatermark, "stop_at_seq_no", cs.stopAtSeqNo, "len(pending_reconfigurations)", len(cs.activeState.PendingReconfigurations), "last_checkpoint_seq_no", lastCEntry.SeqNo)
		cs.transferring = false
		return (&ActionList{}).StateApplied(cs.lowWatermark, cs.activeState)
	}

	cs.logger.Log(LevelInfo, "reinitialized commit-state detected crash during state transfer", "target_seq_no", lastTEntry.SeqNo, "target_value", lastTEntry.Value)

	// We crashed during a state transfer
	cs.transferring = true
	return actions.StateTransfer(lastTEntry.SeqNo, lastTEntry.Value)
}

func (cs *commitState) transferTo(seqNo uint64, value []byte) *ActionList {
	cs.logger.Log(LevelDebug, "initiating state transfer", "target_seq_no", seqNo, "target_value", value)
	assertEqual(cs.transferring, false, "multiple state transfers are not supported concurrently")
	cs.transferring = true
	return cs.persisted.addTEntry(&msgs.TEntry{
		SeqNo: seqNo,
		Value: value,
	}).StateTransfer(seqNo, value)
}

func (cs *commitState) applyCheckpointResult(epochConfig *msgs.EpochConfig, result *state.EventCheckpointResult) *ActionList {
	cs.logger.Log(LevelDebug, "applying checkpoint result", "seq_no", result.SeqNo, "value", result.Value)
	ci := uint64(cs.activeState.Config.CheckpointInterval)

	if cs.transferring {
		return &ActionList{}
	}

	if result.SeqNo != cs.lowWatermark+ci {
		panic("dev sanity test -- this panic is helpful for dev, but needs to be removed as we could get stale checkpoint results")
	}

	if len(result.NetworkState.PendingReconfigurations) == 0 {
		cs.stopAtSeqNo = result.SeqNo + 2*ci
	} else {
		cs.logger.Log(LevelDebug, "checkpoint result has pending reconfigurations, not extending stop", "stop_at_seq_no", cs.stopAtSeqNo)
	}

	cs.activeState = result.NetworkState
	cs.lowerHalfCommits = cs.upperHalfCommits
	cs.upperHalfCommits = make([]*msgs.QEntry, ci)
	cs.lowWatermark = result.SeqNo
	cs.checkpointPending = false

	return cs.persisted.addCEntry(&msgs.CEntry{
		SeqNo:           result.SeqNo,
		CheckpointValue: result.Value,
		NetworkState:    result.NetworkState,
	}).Send(
		cs.activeState.Config.Nodes,
		&msgs.Msg{
			Type: &msgs.Msg_Checkpoint{
				Checkpoint: &msgs.Checkpoint{
					SeqNo: result.SeqNo,
					Value: result.Value,
				},
			},
		},
	).StateApplied(result.SeqNo, result.NetworkState)
}

func (cs *commitState) commit(qEntry *msgs.QEntry) {
	assertEqual(cs.transferring, false, "we should never commit during state transfer")
	assertGreaterThanOrEqual(cs.stopAtSeqNo, qEntry.SeqNo, "commit sequence exceeds stop sequence")

	if qEntry.SeqNo <= cs.lowWatermark {
		// During an epoch change, we may be asked to
		// commit seqnos which we have already committed
		// (and cannot check), so ignore.
		return
	}

	if cs.highestCommit < qEntry.SeqNo {
		assertEqual(cs.highestCommit+1, qEntry.SeqNo, "next commit should always be exactly one greater than the highest")
		cs.highestCommit = qEntry.SeqNo
	}

	ci := uint64(cs.activeState.Config.CheckpointInterval)
	upper := qEntry.SeqNo-cs.lowWatermark > ci
	offset := int((qEntry.SeqNo - (cs.lowWatermark + 1)) % ci)
	var commits []*msgs.QEntry
	if upper {
		commits = cs.upperHalfCommits
	} else {
		commits = cs.lowerHalfCommits
	}

	if commits[offset] != nil {
		assertTruef(bytes.Equal(commits[offset].Digest, qEntry.Digest), "previously committed %x but now have %x for seq_no=%d", commits[offset].Digest, qEntry.Digest, qEntry.SeqNo)
	} else {
		commits[offset] = qEntry
	}
}

func nextNetworkConfig(startingState *msgs.NetworkState, committingClients map[uint64]*committingClient) (*msgs.NetworkState_Config, []*msgs.NetworkState_Client) {
	nextConfig := startingState.Config

	nextClients := make([]*msgs.NetworkState_Client, len(startingState.Clients))
	for i, oldClientState := range startingState.Clients {
		cc, ok := committingClients[oldClientState.Id]
		assertTrue(ok, "must have a committing client instance all client states")
		nextClients[i] = cc.createCheckpointState()
	}

	for _, reconfig := range startingState.PendingReconfigurations {
		switch rc := reconfig.Type.(type) {
		case *msgs.Reconfiguration_NewClient_:
			nextClients = append(nextClients, &msgs.NetworkState_Client{
				Id:    rc.NewClient.Id,
				Width: rc.NewClient.Width,
			})
		case *msgs.Reconfiguration_RemoveClient:
			found := false
			for i, clientConfig := range nextClients {
				if clientConfig.Id != rc.RemoveClient {
					continue
				}

				found = true
				nextClients = append(nextClients[:i], nextClients[i+1:]...)
				break
			}

			// TODO, heavy handed, back off to a warning
			assertTruef(found, "asked to remove client %d which doesn't exist", rc.RemoveClient)
		case *msgs.Reconfiguration_NewConfig:
			nextConfig = rc.NewConfig
		}
	}

	return nextConfig, nextClients
}

// drain returns all available Commits (including checkpoint requests)
func (cs *commitState) drain() *ActionList {
	ci := uint64(cs.activeState.Config.CheckpointInterval)

	actions := &ActionList{}
	for cs.lastAppliedCommit < cs.lowWatermark+2*ci {
		if cs.lastAppliedCommit == cs.lowWatermark+ci && !cs.checkpointPending {
			networkConfig, clientConfigs := nextNetworkConfig(cs.activeState, cs.committingClients)

			actions.Checkpoint(cs.lastAppliedCommit, networkConfig, clientConfigs)

			cs.checkpointPending = true
			cs.logger.Log(LevelDebug, "all previous sequences has committed, requesting checkpoint", "seq_no", cs.lastAppliedCommit)

		}

		nextCommit := cs.lastAppliedCommit + 1
		upper := nextCommit-cs.lowWatermark > ci
		offset := int((nextCommit - (cs.lowWatermark + 1)) % ci)
		var commits []*msgs.QEntry
		if upper {
			commits = cs.upperHalfCommits
		} else {
			commits = cs.lowerHalfCommits
		}
		commit := commits[offset]
		if commit == nil {
			break
		}

		assertEqual(commit.SeqNo, nextCommit, "attempted out of order commit")

		actions.Commit(commit)

		for _, req := range commit.Requests {
			cs.committingClients[req.ClientId].markCommitted(commit.SeqNo, req.ReqNo)
		}

		cs.lastAppliedCommit = nextCommit
	}

	return actions
}

type committingClient struct {
	lastState                    *msgs.NetworkState_Client
	committedSinceLastCheckpoint []*uint64
}

func newCommittingClient(seqNo uint64, clientState *msgs.NetworkState_Client) *committingClient {
	committedSinceLastCheckpoint := make([]*uint64, clientState.Width)
	mask := bitmask(clientState.CommittedMask)
	for i := 0; i < mask.bits(); i++ {
		if !mask.isBitSet(i) {
			continue
		}
		committedSinceLastCheckpoint[i] = &seqNo
	}

	return &committingClient{
		lastState:                    clientState,
		committedSinceLastCheckpoint: committedSinceLastCheckpoint,
	}

}

func (cc *committingClient) markCommitted(seqNo, reqNo uint64) {
	if reqNo < cc.lastState.LowWatermark {
		return
	}
	offset := reqNo - cc.lastState.LowWatermark
	cc.committedSinceLastCheckpoint[offset] = &seqNo
}

func (cc *committingClient) createCheckpointState() (newState *msgs.NetworkState_Client) {
	defer func() {
		cc.lastState = newState
	}()

	var firstUncommitted, lastCommitted *uint64

	for i, seqNoPtr := range cc.committedSinceLastCheckpoint {
		reqNo := cc.lastState.LowWatermark + uint64(i)
		if seqNoPtr != nil {
			lastCommitted = &reqNo
			continue
		}
		if firstUncommitted == nil {
			firstUncommitted = &reqNo
		}
	}

	if lastCommitted == nil {
		return &msgs.NetworkState_Client{
			Id:                          cc.lastState.Id,
			Width:                       cc.lastState.Width,
			WidthConsumedLastCheckpoint: 0,
			LowWatermark:                cc.lastState.LowWatermark,
		}
	}

	if firstUncommitted == nil {
		highWatermark := cc.lastState.LowWatermark + uint64(cc.lastState.Width) - uint64(cc.lastState.WidthConsumedLastCheckpoint) - 1
		assertEqual(*lastCommitted, highWatermark, "if no client reqs are uncommitted, then all though the high watermark should be committed")

		cc.committedSinceLastCheckpoint = []*uint64{}
		return &msgs.NetworkState_Client{
			Id:                          cc.lastState.Id,
			Width:                       cc.lastState.Width,
			WidthConsumedLastCheckpoint: cc.lastState.Width,
			LowWatermark:                *lastCommitted + 1,
		}
	}

	widthConsumed := int(*firstUncommitted - cc.lastState.LowWatermark)
	cc.committedSinceLastCheckpoint = cc.committedSinceLastCheckpoint[widthConsumed:]
	cc.committedSinceLastCheckpoint = append(cc.committedSinceLastCheckpoint, make([]*uint64, int(cc.lastState.Width)-widthConsumed)...)

	var mask bitmask
	if *lastCommitted != *firstUncommitted {
		mask = bitmask(make([]byte, int(*lastCommitted-*firstUncommitted)/8+1))
		for i := 0; i <= int(*lastCommitted-*firstUncommitted); i++ {
			if cc.committedSinceLastCheckpoint[i] == nil {
				continue
			}

			assertNotEqualf(i, 0, "the first uncommitted cannot be marked committed: firstUncommitted=%d, lastCommitted=%d slice=%+v", *firstUncommitted, *lastCommitted, cc.committedSinceLastCheckpoint)

			mask.setBit(i)
		}
	}

	return &msgs.NetworkState_Client{
		Id:                          cc.lastState.Id,
		Width:                       cc.lastState.Width,
		LowWatermark:                *firstUncommitted,
		WidthConsumedLastCheckpoint: uint32(widthConsumed),
		CommittedMask:               mask,
	}
}
