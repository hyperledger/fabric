// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Phase indicates the status of the view
type Phase uint8

// These are the different phases
const (
	COMMITTED = iota
	PROPOSED
	PREPARED
	ABORT
)

// State can save and restore the state
//
//go:generate mockery -dir . -name State -case underscore -output ./mocks/
type State interface {
	// Save saves a message.
	Save(message *protos.SavedMessage) error

	// Restore restores the given view to its latest state
	// before a crash, if applicable.
	Restore(*View) error
}

// Comm adds broadcast to the regular comm interface
type Comm interface {
	api.Comm
	BroadcastConsensus(m *protos.Message)
}

type CheckpointRetriever func() (*protos.Proposal, []*protos.Signature)

// View is responsible for running the view protocol
type View struct {
	// Configuration
	DecisionsPerLeader uint64
	RetrieveCheckpoint CheckpointRetriever
	SelfID             uint64
	N                  uint64
	LeaderID           uint64
	Quorum             int
	Number             uint64
	Decider            Decider
	FailureDetector    FailureDetector
	Sync               Synchronizer
	Logger             api.Logger
	Comm               Comm
	Verifier           api.Verifier
	Signer             api.Signer
	MembershipNotifier api.MembershipNotifier
	ProposalSequence   uint64
	DecisionsInView    uint64
	State              State
	Phase              Phase
	InMsgQSize         int
	// Runtime
	lastVotedProposalByID map[uint64]*protos.Commit
	incMsgs               chan *incMsg
	myProposalSig         *types.Signature
	inFlightProposal      *types.Proposal
	inFlightRequests      []types.RequestInfo
	lastBroadcastSent     *protos.Message
	// Current sequence sent prepare and commit
	currPrepareSent *protos.Message
	currCommitSent  *protos.Message
	// Prev sequence sent prepare and commit
	// to help lagging replicas catch up
	prevPrepareSent *protos.Message
	prevCommitSent  *protos.Message
	// Current proposal
	prePrepare chan *protos.Message
	prepares   *voteSet
	commits    *voteSet
	// Next proposal
	nextPrePrepare chan *protos.Message
	nextPrepares   *voteSet
	nextCommits    *voteSet

	beginPrePrepare    time.Time
	MetricsBlacklist   *MetricsBlacklist
	MetricsView        *MetricsView
	blacklistSupported bool
	abortChan          chan struct{}
	stopOnce           sync.Once
	viewEnded          sync.WaitGroup

	ViewSequences *atomic.Value
}

// Start starts the view
func (v *View) Start() {
	v.stopOnce = sync.Once{}
	v.incMsgs = make(chan *incMsg, v.InMsgQSize)
	v.abortChan = make(chan struct{})
	v.lastVotedProposalByID = make(map[uint64]*protos.Commit)
	v.viewEnded.Add(1)

	v.prePrepare = make(chan *protos.Message, 1)
	v.nextPrePrepare = make(chan *protos.Message, 1)

	v.setupVotes()

	go func() {
		v.run()
	}()
}

func (v *View) setupVotes() {
	// Prepares
	acceptPrepares := func(_ uint64, message *protos.Message) bool {
		return message.GetPrepare() != nil
	}

	v.prepares = &voteSet{
		validVote: acceptPrepares,
	}
	v.prepares.clear(v.N)

	v.nextPrepares = &voteSet{
		validVote: acceptPrepares,
	}
	v.nextPrepares.clear(v.N)

	// Commits
	acceptCommits := func(sender uint64, message *protos.Message) bool {
		commit := message.GetCommit()
		if commit == nil {
			return false
		}
		if commit.Signature == nil {
			return false
		}
		// Sender needs to match the inner signature sender
		return commit.Signature.Signer == sender
	}

	v.commits = &voteSet{
		validVote: acceptCommits,
	}
	v.commits.clear(v.N)

	v.nextCommits = &voteSet{
		validVote: acceptCommits,
	}
	v.nextCommits.clear(v.N)
}

// HandleMessage handles incoming messages
func (v *View) HandleMessage(sender uint64, m *protos.Message) {
	msg := &incMsg{sender: sender, Message: m}
	select {
	case <-v.abortChan:
		return
	case v.incMsgs <- msg:
	}
}

