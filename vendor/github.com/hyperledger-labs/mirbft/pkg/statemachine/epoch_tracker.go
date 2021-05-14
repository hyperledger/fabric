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

type epochTracker struct {
	currentEpoch           *epochTarget
	persisted              *persisted
	nodeBuffers            *nodeBuffers
	commitState            *commitState
	networkConfig          *msgs.NetworkState_Config
	logger                 Logger
	myConfig               *state.EventInitialParameters
	batchTracker           *batchTracker
	clientTracker          *clientTracker
	clientHashDisseminator *clientHashDisseminator
	futureMsgs             map[nodeID]*msgBuffer
	needsStateTransfer     bool

	maxEpochs              map[nodeID]uint64
	maxCorrectEpoch        uint64
	ticksOutOfCorrectEpoch int
}

func newEpochTracker(
	persisted *persisted,
	nodeBuffers *nodeBuffers,
	commitState *commitState,
	networkConfig *msgs.NetworkState_Config,
	logger Logger,
	myConfig *state.EventInitialParameters,
	batchTracker *batchTracker,
	clientTracker *clientTracker,
	clientHashDisseminator *clientHashDisseminator,
) *epochTracker {
	return &epochTracker{
		persisted:              persisted,
		nodeBuffers:            nodeBuffers,
		commitState:            commitState,
		myConfig:               myConfig,
		logger:                 logger,
		batchTracker:           batchTracker,
		clientTracker:          clientTracker,
		clientHashDisseminator: clientHashDisseminator,
		maxEpochs:              map[nodeID]uint64{},
	}
}

func (et *epochTracker) reinitialize() *ActionList {
	et.networkConfig = et.commitState.activeState.Config

	newFutureMsgs := map[nodeID]*msgBuffer{}
	for _, id := range et.networkConfig.Nodes {
		futureMsgs, ok := et.futureMsgs[nodeID(id)]
		if !ok {
			futureMsgs = newMsgBuffer(
				"future-epochs",
				et.nodeBuffers.nodeBuffer(nodeID(id)),
			)
		}
		newFutureMsgs[nodeID(id)] = futureMsgs
	}
	et.futureMsgs = newFutureMsgs

	actions := &ActionList{}
	var lastNEntry *msgs.NEntry
	var lastECEntry *msgs.ECEntry
	var lastFEntry *msgs.FEntry
	var highestPreprepared uint64

	et.persisted.iterate(logIterator{
		onNEntry: func(nEntry *msgs.NEntry) {
			lastNEntry = nEntry
		},
		onFEntry: func(fEntry *msgs.FEntry) {
			lastFEntry = fEntry
		},
		onECEntry: func(ecEntry *msgs.ECEntry) {
			lastECEntry = ecEntry
		},
		onQEntry: func(qEntry *msgs.QEntry) {
			if qEntry.SeqNo > highestPreprepared {
				highestPreprepared = qEntry.SeqNo
			}
		},
		onCEntry: func(cEntry *msgs.CEntry) {
			// In the state transfer case, we may
			// have a CEntry for a seqno we have no QEntry
			if cEntry.SeqNo > highestPreprepared {
				highestPreprepared = cEntry.SeqNo
			}
		},

		// TODO, implement
		onSuspect: func(*msgs.Suspect) {},
	})

	var lastEpochConfig *msgs.EpochConfig
	graceful := false
	switch {
	case lastNEntry != nil && lastFEntry != nil:
		assertGreaterThan(lastNEntry.EpochConfig.Number, lastFEntry.EndsEpochConfig.Number, "new epoch number must not be less than last terminated epoch")
		lastEpochConfig = lastNEntry.EpochConfig
		graceful = false
	case lastNEntry != nil:
		lastEpochConfig = lastNEntry.EpochConfig
		graceful = false
	case lastFEntry != nil:
		lastEpochConfig = lastFEntry.EndsEpochConfig
		graceful = true
	default:
		panic("no active epoch and no last epoch in log")
	}

	switch {
	case lastNEntry != nil && (lastECEntry == nil || lastECEntry.EpochNumber <= lastNEntry.EpochConfig.Number):
		et.logger.Log(LevelDebug, "reinitializing during a currently active epoch")

		et.currentEpoch = newEpochTarget(
			lastNEntry.EpochConfig.Number,
			et.persisted,
			et.nodeBuffers,
			et.commitState,
			et.clientTracker,
			et.clientHashDisseminator,
			et.batchTracker,
			et.networkConfig,
			et.myConfig,
			et.logger,
		)

		startingSeqNo := highestPreprepared + 1
		for startingSeqNo%uint64(et.networkConfig.CheckpointInterval) != 1 {
			// Advance the starting seqno to the first sequence after
			// some checkpoint.  This ensures we do not start consenting
			// on sequences we have already consented on.  If we have
			// startingSeqNo != highestPreprepared + 1 after this loop,
			// then state transfer will be required, though we do
			// not have a state target yet.
			startingSeqNo++
			et.needsStateTransfer = true
		}
		et.currentEpoch.startingSeqNo = startingSeqNo
		et.currentEpoch.state = etResuming
		suspect := &msgs.Suspect{
			Epoch: lastNEntry.EpochConfig.Number,
		}
		actions.concat(et.persisted.addSuspect(suspect))
		actions.Send(et.networkConfig.Nodes, &msgs.Msg{
			Type: &msgs.Msg_Suspect{
				Suspect: suspect,
			},
		})
	case lastFEntry != nil && (lastECEntry == nil || lastECEntry.EpochNumber <= lastFEntry.EndsEpochConfig.Number):
		et.logger.Log(LevelDebug, "reinitializing immediately after graceful epoch end, but before epoch change sent, creating epoch change")
		// An epoch has just gracefully ended, and we have not yet tried to move to the next
		lastECEntry = &msgs.ECEntry{
			EpochNumber: lastFEntry.EndsEpochConfig.Number + 1,
		}
		actions.concat(et.persisted.addECEntry(lastECEntry))
		fallthrough
	case lastECEntry != nil:
		// An epoch has ended (ungracefully or otherwise), and we have sent our epoch change

		et.logger.Log(LevelDebug, "reinitializing after epoch change persisted")

		if et.currentEpoch != nil && et.currentEpoch.number == lastECEntry.EpochNumber {
			// We have been reinitialized during an epoch change, no need to start fresh
			return actions.concat(et.currentEpoch.advanceState())
		}

		epochChange := et.persisted.constructEpochChange(lastECEntry.EpochNumber)
		parsedEpochChange, err := newParsedEpochChange(epochChange)
		assertEqualf(err, nil, "could not parse epoch change we generated: %s", err)

		et.currentEpoch = newEpochTarget(
			epochChange.NewEpoch,
			et.persisted,
			et.nodeBuffers,
			et.commitState,
			et.clientTracker,
			et.clientHashDisseminator,
			et.batchTracker,
			et.networkConfig,
			et.myConfig,
			et.logger,
		)

		et.currentEpoch.myEpochChange = parsedEpochChange

		// XXX this leader selection is wrong, but using while we modify the startup.
		// instead base it on the lastEpochConfig and whether that epoch ended gracefully.
		_, _ = lastEpochConfig, graceful
		et.currentEpoch.myLeaderChoice = et.networkConfig.Nodes
	default:
		// There's no active epoch, it did not end gracefully, or ungracefully
		panic("no recorded active epoch, ended epoch, or epoch change in log")
	}

	for _, id := range et.networkConfig.Nodes {
		et.futureMsgs[nodeID(id)].iterate(et.filter, func(source nodeID, msg *msgs.Msg) {
			actions.concat(et.applyMsg(source, msg))
		})
	}

	return actions
}

