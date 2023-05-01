// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"encoding/base64"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/metrics/disabled"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ViewController controls the view
//
//go:generate mockery -dir . -name ViewController -case underscore -output ./mocks/
type ViewController interface {
	ViewChanged(newViewNumber uint64, newProposalSequence uint64)
	AbortView(view uint64)
}

// Pruner prunes revoked requests
//
//go:generate mockery -dir . -name Pruner -case underscore -output ./mocks/
type Pruner interface {
	MaybePruneRevokedRequests()
}

// RequestsTimer controls requests
//
//go:generate mockery -dir . -name RequestsTimer -case underscore -output ./mocks/
type RequestsTimer interface {
	StopTimers()
	RestartTimers()
	RemoveRequest(request types.RequestInfo) error
}

type change struct {
	view     uint64
	stopView bool
}

// ViewChanger is responsible for running the view change protocol
type ViewChanger struct {
	// Configuration
	SelfID             uint64
	NodesList          []uint64
	N                  uint64
	f                  int
	quorum             int
	SpeedUpViewChange  bool
	LeaderRotation     bool
	DecisionsPerLeader uint64

	Logger       api.Logger
	Comm         Comm
	Signer       api.Signer
	Verifier     api.Verifier
	Application  api.Application
	Synchronizer Synchronizer

	Checkpoint *types.Checkpoint
	InFlight   *InFlightData
	State      State

	Controller    ViewController
	RequestsTimer RequestsTimer
	Pruner        Pruner

	// for the in flight proposal view
	ViewSequences      *atomic.Value
	inFlightDecideChan chan struct{}
	inFlightSyncChan   chan struct{}
	inFlightView       *View
	inFlightViewLock   sync.RWMutex

	Ticker              <-chan time.Time
	lastTick            time.Time
	ResendTimeout       time.Duration
	lastResend          time.Time
	ViewChangeTimeout   time.Duration
	startViewChangeTime time.Time
	checkTimeout        bool
	backOffFactor       uint64

	// Runtime
	MetricsViewChange         *MetricsViewChange
	MetricsBlacklist          *MetricsBlacklist
	MetricsView               *MetricsView
	Restore                   chan struct{}
	InMsqQSize                int
	incMsgs                   chan *incMsg
	viewChangeMsgs            *voteSet
	viewDataMsgs              *voteSet
	nvs                       *nextViews
	realView                  uint64
	currView                  uint64
	nextView                  uint64
	startChangeChan           chan *change
	informChan                chan uint64
	committedDuringViewChange *protos.ViewMetadata

	stopOnce sync.Once
	stopChan chan struct{}
	vcDone   sync.WaitGroup

	ControllerStartedWG sync.WaitGroup
}

// Start the view changer
func (v *ViewChanger) Start(startViewNumber uint64) {
	v.incMsgs = make(chan *incMsg, v.InMsqQSize)
	v.startChangeChan = make(chan *change, 2)
	v.informChan = make(chan uint64, 1)

	if v.MetricsViewChange == nil {
		v.MetricsViewChange = NewMetricsViewChange(api.NewCustomerProvider(&disabled.Provider{}))
	}

	v.quorum, v.f = computeQuorum(v.N)

	v.stopChan = make(chan struct{})
	v.stopOnce = sync.Once{}
	v.vcDone.Add(1)

	v.setupVotes()

	v.nvs = &nextViews{}
	v.nvs.clear()
	// set without locking
	v.currView = startViewNumber
	v.realView = v.currView
	v.nextView = v.currView
	v.MetricsViewChange.CurrentView.Set(float64(v.currView))
	v.MetricsViewChange.RealView.Set(float64(v.realView))
	v.MetricsViewChange.NextView.Set(float64(v.nextView))

	v.lastTick = time.Now()
	v.lastResend = v.lastTick

	v.backOffFactor = 1

	v.inFlightDecideChan = make(chan struct{})
	v.inFlightSyncChan = make(chan struct{})

	go func() {
		defer v.vcDone.Done()
		v.ControllerStartedWG.Wait()
		v.run()
	}()
}

func (v *ViewChanger) setupVotes() {
	// view change
	acceptViewChange := func(_ uint64, message *protos.Message) bool {
		return message.GetViewChange() != nil
	}
	v.viewChangeMsgs = &voteSet{
		validVote: acceptViewChange,
	}
	v.viewChangeMsgs.clear(v.N)

	// view data
	acceptViewData := func(_ uint64, message *protos.Message) bool {
		return message.GetViewData() != nil
	}
	v.viewDataMsgs = &voteSet{
		validVote: acceptViewData,
	}
	v.viewDataMsgs.clear(v.N)
}

func (v *ViewChanger) close() {
	v.stopOnce.Do(
		func() {
			select {
			case <-v.stopChan:
				return
			default:
				close(v.stopChan)
			}
		},
	)
}

// Stop the view changer
func (v *ViewChanger) Stop() {
	v.close()
	v.vcDone.Wait()
}

// HandleMessage passes a message to the view changer
func (v *ViewChanger) HandleMessage(sender uint64, m *protos.Message) {
	msg := &incMsg{sender: sender, Message: m}
	select {
	case <-v.stopChan:
		return
	case v.incMsgs <- msg:
	}
}

func (v *ViewChanger) run() {
	for {
		select {
		case <-v.stopChan:
			return
		case changeMsg := <-v.startChangeChan:
			v.startViewChange(changeMsg)
		case msg := <-v.incMsgs:
			v.processMsg(msg.sender, msg.Message)
		case now := <-v.Ticker:
			v.lastTick = now
			v.checkIfResendViewChange(now)
			v.checkIfTimeout(now)
		case info := <-v.informChan:
			v.informNewView(info)
		case <-v.Restore:
			v.processViewChangeMsg(true)
		}
	}
}

func (v *ViewChanger) getLeader() uint64 {
	return getLeaderID(v.currView, v.N, v.NodesList, v.LeaderRotation, 0, v.DecisionsPerLeader, v.blacklist())
}

func (v *ViewChanger) checkIfResendViewChange(now time.Time) {
	nextTimeout := v.lastResend.Add(v.ResendTimeout)
	if nextTimeout.After(now) { // check if it is time to resend
		return
	}
	if v.checkTimeout { // during view change process
		msg := &protos.Message{
			Content: &protos.Message_ViewChange{
				ViewChange: &protos.ViewChange{
					NextView: v.nextView,
				},
			},
		}
		v.Comm.BroadcastConsensus(msg)
		v.Logger.Debugf("Node %d resent a view change message with next view %d", v.SelfID, v.nextView)
		v.lastResend = now // update last resend time, or at least last time we checked if we should resend
	}
}

