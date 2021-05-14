/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"bytes"
	"container/list"
	"fmt"
	"sort"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/status"
)

type checkpointState int

const (
	cpsIdle checkpointState = iota
	cpsGarbageCollectable
	cpsPendingReconfig // TODO, implement
	cpsStateTransfer   // TODO, implement
)

type checkpointTracker struct {
	state checkpointState

	highestCheckpoints map[nodeID]uint64
	checkpointMap      map[uint64]*checkpoint
	activeCheckpoints  *list.List
	msgBuffers         map[nodeID]*msgBuffer
	networkConfig      *msgs.NetworkState_Config
	persisted          *persisted

	nodeBuffers *nodeBuffers
	myConfig    *state.EventInitialParameters
	logger      Logger
}

func newCheckpointTracker(seqNo uint64, networkState *msgs.NetworkState, persisted *persisted, nodeBuffers *nodeBuffers, myConfig *state.EventInitialParameters, logger Logger) *checkpointTracker {
	ct := &checkpointTracker{
		myConfig:    myConfig,
		state:       cpsIdle,
		persisted:   persisted,
		nodeBuffers: nodeBuffers,
		logger:      logger,
	}

	return ct
}

func (ct *checkpointTracker) reinitialize() {
	oldCheckpointMap := ct.checkpointMap
	oldMsgBuffers := ct.msgBuffers

	ct.highestCheckpoints = map[nodeID]uint64{}
	ct.checkpointMap = map[uint64]*checkpoint{}
	ct.activeCheckpoints = list.New()
	ct.msgBuffers = map[nodeID]*msgBuffer{}
	ct.networkConfig = nil

	ct.persisted.iterate(logIterator{
		onCEntry: func(cEntry *msgs.CEntry) {
			if ct.networkConfig == nil {
				// Initialize this once, it will not change until the next
				// time we reinitialize.
				ct.networkConfig = cEntry.NetworkState.Config
			}
			cp := ct.checkpoint(cEntry.SeqNo)
			cp.applyCheckpointMsg(nodeID(ct.myConfig.Id), cEntry.CheckpointValue)
			ct.activeCheckpoints.PushBack(cp)
		},
	})

	ct.activeCheckpoints.Front().Value.(*checkpoint).stable = true

	validNodes := map[nodeID]struct{}{}
	for _, id := range ct.networkConfig.Nodes {
		if buffer, ok := oldMsgBuffers[nodeID(id)]; ok {
			ct.msgBuffers[nodeID(id)] = buffer
		} else {
			ct.msgBuffers[nodeID(id)] = newMsgBuffer("checkpoints", ct.nodeBuffers.nodeBuffer(nodeID(id)))
		}
		validNodes[nodeID(id)] = struct{}{}
	}

	// Lots of non-determinism in this iteration... but it should
	// all be commutative.
	for seqNo, cp := range oldCheckpointMap {
		if seqNo < ct.lowWatermark() {
			continue
		}

		for value, agreements := range cp.values {
			for _, node := range agreements {
				if _, ok := validNodes[node]; !ok {
					continue
				}

				ct.applyCheckpointMsg(node, seqNo, []byte(value))
			}
		}
	}

	ct.garbageCollect()
}

func (ct *checkpointTracker) filter(_ nodeID, msg *msgs.Msg) applyable {
	cpMsg := msg.Type.(*msgs.Msg_Checkpoint).Checkpoint

	switch {
	case cpMsg.SeqNo < ct.activeCheckpoints.Front().Value.(*checkpoint).seqNo:
		return past
	case cpMsg.SeqNo > ct.highWatermark():
		return future
	default:
		return current
	}
}

func (ct *checkpointTracker) step(source nodeID, msg *msgs.Msg) {
	switch ct.filter(source, msg) {
	case past:
		return
	case future:
		ct.msgBuffers[source].store(msg)
		fallthrough
	case current:
		ct.applyMsg(source, msg)
	}
}

func (ct *checkpointTracker) applyMsg(source nodeID, msg *msgs.Msg) {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_Checkpoint:
		msg := innerMsg.Checkpoint
		ct.applyCheckpointMsg(source, msg.SeqNo, msg.Value)
	default:
		panic(fmt.Sprintf("unexpected bad checkpoint message type %T, this indicates a bug", msg.Type))
	}
}

func (ct *checkpointTracker) garbageCollect() uint64 {
	var highestStable *list.Element
	for el := ct.activeCheckpoints.Front(); el != nil; el = el.Next() {
		cp := el.Value.(*checkpoint)
		if !cp.stable {
			break
		}

		highestStable = el
	}

	for el := highestStable.Prev(); el != nil; el = highestStable.Prev() {
		delete(ct.checkpointMap, el.Value.(*checkpoint).seqNo)
		ct.activeCheckpoints.Remove(el)
	}

	for ct.activeCheckpoints.Len() < 3 {
		nextCpSeq := ct.highWatermark() + uint64(ct.networkConfig.CheckpointInterval)
		ct.activeCheckpoints.PushBack(ct.checkpoint(nextCpSeq))
	}

	for _, id := range ct.networkConfig.Nodes {
		ct.msgBuffers[nodeID(id)].iterate(ct.filter, ct.applyMsg)
	}

	ct.state = cpsIdle
	return highestStable.Value.(*checkpoint).seqNo
}