func (et *epochTracker) advanceState() *ActionList {
	if et.currentEpoch.state < etDone {
		return et.currentEpoch.advanceState()
	}

	if et.commitState.checkpointPending {
		// It simplifies our lives considerably to wait for checkpoints
		// before initiating epoch change.
		return &ActionList{}
	}

	newEpochNumber := et.currentEpoch.number + 1
	if et.maxCorrectEpoch > newEpochNumber {
		newEpochNumber = et.maxCorrectEpoch
	}
	epochChange := et.persisted.constructEpochChange(newEpochNumber)

	myEpochChange, err := newParsedEpochChange(epochChange)
	assertEqualf(err, nil, "could not parse epoch change we generated: %s", err)

	et.currentEpoch = newEpochTarget(
		newEpochNumber,
		et.persisted,
		et.nodeBuffers,
		et.commitState,
		et.clientTracker,
		et.clientHashDisseminator,
		et.batchTracker,
		et.networkConfig,
		et.myConfig,
		et.logger,
	)
	et.currentEpoch.myEpochChange = myEpochChange
	et.currentEpoch.myLeaderChoice = []uint64{et.myConfig.Id} // XXX, wrong

	actions := et.persisted.addECEntry(&msgs.ECEntry{
		EpochNumber: newEpochNumber,
	}).Send(
		et.networkConfig.Nodes,
		&msgs.Msg{
			Type: &msgs.Msg_EpochChange{
				EpochChange: epochChange,
			},
		},
	)

	for _, id := range et.networkConfig.Nodes {
		et.futureMsgs[nodeID(id)].iterate(et.filter, func(source nodeID, msg *msgs.Msg) {
			actions.concat(et.applyMsg(source, msg))
		})
	}

	return actions
}

func epochForMsg(msg *msgs.Msg) uint64 {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_Preprepare:
		return innerMsg.Preprepare.Epoch
	case *msgs.Msg_Prepare:
		return innerMsg.Prepare.Epoch
	case *msgs.Msg_Commit:
		return innerMsg.Commit.Epoch
	case *msgs.Msg_Suspect:
		return innerMsg.Suspect.Epoch
	case *msgs.Msg_EpochChange:
		return innerMsg.EpochChange.NewEpoch
	case *msgs.Msg_EpochChangeAck:
		return innerMsg.EpochChangeAck.EpochChange.NewEpoch
	case *msgs.Msg_NewEpoch:
		return innerMsg.NewEpoch.NewConfig.Config.Number
	case *msgs.Msg_NewEpochEcho:
		return innerMsg.NewEpochEcho.Config.Number
	case *msgs.Msg_NewEpochReady:
		return innerMsg.NewEpochReady.Config.Number
	default:
		panic(fmt.Sprintf("unexpected bad epoch message type %T, this indicates a bug", msg.Type))
	}
}