func (v *ViewChanger) checkIfTimeout(now time.Time) bool {
	if !v.checkTimeout {
		return false
	}
	nextTimeout := v.startViewChangeTime.Add(v.ViewChangeTimeout * time.Duration(v.backOffFactor))
	if nextTimeout.After(now) { // check if timeout has passed
		return false
	}
	v.Logger.Debugf("Node %d got a view change timeout, the current view is %d", v.SelfID, v.currView)
	v.checkTimeout = false // stop timeout for now, a new one will start when a new view change begins
	v.backOffFactor++      // next timeout will be longer
	// the timeout has passed, something went wrong, try sync and complain
	v.Logger.Debugf("Node %d is calling sync because it got a view change timeout", v.SelfID)
	v.Synchronizer.Sync()
	v.StartViewChange(v.currView, false) // don't stop the view, the sync maybe created a good view
	return true
}

func (v *ViewChanger) processMsg(sender uint64, m *protos.Message) {
	// viewChange message
	if vc := m.GetViewChange(); vc != nil {
		v.Logger.Debugf("Node %d is processing a view change message %v from %d with next view %d", v.SelfID, m, sender, vc.NextView)
		v.nvs.registerNext(vc.NextView, sender)
		// check view number
		if vc.NextView == v.currView+1 { // accept view change only to immediate next view number
			v.viewChangeMsgs.registerVote(sender, m)
			v.processViewChangeMsg(false)
			return
		}
		if v.nextView == v.currView+1 && // node has already started view change with last view
			vc.NextView > v.realView &&
			vc.NextView < v.currView+1 &&
			v.nvs.sendRecv(vc.NextView, sender) {
			// Let's help the lagging nodes.
			msg := &protos.Message{
				Content: &protos.Message_ViewChange{
					ViewChange: &protos.ViewChange{
						NextView: vc.NextView,
					},
				},
			}
			v.Comm.BroadcastConsensus(msg)
			v.Logger.Warnf("Node %d got viewChange message %v from %d with view %d, expected view %d, help the lagging nodes", v.SelfID, m, sender, vc.NextView, v.currView+1)
			return
		}
		v.Logger.Warnf("Node %d got viewChange message %v from %d with view %d, expected view %d", v.SelfID, m, sender, vc.NextView, v.currView+1)
		return
	}

	// viewData message
	if vd := m.GetViewData(); vd != nil {
		v.Logger.Debugf("Node %d is processing a view data message %s from %d", v.SelfID, MsgToString(m), sender)
		if !v.validateViewDataMsg(vd, sender) {
			return
		}
		v.viewDataMsgs.registerVote(sender, m)
		v.processViewDataMsg()
		return
	}

	// newView message
	if nv := m.GetNewView(); nv != nil {
		v.Logger.Debugf("Node %d is processing a new view message %s from %d", v.SelfID, MsgToString(m), sender)
		leader := v.getLeader()
		if sender != leader {
			v.Logger.Warnf("Node %d got newView message %v from %d, expected sender to be %d the next leader", v.SelfID, MsgToString(m), sender, leader)
			return
		}
		v.processNewViewMsg(nv)
	}
}

// InformNewView tells the view changer to advance to a new view number
func (v *ViewChanger) InformNewView(view uint64) {
	select {
	case v.informChan <- view:
	case <-v.stopChan:
		return
	}
}

func (v *ViewChanger) informNewView(view uint64) {
	if view < v.currView {
		v.Logger.Debugf("Node %d was informed of view %d, but the current view is %d", v.SelfID, view, v.currView)
		return
	}
	v.Logger.Debugf("Node %d was informed of a new view %d", v.SelfID, view)
	v.currView = view
	v.realView = v.currView
	v.nextView = v.currView
	v.MetricsViewChange.CurrentView.Set(float64(v.currView))
	v.MetricsViewChange.RealView.Set(float64(v.realView))
	v.MetricsViewChange.NextView.Set(float64(v.nextView))
	v.nvs.clear()
	v.viewChangeMsgs.clear(v.N)
	v.viewDataMsgs.clear(v.N)
	v.checkTimeout = false
	v.backOffFactor = 1 // reset
	v.RequestsTimer.RestartTimers()
}

// StartViewChange initiates a view change
func (v *ViewChanger) StartViewChange(view uint64, stopView bool) {
	select {
	case v.startChangeChan <- &change{view: view, stopView: stopView}:
	default:
	}
}

// StartViewChange stops current view and timeouts, and broadcasts a view change message to all
func (v *ViewChanger) startViewChange(change *change) {
	if change.view < v.currView { // this is about an old view
		v.Logger.Debugf("Node %d has a view change request with an old view %d, while the current view is %d", v.SelfID, change.view, v.currView)
		return
	}
	if v.nextView == v.currView+1 {
		v.Logger.Debugf("Node %d has already started view change with last view %d", v.SelfID, v.currView)
		v.checkTimeout = true // make sure timeout is checked anyway
		return
	}
	v.nextView = v.currView + 1
	v.MetricsViewChange.NextView.Set(float64(v.nextView))
	v.RequestsTimer.StopTimers()
	msg := &protos.Message{
		Content: &protos.Message_ViewChange{
			ViewChange: &protos.ViewChange{
				NextView: v.nextView,
			},
		},
	}
	v.Comm.BroadcastConsensus(msg)
	v.Logger.Debugf("Node %d started view change, last view is %d", v.SelfID, v.currView)
	if change.stopView {
		v.Controller.AbortView(v.currView) // abort the current view when joining view change
	}
	v.startViewChangeTime = v.lastTick
	v.checkTimeout = true
}