func (v *View) processMsg(sender uint64, m *protos.Message) {
	if v.Stopped() {
		return
	}
	// Ensure view number is equal to our view
	msgViewNum := viewNumber(m)
	msgProposalSeq := proposalSequence(m)

	if msgViewNum != v.Number {
		v.Logger.Warnf("%d got message %v from %d of view %d, expected view %d", v.SelfID, m, sender, msgViewNum, v.Number)
		if sender != v.LeaderID {
			v.discoverIfSyncNeeded(sender, m)
			return
		}
		v.FailureDetector.Complain(v.Number, false)
		// Else, we got a message with a wrong view from the leader.
		if msgViewNum > v.Number {
			v.Sync.Sync()
		}
		v.stop()
		return
	}

	if msgProposalSeq == v.ProposalSequence-1 && v.ProposalSequence > 0 {
		v.handlePrevSeqMessage(msgProposalSeq, sender, m)
		return
	}

	v.Logger.Debugf("%d got message %s from %d with seq %d", v.SelfID, MsgToString(m), sender, msgProposalSeq)
	// This message is either for this proposal or the next one (we might be behind the rest)
	if msgProposalSeq != v.ProposalSequence && msgProposalSeq != v.ProposalSequence+1 {
		v.Logger.Warnf("%d got message from %d with sequence %d but our sequence is %d", v.SelfID, sender, msgProposalSeq, v.ProposalSequence)
		v.discoverIfSyncNeeded(sender, m)
		return
	}

	msgForNextProposal := msgProposalSeq == v.ProposalSequence+1

	if pp := m.GetPrePrepare(); pp != nil {
		v.processPrePrepare(pp, m, msgForNextProposal, sender)
		return
	}

	// Else, it's a prepare or a commit.
	// Ignore votes from ourselves.
	if sender == v.SelfID {
		return
	}

	if prp := m.GetPrepare(); prp != nil {
		if msgForNextProposal {
			v.nextPrepares.registerVote(sender, m)
		} else {
			v.prepares.registerVote(sender, m)
		}
		return
	}

	if cmt := m.GetCommit(); cmt != nil {
		if msgForNextProposal {
			v.nextCommits.registerVote(sender, m)
		} else {
			v.commits.registerVote(sender, m)
		}
		return
	}
}

func (v *View) run() {
	defer v.viewEnded.Done()
	defer func() {
		v.ViewSequences.Store(ViewSequence{
			ProposalSeq: v.ProposalSequence,
			ViewActive:  false,
		})
	}()
	for {
		select {
		case <-v.abortChan:
			return
		case msg := <-v.incMsgs:
			v.processMsg(msg.sender, msg.Message)
		default:
			v.doPhase()
		}
	}
}

func (v *View) doPhase() {
	switch v.Phase {
	case PROPOSED:
		v.Comm.BroadcastConsensus(v.lastBroadcastSent) // broadcast here serves also recovery
		v.Phase = v.processPrepares()
	case PREPARED:
		v.Comm.BroadcastConsensus(v.lastBroadcastSent)
		v.Phase = v.prepared()
	case COMMITTED:
		v.Phase = v.processProposal()
	case ABORT:
		return
	default:
		v.Logger.Panicf("Unknown phase in view : %v", v)
	}

	v.MetricsView.Phase.Set(float64(v.Phase))
}

func (v *View) processPrePrepare(pp *protos.PrePrepare, m *protos.Message, msgForNextProposal bool, sender uint64) {
	if pp.Proposal == nil {
		v.Logger.Warnf("%d got pre-prepare from %d with empty proposal", v.SelfID, sender)
		return
	}
	if sender != v.LeaderID {
		v.Logger.Warnf("%d got pre-prepare from %d but the leader is %d", v.SelfID, sender, v.LeaderID)
		return
	}

	prePrepareChan := v.prePrepare
	currentOrNext := "current"

	if msgForNextProposal {
		prePrepareChan = v.nextPrePrepare
		currentOrNext = "next"
	}

	select {
	case prePrepareChan <- m:
	default:
		v.Logger.Warnf("Got a pre-prepare for %s sequence without processing previous one, dropping message", currentOrNext)
	}
}

func (v *View) prepared() Phase {
	proposal := v.inFlightProposal
	signatures, phase := v.processCommits(proposal)
	if phase == ABORT {
		return ABORT
	}

	seq := v.ProposalSequence

	v.Logger.Infof("%d processed commits for proposal with seq %d", v.SelfID, seq)

	v.MetricsView.CountBatchAll.Add(1)
	v.MetricsView.CountTxsAll.Add(float64(len(v.inFlightRequests)))
	size := 0
	size += len(proposal.Metadata) + len(proposal.Header) + len(proposal.Payload)
	for i := range signatures {
		size += len(signatures[i].Value) + len(signatures[i].Msg)
	}
	v.MetricsView.SizeOfBatch.Add(float64(size))
	v.MetricsView.LatencyBatchProcessing.Observe(time.Since(v.beginPrePrepare).Seconds())

	v.decide(proposal, signatures, v.inFlightRequests)
	return COMMITTED
}

