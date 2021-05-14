/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/status"

	"google.golang.org/protobuf/proto"
)

type epochTargetState int

const (
	etPrepending = iota // Have sent an epoch-change, but waiting for a quorum
	etPending           // Have a quorum of epoch-change messages, waits on new-epoch
	etVerifying         // Have a new view message but it references epoch changes we cannot yet verify
	etFetching          // Have received and verified a new epoch messages, and are waiting to get state
	etEchoing           // Have received and validated a new-epoch, waiting for a quorum of echos
	etReadying          // Have received a quorum of echos, waiting a on qourum of readies
	etResuming          // We crashed during this epoch, and are waiting to resume
	etReady             // New epoch is ready to begin
	etInProgress        // No pending change
	etEnding            // The epoch has committed everything it can, and we have a stable checkpoint
	etDone              // We have sent an epoch change, ending this epoch for us
)

// epochTarget is like an epoch, but this node need not have agreed
// to transition to this target, and may not have information like the
// epoch configuration
type epochTarget struct {
	state           epochTargetState
	commitState     *commitState
	stateTicks      uint64
	number          uint64
	startingSeqNo   uint64
	changes         map[nodeID]*epochChange
	strongChanges   map[nodeID]*parsedEpochChange // Parsed EpochChange messages acknowledged by enough nodes
	echos           map[*msgs.NewEpochConfig]map[nodeID]struct{}
	readies         map[*msgs.NewEpochConfig]map[nodeID]struct{}
	activeEpoch     *activeEpoch
	suspicions      map[nodeID]struct{}
	myNewEpoch      *msgs.NewEpoch // The NewEpoch msg we computed from the epoch changes we know of
	myEpochChange   *parsedEpochChange
	myLeaderChoice  []uint64             // Set along with myEpochChange
	leaderNewEpoch  *msgs.NewEpoch       // The NewEpoch msg we received directly from the leader
	networkNewEpoch *msgs.NewEpochConfig // The NewEpoch msg as received via the bracha broadcast
	isPrimary       bool
	prestartBuffers map[nodeID]*msgBuffer

	persisted              *persisted
	nodeBuffers            *nodeBuffers
	clientTracker          *clientTracker
	clientHashDisseminator *clientHashDisseminator
	batchTracker           *batchTracker
	networkConfig          *msgs.NetworkState_Config
	myConfig               *state.EventInitialParameters
	logger                 Logger
}

func newEpochTarget(
	number uint64,
	persisted *persisted,
	nodeBuffers *nodeBuffers,
	commitState *commitState,
	clientTracker *clientTracker,
	clientHashDisseminator *clientHashDisseminator,
	batchTracker *batchTracker,
	networkConfig *msgs.NetworkState_Config,
	myConfig *state.EventInitialParameters,
	logger Logger,
) *epochTarget {
	prestartBuffers := map[nodeID]*msgBuffer{}
	for _, id := range networkConfig.Nodes {
		prestartBuffers[nodeID(id)] = newMsgBuffer(
			fmt.Sprintf("epoch-%d-prestart", number),
			nodeBuffers.nodeBuffer(nodeID(id)),
		)
	}

	return &epochTarget{
		state:                  etPrepending,
		number:                 number,
		commitState:            commitState,
		suspicions:             map[nodeID]struct{}{},
		changes:                map[nodeID]*epochChange{},
		strongChanges:          map[nodeID]*parsedEpochChange{},
		echos:                  map[*msgs.NewEpochConfig]map[nodeID]struct{}{},
		readies:                map[*msgs.NewEpochConfig]map[nodeID]struct{}{},
		isPrimary:              number%uint64(len(networkConfig.Nodes)) == myConfig.Id,
		prestartBuffers:        prestartBuffers,
		persisted:              persisted,
		nodeBuffers:            nodeBuffers,
		clientTracker:          clientTracker,
		clientHashDisseminator: clientHashDisseminator,
		batchTracker:           batchTracker,
		networkConfig:          networkConfig,
		myConfig:               myConfig,
		logger:                 logger,
	}
}

func (et *epochTarget) step(source nodeID, msg *msgs.Msg) *ActionList {
	if et.state < etInProgress {
		et.prestartBuffers[source].store(msg)
		return &ActionList{}
	}

	if et.state == etDone {
		return &ActionList{}
	}

	return et.activeEpoch.step(source, msg)
}