func (v *ViewChanger) processViewChangeMsg(restore bool) {
	if ((uint64(len(v.viewChangeMsgs.voted)) == uint64(v.f+1)) && v.SpeedUpViewChange) || restore { // join view change
		v.Logger.Debugf("Node %d is joining view change, last view is %d", v.SelfID, v.currView)
		v.startViewChange(&change{v.currView, true})
	}
	if (len(v.viewChangeMsgs.voted) < v.quorum-1) && !restore {
		return
	}
	// send view data
	if !v.SpeedUpViewChange {
		v.Logger.Debugf("Node %d is joining view change, last view is %d", v.SelfID, v.currView)
		v.startViewChange(&change{v.currView, true})
	}
	if !restore {
		msgToSave := &protos.SavedMessage{
			Content: &protos.SavedMessage_ViewChange{
				ViewChange: &protos.ViewChange{
					NextView: v.currView,
				},
			},
		}
		if err := v.State.Save(msgToSave); err != nil {
			v.Logger.Panicf("Failed to save message to state, error: %v", err)
		}
	}
	v.currView = v.nextView
	v.MetricsViewChange.CurrentView.Set(float64(v.currView))
	v.viewChangeMsgs.clear(v.N)
	v.viewDataMsgs.clear(v.N) // clear because currView changed
	msg := v.prepareViewDataMsg()
	leader := v.getLeader()
	if leader == v.SelfID {
		v.viewDataMsgs.registerVote(v.SelfID, msg)
	} else {
		v.Comm.SendConsensus(leader, msg)
	}
	v.Logger.Debugf("Node %d sent view data msg, with next view %d, to the new leader %d", v.SelfID, v.currView, leader)
}

func (v *ViewChanger) prepareViewDataMsg() *protos.Message {
	lastDecision, lastDecisionSignatures := v.Checkpoint.Get()
	inFlight := v.getInFlight(lastDecision)
	prepared := v.InFlight.IsInFlightPrepared()
	vd := &protos.ViewData{
		NextView:               v.currView,
		LastDecision:           lastDecision,
		LastDecisionSignatures: lastDecisionSignatures,
		InFlightProposal:       inFlight,
		InFlightPrepared:       prepared,
	}
	vdBytes := MarshalOrPanic(vd)
	sig := v.Signer.Sign(vdBytes)
	msg := &protos.Message{
		Content: &protos.Message_ViewData{
			ViewData: &protos.SignedViewData{
				RawViewData: vdBytes,
				Signer:      v.SelfID,
				Signature:   sig,
			},
		},
	}
	return msg
}

func (v *ViewChanger) getInFlight(lastDecision *protos.Proposal) *protos.Proposal {
	inFlight := v.InFlight.InFlightProposal()
	if inFlight == nil {
		v.Logger.Debugf("Node %d's in flight proposal is not set", v.SelfID)
		return nil
	}
	if inFlight.Metadata == nil {
		v.Logger.Panicf("Node %d's in flight proposal metadata is not set", v.SelfID)
	}
	inFlightMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(inFlight.Metadata, inFlightMetadata); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal its own in flight metadata, err: %v", v.SelfID, err)
	}
	proposal := &protos.Proposal{
		Header:               inFlight.Header,
		Metadata:             inFlight.Metadata,
		Payload:              inFlight.Payload,
		VerificationSequence: uint64(inFlight.VerificationSequence),
	}
	if lastDecision == nil {
		v.Logger.Panicf("%d The given last decision is nil", v.SelfID)
		return nil
	}
	if lastDecision.Metadata == nil {
		return proposal // this is the first proposal after genesis
	}
	lastDecisionMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(lastDecision.Metadata, lastDecisionMetadata); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
	}
	if inFlightMetadata.LatestSequence == lastDecisionMetadata.LatestSequence {
		v.Logger.Debugf("Node %d's in flight proposal and the last decision has the same sequence: %d", v.SelfID, inFlightMetadata.LatestSequence)
		return nil // this is not an actual in flight proposal
	}
	if inFlightMetadata.LatestSequence+1 == lastDecisionMetadata.LatestSequence && v.committedDuringViewChange != nil &&
		v.committedDuringViewChange.LatestSequence == lastDecisionMetadata.LatestSequence {
		v.Logger.Infof("Node %d's in flight proposal sequence is %d while already committed decision %d, "+
			"but that is because it committed it during the view change", v.SelfID, inFlightMetadata.LatestSequence, lastDecisionMetadata.LatestSequence)
		return nil
	}
	return proposal
}