func (v *View) processProposal() Phase {
	v.prevPrepareSent = v.currPrepareSent
	v.prevCommitSent = v.currCommitSent
	v.currPrepareSent = nil
	v.currCommitSent = nil
	v.inFlightProposal = nil
	v.inFlightRequests = nil
	v.lastBroadcastSent = nil

	var proposal types.Proposal
	var receivedProposal *protos.Message
	var prevCommits []*protos.Signature

	var gotPrePrepare bool
	for !gotPrePrepare {
		select {
		case <-v.abortChan:
			return ABORT
		case msg := <-v.incMsgs:
			v.processMsg(msg.sender, msg.Message)
		case msg := <-v.prePrepare:
			gotPrePrepare = true
			receivedProposal = msg
			prePrepare := msg.GetPrePrepare()
			prop := prePrepare.Proposal
			prevCommits = prePrepare.PrevCommitSignatures
			proposal = types.Proposal{
				VerificationSequence: int64(prop.VerificationSequence),
				Metadata:             prop.Metadata,
				Payload:              prop.Payload,
				Header:               prop.Header,
			}
		}
	}

	requests, err := v.verifyProposal(proposal, prevCommits)
	if err != nil {
		v.Logger.Warnf("%d received bad proposal from %d: %v", v.SelfID, v.LeaderID, err)
		v.FailureDetector.Complain(v.Number, false)
		v.Sync.Sync()
		v.stop()
		return ABORT
	}

	v.MetricsView.CountTxsInBatch.Set(float64(len(requests)))
	v.beginPrePrepare = time.Now()

	seq := v.ProposalSequence

	prepareMessage := v.createPrepare(seq, proposal)

	// We are about to send a prepare for a pre-prepare,
	// so we record the pre-prepare.
	savedMsg := &protos.SavedMessage{
		Content: &protos.SavedMessage_ProposedRecord{
			ProposedRecord: &protos.ProposedRecord{
				PrePrepare: receivedProposal.GetPrePrepare(),
				Prepare:    prepareMessage.GetPrepare(),
			},
		},
	}
	if err = v.State.Save(savedMsg); err != nil {
		v.Logger.Panicf("Failed to save message to state, error: %v", err)
	}
	v.lastBroadcastSent = prepareMessage
	v.currPrepareSent = proto.Clone(prepareMessage).(*protos.Message)
	v.currPrepareSent.GetPrepare().Assist = true
	v.inFlightProposal = &proposal
	v.inFlightRequests = requests

	if v.SelfID == v.LeaderID {
		v.Comm.BroadcastConsensus(receivedProposal)
	}

	v.Logger.Infof("Processed proposal with seq %d", seq)
	return PROPOSED
}

func (v *View) createPrepare(seq uint64, proposal types.Proposal) *protos.Message {
	return &protos.Message{
		Content: &protos.Message_Prepare{
			Prepare: &protos.Prepare{
				Seq:    seq,
				View:   v.Number,
				Digest: proposal.Digest(),
			},
		},
	}
}