func (et *epochTracker) filter(_ nodeID, msg *msgs.Msg) applyable {
	epochNumber := epochForMsg(msg)

	switch {
	case epochNumber < et.currentEpoch.number:
		return past
	case epochNumber > et.currentEpoch.number:
		return future
	default:
		return current
	}
}

func (et *epochTracker) step(source nodeID, msg *msgs.Msg) *ActionList {
	epochNumber := epochForMsg(msg)

	switch {
	case epochNumber < et.currentEpoch.number:
		// past
		return &ActionList{}
	case epochNumber > et.currentEpoch.number:
		// future
		maxEpoch := et.maxEpochs[source]
		if maxEpoch < epochNumber {
			et.maxEpochs[source] = epochNumber
		}
		et.futureMsgs[source].store(msg)
		return &ActionList{}
	default:
		// current
		return et.applyMsg(source, msg)
	}
}

func (et *epochTracker) applyMsg(source nodeID, msg *msgs.Msg) *ActionList {
	target := et.currentEpoch

	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_Preprepare:
		return target.step(source, msg)
	case *msgs.Msg_Prepare:
		return target.step(source, msg)
	case *msgs.Msg_Commit:
		return target.step(source, msg)
	case *msgs.Msg_Suspect:
		target.applySuspectMsg(source)
		return &ActionList{}
	case *msgs.Msg_EpochChange:
		return target.applyEpochChangeMsg(source, innerMsg.EpochChange)
	case *msgs.Msg_EpochChangeAck:
		return target.applyEpochChangeAckMsg(source, nodeID(innerMsg.EpochChangeAck.Originator), innerMsg.EpochChangeAck.EpochChange)
	case *msgs.Msg_NewEpoch:
		// Ignore NewEpoch message if not sent by the epoch primary.
		if innerMsg.NewEpoch.NewConfig.Config.Number%uint64(len(et.networkConfig.Nodes)) != uint64(source) {
			// TODO, log oddity
			return &ActionList{}
		}
		return target.applyNewEpochMsg(innerMsg.NewEpoch)
	case *msgs.Msg_NewEpochEcho:
		return target.applyNewEpochEchoMsg(source, innerMsg.NewEpochEcho)
	case *msgs.Msg_NewEpochReady:
		return target.applyNewEpochReadyMsg(source, innerMsg.NewEpochReady)
	default:
		panic(fmt.Sprintf("unexpected bad epoch message type %T, this indicates a bug", msg.Type))
	}
}

func (et *epochTracker) applyBatchHashResult(epoch, seqNo uint64, digest []byte) *ActionList {
	if epoch != et.currentEpoch.number || et.currentEpoch.state != etInProgress {
		// TODO, should we try to see if it applies to the current epoch?
		return &ActionList{}
	}

	return et.currentEpoch.activeEpoch.applyBatchHashResult(seqNo, digest)
}

func (et *epochTracker) tick() *ActionList {
	for _, maxEpoch := range et.maxEpochs {
		if maxEpoch <= et.maxCorrectEpoch {
			continue
		}
		matches := 1
		for _, matchingEpoch := range et.maxEpochs {
			if matchingEpoch < maxEpoch {
				continue
			}
			matches++
		}

		if matches < someCorrectQuorum(et.networkConfig) {
			continue
		}

		et.maxCorrectEpoch = maxEpoch
	}

	if et.maxCorrectEpoch > et.currentEpoch.number {
		et.ticksOutOfCorrectEpoch++

		// TODO make this configurable
		if et.ticksOutOfCorrectEpoch > 10 {
			et.currentEpoch.state = etDone
		}
	}

	return et.currentEpoch.tick()
}

func (et *epochTracker) moveLowWatermark(seqNo uint64) *ActionList {
	return et.currentEpoch.moveLowWatermark(seqNo)
}

func (et *epochTracker) applyEpochChangeDigest(origin *state.HashOrigin_EpochChange, digest []byte) *ActionList {
	targetNumber := origin.EpochChange.NewEpoch
	switch {
	case targetNumber < et.currentEpoch.number:
		// This is for an old epoch we no long care about
		return &ActionList{}
	case targetNumber > et.currentEpoch.number:
		assertFailed("", "got an epoch change digest for epoch %d we are processing %d", targetNumber, et.currentEpoch.number)

	}
	return et.currentEpoch.applyEpochChangeDigest(origin, digest)
}

func (et *epochTracker) status() *status.EpochTracker {
	return &status.EpochTracker{
		ActiveEpoch: et.currentEpoch.status(),
	}
}