func (v *ViewChanger) validateViewDataMsg(svd *protos.SignedViewData, sender uint64) bool {
	if v.getLeader() != v.SelfID { // check if I am the next leader
		v.Logger.Warnf("Node %d got %s from %d, but %d is not the next leader of view %d", v.SelfID, signedViewDataToString(svd), sender, v.SelfID, v.currView)
		return false
	}

	vd := &protos.ViewData{}
	if err := proto.Unmarshal(svd.RawViewData, vd); err != nil {
		v.Logger.Errorf("Node %d was unable to unmarshal viewData message from %d, error: %v", v.SelfID, sender, err)
		return false
	}
	if vd.NextView != v.currView { // check that the message is aligned to this view
		v.Logger.Warnf("Node %d got %s from %d, but %d is in view %d", v.SelfID, signedViewDataToString(svd), sender, v.SelfID, v.currView)
		return false
	}

	valid, lastDecisionSequence := v.checkLastDecision(svd, sender)
	if !valid {
		v.Logger.Warnf("Node %d got %v from %d, but the check of the last decision didn't pass", v.SelfID, signedViewDataToString(svd), sender)
		return false
	}

	v.Logger.Debugf("Node %d got %s from %d, and it passed the last decision check", v.SelfID, signedViewDataToString(svd), sender)

	if err := ValidateInFlight(vd.InFlightProposal, lastDecisionSequence); err != nil {
		v.Logger.Warnf("Node %d got %v from %d, but the in flight proposal is invalid, reason: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false
	}

	v.Logger.Debugf("Node %d got %s from %d, and the in flight proposal is valid", v.SelfID, signedViewDataToString(svd), sender)

	return true
}

func (v *ViewChanger) checkLastDecision(svd *protos.SignedViewData, sender uint64) (valid bool, lastDecisionSequence uint64) {
	vd := &protos.ViewData{}
	if err := proto.Unmarshal(svd.RawViewData, vd); err != nil {
		v.Logger.Errorf("Node %d was unable to unmarshal viewData message from %d, error: %v", v.SelfID, sender, err)
		return false, 0
	}

	if vd.LastDecision == nil {
		v.Logger.Warnf("Node %d got %s from %d, but the last decision is not set", v.SelfID, signedViewDataToString(svd), sender)
		return false, 0
	}

	mySequence, myLastDecision := v.extractCurrentSequence()

	// Begin to check the last decision within the view data message.
	//
	// The sender might be behind, in which case the new leader might not have the right config to validate
	// the decision and signatures, and so the view data message is deemed invalid.
	//
	// If the sender is too far ahead, the new leader might not have the appropriate config.
	// We do not want the new leader to perform a sync at this point, since the sender might be malicious.
	// So this message is considered invalid. If the leader is actually behind this view change will eventually timeout.
	//
	// If the new leader and the sender have the same last decision sequence then we check that the decisions are equal.
	// However, we cannot validate the decision signatures since this last decision might have been a reconfig.
	//
	// Lastly, the sender is ahead by one sequence, and so the new leader validates the decision and delivers it.
	// Only after delivery the message signature is verified, again since this decision might have been a reconfig.

	if vd.LastDecision.Metadata == nil { // this is a genesis proposal
		if mySequence > 0 {
			v.Logger.Debugf("Node %d got %s from %d, but the last decision seq (0) is lower than this node's current sequence %d", v.SelfID, signedViewDataToString(svd), sender, mySequence)
			return false, 0 // this node is ahead
		}
		return true, 0
	}
	lastDecisionMD := &protos.ViewMetadata{}
	if err := proto.Unmarshal(vd.LastDecision.Metadata, lastDecisionMD); err != nil {
		v.Logger.Warnf("Node %d got %s from %d, but was unable to unmarshal last decision metadata, err: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false, 0
	}
	if lastDecisionMD.ViewId >= vd.NextView {
		v.Logger.Warnf("Node %d got %s from %d, but the last decision view %d is greater or equal to requested next view %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.ViewId, vd.NextView)
		return false, 0
	}

	v.Logger.Debugf("Node %d got %s from %d, the last decision seq is %d and this node's current sequence is %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.LatestSequence, mySequence)

	if lastDecisionMD.LatestSequence > mySequence+1 { // this is a decision in the future, ignoring since the node might not have the right configuration to validate
		v.Logger.Debugf("Node %d got %s from %d, but the last decision seq %d is greater than this node's current sequence %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.LatestSequence, mySequence)
		return false, 0
	}
	if lastDecisionMD.LatestSequence < mySequence { // this is a decision in the past, ignoring since the node might not have the right configuration to validate
		v.Logger.Debugf("Node %d got %s from %d, but the last decision seq %d is lower than this node's current sequence %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.LatestSequence, mySequence)
		return false, 0
	}

	if lastDecisionMD.LatestSequence == mySequence { // just make sure that we have the same last decision, can't verify the signatures of this last decision since this might have been a reconfiguration
		// the signature on this message can be verified
		if svd.Signer != sender {
			v.Logger.Warnf("Node %d got %s from %d, but signer %d is not the sender %d", v.SelfID, signedViewDataToString(svd), sender, svd.Signer, sender)
			return false, 0
		}
		if err := v.Verifier.VerifySignature(types.Signature{ID: svd.Signer, Value: svd.Signature, Msg: svd.RawViewData}); err != nil {
			v.Logger.Warnf("Node %d got %s from %d, but signature is invalid, error: %v", v.SelfID, signedViewDataToString(svd), sender, err)
			return false, 0
		}

		// compare the last decision itself
		if !proto.Equal(vd.LastDecision, myLastDecision) {
			v.Logger.Warnf("Node %d got %s from %d, they are at the same sequence but the last decisions are not equal", v.SelfID, signedViewDataToString(svd), sender)
			return false, 0
		}

		return true, lastDecisionMD.LatestSequence
	}

	if lastDecisionMD.LatestSequence != mySequence+1 {
		v.Logger.Warnf("Node %d got %s from %d, the last decision sequence is not equal to this node's sequence + 1", v.SelfID, signedViewDataToString(svd), sender)
		return false, 0
	}

	// This node is one sequence behind, validate the last decision and deliver

	_, err := ValidateLastDecision(vd, v.quorum, v.N, v.Verifier)
	if err != nil {
		v.Logger.Warnf("Node %d got %s from %d, but the last decision is invalid, reason: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false, 0
	}

	proposal := types.Proposal{
		Header:               vd.LastDecision.Header,
		Metadata:             vd.LastDecision.Metadata,
		Payload:              vd.LastDecision.Payload,
		VerificationSequence: int64(vd.LastDecision.VerificationSequence),
	}
	signatures := make([]types.Signature, 0, len(vd.LastDecisionSignatures))
	for _, sig := range vd.LastDecisionSignatures {
		signature := types.Signature{
			ID:    sig.Signer,
			Value: sig.Value,
			Msg:   sig.Msg,
		}
		signatures = append(signatures, signature)
	}
	v.deliverDecision(proposal, signatures)

	// Make note that we have advanced the sequence during a view change,
	// so our in-flight sequence may be behind.
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(proposal.Metadata, md); err != nil {
		v.Logger.Panicf("Node %d got %s from %d, but was unable to unmarshal proposal metadata, err: %v", v.SelfID, signedViewDataToString(svd), sender, err)
	}
	v.committedDuringViewChange = md

	select { // if there was a delivery with a reconfig we need to stop here before verify signature
	case <-v.stopChan:
		return false, 0
	default:
	}

	if svd.Signer != sender {
		v.Logger.Warnf("Node %d got %s from %d, but signer %d is not the sender %d", v.SelfID, signedViewDataToString(svd), sender, svd.Signer, sender)
		return false, 0
	}
	if err = v.Verifier.VerifySignature(types.Signature{ID: svd.Signer, Value: svd.Signature, Msg: svd.RawViewData}); err != nil {
		v.Logger.Warnf("Node %d got %s from %d, but signature is invalid, error: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false, 0
	}

	return true, lastDecisionMD.LatestSequence
}

func (v *ViewChanger) extractCurrentSequence() (uint64, *protos.Proposal) {
	myMetadata := &protos.ViewMetadata{}
	myLastDesicion, _ := v.Checkpoint.Get()
	if myLastDesicion.Metadata == nil {
		return 0, myLastDesicion
	}
	if err := proto.Unmarshal(myLastDesicion.Metadata, myMetadata); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
	}
	return myMetadata.LatestSequence, myLastDesicion
}

// ValidateLastDecision validates the given decision, and returns its sequence when valid
func ValidateLastDecision(vd *protos.ViewData, quorum int, n uint64, verifier api.Verifier) (lastSequence uint64, err error) {
	if vd.LastDecision == nil {
		return 0, errors.Errorf("the last decision is not set")
	}
	if vd.LastDecision.Metadata == nil {
		// This is a genesis proposal, there are no signatures to validate, so we return at this point
		return 0, nil
	}
	md := &protos.ViewMetadata{}
	if err = proto.Unmarshal(vd.LastDecision.Metadata, md); err != nil {
		return 0, errors.Errorf("unable to unmarshal last decision metadata, err: %v", err)
	}
	if md.ViewId >= vd.NextView {
		return 0, errors.Errorf("last decision view %d is greater or equal to requested next view %d", md.ViewId, vd.NextView)
	}
	numSigs := len(vd.LastDecisionSignatures)
	if numSigs < quorum {
		return 0, errors.Errorf("there are only %d last decision signatures", numSigs)
	}
	nodesMap := make(map[uint64]struct{}, n)
	validSig := 0
	for _, sig := range vd.LastDecisionSignatures {
		if _, exist := nodesMap[sig.Signer]; exist {
			continue // seen signature from this node already
		}
		nodesMap[sig.Signer] = struct{}{}
		signature := types.Signature{
			ID:    sig.Signer,
			Value: sig.Value,
			Msg:   sig.Msg,
		}
		proposal := types.Proposal{
			Header:               vd.LastDecision.Header,
			Payload:              vd.LastDecision.Payload,
			Metadata:             vd.LastDecision.Metadata,
			VerificationSequence: int64(vd.LastDecision.VerificationSequence),
		}
		if _, err = verifier.VerifyConsenterSig(signature, proposal); err != nil {
			return 0, errors.Errorf("last decision signature is invalid, error: %v", err)
		}
		validSig++
	}
	if validSig < quorum {
		return 0, errors.Errorf("there are only %d valid last decision signatures", validSig)
	}
	return md.LatestSequence, nil
}

// ValidateInFlight validates the given in-flight proposal
func ValidateInFlight(inFlightProposal *protos.Proposal, lastSequence uint64) error {
	if inFlightProposal == nil {
		return nil
	}
	if inFlightProposal.Metadata == nil {
		return errors.Errorf("in flight proposal metadata is nil")
	}
	inFlightMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(inFlightProposal.Metadata, inFlightMetadata); err != nil {
		return errors.Errorf("unable to unmarshal the in flight proposal metadata, err: %v", err)
	}
	if inFlightMetadata.LatestSequence != lastSequence+1 {
		return errors.Errorf("the in flight proposal sequence is %d while the last decision sequence is %d", inFlightMetadata.LatestSequence, lastSequence)
	}
	return nil
}

func (v *ViewChanger) processViewDataMsg() {
	if len(v.viewDataMsgs.voted) < v.quorum {
		return // need enough (quorum) data to continue
	}
	v.Logger.Debugf("Node %d got a quorum of viewData messages", v.SelfID)
	ok, _, _, err := CheckInFlight(v.getViewDataMessages(), v.f, v.quorum, v.N, v.Verifier)
	if err != nil {
		v.Logger.Panicf("Node %d checked the in flight and it got an error: %v", v.SelfID, err)
	}
	if !ok {
		v.Logger.Debugf("Node %d checked the in flight and it was invalid", v.SelfID)
		return
	}
	v.Logger.Debugf("Node %d checked the in flight and it was valid", v.SelfID)
	// create the new view message
	signedMsgs := make([]*protos.SignedViewData, 0)
	myMsg := v.prepareViewDataMsg()                      // since it might have changed by now
	signedMsgs = append(signedMsgs, myMsg.GetViewData()) // leader's message will always be the first
	close(v.viewDataMsgs.votes)
	for vt := range v.viewDataMsgs.votes {
		if vt.sender == v.SelfID {
			continue // ignore my old message
		}
		signedMsgs = append(signedMsgs, vt.GetViewData())
	}
	msg := &protos.Message{
		Content: &protos.Message_NewView{
			NewView: &protos.NewView{
				SignedViewData: signedMsgs,
			},
		},
	}
	v.Logger.Debugf("Node %d is broadcasting a new view msg", v.SelfID)
	v.Comm.BroadcastConsensus(msg)
	v.Logger.Debugf("Node %d sent a new view msg to self", v.SelfID)
	v.processMsg(v.SelfID, msg) // also send to myself // TODO consider not reprocessing this message
	v.viewDataMsgs.clear(v.N)
	v.Logger.Debugf("Node %d sent a new view msg", v.SelfID)
}

// returns view data messages included in votes
func (v *ViewChanger) getViewDataMessages() []*protos.ViewData {
	num := len(v.viewDataMsgs.votes)
	var messages []*protos.ViewData
	for i := 0; i < num; i++ {
		vt := <-v.viewDataMsgs.votes
		vd := &protos.ViewData{}
		if err := proto.Unmarshal(vt.GetViewData().RawViewData, vd); err != nil {
			v.Logger.Panicf("Node %d was unable to unmarshal viewData message, error: %v", v.SelfID, err)
		}
		messages = append(messages, vd)
		v.viewDataMsgs.votes <- vt
	}
	return messages
}

type possibleProposal struct {
	proposal    *protos.Proposal
	preprepared int
	noArgument  int
}

type proposalAndMetadata struct {
	proposal *protos.Proposal
	metadata *protos.ViewMetadata
}

// CheckInFlight checks if there is an in-flight proposal that needs to be decided on (because a node might decided on it already)
func CheckInFlight(messages []*protos.ViewData, f int, quorum int, N uint64, verifier api.Verifier) (ok, noInFlight bool, inFlightProposal *protos.Proposal, err error) {
	expectedSequence := maxLastDecisionSequence(messages) + 1
	possibleProposals := make([]*possibleProposal, 0)
	proposalsAndMetadata := make([]*proposalAndMetadata, 0)
	noInFlightCount := 0
	for _, vd := range messages {
		if vd.InFlightProposal == nil { // there is no in flight proposal here
			noInFlightCount++
			proposalsAndMetadata = append(proposalsAndMetadata, &proposalAndMetadata{nil, nil})
			continue
		}

		if vd.InFlightProposal.Metadata == nil { // should have been validated earlier
			return false, false, nil, errors.Errorf("Node has a view data message where the in flight proposal metadata is nil")
		}

		inFlightMetadata := &protos.ViewMetadata{}
		if err = proto.Unmarshal(vd.InFlightProposal.Metadata, inFlightMetadata); err != nil { // should have been validated earlier
			return false, false, nil, errors.Errorf("Node was unable to unmarshal the in flight proposal metadata, error: %v", err)
		}

		proposalsAndMetadata = append(proposalsAndMetadata, &proposalAndMetadata{vd.InFlightProposal, inFlightMetadata})

		if inFlightMetadata.LatestSequence != expectedSequence { // the in flight proposal sequence is not as expected
			noInFlightCount++
			continue
		}

		// now the in flight proposal is with the expected sequence
		// find possible proposals

		if !vd.InFlightPrepared { // no prepared so isn't a possible proposal
			noInFlightCount++
			continue
		}

		// this proposal is prepared and so it is possible
		alreadyExists := false
		for _, p := range possibleProposals {
			if proto.Equal(p.proposal, vd.InFlightProposal) {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			// this is not a proposal we have seen before
			possibleProposals = append(possibleProposals, &possibleProposal{proposal: vd.InFlightProposal})
		}
	}

	// fill out info on all possible proposals
	for _, prop := range proposalsAndMetadata {
		for _, possible := range possibleProposals {
			if prop.proposal == nil {
				possible.noArgument++
				continue
			}

			if prop.metadata.LatestSequence != expectedSequence {
				possible.noArgument++
				continue
			}

			if proto.Equal(prop.proposal, possible.proposal) {
				possible.noArgument++
				possible.preprepared++
			}
		}
	}

	// see if there is an in flight proposal that is agreed on
	agreed := -1
	for i, possible := range possibleProposals {
		if possible.preprepared < f+1 { // condition A2 doesn't hold
			continue
		}
		if possible.noArgument < quorum { // condition A1 doesn't hold
			continue
		}
		agreed = i
		break
	}

	// condition A holds
	if agreed != -1 {
		return true, false, possibleProposals[agreed].proposal, nil
	}

	// condition B holds
	if noInFlightCount >= quorum { // there is a quorum of messages that support that there is no prepared in flight proposal
		return true, true, nil, nil
	}

	return false, false, nil, nil
}

// returns the highest sequence of a last decision within the given view data messages
func maxLastDecisionSequence(messages []*protos.ViewData) uint64 {
	max := uint64(0)
	for _, vd := range messages {
		if vd.LastDecision == nil {
			panic("The last decision is not set")
		}
		if vd.LastDecision.Metadata == nil { // this is a genesis proposal
			continue
		}
		md := &protos.ViewMetadata{}
		if err := proto.Unmarshal(vd.LastDecision.Metadata, md); err != nil {
			panic(fmt.Sprintf("Unable to unmarshal the last decision metadata, err: %v", err))
		}
		if md.LatestSequence > max {
			max = md.LatestSequence
		}
	}
	return max
}

func (v *ViewChanger) validateNewViewMsg(msg *protos.NewView) (valid bool, sync bool, deliver bool) {
	signed := msg.GetSignedViewData()
	nodesMap := make(map[uint64]struct{}, v.N)
	validViewDataMsgs := 0
	mySequence, myLastDecision := v.extractCurrentSequence()
	for _, svd := range signed {
		if _, exist := nodesMap[svd.Signer]; exist {
			continue // seen data from this node already
		}
		nodesMap[svd.Signer] = struct{}{}

		vd := &protos.ViewData{}
		if err := proto.Unmarshal(svd.RawViewData, vd); err != nil {
			v.Logger.Errorf("Node %d was unable to unmarshal viewData from the newView message, error: %v", v.SelfID, err)
			return false, false, false
		}

		if vd.NextView != v.currView {
			v.Logger.Warnf("Node %d is processing newView message, but nextView of %s is %d, while the currView is %d", v.SelfID, signedViewDataToString(svd), vd.NextView, v.currView)
			return false, false, false
		}

		if vd.LastDecision == nil {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is not set", v.SelfID, signedViewDataToString(svd))
			return false, false, false
		}

		// Begin to check the last decision within the view data message.
		//
		// This node might be ahead, in which case it might not have the right config to validate
		// the decision and signatures, and so the view data message is deemed invalid.
		//
		// If this node is too far behind then it needs to sync.
		// No validation can be done since it might not have the appropriate config.
		//
		// If the last decision sequence is equal to this node's sequence then we check that the decisions are equal.
		// However, we cannot validate the decision signatures since this last decision might have been a reconfig.
		//
		// Lastly, this node is behind by one sequence, and so it validates the decision and delivers it.
		// Only after delivery the message signature is verified, again since this decision might have been a reconfig.

		if vd.LastDecision.Metadata == nil { // this is a genesis proposal
			if mySequence > 0 {
				// can't validate the signature since I am ahead
				if err := ValidateInFlight(vd.InFlightProposal, 0); err != nil {
					v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
					return false, false, false
				}
				validViewDataMsgs++
				continue
			}
			if err := v.Verifier.VerifySignature(types.Signature{ID: svd.Signer, Value: svd.Signature, Msg: svd.RawViewData}); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false
			}
			if err := ValidateInFlight(vd.InFlightProposal, 0); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false
			}
			validViewDataMsgs++
			continue
		}

		lastDecisionMD := &protos.ViewMetadata{}
		if err := proto.Unmarshal(vd.LastDecision.Metadata, lastDecisionMD); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but was unable to unmarshal the last decision of %s, err: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false
		}
		if lastDecisionMD.ViewId >= vd.NextView {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision view %d is greater or equal to requested next view %d of %s", v.SelfID, lastDecisionMD.ViewId, vd.NextView, signedViewDataToString(svd))
			return false, false, false
		}

		if lastDecisionMD.LatestSequence > mySequence+1 { // this is a decision in the future, can't verify it and should sync
			v.Synchronizer.Sync() // TODO check if I manged to sync to latest decision, revalidate new view, and join the other nodes
			return true, true, false
		}

		if lastDecisionMD.LatestSequence < mySequence { // this is a decision in the past
			// can't validate the signature since I am ahead
			if err := ValidateInFlight(vd.InFlightProposal, lastDecisionMD.LatestSequence); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false
			}
			validViewDataMsgs++
			continue
		}

		if lastDecisionMD.LatestSequence == mySequence { // just make sure that we have the same last decision, can't verify the signatures of this last decision since this might have been a reconfiguration
			// the signature on this message can be verified
			if err := v.Verifier.VerifySignature(types.Signature{ID: svd.Signer, Value: svd.Signature, Msg: svd.RawViewData}); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false
			}

			// compare the last decision itself
			if !proto.Equal(vd.LastDecision, myLastDecision) {
				v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is with the same sequence but is not equal", v.SelfID, signedViewDataToString(svd))
				return false, false, false
			}

			if err := ValidateInFlight(vd.InFlightProposal, lastDecisionMD.LatestSequence); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false
			}

			validViewDataMsgs++
			continue
		}

		if lastDecisionMD.LatestSequence != mySequence+1 {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision sequence is not equal to this node's sequence + 1", v.SelfID)
			return false, false, false
		}

		_, err := ValidateLastDecision(vd, v.quorum, v.N, v.Verifier)
		if err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is invalid, reason: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false
		}

		proposal := types.Proposal{
			Header:               vd.LastDecision.Header,
			Metadata:             vd.LastDecision.Metadata,
			Payload:              vd.LastDecision.Payload,
			VerificationSequence: int64(vd.LastDecision.VerificationSequence),
		}
		signatures := make([]types.Signature, 0)
		for _, sig := range vd.LastDecisionSignatures {
			signature := types.Signature{
				ID:    sig.Signer,
				Value: sig.Value,
				Msg:   sig.Msg,
			}
			signatures = append(signatures, signature)
		}
		v.deliverDecision(proposal, signatures)

		select { // if there was a delivery with a reconfig we need to stop here before verify signature
		case <-v.stopChan:
			return false, false, false
		default:
		}

		if err = v.Verifier.VerifySignature(types.Signature{ID: svd.Signer, Value: svd.Signature, Msg: svd.RawViewData}); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false
		}

		if err = ValidateInFlight(vd.InFlightProposal, lastDecisionMD.LatestSequence); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false
		}

		return true, false, true
	}

	if validViewDataMsgs < v.quorum {
		v.Logger.Warnf("Node %d is processing newView message, but there was only %d valid view data messages while the quorum is %d", v.SelfID, validViewDataMsgs, v.quorum)
		return false, false, false
	}

	v.Logger.Debugf("Node %d found a quorum of valid view data messages within the new view message", v.SelfID)
	return true, false, false
}