func (v *View) processPrepares() Phase {
	proposal := v.inFlightProposal
	expectedDigest := proposal.Digest()

	var voterIDs []uint64
	for len(voterIDs) < v.Quorum-1 {
		select {
		case <-v.abortChan:
			return ABORT
		case msg := <-v.incMsgs:
			v.processMsg(msg.sender, msg.Message)
		case vote := <-v.prepares.votes:
			prepare := vote.GetPrepare()
			if prepare.Digest != expectedDigest {
				seq := v.ProposalSequence
				v.Logger.Warnf("Got wrong digest at processPrepares for prepare with seq %d, expecting %v but got %v, we are in seq %d", prepare.Seq, expectedDigest, prepare.Digest, seq)
				continue
			}
			voterIDs = append(voterIDs, vote.sender)
		}
	}

	v.Logger.Infof("%d collected %d prepares from %v", v.SelfID, len(voterIDs), voterIDs)

	// SignProposal returns a types.Signature with the following 3 fields:
	// ID: The integer that represents this node.
	// Value: The signature, encoded according to the specific signature specification.
	// Msg: A succinct representation of the proposal that binds this proposal unequivocally.

	// The block proof consists of the aggregation of all these signatures from 2f+1 commits of different nodes.

	prpFrom := &protos.PreparesFrom{
		Ids: voterIDs,
	}

	prpFromRaw, err := proto.Marshal(prpFrom)
	if err != nil {
		v.Logger.Panicf("Failed marshaling prepares from: %v", err)
	}

	v.myProposalSig = v.Signer.SignProposal(*proposal, prpFromRaw)

	seq := v.ProposalSequence

	commitMsg := &protos.Message{
		Content: &protos.Message_Commit{
			Commit: &protos.Commit{
				View:   v.Number,
				Digest: expectedDigest,
				Seq:    seq,
				Signature: &protos.Signature{
					Signer: v.myProposalSig.ID,
					Value:  v.myProposalSig.Value,
					Msg:    v.myProposalSig.Msg,
				},
			},
		},
	}

	preparedProof := &protos.SavedMessage{
		Content: &protos.SavedMessage_Commit{
			Commit: commitMsg,
		},
	}

	// We received enough prepares to send a commit.
	// Save the commit message we are about to send.
	if err = v.State.Save(preparedProof); err != nil {
		v.Logger.Panicf("Failed to save message to state, error: %v", err)
	}
	v.currCommitSent = proto.Clone(commitMsg).(*protos.Message)
	v.currCommitSent.GetCommit().Assist = true
	v.lastBroadcastSent = commitMsg

	v.Logger.Infof("Processed prepares for proposal with seq %d", seq)
	return PREPARED
}

func (v *View) processCommits(proposal *types.Proposal) ([]types.Signature, Phase) {
	var signatures []types.Signature

	signatureCollector := &voteVerifier{
		validVotes:     make(chan types.Signature, cap(v.commits.votes)),
		expectedDigest: proposal.Digest(),
		proposal:       proposal,
		v:              v,
	}

	var voterIDs []uint64

	for len(signatures) < v.Quorum-1 {
		select {
		case <-v.abortChan:
			return nil, ABORT
		case msg := <-v.incMsgs:
			v.processMsg(msg.sender, msg.Message)
		case vote := <-v.commits.votes:
			// Valid votes end up written into the 'validVotes' channel.
			go func(vote *protos.Message) {
				signatureCollector.verifyVote(vote)
			}(vote.Message)
		case signature := <-signatureCollector.validVotes:
			signatures = append(signatures, signature)
			voterIDs = append(voterIDs, signature.ID)
		}
	}

	v.Logger.Infof("%d collected %d commits from %v", v.SelfID, len(signatures), voterIDs)

	return signatures, COMMITTED
}

func (v *View) verifyProposal(proposal types.Proposal, prevCommits []*protos.Signature) ([]types.RequestInfo, error) {
	// Verify proposal has correct structure and contains authorized requests.
	requests, err := v.Verifier.VerifyProposal(proposal)
	if err != nil {
		v.Logger.Warnf("Received bad proposal: %v", err)
		return nil, err
	}

	// Verify proposal's metadata is valid.
	md := &protos.ViewMetadata{}
	if err = proto.Unmarshal(proposal.Metadata, md); err != nil {
		return nil, err
	}

	if md.ViewId != v.Number {
		v.Logger.Warnf("Expected view number %d but got %d", v.Number, md.ViewId)
		return nil, errors.New("invalid view number")
	}

	if md.LatestSequence != v.ProposalSequence {
		v.Logger.Warnf("Expected proposal sequence %d but got %d", v.ProposalSequence, md.LatestSequence)
		return nil, errors.New("invalid proposal sequence")
	}

	if md.DecisionsInView != v.DecisionsInView {
		v.Logger.Warnf("Expected decisions in view %d but got %d", v.DecisionsInView, md.DecisionsInView)
		return nil, errors.New("invalid decisions in view")
	}

	expectedSeq := v.Verifier.VerificationSequence()
	if uint64(proposal.VerificationSequence) != expectedSeq {
		v.Logger.Warnf("Expected verification sequence %d but got %d", expectedSeq, proposal.VerificationSequence)
		return nil, errors.New("verification sequence mismatch")
	}

	prepareAcknowledgements, err := v.verifyPrevCommitSignatures(prevCommits, expectedSeq)
	if err != nil {
		return nil, err
	}

	if err = v.verifyBlacklist(prevCommits, expectedSeq, md.BlackList, prepareAcknowledgements); err != nil {
		return nil, err
	}

	// Check that the metadata contains a digest of the previous commit signatures
	prevCommitDigest := CommitSignaturesDigest(prevCommits)
	if !bytes.Equal(prevCommitDigest, md.PrevCommitSignatureDigest) && v.DecisionsPerLeader > 0 {
		return nil, errors.Errorf("prev commit signatures received from leader mismatches the metadata digest")
	}

	return requests, nil
}