// Constructs and returns a NewEpoch message.
// This message consists of the configuration for the new epoch
// and a set of digests of acked EpochChange messages.
// Note specifically that this NewEpoch message does not contain the payload of the EpochChange messages.
// The recipient receives those directly from other nodes.
func (et *epochTarget) constructNewEpoch(newLeaders []uint64, nc *msgs.NetworkState_Config) *msgs.NewEpoch {

	// Sanity check.
	assertGreaterThanOrEqualf(uint64(len(et.strongChanges)), uint64(intersectionQuorum(nc)),
		"received %d acked epoch change messages, only received %d",
		uint64(len(et.strongChanges)), uint64(intersectionQuorum(nc)))

	// Compute the new epoch configuration based on the received EpochChange messages.
	newConfig := constructNewEpochConfig(nc, newLeaders, et.strongChanges)
	if newConfig == nil {
		return nil
	}

	// Include the digests of the received EpochChange messages in the NewEpoch message.
	remoteChanges := make([]*msgs.NewEpoch_RemoteEpochChange, 0, len(et.changes))
	for _, id := range et.networkConfig.Nodes {
		// Deterministic iteration over strong changes
		_, ok := et.strongChanges[nodeID(id)]
		if !ok {
			continue
		}

		remoteChanges = append(remoteChanges, &msgs.NewEpoch_RemoteEpochChange{
			NodeId: uint64(id),
			Digest: et.changes[nodeID(id)].strongCert,
		})
	}

	// Return newly computed NewEpoch message.
	return &msgs.NewEpoch{
		NewConfig:    newConfig,
		EpochChanges: remoteChanges,
	}
}

// Verifies that the NewEpoch message we obtained from the new primary is valid
// and that we have received all the EpochChange messages it references.
// If this is the case, advances the state to etFetching.
func (et *epochTarget) verifyNewEpochState() {
	epochChanges := map[nodeID]*parsedEpochChange{}

	// Verify that:
	for _, remoteEpochChange := range et.leaderNewEpoch.EpochChanges {

		// Each EpochChange is only referenced once.
		if _, ok := epochChanges[nodeID(remoteEpochChange.NodeId)]; ok {
			// TODO, references multiple epoch changes from the same node, malformed, log oddity
			return
		}

		// We have received an EpochChange from the source of the referenced message.
		change, ok := et.changes[nodeID(remoteEpochChange.NodeId)]
		if !ok {
			// Either the primary is lying, or we simply don't have enough information yet.
			return
		}

		// The received EpochChange has the correct digest and is acknowledged.
		parsedChange, ok := change.parsedByDigest[string(remoteEpochChange.Digest)]
		if !ok || len(parsedChange.acks) < someCorrectQuorum(et.networkConfig) {
			return
		}

		epochChanges[nodeID(remoteEpochChange.NodeId)] = parsedChange
	}

	// TODO, validate the planned expiration makes sense

	// TODO, do we need to try to validate the leader set?

	// Reconstruct the new epoch configuration based on the received information.
	newEpochConfig := constructNewEpochConfig(et.networkConfig, et.leaderNewEpoch.NewConfig.Config.Leaders, epochChanges)

	// The reconstructed new epoch configuration must be the same as the one obtained from the leader.
	// Otherwise the leader must be faulty.
	if !proto.Equal(newEpochConfig, et.leaderNewEpoch.NewConfig) {
		// TODO byzantine, log oddity
		return
	}

	et.logger.Log(LevelDebug, "epoch transitioning from from verifying to fetching", "epoch_no", et.number)
	et.state = etFetching
}