func (v *ViewChanger) extractViewDataMessages(msg *protos.NewView) []*protos.ViewData {
	signed := msg.GetSignedViewData()
	vds := make([]*protos.ViewData, 0)
	for _, svd := range signed {
		vd := &protos.ViewData{}
		if err := proto.Unmarshal(svd.RawViewData, vd); err != nil {
			v.Logger.Panicf("Node %d was unable to unmarshal viewData from the newView message, error: %v", v.SelfID, err)
		}
		vds = append(vds, vd)
	}
	return vds
}

func (v *ViewChanger) processNewViewMsg(msg *protos.NewView) {
	valid, calledSync, calledDeliver := v.validateNewViewMsg(msg)
	for calledDeliver {
		v.Logger.Debugf("Node %d is processing a newView message, and delivered a proposal", v.SelfID)
		valid, calledSync, calledDeliver = v.validateNewViewMsg(msg)
	}
	if !valid {
		v.Logger.Warnf("Node %d is processing a newView message, but the message is invalid", v.SelfID)
		return
	}
	if calledSync {
		v.Logger.Debugf("Node %d is processing a newView message, and requested a sync", v.SelfID)
		return
	}

	ok, noInFlight, inFlightProposal, err := CheckInFlight(v.extractViewDataMessages(msg), v.f, v.quorum, v.N, v.Verifier)
	if err != nil {
		v.Logger.Panicf("The check of the in flight proposal by node %d returned an error: %v", v.SelfID, err)
	}
	if !ok {
		v.Logger.Debugf("The check of the in flight proposal by node %d did not pass", v.SelfID)
		return
	}

	if !noInFlight && !v.commitInFlightProposal(inFlightProposal) {
		v.Logger.Warnf("Node %d was unable to commit the in flight proposal, not changing the view", v.SelfID)
		return
	}

	mySequence, _ := v.extractCurrentSequence()

	newViewToSave := &protos.SavedMessage{
		Content: &protos.SavedMessage_NewView{
			NewView: &protos.ViewMetadata{
				ViewId:         v.currView,
				LatestSequence: mySequence,
			},
		},
	}
	if err = v.State.Save(newViewToSave); err != nil {
		v.Logger.Panicf("Failed to save message to state, error: %v", err)
	}

	select { // if there was a delivery or sync with a reconfig when committing the in-flight proposal we should stop
	case <-v.stopChan:
		return
	default:
	}

	v.realView = v.currView
	v.MetricsViewChange.RealView.Set(float64(v.realView))
	v.nvs.clear()
	v.Controller.ViewChanged(v.currView, mySequence+1)

	v.RequestsTimer.RestartTimers()
	v.checkTimeout = false
	v.backOffFactor = 1 // reset
}