func (v *View) verifyPrevCommitSignatures(prevCommitSignatures []*protos.Signature, currVerificationSeq uint64) (map[uint64]*protos.PreparesFrom, error) {
	prevPropRaw, _ := v.RetrieveCheckpoint()
	prevProposalMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prevPropRaw.Metadata, prevProposalMetadata); err != nil {
		v.Logger.Panicf("Couldn't unmarshal the previous persisted proposal metadata: %v", err)
	}

	v.Logger.Debugf("Previous proposal verification sequence: %d, current verification sequence: %d", prevPropRaw.VerificationSequence, currVerificationSeq)
	if prevPropRaw.VerificationSequence != currVerificationSeq {
		v.Logger.Infof("Skipping verifying prev commit signatures due to verification sequence advancing from %d to %d",
			prevPropRaw.VerificationSequence, currVerificationSeq)
		return nil, nil
	}

	prepareAcknowledgements := make(map[uint64]*protos.PreparesFrom)

	prevProp := types.Proposal{
		VerificationSequence: int64(prevPropRaw.VerificationSequence),
		Metadata:             prevPropRaw.Metadata,
		Payload:              prevPropRaw.Payload,
		Header:               prevPropRaw.Header,
	}

	// All previous commit signatures should be verifiable
	for _, sig := range prevCommitSignatures {
		aux, err := v.Verifier.VerifyConsenterSig(types.Signature{
			ID:    sig.Signer,
			Msg:   sig.Msg,
			Value: sig.Value,
		}, prevProp)
		if err != nil {
			return nil, errors.Errorf("failed verifying consenter signature of %d: %v", sig.Signer, err)
		}
		prpf := &protos.PreparesFrom{}
		if err = proto.Unmarshal(aux, prpf); err != nil {
			return nil, errors.Errorf("failed unmarshaling auxiliary input from %d: %v", sig.Signer, err)
		}
		prepareAcknowledgements[sig.Signer] = prpf
	}

	return prepareAcknowledgements, nil
}

func (v *View) verifyBlacklist(prevCommitSignatures []*protos.Signature, currVerificationSeq uint64, pendingBlacklist []uint64, prepareAcknowledgements map[uint64]*protos.PreparesFrom) error {
	if v.DecisionsPerLeader == 0 {
		v.Logger.Debugf("DecisionsPerLeader is 0, hence leader rotation is inactive")
		if len(pendingBlacklist) > 0 {
			v.Logger.Warnf("Blacklist cannot be non-empty (%v) if rotation is inactive", pendingBlacklist)
			return errors.Errorf("rotation is inactive but blacklist is not empty: %v", pendingBlacklist)
		}
		return nil
	}

	prevPropRaw, myLastCommitSignatures := v.RetrieveCheckpoint()
	prevProposalMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prevPropRaw.Metadata, prevProposalMetadata); err != nil {
		v.Logger.Panicf("Couldn't unmarshal the previous persisted proposal metadata: %v", err)
	}

	v.Logger.Debugf("Previous proposal verification sequence: %d, current verification sequence: %d", prevPropRaw.VerificationSequence, currVerificationSeq)
	if prevPropRaw.VerificationSequence != currVerificationSeq {
		// If there has been a reconfiguration, black list should remain the same
		if !equalIntLists(prevProposalMetadata.BlackList, pendingBlacklist) {
			return errors.Errorf("blacklist changed (%v --> %v) during reconfiguration", prevProposalMetadata.BlackList, pendingBlacklist)
		}
		v.Logger.Infof("Skipping verifying prev commits due to verification sequence advancing from %d to %d",
			prevPropRaw.VerificationSequence, currVerificationSeq)
		return nil
	}

	if v.MembershipNotifier != nil && v.MembershipNotifier.MembershipChange() {
		// If there has been a membership change, black list should remain the same
		if !equalIntLists(prevProposalMetadata.BlackList, pendingBlacklist) {
			return errors.Errorf("blacklist changed (%v --> %v) during membership change", prevProposalMetadata.BlackList, pendingBlacklist)
		}
		v.Logger.Infof("Skipping verifying prev commits due to membership change")
		return nil
	}

	_, f := computeQuorum(v.N)

	if v.blacklistingSupported(f, myLastCommitSignatures) && len(prevCommitSignatures) < len(myLastCommitSignatures) {
		return errors.Errorf("only %d out of %d required previous commits is included in pre-prepare",
			len(prevCommitSignatures), len(myLastCommitSignatures))
	}

	// We previously verified the previous commit signatures, now we need to ensure that the blacklist
	// of this proposal is obtained by applying the deterministic blacklist maintenance algorithm
	// on the blacklist of the previous proposal which has been committed.

	blacklist := &blacklist{
		currentLeader:      v.LeaderID,
		leaderRotation:     v.DecisionsPerLeader > 0,
		n:                  v.N,
		prevMD:             prevProposalMetadata,
		decisionsPerLeader: v.DecisionsPerLeader,
		preparesFrom:       prepareAcknowledgements,
		f:                  f,
		logger:             v.Logger,
		metricsBlacklist:   v.MetricsBlacklist,
		nodes:              v.Comm.Nodes(),
		currView:           v.Number,
	}

	expectedBlacklist := blacklist.computeUpdate()
	if !equalIntLists(pendingBlacklist, expectedBlacklist) {
		return errors.Errorf("proposed blacklist %v differs from expected %v blacklist", pendingBlacklist, expectedBlacklist)
	}

	return nil
}

