/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"fmt"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/status"
)

type preprepareBuffer struct {
	nextSeqNo uint64
	buffer    *msgBuffer
}

type activeEpoch struct {
	epochConfig   *msgs.EpochConfig
	networkConfig *msgs.NetworkState_Config
	myConfig      *state.EventInitialParameters
	logger        Logger

	outstandingReqs *allOutstandingReqs
	proposer        *proposer
	persisted       *persisted
	commitState     *commitState

	buckets   map[bucketID]nodeID
	sequences [][]*sequence

	preprepareBuffers []*preprepareBuffer // indexed by bucket
	otherBuffers      map[nodeID]*msgBuffer
	lowestUncommitted uint64   // seqNo
	lowestUnallocated []uint64 // seqNo indexed by bucket

	lastCommittedAtTick uint64
	ticksSinceProgress  uint32
}

func newActiveEpoch(epochConfig *msgs.EpochConfig, persisted *persisted, nodeBuffers *nodeBuffers, commitState *commitState, clientTracker *clientTracker, myConfig *state.EventInitialParameters, logger Logger) *activeEpoch {
	networkConfig := commitState.activeState.Config
	startingSeqNo := commitState.highestCommit

	logger.Log(LevelInfo, "starting new active epoch", "epoch_no", epochConfig.Number, "seq_no", startingSeqNo)

	outstandingReqs := newOutstandingReqs(clientTracker, commitState.activeState, logger)

	buckets := map[bucketID]nodeID{}

	leaders := map[uint64]struct{}{}
	for _, leader := range epochConfig.Leaders {
		leaders[leader] = struct{}{}
	}

	overflowIndex := 0 // TODO, this should probably start after the last assigned node
	for i := 0; i < int(networkConfig.NumberOfBuckets); i++ {
		bucketID := bucketID(i)
		leader := networkConfig.Nodes[(uint64(i)+epochConfig.Number)%uint64(len(networkConfig.Nodes))]
		if _, ok := leaders[leader]; !ok {
			buckets[bucketID] = nodeID(epochConfig.Leaders[overflowIndex%len(epochConfig.Leaders)])
			overflowIndex++
		} else {
			buckets[bucketID] = nodeID(leader)
		}
	}

	lowestUnallocated := make([]uint64, len(buckets))
	for i := range lowestUnallocated {
		firstSeqNo := startingSeqNo + uint64(i+1)
		lowestUnallocated[int(seqToBucket(firstSeqNo, networkConfig))] = firstSeqNo
	}

	lowestUncommitted := commitState.highestCommit + 1

	proposer := newProposer(
		startingSeqNo,
		uint64(networkConfig.CheckpointInterval),
		myConfig,
		clientTracker,
		buckets,
	)

	preprepareBuffers := make([]*preprepareBuffer, len(lowestUnallocated))
	for i, lu := range lowestUnallocated {
		preprepareBuffers[i] = &preprepareBuffer{
			nextSeqNo: lu,
			buffer: newMsgBuffer(
				fmt.Sprintf("epoch-%d-preprepare", epochConfig.Number),
				nodeBuffers.nodeBuffer(buckets[bucketID(i)]),
			),
		}
	}

	otherBuffers := map[nodeID]*msgBuffer{}
	for _, node := range networkConfig.Nodes {
		otherBuffers[nodeID(node)] = newMsgBuffer(
			fmt.Sprintf("epoch-%d-other", epochConfig.Number),
			nodeBuffers.nodeBuffer(nodeID(node)),
		)
	}

	return &activeEpoch{
		buckets:           buckets,
		myConfig:          myConfig,
		epochConfig:       epochConfig,
		networkConfig:     networkConfig,
		persisted:         persisted,
		commitState:       commitState,
		proposer:          proposer,
		preprepareBuffers: preprepareBuffers,
		otherBuffers:      otherBuffers,
		lowestUnallocated: lowestUnallocated,
		lowestUncommitted: lowestUncommitted,
		outstandingReqs:   outstandingReqs,
		logger:            logger,
	}
}

func (e *activeEpoch) seqToBucket(seqNo uint64) bucketID {
	return seqToBucket(seqNo, e.networkConfig)
}