func (v *ViewChanger) deliverDecision(proposal types.Proposal, signatures []types.Signature) {
	v.Logger.Debugf("Delivering to app the last decision proposal")
	reconfig := v.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		v.close()
	}
	if v.isProposalLatestComparedToCheckpoint(proposal) {
		// Only set the proposal in case it is later than the already known checkpoint.
		v.Checkpoint.Set(proposal, signatures)
	}
	requests := v.Verifier.RequestsFromProposal(proposal)
	for _, reqInfo := range requests {
		if err := v.RequestsTimer.RemoveRequest(reqInfo); err != nil {
			v.Logger.Warnf("Error during remove of request %s from the pool, err: %v", reqInfo, err)
		}
	}
	v.Pruner.MaybePruneRevokedRequests()
}

func (v *ViewChanger) isProposalLatestComparedToCheckpoint(proposal types.Proposal) bool {
	checkpointProposal, _ := v.Checkpoint.Get()
	return v.sequenceFromProposal(proposal.Metadata) > v.sequenceFromProposal(checkpointProposal.Metadata)
}

func (v *ViewChanger) sequenceFromProposal(rawMetadata []byte) uint64 {
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(rawMetadata, md); err != nil {
		v.Logger.Panicf("Failed extracting view metadata from proposal metadata %s: %v",
			base64.StdEncoding.EncodeToString(rawMetadata), err)
	}
	return md.LatestSequence
}