func (et *epochTarget) fetchNewEpochState() *ActionList {
	newEpochConfig := et.leaderNewEpoch.NewConfig

	if et.commitState.transferring {
		// Wait until state transfer completes before attempting to process the new epoch
		et.logger.Log(LevelDebug, "delaying fetching of epoch state until state transfer completes", "epoch_no", et.number)
		return &ActionList{}
	}

	if newEpochConfig.StartingCheckpoint.SeqNo > et.commitState.highestCommit {
		et.logger.Log(LevelDebug, "delaying fetching of epoch state until outstanding checkpoint is computed", "epoch_no", et.number, "seq_no", newEpochConfig.StartingCheckpoint.SeqNo)
		return et.commitState.transferTo(newEpochConfig.StartingCheckpoint.SeqNo, newEpochConfig.StartingCheckpoint.Value)
	}

	actions := &ActionList{}
	fetchPending := false

	// The NewEpoch message only references (by digest) Preprepare messages instead of including them.
	// Here we make sure we obtain the payload if we still do not have it.
	for i, digest := range newEpochConfig.FinalPreprepares {

		// An empty digest corresponds to a "null request"/empty batch.
		if len(digest) == 0 {
			continue
		}

		seqNo := uint64(i) + newEpochConfig.StartingCheckpoint.SeqNo + 1

		// We already committed this SN, no need to fetch Preprepare.
		if seqNo <= et.commitState.highestCommit {
			continue
		}

		// Find the nodes which claimed to have received the referenced Preprepare
		// by inspecting (the qSets of) the EpochChange messages they sent.
		var sources []uint64
		for _, remoteEpochChange := range et.leaderNewEpoch.EpochChanges {
			// Previous state verified these exist
			change := et.changes[nodeID(remoteEpochChange.NodeId)]
			parsedChange := change.parsedByDigest[string(remoteEpochChange.Digest)]
			for _, qEntryDigest := range parsedChange.qSet[seqNo] {
				if bytes.Equal(qEntryDigest, digest) {
					sources = append(sources, remoteEpochChange.NodeId)
					break
				}
			}
		}

		// Sanity check.
		if len(sources) < someCorrectQuorum(et.networkConfig) {
			panic(fmt.Sprintf("dev only, should never be true, we only found %d sources for seqno=%d with digest=%x", len(sources), seqNo, digest))
		}

		// If we are missing the referenced batch (i.e. we did not receive the Preprepare), fetch it.
		batch, ok := et.batchTracker.getBatch(digest)
		if !ok {
			actions.concat(et.batchTracker.fetchBatch(seqNo, digest, sources))
			fetchPending = true
			continue
		}

		// At this point we know we have the batch.
		// However, we might have received it for a different sequence number.
		// Mark it as observed also for seqNo.
		batch.observedFor[seqNo] = struct{}{}

		// TODO: Explain what this block does.
		for _, requestAck := range batch.requestAcks {
			// TODO, do we really want this? We should not need acks to commit, and we fetch explicitly maybe?
			var cr *clientRequest
			for _, id := range sources {
				iActions, icr := et.clientHashDisseminator.ack(nodeID(id), requestAck)
				cr = icr
				actions.concat(iActions)
			}

			if cr.stored {
				continue
			}

			// We are missing this request data and must fetch before proceeding
			fetchPending = true
			actions.concat(cr.fetch())
		}
	}

	// We cannot continue until more data is fetched.
	if fetchPending {
		return actions
	}

	if newEpochConfig.StartingCheckpoint.SeqNo > et.commitState.lowWatermark {
		// Per the check above, we know
		//   newEpochConfig.StartingCheckpoint.SeqNo <= et.commitState.lastCommit
		// So we've committed through this checkpoint, but need to wait for it
		// to be computed before we can safely echo
		return actions
	}

	et.logger.Log(LevelDebug, "epoch transitioning from fetching to echoing", "epoch_no", et.number)
	et.state = etEchoing

	if newEpochConfig.StartingCheckpoint.SeqNo == et.commitState.stopAtSeqNo && len(newEpochConfig.FinalPreprepares) > 0 {
		// We know at this point that
		// newEpochConfig.StartingCheckpoint.SeqNo <= et.commitState.lowWatermark
		// and always et.commitState.lowWatermark <= et.commitState.stopAtSeqNo
		// Further, since this epoch change is correct, we know that some correct replica
		// prepared some sequence beyond the starting checkpoint.  Since a correct replica
		// will wait for a strong checkpoint quorum before preparing beyond a reconfiguration
		// we therefore know that this checkpoint is in fact stable, and we must
		// reinitialize under the new network configuration before processing further.

		// XXX the problem becomes though, that in order to reinitialize, we need to
		// append the FEntry, but, then we truncate the log, and if we crash, we come
		// back up unaware of our previous epoch change message.  Fine, we know
		// the checkpoint is strong, but, without epoch message reliable
		// (re)broadcast, the new view timer might never start at other nodes
		// and we could get stuck in this epoch forever.

		panic("deal with this")
	}

	// XXX what if the final preprepares span both an old and new config?

	// "Allocate" the sequence numbers corresponding to the new epoch by appending an NEntry to the persistent log.
	actions.concat(et.persisted.addNEntry(&msgs.NEntry{
		SeqNo:       newEpochConfig.StartingCheckpoint.SeqNo + 1,
		EpochConfig: newEpochConfig.Config,
	}))

	// Handle the Preprepares contained in the NewEpoch message.
	for i, digest := range newEpochConfig.FinalPreprepares {
		seqNo := uint64(i) + newEpochConfig.StartingCheckpoint.SeqNo + 1

		// For "null requests", only append an "empty" QEntry to the persistent log.
		if len(digest) == 0 {
			actions.concat(et.persisted.addQEntry(&msgs.QEntry{
				SeqNo: seqNo,
			}))
			continue
		}

		// For an actual batch, we know already we have the payload
		// (otherwise we would return before reaching this code).
		batch, ok := et.batchTracker.getBatch(digest)
		if !ok {
			panic(fmt.Sprintf("dev sanity check -- batch %x was just found above, not is now missing", digest))
		}

		// Add a new QEntry to the persistent log (i.e. consider the batch preprepared).
		qEntry := &msgs.QEntry{
			SeqNo:    seqNo,
			Digest:   digest,
			Requests: batch.requestAcks,
		}
		actions.concat(et.persisted.addQEntry(qEntry))

		// If this SN corresponds to a checkpoint, allocate the next epoch by appending an NEntry to the WAL.
		if seqNo%uint64(et.networkConfig.CheckpointInterval) == 0 && seqNo < et.commitState.stopAtSeqNo {
			actions.concat(et.persisted.addNEntry(&msgs.NEntry{
				SeqNo:       seqNo + 1,
				EpochConfig: newEpochConfig.Config,
			}))
		}
	}

	// TODO: Explain this. What is the logic behind FinalPreprepares and their sequence numbers?
	//       Are the FinalPreprepares meant to be part of the new epoch?
	//       Or are they "fillers" for the old epoch until the next checkpoint?
	et.startingSeqNo = newEpochConfig.StartingCheckpoint.SeqNo +
		uint64(len(newEpochConfig.FinalPreprepares)) + 1

	// Echo the NewEpoch message to all nodes (2nd phase of Bracha broadcast).
	// Note that this echo message also serves as a PBFT prepare message
	// for all the outstanding sequence numbers included in the epoch change.
	return actions.Send(
		et.networkConfig.Nodes,
		&msgs.Msg{
			Type: &msgs.Msg_NewEpochEcho{
				NewEpochEcho: et.leaderNewEpoch.NewConfig,
			},
		},
	)
}