func (ct *checkpointTracker) checkpoint(seqNo uint64) *checkpoint {
	cp, ok := ct.checkpointMap[seqNo]
	if !ok {
		cp = &checkpoint{
			seqNo:         seqNo,
			networkConfig: ct.networkConfig,
			myConfig:      ct.myConfig,
			logger:        ct.logger,
		}
		ct.checkpointMap[seqNo] = cp
	}

	return cp
}

func (ct *checkpointTracker) highWatermark() uint64 {
	return ct.activeCheckpoints.Back().Value.(*checkpoint).seqNo
}

func (ct *checkpointTracker) lowWatermark() uint64 {
	return ct.activeCheckpoints.Front().Value.(*checkpoint).seqNo
}

func (ct *checkpointTracker) applyCheckpointMsg(source nodeID, seqNo uint64, value []byte) {
	aboveHighWatermark := seqNo > ct.highWatermark()
	if aboveHighWatermark {
		highest, ok := ct.highestCheckpoints[source]
		if ok && highest <= seqNo {
			return
		}

		ct.highestCheckpoints[source] = seqNo
	}

	cp := ct.checkpoint(seqNo)
	cp.applyCheckpointMsg(source, value)

	if cp.stable && seqNo > ct.lowWatermark() && !aboveHighWatermark {
		ct.state = cpsGarbageCollectable
		return
	}

	if !aboveHighWatermark {
		return
	}

	// We just added a new entry to our highest checkpoints map,
	// so we need to garbage collect any above window checkpoint
	// references that no node claims is the most current anymore.

	referencedCPs := map[uint64]struct{}{}

	for el := ct.activeCheckpoints.Front(); el != nil; el = el.Next() {
		referencedCPs[el.Value.(*checkpoint).seqNo] = struct{}{}
	}

	for _, seqNo := range ct.highestCheckpoints {
		referencedCPs[seqNo] = struct{}{}
	}

	for seqNo := range ct.checkpointMap {
		if _, ok := referencedCPs[seqNo]; !ok {
			delete(ct.checkpointMap, seqNo)
		}
	}
}

func (ct *checkpointTracker) status() []*status.Checkpoint {
	result := make([]*status.Checkpoint, len(ct.checkpointMap))
	i := 0
	for _, cp := range ct.checkpointMap {
		result[i] = cp.status()
		i++
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SeqNo < result[j].SeqNo
	})

	return result
}

type checkpoint struct {
	seqNo         uint64
	myConfig      *state.EventInitialParameters
	networkConfig *msgs.NetworkState_Config
	logger        Logger

	values         map[string][]nodeID
	committedValue []byte
	myValue        []byte
	stable         bool
}

func (cw *checkpoint) applyCheckpointMsg(source nodeID, value []byte) {
	if cw.values == nil {
		cw.values = map[string][]nodeID{}
	}

	checkpointValueNodes := append(cw.values[string(value)], source)
	cw.values[string(value)] = checkpointValueNodes

	agreements := len(checkpointValueNodes)

	if agreements == someCorrectQuorum(cw.networkConfig) {
		cw.committedValue = value
	}

	if source == nodeID(cw.myConfig.Id) {
		cw.myValue = value
	}

	// If I have completed this checkpoint, along with a quorum of the network, and I've not already run this path
	if cw.myValue != nil && cw.committedValue != nil && !cw.stable {
		if !bytes.Equal(value, cw.committedValue) {
			// TODO optionally handle this more gracefully, with state transfer (though this
			// indicates a violation of the byzantine assumptions)
			panic("my checkpoint disagrees with the committed network view of this checkpoint")
		}

		// This checkpoint has enough agreements, including my own, it may now be garbage collectable
		// Note, this must be >= (not ==) because my agreement could come after 2f+1 from the network.
		if agreements >= intersectionQuorum(cw.networkConfig) {
			if !cw.stable {
				cw.logger.Log(LevelDebug, "checkpoint is now stable", "seq_no", cw.seqNo)
			}
			cw.stable = true
		}
	}
}

func (cw *checkpoint) status() *status.Checkpoint {
	maxAgreements := 0
	for _, nodes := range cw.values {
		if len(nodes) > maxAgreements {
			maxAgreements = len(nodes)
		}
	}
	return &status.Checkpoint{
		SeqNo:         cw.seqNo,
		MaxAgreements: maxAgreements,
		NetQuorum:     cw.committedValue != nil,
		LocalDecision: cw.myValue != nil,
	}
}