func (v *View) handlePrevSeqMessage(msgProposalSeq, sender uint64, m *protos.Message) {
	if m.GetPrePrepare() != nil {
		v.Logger.Warnf("Got pre-prepare for sequence %d but we're in sequence %d", msgProposalSeq, v.ProposalSequence)
		return
	}
	msgType := "prepare"
	if m.GetCommit() != nil {
		msgType = "commit"
	}

	var found bool

	switch msgType {
	case "prepare":
		// This is an assist message, we don't need to reply to it.
		if m.GetPrepare().Assist {
			return
		}
		if v.prevPrepareSent != nil {
			v.Comm.SendConsensus(sender, v.prevPrepareSent)
			found = true
		}
	case "commit":
		// This is an assist message, we don't need to reply to it.
		if m.GetCommit().Assist {
			return
		}
		if v.prevCommitSent != nil {
			v.Comm.SendConsensus(sender, v.prevCommitSent)
			found = true
		}
	}

	prevMsgFound := fmt.Sprintf("but didn't have a previous %s to send back.", msgType)
	if found {
		prevMsgFound = fmt.Sprintf("sent back previous %s.", msgType)
	}
	v.Logger.Debugf("Got %s for previous sequence (%d) from %d, %s", msgType, msgProposalSeq, sender, prevMsgFound)
}

func (v *View) discoverIfSyncNeeded(sender uint64, m *protos.Message) {
	// We're only interested in commit messages.
	commit := m.GetCommit()
	if commit == nil {
		return
	}

	// To commit a block we need 2f + 1 votes.
	// at least f+1 of them are honest and will broadcast
	// their commits to votes to everyone including us.
	// In each such a threshold of f+1 votes there is at least
	// a single honest node that prepared for a proposal
	// which we apparently missed.
	_, f := computeQuorum(v.N)
	threshold := f + 1

	v.lastVotedProposalByID[sender] = commit

	v.Logger.Debugf("Got commit of seq %d in view %d from %d while being in seq %d in view %d",
		commit.Seq, commit.View, sender, v.ProposalSequence, v.Number)

	// If we haven't reached a threshold of proposals yet, abort.
	if len(v.lastVotedProposalByID) < threshold {
		return
	}

	// Make a histogram out of all current seen votes.
	countsByVotes := make(map[proposalInfo]int)
	for _, vote := range v.lastVotedProposalByID {
		info := proposalInfo{
			digest: vote.Digest,
			view:   vote.View,
			seq:    vote.Seq,
		}
		countsByVotes[info]++
	}

	// Check if there is a <digest, view, seq> that collected a threshold of votes,
	// and that sequence is higher than our current sequence, or our view is different.
	for vote, count := range countsByVotes {
		if count < threshold {
			continue
		}

		// Disregard votes for past views.
		if vote.view < v.Number {
			continue
		}

		// Disregard votes for past sequences for this view.
		if vote.seq <= v.ProposalSequence && vote.view == v.Number {
			continue
		}

		v.Logger.Warnf("Seen %d votes for digest %s in view %d, sequence %d but I am in view %d and seq %d",
			count, vote.digest, vote.view, vote.seq, v.Number, v.ProposalSequence)
		v.stop()
		v.Sync.Sync()
		return
	}
}

type voteVerifier struct {
	v              *View
	proposal       *types.Proposal
	expectedDigest string
	validVotes     chan types.Signature
}