func (et *epochTarget) tick() *ActionList {
	et.stateTicks++
	if et.state == etPrepending {
		// Waiting for a quorum of epoch changes
		return et.tickPrepending()
	} else if et.state <= etResuming {
		// Waiting for the new epoch config
		return et.tickPending()
	} else if et.state <= etInProgress {
		// Active in the epoch
		return et.activeEpoch.tick()
	}

	return &ActionList{}
}

func (et *epochTarget) repeatEpochChangeBroadcast() *ActionList {
	return (&ActionList{}).Send(
		et.networkConfig.Nodes,
		&msgs.Msg{
			Type: &msgs.Msg_EpochChange{
				EpochChange: et.myEpochChange.underlying,
			},
		},
	)
}

func (et *epochTarget) tickPrepending() *ActionList {
	if et.myNewEpoch == nil {
		if et.stateTicks%uint64(et.myConfig.NewEpochTimeoutTicks/2) == 0 {
			return et.repeatEpochChangeBroadcast()
		}

		return &ActionList{}
	}

	if et.isPrimary {
		return (&ActionList{}).Send(
			et.networkConfig.Nodes,
			&msgs.Msg{
				Type: &msgs.Msg_NewEpoch{
					NewEpoch: et.myNewEpoch,
				},
			},
		)
	}

	return &ActionList{}
}

func (et *epochTarget) tickPending() *ActionList {
	pendingTicks := et.stateTicks % uint64(et.myConfig.NewEpochTimeoutTicks)
	if et.isPrimary {
		// resend the new-view if others perhaps missed it
		if pendingTicks%2 == 0 {
			return (&ActionList{}).Send(
				et.networkConfig.Nodes,
				&msgs.Msg{
					Type: &msgs.Msg_NewEpoch{
						NewEpoch: et.myNewEpoch,
					},
				},
			)
		}
	} else {
		if pendingTicks == 0 {
			suspect := &msgs.Suspect{
				Epoch: et.myNewEpoch.NewConfig.Config.Number,
			}
			return (&ActionList{}).Send(
				et.networkConfig.Nodes,
				&msgs.Msg{
					Type: &msgs.Msg_Suspect{
						Suspect: suspect,
					},
				},
			).concat(et.persisted.addSuspect(suspect))
		}
		if pendingTicks%2 == 0 {
			return et.repeatEpochChangeBroadcast()
		}
	}
	return &ActionList{}
}

// Applying an EpochChange message only involves sending ACKs to all other nodes
// and locally handling own ACK.
func (et *epochTarget) applyEpochChangeMsg(source nodeID, msg *msgs.EpochChange) *ActionList {
	actions := &ActionList{}
	if source != nodeID(et.myConfig.Id) {
		// We don't want to echo our own EpochChange message,
		// as we already broadcast/rebroadcast it.
		actions.Send(
			et.networkConfig.Nodes,
			&msgs.Msg{
				Type: &msgs.Msg_EpochChangeAck{
					EpochChangeAck: &msgs.EpochChangeAck{
						Originator:  uint64(source),
						EpochChange: msg,
					},
				},
			},
		)
	}

	// Automatically apply an ACK from the originator
	return actions.concat(et.applyEpochChangeAckMsg(source, source, msg))
}