func (e *activeEpoch) sequence(seqNo uint64) *sequence {
	ci := int(e.networkConfig.CheckpointInterval)
	ciIndex := int(seqNo-e.lowWatermark()) / ci
	ciOffset := int(seqNo-e.lowWatermark()) % ci
	if ciIndex >= len(e.sequences) || ciIndex < 0 || ciOffset < 0 {
		panic(fmt.Sprintf("dev error: low=%d high=%d seqno=%d ciIndex=%d ciOffset=%d len(sequences)=%d", e.lowWatermark(), e.highWatermark(), seqNo, ciIndex, ciOffset, len(e.sequences)))
	}

	sequence := e.sequences[ciIndex][ciOffset]
	assertEqual(sequence.seqNo, seqNo, "sequence retrieved had different seq_no than expected")

	return sequence
}

func (ae *activeEpoch) filter(source nodeID, msg *msgs.Msg) applyable {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_Preprepare:
		seqNo := innerMsg.Preprepare.SeqNo

		bucketID := ae.seqToBucket(seqNo)
		owner := ae.buckets[bucketID]
		if owner != source {
			return invalid
		}

		if seqNo > ae.epochConfig.PlannedExpiration {
			return invalid
		}

		if seqNo > ae.highWatermark() {
			return future
		}

		if seqNo < ae.lowWatermark() {
			return past
		}

		nextPreprepare := ae.preprepareBuffers[int(bucketID)].nextSeqNo
		switch {
		case seqNo < nextPreprepare:
			return past
		case seqNo > nextPreprepare:
			return future
		default:
			return current
		}
	case *msgs.Msg_Prepare:
		seqNo := innerMsg.Prepare.SeqNo

		bucketID := ae.seqToBucket(seqNo)
		owner := ae.buckets[bucketID]
		if owner == source {
			return invalid
		}

		if seqNo > ae.epochConfig.PlannedExpiration {
			return invalid
		}

		switch {
		case seqNo < ae.lowWatermark():
			return past
		case seqNo > ae.highWatermark():
			return future
		default:
			return current
		}
	case *msgs.Msg_Commit:
		seqNo := innerMsg.Commit.SeqNo

		if seqNo > ae.epochConfig.PlannedExpiration {
			return invalid
		}

		switch {
		case seqNo < ae.lowWatermark():
			return past
		case seqNo > ae.highWatermark():
			return future
		default:
			return current
		}
	default:
		panic(fmt.Sprintf("unexpected msg type: %T", msg.Type))
	}
}

func (ae *activeEpoch) apply(source nodeID, msg *msgs.Msg) *ActionList {
	actions := &ActionList{}

	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_Preprepare:
		bucket := ae.seqToBucket(innerMsg.Preprepare.SeqNo)
		preprepareBuffer := ae.preprepareBuffers[bucket]
		nextMsg := msg
		for nextMsg != nil {
			ppMsg := nextMsg.Type.(*msgs.Msg_Preprepare).Preprepare
			actions.concat(ae.applyPreprepareMsg(source, ppMsg.SeqNo, ppMsg.Batch))
			preprepareBuffer.nextSeqNo += uint64(len(ae.buckets))
			nextMsg = preprepareBuffer.buffer.next(ae.filter)
		}
	case *msgs.Msg_Prepare:
		msg := innerMsg.Prepare
		actions.concat(ae.applyPrepareMsg(source, msg.SeqNo, msg.Digest))
	case *msgs.Msg_Commit:
		msg := innerMsg.Commit
		actions.concat(ae.applyCommitMsg(source, msg.SeqNo, msg.Digest))
	default:
		panic(fmt.Sprintf("unexpected msg type: %T", msg.Type))
	}

	return actions
}

func (ae *activeEpoch) step(source nodeID, msg *msgs.Msg) *ActionList {
	switch ae.filter(source, msg) {
	case past:
	case future:
		switch innerMsg := msg.Type.(type) {
		case *msgs.Msg_Preprepare:
			bucket := ae.seqToBucket(innerMsg.Preprepare.SeqNo)
			ae.preprepareBuffers[int(bucket)].buffer.store(msg)
		default:
			ae.otherBuffers[source].store(msg)
		}
	case invalid:
		// TODO Log?
	default: // current
		return ae.apply(source, msg)
	}
	return &ActionList{}
}

func (e *activeEpoch) inWatermarks(seqNo uint64) bool {
	return seqNo >= e.lowWatermark() && seqNo <= e.highWatermark()
}