func (vv *voteVerifier) verifyVote(vote *protos.Message) {
	commit := vote.GetCommit()
	if commit.Digest != vv.expectedDigest {
		vv.v.Logger.Warnf("Got wrong digest at processCommits for seq %d", commit.Seq)
		return
	}

	_, err := vv.v.Verifier.VerifyConsenterSig(types.Signature{
		ID:    commit.Signature.Signer,
		Value: commit.Signature.Value,
		Msg:   commit.Signature.Msg,
	}, *vv.proposal)
	if err != nil {
		vv.v.Logger.Warnf("Couldn't verify %d's signature: %v", commit.Signature.Signer, err)
		return
	}

	vv.validVotes <- types.Signature{
		ID:    commit.Signature.Signer,
		Value: commit.Signature.Value,
		Msg:   commit.Signature.Msg,
	}
}

func (v *View) decide(proposal *types.Proposal, signatures []types.Signature, requests []types.RequestInfo) {
	v.Logger.Infof("Deciding on seq %d", v.ProposalSequence)
	v.ViewSequences.Store(ViewSequence{ProposalSeq: v.ProposalSequence, ViewActive: true})
	// first make preparations for the next sequence so that the view will be ready to continue right after delivery
	v.startNextSeq()
	signatures = append(signatures, *v.myProposalSig)
	v.Decider.Decide(*proposal, signatures, requests)
}

func (v *View) startNextSeq() {
	prevSeq := v.ProposalSequence

	v.ProposalSequence++
	v.DecisionsInView++

	nextSeq := v.ProposalSequence

	v.MetricsView.ProposalSequence.Set(float64(v.ProposalSequence))
	v.MetricsView.DecisionsInView.Set(float64(v.DecisionsInView))

	v.Logger.Infof("Sequence: %d-->%d", prevSeq, nextSeq)

	// swap next prePrepare
	tmp := v.prePrepare
	v.prePrepare = v.nextPrePrepare
	// clear tmp
	for len(tmp) > 0 {
		<-tmp
	}
	tmp = make(chan *protos.Message, 1)
	v.nextPrePrepare = tmp

	// swap next prepares
	tmpVotes := v.prepares
	v.prepares = v.nextPrepares
	tmpVotes.clear(v.N)
	v.nextPrepares = tmpVotes

	// swap next commits
	tmpVotes = v.commits
	v.commits = v.nextCommits
	tmpVotes.clear(v.N)
	v.nextCommits = tmpVotes
}

// GetMetadata returns the current sequence and view number (in a marshaled ViewMetadata protobuf message)
func (v *View) GetMetadata() []byte {
	metadata := &protos.ViewMetadata{
		ViewId:          v.Number,
		LatestSequence:  v.ProposalSequence,
		DecisionsInView: v.DecisionsInView,
	}

	var (
		prevSigs []*protos.Signature
		prevProp *protos.Proposal
	)
	verificationSeq := v.Verifier.VerificationSequence()

	prevProp, prevSigs = v.RetrieveCheckpoint()

	prevMD := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prevProp.Metadata, prevMD); err != nil {
		v.Logger.Panicf("Attempted to propose a proposal with invalid unchanged previous proposal view metadata: %v", err)
	}

	metadata.BlackList = prevMD.BlackList

	metadata = v.metadataWithUpdatedBlacklist(metadata, verificationSeq, prevProp, prevSigs)
	metadata = v.bindCommitSignaturesToProposalMetadata(metadata, prevSigs)

	return MarshalOrPanic(metadata)
}

func (v *View) metadataWithUpdatedBlacklist(metadata *protos.ViewMetadata, verificationSeq uint64, prevProp *protos.Proposal, prevSigs []*protos.Signature) *protos.ViewMetadata {
	var membershipChange bool
	if v.MembershipNotifier != nil {
		membershipChange = v.MembershipNotifier.MembershipChange()
	}

	if verificationSeq == prevProp.VerificationSequence && !membershipChange {
		v.Logger.Debugf("Proposing proposal %d with verification sequence of %d and %d commit signatures",
			v.ProposalSequence, verificationSeq, len(prevSigs))
		return v.updateBlacklistMetadata(metadata, prevSigs, prevProp.Metadata)
	}

	if verificationSeq != prevProp.VerificationSequence {
		v.Logger.Infof("Skipping updating blacklist due to verification sequence changing from %d to %d",
			prevProp.VerificationSequence, verificationSeq)
	}
	if membershipChange {
		v.Logger.Infof("Skipping updating blacklist due to membership change")
	}

	return metadata
}