// An incoming EpochChangeAck message first needs to be hashed.
// Here we only request the hast, and the actual processing
// happens in epochTarget.applyEpochChangeDigest().
//
// Note that the message passed as argument is not the ACK itself (sent by source),
// but the (contained) original EpochChange message (sent to source by origin)
func (et *epochTarget) applyEpochChangeAckMsg(source nodeID, origin nodeID, msg *msgs.EpochChange) *ActionList {

	return (&ActionList{}).Hash(
		epochChangeHashData(msg),
		&state.HashOrigin{
			Type: &state.HashOrigin_EpochChange_{
				EpochChange: &state.HashOrigin_EpochChange{
					Source:      uint64(source),
					Origin:      uint64(origin),
					EpochChange: msg,
				},
			},
		},
	)
}

// Processes an EpochChange ACK that already includes a hash of the EpochChange.
// Note that the ACK message itself is not used here any more, as all the related
// information is contained in the HashOrigin, that is, in turn, part of the result
// of hashing (the relevant parts of) the ACK message.
func (et *epochTarget) applyEpochChangeDigest(processedChange *state.HashOrigin_EpochChange, digest []byte) *ActionList {
	originNode := nodeID(processedChange.Origin)
	sourceNode := nodeID(processedChange.Source)

	// If this is the first ACK for an EpochChange from this origin,
	// create a new EpochChange data structure for tracking ACKs.
	change, ok := et.changes[originNode]
	if !ok {
		change = newEpochChange(et.networkConfig)
		et.changes[originNode] = change
	}

	// Register reception of the acknowledgment.
	change.addAck(sourceNode, processedChange.EpochChange, digest)

	// If the (parsed) EpochChange has been acknowledged
	if change.strongCert != nil {
		// and has not yet been considered,
		if _, alreadyConsidered := et.strongChanges[originNode]; !alreadyConsidered {
			// assign the EpochChange to its originating node and try to advance the state.
			et.strongChanges[originNode] = change.parsedByDigest[string(change.strongCert)]
			return et.advanceState()
		}
	}

	return &ActionList{}
}

// Checks whether enough EpochChange messages have been received and acknowledged.
// If so, advances the state to etPending and sends a NewEpoch message if this node is the primary.
func (et *epochTarget) checkEpochQuorum() *ActionList {

	// Do nothing if not enough EpochChanges have been received/acknowledged
	// or the node itself is not yet ready for an epoch change.
	if len(et.strongChanges) < intersectionQuorum(et.networkConfig) || et.myEpochChange == nil {
		return &ActionList{}
	}

	// Compute the NewEpoch message and advance to the next state.
	et.myNewEpoch = et.constructNewEpoch(et.myLeaderChoice, et.networkConfig)
	if et.myNewEpoch == nil {
		return &ActionList{}
	}
	et.stateTicks = 0
	et.state = etPending

	// If this node is the primary, send the NewEpoch message to all others.
	if et.isPrimary {
		return (&ActionList{}).Send(
			et.networkConfig.Nodes,
			&msgs.Msg{
				Type: &msgs.Msg_NewEpoch{
					NewEpoch: et.myNewEpoch,
				},
			},
		)
	}

	return &ActionList{}
}

// Applies a NewEpoch message received from the new primary.
// Saves the message and tries to advance the state.
// This is important for the case where the NewEpoch cannot be processed yet.
func (et *epochTarget) applyNewEpochMsg(msg *msgs.NewEpoch) *ActionList {
	et.leaderNewEpoch = msg
	return et.advanceState()
}

// Registers a received NewEpochEcho message received during epoch change.
// Note that a NewEpochEcho also serves as the PBFT prepare message for all
// the outstanding sequence numbers in an epoch change.
func (et *epochTarget) applyNewEpochEchoMsg(source nodeID, msg *msgs.NewEpochConfig) *ActionList {
	var msgEchos map[nodeID]struct{}

	// Check if we already received this echo message from some node.
	for config, echos := range et.echos {
		if proto.Equal(config, msg) {
			msgEchos = echos
			break
		}
	}

	// If we did not, create a new entry for this echo message.
	if msgEchos == nil {
		msgEchos = map[nodeID]struct{}{}
		et.echos[msg] = msgEchos
	}

	// Register reception of this echo message from source.
	msgEchos[source] = struct{}{}

	// Advancing the state checks whether we now have enough echoes and acts accordingly.
	return et.advanceState()
}