func (v *ViewChanger) commitInFlightProposal(proposal *protos.Proposal) (success bool) {
	myLastDecision, _ := v.Checkpoint.Get()
	if proposal == nil {
		v.Logger.Panicf("The in flight proposal is nil")
		return
	}
	proposalMD := &protos.ViewMetadata{}
	if err := proto.Unmarshal(proposal.Metadata, proposalMD); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal the in flight proposal metadata, err: %v", v.SelfID, err)
	}

	if myLastDecision.Metadata != nil { // if metadata is nil then I am at genesis proposal and I should commit the in flight proposal anyway
		lastDecisionMD := &protos.ViewMetadata{}
		if err := proto.Unmarshal(myLastDecision.Metadata, lastDecisionMD); err != nil {
			v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
		}
		if lastDecisionMD.LatestSequence == proposalMD.LatestSequence {
			v.Logger.Debugf("Node %d already decided on sequence %d and so it will not commit the in flight proposal with the same sequence", v.SelfID, lastDecisionMD.LatestSequence)
			v.Logger.Debugf("Node %d is comparing its last decision with the in flight proposal with the same sequence %d", v.SelfID, lastDecisionMD.LatestSequence)
			if !proto.Equal(myLastDecision, proposal) {
				v.Logger.Warnf("Node %d compared its last decision with the in flight proposal, which has the same sequence, but they are not equal", v.SelfID)
				return false
			}
			return true // I already decided on the in flight proposal
		}
		if lastDecisionMD.LatestSequence != proposalMD.LatestSequence-1 {
			v.Logger.Panicf("Node %d got an in flight proposal with sequence %d while its last decision was on sequence %d", v.SelfID, proposalMD.LatestSequence, lastDecisionMD.LatestSequence)
		}
	}

	v.Logger.Debugf("Node %d is creating a view %d for the in flight proposal", v.SelfID, proposalMD.ViewId)

	inFlightViewNum := proposalMD.ViewId
	inFlightViewLatestSeq := proposalMD.LatestSequence

	v.inFlightViewLock.Lock()
	inFlightView := &View{
		RetrieveCheckpoint: v.Checkpoint.Get,
		DecisionsPerLeader: v.DecisionsPerLeader,
		SelfID:             v.SelfID,
		N:                  v.N,
		Number:             inFlightViewNum,
		LeaderID:           v.SelfID, // so that no byzantine leader will cause a complain
		Quorum:             v.quorum,
		Decider:            v,
		FailureDetector:    v,
		Sync:               v,
		Logger:             v.Logger,
		Comm:               v.Comm,
		Verifier:           v.Verifier,
		Signer:             v.Signer,
		ProposalSequence:   inFlightViewLatestSeq,
		State:              v.State,
		InMsgQSize:         v.InMsqQSize,
		ViewSequences:      v.ViewSequences,
		Phase:              PREPARED,
		MetricsBlacklist:   v.MetricsBlacklist,
		MetricsView:        v.MetricsView,
	}
	inFlightView.MetricsView.ViewNumber.Set(float64(inFlightView.Number))
	inFlightView.MetricsView.LeaderID.Set(float64(inFlightView.LeaderID))
	inFlightView.MetricsView.ProposalSequence.Set(float64(inFlightView.ProposalSequence))
	inFlightView.MetricsView.DecisionsInView.Set(float64(inFlightView.DecisionsInView))
	inFlightView.MetricsView.Phase.Set(float64(inFlightView.Phase))

	v.inFlightView = inFlightView
	v.inFlightView.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(proposal.VerificationSequence),
		Metadata:             proposal.Metadata,
		Payload:              proposal.Payload,
		Header:               proposal.Header,
	}
	v.inFlightView.myProposalSig = v.Signer.SignProposal(*v.inFlightView.inFlightProposal, nil)
	v.inFlightView.lastBroadcastSent = &protos.Message{
		Content: &protos.Message_Commit{
			Commit: &protos.Commit{
				View:   v.inFlightView.Number,
				Digest: v.inFlightView.inFlightProposal.Digest(),
				Seq:    v.inFlightView.ProposalSequence,
				Signature: &protos.Signature{
					Signer: v.inFlightView.myProposalSig.ID,
					Value:  v.inFlightView.myProposalSig.Value,
					Msg:    v.inFlightView.myProposalSig.Msg,
				},
			},
		},
	}

	v.Logger.Debugf("Waiting two ticks before starting in-flight view")
	<-v.Ticker
	<-v.Ticker

	inFlightView.Start()
	defer inFlightView.Abort()

	v.inFlightViewLock.Unlock()

	v.Logger.Debugf("Node %d started a view %d for the in flight proposal", v.SelfID, v.inFlightView.Number)

	// wait for view to finish or time out
	for {
		select {
		case <-v.inFlightDecideChan:
			v.Logger.Infof("In-flight view %d with latest sequence %d has committed a decision", inFlightViewNum, inFlightViewLatestSeq)
			return true
		case <-v.inFlightSyncChan:
			v.Logger.Infof("In-flight view %d with latest sequence %d has asked to sync", inFlightViewNum, inFlightViewLatestSeq)
			return false
		case now := <-v.Ticker:
			v.lastTick = now
			if v.checkIfTimeout(now) {
				v.Logger.Infof("Timeout expired waiting on In-flight %d with latest sequence view to commit %d", inFlightViewNum, inFlightViewLatestSeq)
				return false
			}
		case <-v.stopChan:
			v.Logger.Infof("View changer was instructed to stop")
			return false
		}
	}
}