// Propose broadcasts a prePrepare message with the given proposal
func (v *View) Propose(proposal types.Proposal) {
	_, prevSigs := v.RetrieveCheckpoint()

	seq := v.ProposalSequence
	msg := &protos.Message{
		Content: &protos.Message_PrePrepare{
			PrePrepare: &protos.PrePrepare{
				View: v.Number,
				Seq:  seq,
				Proposal: &protos.Proposal{
					Header:               proposal.Header,
					Payload:              proposal.Payload,
					Metadata:             proposal.Metadata,
					VerificationSequence: uint64(proposal.VerificationSequence),
				},
				PrevCommitSignatures: prevSigs,
			},
		},
	}
	// Send the proposal to yourself in order to pre-prepare yourself and record
	// it in the WAL before sending it to other nodes.
	v.HandleMessage(v.LeaderID, msg)
	v.Logger.Debugf("Proposing proposal sequence %d in view %d", seq, v.Number)
}

func (v *View) bindCommitSignaturesToProposalMetadata(metadata *protos.ViewMetadata, prevSigs []*protos.Signature) *protos.ViewMetadata {
	if v.DecisionsPerLeader == 0 {
		v.Logger.Debugf("Leader rotation is disabled, will not bind signatures to proposals")
		return metadata
	}
	metadata.PrevCommitSignatureDigest = CommitSignaturesDigest(prevSigs)

	if len(metadata.PrevCommitSignatureDigest) == 0 {
		v.Logger.Debugf("No previous commit signatures detected")
	} else {
		v.Logger.Debugf("Bound %d commit signatures to proposal", len(prevSigs))
	}
	return metadata
}

func (v *View) stop() {
	v.stopOnce.Do(func() {
		if v.abortChan == nil {
			return
		}
		close(v.abortChan)
	})
}

// Abort forces the view to end
func (v *View) Abort() {
	v.stop()
	v.viewEnded.Wait()
}

func (v *View) Stopped() bool {
	select {
	case <-v.abortChan:
		return true
	default:
		return false
	}
}

func (v *View) GetLeaderID() uint64 {
	return v.LeaderID
}

func (v *View) updateBlacklistMetadata(metadata *protos.ViewMetadata, prevSigs []*protos.Signature, prevMetadata []byte) *protos.ViewMetadata {
	if v.DecisionsPerLeader == 0 {
		v.Logger.Debugf("Rotation is disabled, setting blacklist to be empty")
		metadata.BlackList = nil
		return metadata
	}

	preparesFrom := make(map[uint64]*protos.PreparesFrom)

	for _, sig := range prevSigs {
		aux := v.Verifier.AuxiliaryData(sig.Msg)
		prpf := &protos.PreparesFrom{}
		if err := proto.Unmarshal(aux, prpf); err != nil {
			v.Logger.Panicf("Failed unmarshalling auxiliary data from previously persisted signatures: %v", err)
		}
		preparesFrom[sig.Signer] = prpf
	}

	prevMD := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prevMetadata, prevMD); err != nil {
		v.Logger.Panicf("Attempted to propose a proposal with invalid previous proposal view metadata: %v", err)
	}

	_, f := computeQuorum(v.N)

	blacklist := &blacklist{
		currentLeader:      v.LeaderID,
		leaderRotation:     v.DecisionsPerLeader > 0,
		currView:           metadata.ViewId,
		prevMD:             prevMD,
		nodes:              v.Comm.Nodes(),
		f:                  f,
		n:                  v.N,
		logger:             v.Logger,
		metricsBlacklist:   v.MetricsBlacklist,
		preparesFrom:       preparesFrom,
		decisionsPerLeader: v.DecisionsPerLeader,
	}
	metadata.BlackList = blacklist.computeUpdate()
	return metadata
}

func (v *View) blacklistingSupported(f int, myLastCommitSignatures []*protos.Signature) bool {
	// Once we blacklist, there is no way back. This is a one way trip, unless we downgrade the version
	// in all nodes and view change.
	if v.blacklistSupported {
		return true
	}
	// We wish to find whether there are f+1 witnesses for blacklisting being
	// activated among the signed commits of the previous proposal.
	var count int
	for _, commitSig := range myLastCommitSignatures {
		aux := v.Verifier.AuxiliaryData(commitSig.Msg)
		if len(aux) > 0 {
			count++
		}
	}

	v.Logger.Debugf("Found %d out of %d required witnesses for auxiliary data", count, f+1)

	blacklistSupported := count > f

	// We cache the result in case it is 'true'.
	// Subsequent invocations will skip the parsing.
	v.blacklistSupported = v.blacklistSupported || blacklistSupported
	return blacklistSupported
}