// Checks whether enough NewEpochEcho messages have been received.
// If yes, enters the READY phase of Bracha-broadcasting the NewEpoch message.
func (et *epochTarget) checkNewEpochEchoQuorum() *ActionList {
	actions := &ActionList{}

	// Go through all different echo messages and look for a quorum.
	// An echo message is defined uniquely by the new epoch configuration (NewEpochConfig) it contains.
	for config, msgEchos := range et.echos {

		// If a quorum of echoes has been obtained for some configuration
		if len(msgEchos) >= intersectionQuorum(et.networkConfig) {

			// Advance to the READY phase of Bracha broadcast.
			et.state = etReadying

			// The NewEpochEcho simultaneously serves as the PBFT prepare message
			// for all the outstanding sequence numbers included in the epoch change.
			// Thus, mark those sequence numbers as prepared.
			for i, digest := range config.FinalPreprepares {
				seqNo := uint64(i) + config.StartingCheckpoint.SeqNo + 1
				actions.concat(et.persisted.addPEntry(&msgs.PEntry{
					SeqNo:  seqNo,
					Digest: digest,
				}))
			}

			// Send a READY message (3rd phase of Bracha broadcast).
			// Note that this message also serves as a PBFT commit message
			// for all the outstanding sequence numbers included in the epoch change.
			return actions.Send(
				et.networkConfig.Nodes,
				&msgs.Msg{
					Type: &msgs.Msg_NewEpochReady{
						NewEpochReady: config,
					},
				},
			)
		}
	}

	return actions
}

// Registers a received NewEpochReady message received during epoch change.
// Note that a NewEpochReady also serves as the PBFT commit message for all
// the outstanding sequence numbers in an epoch change.
func (et *epochTarget) applyNewEpochReadyMsg(source nodeID, msg *msgs.NewEpochConfig) *ActionList {
	if et.state > etReadying {
		// We've already accepted the epoch config, move along
		return &ActionList{}
	}

	var msgReadies map[nodeID]struct{}

	// Check if we already received this ready message from some node.
	for config, readies := range et.readies {
		if proto.Equal(config, msg) {
			msgReadies = readies
			break
		}
	}

	// If we did not, create a new entry for this ready message.
	if msgReadies == nil {
		msgReadies = map[nodeID]struct{}{}
		et.readies[msg] = msgReadies
	}

	// Register reception of this ready message from source.
	msgReadies[source] = struct{}{}

	// If the received message does not contribute to even a weak quorum, return immediately.
	if len(msgReadies) < someCorrectQuorum(et.networkConfig) {
		return &ActionList{}
	}

	// TODO: Explain why we are advancing the state here.
	//       It looks like a ready message cannot make any difference before the etEchoing state.
	if et.state < etEchoing {
		return et.advanceState()
	}

	// If we are not yet in etReadying state (i.e., given the previous check, we must be in etEchoing),
	// move to the etReadying state (since we've received a weak quorum of readies).
	// This is the case in Bracha broadcast where we receive a weak quorum of readies
	// before having received a strong quorum of echoes.
	if et.state < etReadying {

		// Advance state.
		et.logger.Log(LevelDebug, "epoch transitioning from echoing to ready", "epoch_no", et.number)
		et.state = etReadying

		// Send a READY message to all nodes.
		actions := (&ActionList{}).Send(
			et.networkConfig.Nodes,
			&msgs.Msg{
				Type: &msgs.Msg_NewEpochReady{
					NewEpochReady: msg,
				},
			},
		)

		// TODO: Don't we need to call advanceState() here as well?
		//       (This function is not called from within advanceState(),
		//        so changing et.state does not help.)
		return actions
	}

	return et.advanceState()
}

// TODO: Should we move part of the functionality of applyNewEpochReadyMst() inside checkNewEpochReadyQuorum()?
//       It might make the code more readable by keeping the same pattern as the one used with echoes.
func (et *epochTarget) checkNewEpochReadyQuorum() {
	for config, msgReadies := range et.readies {
		if len(msgReadies) < intersectionQuorum(et.networkConfig) {
			continue
		}

		et.logger.Log(LevelDebug, "epoch transitioning from ready to resuming", "epoch_no", et.number)
		et.state = etResuming

		et.networkNewEpoch = config

		currentEpoch := false
		et.persisted.iterate(logIterator{
			onQEntry: func(qEntry *msgs.QEntry) {
				if !currentEpoch {
					return
				}

				et.logger.Log(LevelDebug, "epoch change triggering commit", "epoch_no", et.number, "seq_no", qEntry.SeqNo)
				et.commitState.commit(qEntry)
			},
			onECEntry: func(ecEntry *msgs.ECEntry) {
				if ecEntry.EpochNumber < config.Config.Number {
					return
				}

				assertGreaterThanOrEqual(config.Config.Number, ecEntry.EpochNumber, "my epoch change entries cannot exceed the current target epoch")

				currentEpoch = true
			},
		})
	}
}