// Decide delivers to the application and informs the view changer after delivery
func (v *ViewChanger) Decide(proposal types.Proposal, signatures []types.Signature, requests []types.RequestInfo) {
	v.inFlightView.stop()
	v.Logger.Debugf("Delivering to app the last decision proposal")
	reconfig := v.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		v.close()
	}
	if v.isProposalLatestComparedToCheckpoint(proposal) {
		// Only set the proposal in case it is later than the already known checkpoint.
		v.Checkpoint.Set(proposal, signatures)
	}
	for _, reqInfo := range requests {
		if err := v.RequestsTimer.RemoveRequest(reqInfo); err != nil {
			v.Logger.Warnf("Error during remove of request %s from the pool, err: %v", reqInfo, err)
		}
	}
	v.Pruner.MaybePruneRevokedRequests()
	v.inFlightDecideChan <- struct{}{}
}

// Complain panics when a view change is requested
func (v *ViewChanger) Complain(viewNum uint64, stopView bool) {
	v.Logger.Panicf("Node %d has complained while in the view for the in flight proposal", v.SelfID)
}

// Sync calls the synchronizer and informs the view changer of the sync
func (v *ViewChanger) Sync() {
	// the in flight proposal view asked to sync
	v.Logger.Debugf("Node %d is calling sync because the in flight proposal view has asked to sync", v.SelfID)
	v.Synchronizer.Sync()
	v.inFlightSyncChan <- struct{}{}
}

// HandleViewMessage passes a message to the in flight proposal view if applicable
func (v *ViewChanger) HandleViewMessage(sender uint64, m *protos.Message) {
	v.inFlightViewLock.RLock()
	defer v.inFlightViewLock.RUnlock()
	if view := v.inFlightView; view != nil {
		v.Logger.Debugf("Node %d is passing a message to the in flight view", v.SelfID)
		view.HandleMessage(sender, m)
	}
}

func (v *ViewChanger) blacklist() []uint64 {
	prop, _ := v.Checkpoint.Get()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prop.Metadata, md); err != nil {
		v.Logger.Panicf("Failed unmarshalling metadata: %v", err)
	}
	return md.BlackList
}