func (e *activeEpoch) applyPreprepareMsg(source nodeID, seqNo uint64, batch []*msgs.RequestAck) *ActionList {
	seq := e.sequence(seqNo)

	if seq.owner == nodeID(e.myConfig.Id) {
		// We already performed the unallocated movement when we allocated the seq
		return seq.applyPrepareMsg(source, seq.digest)
	}

	bucketID := e.seqToBucket(seqNo)

	assertEqualf(seqNo, e.lowestUnallocated[int(bucketID)], "step should defer all but the next expected preprepare")

	e.lowestUnallocated[int(bucketID)] += uint64(len(e.buckets))

	// Note, this allocates the sequence inside, as we need to track
	// outstanding requests before transitioning the sequence to preprepared
	actions, err := e.outstandingReqs.applyAcks(bucketID, seq, batch)
	if err != nil {
		// TODO implement suspect on bad batch
		panic(fmt.Sprintf("handle me, seq_no=%d we need to stop the bucket and suspect: %s", seqNo, err))
	}

	return actions
}

func (e *activeEpoch) applyPrepareMsg(source nodeID, seqNo uint64, digest []byte) *ActionList {
	seq := e.sequence(seqNo)

	return seq.applyPrepareMsg(source, digest)
}

func (e *activeEpoch) applyCommitMsg(source nodeID, seqNo uint64, digest []byte) *ActionList {
	seq := e.sequence(seqNo)

	seq.applyCommitMsg(source, digest)
	if seq.state != sequenceCommitted || seqNo != e.lowestUncommitted {
		return &ActionList{}
	}

	actions := &ActionList{}

	for e.lowestUncommitted <= e.highWatermark() {
		seq := e.sequence(e.lowestUncommitted)
		if seq.state != sequenceCommitted {
			break
		}

		e.commitState.commit(seq.qEntry)
		e.lowestUncommitted++
	}

	return actions
}

func (e *activeEpoch) moveLowWatermark(seqNo uint64) (*ActionList, bool) {
	if seqNo == e.epochConfig.PlannedExpiration {
		return &ActionList{}, true
	}

	if seqNo == e.commitState.stopAtSeqNo {
		return &ActionList{}, true
	}

	actions := e.advance()

	for seqNo > e.lowWatermark() {
		e.logger.Log(LevelDebug, "moved active epoch low watermarks", "low_watermark", e.lowWatermark(), "high_watermark", e.highWatermark())

		e.sequences = e.sequences[1:]
	}

	return actions, false
}

func (e *activeEpoch) drainBuffers() *ActionList {
	actions := &ActionList{}

	for i := 0; i < len(e.buckets); i++ {
		preprepareBuffer := e.preprepareBuffers[bucketID(i)]
		source := e.buckets[bucketID(i)]
		nextMsg := preprepareBuffer.buffer.next(e.filter)
		if nextMsg == nil {
			continue
		}

		actions.concat(e.apply(source, nextMsg))
		// Note, below, we iterate over all msgs
		// but apply actually loops for us in the preprepare case
		// the difference being that non-preprepare messages
		// only change applayble state when watermarks move,
		// preprepare messages change applyable state after the
		// previous preprepare applies.
	}

	for _, id := range e.networkConfig.Nodes {
		e.otherBuffers[nodeID(id)].iterate(e.filter, func(id nodeID, msg *msgs.Msg) {
			actions.concat(e.apply(id, msg))
		})
	}

	return actions
}

func (e *activeEpoch) advance() *ActionList {
	actions := &ActionList{}

	assertGreaterThanOrEqual(e.epochConfig.PlannedExpiration, e.highWatermark(), "high watermark should never extend beyond the planned epoch expiration")
	assertGreaterThanOrEqual(e.commitState.stopAtSeqNo, e.highWatermark(), "high watermark should never extend beyond the stop at sequence")

	ci := int(e.networkConfig.CheckpointInterval)

	for e.highWatermark() < e.epochConfig.PlannedExpiration &&
		e.highWatermark() < e.commitState.stopAtSeqNo {
		newSequences := make([]*sequence, ci)
		actions.concat(e.persisted.addNEntry(&msgs.NEntry{
			SeqNo:       e.highWatermark() + 1,
			EpochConfig: e.epochConfig,
		}))
		for i := range newSequences {
			seqNo := e.highWatermark() + 1 + uint64(i)
			epoch := e.epochConfig.Number
			owner := e.buckets[e.seqToBucket(seqNo)]
			newSequences[i] = newSequence(owner, epoch, seqNo, e.persisted, e.networkConfig, e.myConfig, e.logger)
		}
		e.sequences = append(e.sequences, newSequences)
	}

	actions.concat(e.drainBuffers())

	e.proposer.advance(e.lowestUncommitted)

	for bid := bucketID(0); bid < bucketID(e.networkConfig.NumberOfBuckets); bid++ {
		ownerID := e.buckets[bid]
		if ownerID != nodeID(e.myConfig.Id) {
			continue
		}

		prb := e.proposer.proposalBucket(bid)

		for {
			seqNo := e.lowestUnallocated[int(bid)]
			if seqNo > e.highWatermark() {
				break
			}

			if !prb.hasPending(seqNo) {
				break
			}

			seq := e.sequence(seqNo)

			actions.concat(seq.allocateAsOwner(prb.next()))

			e.lowestUnallocated[int(bid)] += uint64(len(e.buckets))
		}
	}

	return actions
}