func (et *epochTarget) checkEpochResumed() {
	switch {
	case et.commitState.stopAtSeqNo < et.startingSeqNo:
		et.logger.Log(LevelDebug, "epoch waiting to resume until outstanding checkpoint commits", "epoch_no", et.number)
	case et.commitState.lowWatermark+1 != et.startingSeqNo:
		et.logger.Log(LevelDebug, "epoch waiting for state transfer to complete (and possibly to initiate)", "epoch_no", et.number)
		// we are waiting for state transfer to initiate and complete
	default:
		// There is room to allocate sequences, and the commit
		// state is ready for those sequences to commit, begin
		// processing the epoch.
		et.state = etReady
		et.logger.Log(LevelDebug, "epoch transitioning from resuming to ready", "epoch_no", et.number)
	}

}

// Repeatedly checks the epochTarget.state field and performs the corresponding actions,
// until epochTarget.state stops changing, meaning there is nothing left to be done
// without further input.
func (et *epochTarget) advanceState() *ActionList {
	actions := &ActionList{}
	for {
		oldState := et.state
		switch et.state {
		case etPrepending: // Have sent an epoch-change, but waiting for a quorum
			actions.concat(et.checkEpochQuorum())
		case etPending: // Have a quorum of epoch-change messages, waits on new-epoch
			if et.leaderNewEpoch == nil {
				return actions
			}
			et.logger.Log(LevelDebug, "epoch transitioning from pending to verifying", "epoch_no", et.number)
			et.state = etVerifying
		case etVerifying: // Have a NewEpoch message but it references epoch changes we cannot yet verify
			et.verifyNewEpochState()
		case etFetching: // Have received and verified a new epoch messages, and are waiting to get state
			actions.concat(et.fetchNewEpochState())
		case etEchoing: // Have received and validated a new-epoch, waiting for a quorum of echos
			actions.concat(et.checkNewEpochEchoQuorum())
		case etReadying: // Have received a quorum of echos, waiting a on qourum of readies
			et.checkNewEpochReadyQuorum()
		case etResuming: // We crashed during this epoch, and are waiting for it to resume or fail
			et.checkEpochResumed()
		case etReady: // New epoch is ready to begin
			// TODO, handle case where planned epoch expiration is now
			et.activeEpoch = newActiveEpoch(et.networkNewEpoch.Config, et.persisted, et.nodeBuffers, et.commitState, et.clientTracker, et.myConfig, et.logger)

			actions.concat(et.activeEpoch.advance())

			et.logger.Log(LevelDebug, "epoch transitioning from ready to in progress", "epoch_no", et.number)
			et.state = etInProgress
			for _, id := range et.networkConfig.Nodes {
				et.prestartBuffers[nodeID(id)].iterate(
					func(nodeID, *msgs.Msg) applyable {
						return current // A bit of a hack, just iterating
					},
					func(id nodeID, msg *msgs.Msg) {
						actions.concat(et.activeEpoch.step(nodeID(id), msg))
					},
				)
			}
			actions.concat(et.activeEpoch.drainBuffers())
		case etInProgress: // No pending change
			actions.concat(et.activeEpoch.outstandingReqs.advanceRequests())
			actions.concat(et.activeEpoch.advance())
		case etDone: // This epoch is over, the tracker will send the epoch change
			// TODO, release/empty buffers
		default:
			panic("dev sanity test")
		}
		if et.state == oldState {
			return actions
		}
	}
}

func (et *epochTarget) moveLowWatermark(seqNo uint64) *ActionList {
	if et.state != etInProgress {
		return &ActionList{}
	}

	actions, done := et.activeEpoch.moveLowWatermark(seqNo)
	if done {
		et.logger.Log(LevelDebug, "epoch gracefully transitioning from in progress to done", "epoch_no", et.number)
		et.state = etDone
	}

	return actions
}

func (et *epochTarget) applySuspectMsg(source nodeID) {
	et.suspicions[source] = struct{}{}

	if len(et.suspicions) >= intersectionQuorum(et.networkConfig) {
		et.logger.Log(LevelDebug, "epoch ungracefully transitioning from in progress to done", "epoch_no", et.number)
		et.state = etDone
	}
}

func (et *epochTarget) bucketStatus() (lowWatermark, highWatermark uint64, bucketStatus []*status.Bucket) {
	if et.activeEpoch != nil && len(et.activeEpoch.sequences) != 0 {
		bucketStatus = et.activeEpoch.status()
		lowWatermark = et.activeEpoch.lowWatermark()
		highWatermark = et.activeEpoch.highWatermark()
		return
	}

	if et.state <= etFetching || et.leaderNewEpoch == nil {
		if et.myEpochChange != nil {
			lowWatermark = et.myEpochChange.lowWatermark + 1
			highWatermark = lowWatermark + uint64(2*et.networkConfig.CheckpointInterval) - 1
		}
	} else {
		lowWatermark = et.leaderNewEpoch.NewConfig.StartingCheckpoint.SeqNo + 1
		highWatermark = lowWatermark + uint64(2*et.networkConfig.CheckpointInterval) - 1
	}

	bucketStatus = make([]*status.Bucket, int(et.networkConfig.NumberOfBuckets))
	for i := range bucketStatus {
		bucketStatus[i] = &status.Bucket{
			ID:        uint64(i),
			Sequences: make([]status.SequenceState, int(highWatermark-lowWatermark)/len(bucketStatus)+1),
		}
	}

	setStatus := func(seqNo uint64, status status.SequenceState) {
		bucket := int(seqToBucket(seqNo, et.networkConfig))
		column := int(seqNo-lowWatermark) / len(bucketStatus)
		if column >= len(bucketStatus[bucket].Sequences) {
			// XXX this is a nasty case which can happen sometimes,
			// when we've begun echoing a new epoch, before we have
			// actually executed through the checkpoint selected as
			// the base for the new epoch.  Working on a solution
			// but as this is simply status, ignoring
			return
		}
		bucketStatus[bucket].Sequences[column] = status
	}

	if et.state <= etFetching {
		for seqNo := range et.myEpochChange.qSet {
			if seqNo < lowWatermark {
				continue
			}
			setStatus(seqNo, status.SequencePreprepared)
		}

		for seqNo := range et.myEpochChange.pSet {
			if seqNo < lowWatermark {
				continue
			}
			setStatus(seqNo, status.SequencePrepared)
		}

		for seqNo := lowWatermark; seqNo <= et.commitState.highestCommit; seqNo++ {
			setStatus(seqNo, status.SequenceCommitted)
		}
		return
	}

	for seqNo := lowWatermark; seqNo <= highWatermark; seqNo++ {
		var state status.SequenceState

		if et.state == etEchoing {
			state = status.SequencePreprepared
		}

		if et.state == etReadying {
			state = status.SequencePrepared
		}

		if seqNo <= et.commitState.highestCommit || et.state == etReady {
			state = status.SequenceCommitted
		}

		setStatus(seqNo, state)
	}

	return
}

func (et *epochTarget) status() *status.EpochTarget {
	result := &status.EpochTarget{
		Number:       et.number,
		State:        status.EpochTargetState(et.state),
		EpochChanges: make([]*status.EpochChange, 0, len(et.changes)),
		Echos:        make([]uint64, 0, len(et.echos)),
		Readies:      make([]uint64, 0, len(et.readies)),
		Suspicions:   make([]uint64, 0, len(et.suspicions)),
	}

	for node, change := range et.changes {
		result.EpochChanges = append(result.EpochChanges, change.status(uint64(node)))
	}
	sort.Slice(result.EpochChanges, func(i, j int) bool {
		return result.EpochChanges[i].Source < result.EpochChanges[j].Source
	})

	for _, echoMsgs := range et.echos {
		for node := range echoMsgs {
			result.Echos = append(result.Echos, uint64(node))
		}
	}

	sort.Slice(result.Echos, func(i, j int) bool {
		return result.Echos[i] < result.Echos[j]
	})

	for _, readyMsgs := range et.readies {
		for node := range readyMsgs {
			result.Readies = append(result.Readies, uint64(node))
		}
	}
	sort.Slice(result.Readies, func(i, j int) bool {
		return result.Readies[i] < result.Readies[j]
	})

	for node := range et.suspicions {
		result.Suspicions = append(result.Suspicions, uint64(node))
	}
	sort.Slice(result.Suspicions, func(i, j int) bool {
		return result.Suspicions[i] < result.Suspicions[j]
	})

	if et.leaderNewEpoch != nil {
		result.Leaders = et.leaderNewEpoch.NewConfig.Config.Leaders
	}

	return result
}