func (e *activeEpoch) applyBatchHashResult(seqNo uint64, digest []byte) *ActionList {
	if !e.inWatermarks(seqNo) {
		// this possibly could be logged, as it indicates consumer error possibly.
		// on the other hand during or after state transfer this could be entirely
		// benign.
		return &ActionList{}
	}

	seq := e.sequence(seqNo)

	return seq.applyBatchHashResult(digest)
}

func (e *activeEpoch) tick() *ActionList {
	if e.lastCommittedAtTick < e.commitState.highestCommit {
		e.lastCommittedAtTick = e.commitState.highestCommit
		e.ticksSinceProgress = 0
		return &ActionList{}
	}

	e.ticksSinceProgress++
	actions := &ActionList{}

	if e.ticksSinceProgress > e.myConfig.SuspectTicks {
		suspect := &msgs.Suspect{
			Epoch: e.epochConfig.Number,
		}
		actions.Send(e.networkConfig.Nodes, &msgs.Msg{
			Type: &msgs.Msg_Suspect{
				Suspect: suspect,
			},
		})
		actions.concat(e.persisted.addSuspect(suspect))
		e.logger.Log(LevelDebug, "suspect epoch to have failed due to lack of active progress", "epoch_no", e.epochConfig.Number)
	}

	if e.myConfig.HeartbeatTicks == 0 || e.ticksSinceProgress%e.myConfig.HeartbeatTicks != 0 {
		return actions
	}

	for bid, unallocatedSeqNo := range e.lowestUnallocated {
		if unallocatedSeqNo > e.highWatermark() {
			continue
		}

		if e.buckets[bucketID(bid)] != nodeID(e.myConfig.Id) {
			continue
		}

		seq := e.sequence(unallocatedSeqNo)

		prb := e.proposer.proposalBucket(bucketID(bid))

		var clientReqs []*clientRequest

		if prb.hasOutstanding(unallocatedSeqNo) {
			clientReqs = prb.next()
		}

		actions.concat(seq.allocateAsOwner(clientReqs))

		e.lowestUnallocated[bid] += uint64(len(e.buckets))
	}

	return actions
}

func (e *activeEpoch) lowWatermark() uint64 {
	return e.sequences[0][0].seqNo
}

func (e *activeEpoch) highWatermark() uint64 {
	if len(e.sequences) == 0 {
		return e.commitState.lowWatermark
	}

	interval := e.sequences[len(e.sequences)-1]
	assertNotEqualf(interval[len(interval)-1], nil, "sequence in %v should be populated", interval)
	return interval[len(interval)-1].seqNo
}

func (e *activeEpoch) status() []*status.Bucket {
	if len(e.sequences) == 0 {
		return []*status.Bucket{}
	}

	buckets := make([]*status.Bucket, len(e.buckets))
	for i := range buckets {
		buckets[i] = &status.Bucket{
			ID:        uint64(i),
			Leader:    e.buckets[bucketID(i)] == nodeID(e.myConfig.Id),
			Sequences: make([]status.SequenceState, len(e.sequences)*len(e.sequences[0])/len(buckets)),
		}
	}

	for seqNo := e.lowWatermark(); seqNo <= e.highWatermark(); seqNo++ {
		seq := e.sequence(seqNo)
		bucket := int(seqToBucket(seqNo, e.networkConfig))
		index := int(seqNo-e.lowWatermark()) / len(e.buckets)
		buckets[bucket].Sequences[index] = status.SequenceState(seq.state)
	}

	return buckets
}
