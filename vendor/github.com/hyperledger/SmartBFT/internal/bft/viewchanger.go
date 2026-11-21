// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/SmartBFT/pkg/api"
	"github.com/hyperledger/SmartBFT/pkg/metrics/disabled"
	"github.com/hyperledger/SmartBFT/pkg/types"
	protos "github.com/hyperledger/SmartBFT/smartbftprotos"
	"google.golang.org/protobuf/proto"
)

// ViewController controls the view
type ViewController interface {
	ViewChanged(newViewNumber uint64, newProposalSequence uint64)
	// AbortView aborts the given view and waits until it has stopped. It stops
	// waiting when cancel is closed, so that stopping the view changer does not
	// hang on a view that waits for a lock held by a reconfiguration.
	AbortView(view uint64, cancel <-chan struct{})
	// AbortViewAndHold aborts the given view like AbortView and keeps other
	// views from starting or writing until ReleaseViewWrite.
	AbortViewAndHold(view uint64, cancel <-chan struct{}) bool
	// ReleaseViewWrite lets other views start and write again after a
	// successful AbortViewAndHold.
	ReleaseViewWrite()
}

// Pruner prunes revoked requests
type Pruner interface {
	MaybePruneRevokedRequests()
}

// RequestsTimer controls requests
type RequestsTimer interface {
	StopTimers()
	RestartTimers()
	RemoveRequest(request types.RequestInfo) error
}

type change struct {
	view     uint64
	stopView bool
}

// inFlightAttempt is one run of the in-flight proposal view, so that the view's
// completion is signaled to the waiter of that run only.
type inFlightAttempt struct {
	decideCh chan struct{}
	syncCh   chan struct{}
	viewRef  *View
}

type inFlightAttemptCallbacks struct {
	vc      *ViewChanger
	attempt *inFlightAttempt
}

func (c *inFlightAttemptCallbacks) Decide(proposal types.Proposal, signatures []types.Signature, requests []types.RequestInfo) {
	c.vc.decideInFlight(c.attempt, proposal, signatures, requests)
}

func (c *inFlightAttemptCallbacks) Sync() {
	c.vc.syncInFlight(c.attempt)
}

// ViewChanger is responsible for running the view change protocol.
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
	ViewSequences    *atomic.Value
	inFlightView     *View
	inFlightViewLock sync.RWMutex

	Ticker              <-chan time.Time
	lastTick            time.Time
	ResendTimeout       time.Duration
	lastResend          time.Time
	ViewChangeTimeout   time.Duration
	startViewChangeTime time.Time
	checkTimeout        bool
	backOffFactor       uint64
	syncedForNewView    bool
	acceptedNewView     bool // whether a new view message was already accepted for currView

	// Runtime
	MetricsViewChange         *api.MetricsViewChange
	MetricsBlacklist          *api.MetricsBlacklist
	MetricsView               *api.MetricsView
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
		v.MetricsViewChange = api.NewMetricsViewChange(&disabled.Provider{})
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
		v.Logger.Debugf("Node %d is processing a view change message %v from %d with next view %d", v.SelfID, m, sender, vc.GetNextView())
		v.nvs.registerNext(vc.GetNextView(), sender)
		// check view number
		if vc.GetNextView() == v.currView+1 { // accept view change only to immediate next view number
			v.viewChangeMsgs.registerVote(sender, m)
			v.processViewChangeMsg(false)
			return
		}
		if v.nextView == v.currView+1 && // node has already started view change with last view
			vc.GetNextView() > v.realView &&
			vc.GetNextView() < v.currView+1 &&
			v.nvs.sendRecv(vc.GetNextView(), sender) {
			// Let's help the lagging nodes.
			msg := &protos.Message{
				Content: &protos.Message_ViewChange{
					ViewChange: &protos.ViewChange{
						NextView: vc.GetNextView(),
					},
				},
			}
			v.Comm.BroadcastConsensus(msg)
			v.Logger.Warnf("Node %d got viewChange message %v from %d with view %d, expected view %d, help the lagging nodes", v.SelfID, m, sender, vc.GetNextView(), v.currView+1)
			return
		}
		v.Logger.Warnf("Node %d got viewChange message %v from %d with view %d, expected view %d", v.SelfID, m, sender, vc.GetNextView(), v.currView+1)
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
		if v.acceptedNewView {
			v.Logger.Debugf("Node %d got newView message %v from %d, but it is already in view %d, ignoring it", v.SelfID, MsgToString(m), sender, v.currView)
			return
		}
		v.processNewViewMsg(nv)
	}
}

// InformNewView tells the view changer to advance to a new view number.
// It is called from the controller's run loop after a sync, and so it must not wait for the view changer.
// If the view changer did not process a previous view yet, the pending view is replaced by the higher of the two,
// which loses nothing since informNewView ignores views lower than the current one.
// The pending view is held only in informChan, so concurrent calls are safe as well.
func (v *ViewChanger) InformNewView(view uint64) {
	for {
		select {
		case v.informChan <- view:
			return
		case <-v.stopChan:
			return
		default:
		}
		select {
		case pending := <-v.informChan:
			view = max(view, pending)
		default:
		}
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
	v.syncedForNewView = false
	v.acceptedNewView = true // the node is already in this view, so a new view message for it is not needed
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
		v.Controller.AbortView(v.currView, v.stopChan) // abort the current view when joining view change
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
	// Stop the current view before writing the view change record. The view may still
	// append a commit after that record, which would leave the commit without its
	// pre-prepare in the WAL. AbortViewAndHold waits for the view to stop, keeps the
	// sync path from restarting it, and keeps other views from writing until
	// ReleaseViewWrite below. If the controller has already moved to a newer view,
	// this view change is stale.
	aborted := v.Controller.AbortViewAndHold(v.currView, v.stopChan)
	if !aborted {
		if !restore {
			return
		}

		// A restore can race with syncOnStart, which advances the controller to a
		// newer view and informs the view changer through informChan. The restore
		// was already started by the code above, so it must still send its view
		// data instead of silently leaving the view change half done. The pending
		// informChan entry will move the view changer to the newer view next.
		v.Logger.Debugf("Node %d is restoring view change %d while the controller is already in a newer view", v.SelfID, v.currView)
	}

	if !restore {
		msgToSave := &protos.SavedMessage{
			Content: &protos.SavedMessage_ViewChange{
				ViewChange: &protos.ViewChange{
					NextView: v.currView,
				},
			},
		}
		// A view change record is not a truncation point: the proposal in
		// flight, if there is one, travels in the view data message and must
		// stay recoverable.
		if err := v.State.Save(msgToSave, false); err != nil {
			v.Logger.Panicf("Failed to save message to state, error: %v", err)
		}
	}
	if aborted {
		v.Controller.ReleaseViewWrite()
	}
	v.currView = v.nextView
	v.MetricsViewChange.CurrentView.Set(float64(v.currView))
	v.viewChangeMsgs.clear(v.N)
	v.viewDataMsgs.clear(v.N)  // clear because currView changed
	v.syncedForNewView = false // clear because currView changed
	v.acceptedNewView = false  // clear because currView changed
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
	inFlightProposal, prepared := v.InFlight.InFlightProposalAndPrepared()
	inFlight := v.getInFlight(lastDecision, inFlightProposal)
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

func (v *ViewChanger) getInFlight(lastDecision *protos.Proposal, inFlight *types.Proposal) *protos.Proposal {
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
	if err := proto.Unmarshal(lastDecision.GetMetadata(), lastDecisionMetadata); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
	}
	if inFlightMetadata.GetLatestSequence() == lastDecisionMetadata.GetLatestSequence() {
		v.Logger.Debugf("Node %d's in flight proposal and the last decision has the same sequence: %d", v.SelfID, inFlightMetadata.GetLatestSequence())
		return nil // this is not an actual in flight proposal
	}
	if inFlightMetadata.GetLatestSequence()+1 == lastDecisionMetadata.GetLatestSequence() && v.committedDuringViewChange != nil &&
		v.committedDuringViewChange.GetLatestSequence() == lastDecisionMetadata.GetLatestSequence() {
		v.Logger.Infof("Node %d's in flight proposal sequence is %d while already committed decision %d, "+
			"but that is because it committed it during the view change", v.SelfID, inFlightMetadata.GetLatestSequence(), lastDecisionMetadata.GetLatestSequence())
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
	if err := proto.Unmarshal(svd.GetRawViewData(), vd); err != nil {
		v.Logger.Errorf("Node %d was unable to unmarshal viewData message from %d, error: %v", v.SelfID, sender, err)
		return false
	}
	if vd.GetNextView() != v.currView { // check that the message is aligned to this view
		v.Logger.Warnf("Node %d got %s from %d, but %d is in view %d", v.SelfID, signedViewDataToString(svd), sender, v.SelfID, v.currView)
		return false
	}

	valid, lastDecisionSequence := v.checkLastDecision(svd, sender)
	if !valid {
		v.Logger.Warnf("Node %d got %v from %d, but the check of the last decision didn't pass", v.SelfID, signedViewDataToString(svd), sender)
		return false
	}

	v.Logger.Debugf("Node %d got %s from %d, and it passed the last decision check", v.SelfID, signedViewDataToString(svd), sender)

	if err := ValidateInFlight(vd.GetInFlightProposal(), lastDecisionSequence); err != nil {
		v.Logger.Warnf("Node %d got %v from %d, but the in flight proposal is invalid, reason: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false
	}

	v.Logger.Debugf("Node %d got %s from %d, and the in flight proposal is valid", v.SelfID, signedViewDataToString(svd), sender)

	return true
}

func (v *ViewChanger) checkLastDecision(svd *protos.SignedViewData, sender uint64) (valid bool, lastDecisionSequence uint64) {
	vd := &protos.ViewData{}
	if err := proto.Unmarshal(svd.GetRawViewData(), vd); err != nil {
		v.Logger.Errorf("Node %d was unable to unmarshal viewData message from %d, error: %v", v.SelfID, sender, err)
		return false, 0
	}

	if vd.GetLastDecision() == nil {
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
	if err := proto.Unmarshal(vd.GetLastDecision().GetMetadata(), lastDecisionMD); err != nil {
		v.Logger.Warnf("Node %d got %s from %d, but was unable to unmarshal last decision metadata, err: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false, 0
	}
	if lastDecisionMD.GetViewId() >= vd.GetNextView() {
		v.Logger.Warnf("Node %d got %s from %d, but the last decision view %d is greater or equal to requested next view %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.GetViewId(), vd.GetNextView())
		return false, 0
	}

	v.Logger.Debugf("Node %d got %s from %d, the last decision seq is %d and this node's current sequence is %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.GetLatestSequence(), mySequence)

	if lastDecisionMD.GetLatestSequence() > mySequence+1 { // this is a decision in the future, ignoring since the node might not have the right configuration to validate
		v.Logger.Debugf("Node %d got %s from %d, but the last decision seq %d is greater than this node's current sequence %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.GetLatestSequence(), mySequence)
		return false, 0
	}
	if lastDecisionMD.GetLatestSequence() < mySequence { // this is a decision in the past, ignoring since the node might not have the right configuration to validate
		v.Logger.Debugf("Node %d got %s from %d, but the last decision seq %d is lower than this node's current sequence %d", v.SelfID, signedViewDataToString(svd), sender, lastDecisionMD.GetLatestSequence(), mySequence)
		return false, 0
	}

	if lastDecisionMD.GetLatestSequence() == mySequence { // just make sure that we have the same last decision, can't verify the signatures of this last decision since this might have been a reconfiguration
		// the signature on this message can be verified
		if svd.GetSigner() != sender {
			v.Logger.Warnf("Node %d got %s from %d, but signer %d is not the sender %d", v.SelfID, signedViewDataToString(svd), sender, svd.GetSigner(), sender)
			return false, 0
		}
		if err := v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
			v.Logger.Warnf("Node %d got %s from %d, but signature is invalid, error: %v", v.SelfID, signedViewDataToString(svd), sender, err)
			return false, 0
		}

		// compare the last decision itself
		if !proto.Equal(vd.GetLastDecision(), myLastDecision) {
			v.Logger.Warnf("Node %d got %s from %d, they are at the same sequence but the last decisions are not equal", v.SelfID, signedViewDataToString(svd), sender)
			return false, 0
		}

		return true, lastDecisionMD.GetLatestSequence()
	}

	if lastDecisionMD.GetLatestSequence() != mySequence+1 {
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
		Header:               vd.GetLastDecision().GetHeader(),
		Metadata:             vd.GetLastDecision().GetMetadata(),
		Payload:              vd.GetLastDecision().GetPayload(),
		VerificationSequence: int64(vd.GetLastDecision().GetVerificationSequence()),
	}
	signatures := make([]types.Signature, 0, len(vd.GetLastDecisionSignatures()))
	for _, sig := range vd.GetLastDecisionSignatures() {
		signature := types.Signature{
			ID:    sig.GetSigner(),
			Value: sig.GetValue(),
			Msg:   sig.GetMsg(),
		}
		signatures = append(signatures, signature)
	}
	v.deliverDecision(proposal, signatures)

	// Make note that we have advanced the sequence during a view change,
	// so our in-flight sequence may be behind.
	md := &protos.ViewMetadata{}
	if err = proto.Unmarshal(proposal.Metadata, md); err != nil {
		v.Logger.Panicf("Node %d got %s from %d, but was unable to unmarshal proposal metadata, err: %v", v.SelfID, signedViewDataToString(svd), sender, err)
	}
	v.committedDuringViewChange = md

	select { // if there was a delivery with a reconfig we need to stop here before verify signature
	case <-v.stopChan:
		return false, 0
	default:
	}

	if svd.GetSigner() != sender {
		v.Logger.Warnf("Node %d got %s from %d, but signer %d is not the sender %d", v.SelfID, signedViewDataToString(svd), sender, svd.GetSigner(), sender)
		return false, 0
	}
	if err = v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
		v.Logger.Warnf("Node %d got %s from %d, but signature is invalid, error: %v", v.SelfID, signedViewDataToString(svd), sender, err)
		return false, 0
	}

	return true, lastDecisionMD.GetLatestSequence()
}

func (v *ViewChanger) extractCurrentSequence() (uint64, *protos.Proposal) {
	myMetadata := &protos.ViewMetadata{}
	myLastDesicion, _ := v.Checkpoint.Get()
	if myLastDesicion.Metadata == nil {
		return 0, myLastDesicion
	}
	if err := proto.Unmarshal(myLastDesicion.GetMetadata(), myMetadata); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
	}
	return myMetadata.GetLatestSequence(), myLastDesicion
}

// ValidateLastDecision validates the given decision, and returns its sequence when valid
func ValidateLastDecision(vd *protos.ViewData, quorum int, n uint64, verifier api.Verifier) (lastSequence uint64, err error) {
	if vd.GetLastDecision() == nil {
		return 0, errors.New("the last decision is not set")
	}
	if vd.LastDecision.Metadata == nil {
		// This is a genesis proposal, there are no signatures to validate, so we return at this point
		return 0, nil
	}
	md := &protos.ViewMetadata{}
	if err = proto.Unmarshal(vd.GetLastDecision().GetMetadata(), md); err != nil {
		return 0, fmt.Errorf("unable to unmarshal last decision metadata, err: %w", err)
	}
	if md.GetViewId() >= vd.GetNextView() {
		return 0, fmt.Errorf("last decision view %d is greater or equal to requested next view %d", md.GetViewId(), vd.GetNextView())
	}
	numSigs := len(vd.GetLastDecisionSignatures())
	if numSigs < quorum {
		return 0, fmt.Errorf("there are only %d last decision signatures", numSigs)
	}
	nodesMap := make(map[uint64]struct{}, n)
	validSig := 0
	for _, sig := range vd.GetLastDecisionSignatures() {
		if _, exist := nodesMap[sig.GetSigner()]; exist {
			continue // seen signature from this node already
		}
		nodesMap[sig.GetSigner()] = struct{}{}
		signature := types.Signature{
			ID:    sig.GetSigner(),
			Value: sig.GetValue(),
			Msg:   sig.GetMsg(),
		}
		proposal := types.Proposal{
			Header:               vd.GetLastDecision().GetHeader(),
			Payload:              vd.GetLastDecision().GetPayload(),
			Metadata:             vd.GetLastDecision().GetMetadata(),
			VerificationSequence: int64(vd.GetLastDecision().GetVerificationSequence()),
		}
		if _, err = verifier.VerifyConsenterSig(signature, proposal); err != nil {
			return 0, fmt.Errorf("last decision signature is invalid, error: %w", err)
		}
		validSig++
	}
	if validSig < quorum {
		return 0, fmt.Errorf("there are only %d valid last decision signatures", validSig)
	}
	return md.GetLatestSequence(), nil
}

// ValidateInFlight validates the given in-flight proposal
func ValidateInFlight(inFlightProposal *protos.Proposal, lastSequence uint64) error {
	if inFlightProposal == nil {
		return nil
	}
	if inFlightProposal.Metadata == nil {
		return errors.New("in flight proposal metadata is nil")
	}
	inFlightMetadata := &protos.ViewMetadata{}
	if err := proto.Unmarshal(inFlightProposal.GetMetadata(), inFlightMetadata); err != nil {
		return fmt.Errorf("unable to unmarshal the in flight proposal metadata, err: %w", err)
	}
	if inFlightMetadata.GetLatestSequence() != lastSequence+1 {
		return fmt.Errorf("the in flight proposal sequence is %d while the last decision sequence is %d", inFlightMetadata.GetLatestSequence(), lastSequence)
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

// returns view data messages included in votes, keyed by the ID of the node that sent them,
// so that a single node is counted at most once
func (v *ViewChanger) getViewDataMessages() map[uint64]*protos.ViewData {
	messages := make(map[uint64]*protos.ViewData)
	for range len(v.viewDataMsgs.votes) {
		vt := <-v.viewDataMsgs.votes
		vd := &protos.ViewData{}
		if err := proto.Unmarshal(vt.GetViewData().GetRawViewData(), vd); err != nil {
			v.Logger.Panicf("Node %d was unable to unmarshal viewData message, error: %v", v.SelfID, err)
		}
		if _, exist := messages[vt.sender]; !exist {
			messages[vt.sender] = vd
		}
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
// messages maps the ID of the node that sent the view data to the view data itself, so that every node is counted at most once.
func CheckInFlight(messages map[uint64]*protos.ViewData, f int, quorum int, n uint64, verifier api.Verifier) (ok, noInFlight bool, inFlightProposal *protos.Proposal, err error) {
	expectedSequence := maxLastDecisionSequence(messages) + 1
	possibleProposals := make([]*possibleProposal, 0)
	proposalsAndMetadata := make([]*proposalAndMetadata, 0)
	noInFlightCount := 0
	for _, vd := range messages {
		if vd.GetInFlightProposal() == nil { // there is no in flight proposal here
			noInFlightCount++
			proposalsAndMetadata = append(proposalsAndMetadata, &proposalAndMetadata{nil, nil})
			continue
		}

		if vd.InFlightProposal.Metadata == nil { // should have been validated earlier
			return false, false, nil, errors.New("node has a view data message where the in flight proposal metadata is nil")
		}

		inFlightMetadata := &protos.ViewMetadata{}
		if err = proto.Unmarshal(vd.GetInFlightProposal().GetMetadata(), inFlightMetadata); err != nil { // should have been validated earlier
			return false, false, nil, fmt.Errorf("node was unable to unmarshal the in flight proposal metadata, error: %w", err)
		}

		proposalsAndMetadata = append(proposalsAndMetadata, &proposalAndMetadata{vd.GetInFlightProposal(), inFlightMetadata})

		if inFlightMetadata.GetLatestSequence() != expectedSequence { // the in flight proposal sequence is not as expected
			noInFlightCount++
			continue
		}

		// now the in flight proposal is with the expected sequence
		// find possible proposals

		if !vd.GetInFlightPrepared() { // no prepared so isn't a possible proposal
			noInFlightCount++
			continue
		}

		// this proposal is prepared and so it is possible
		alreadyExists := false
		for _, p := range possibleProposals {
			if proto.Equal(p.proposal, vd.GetInFlightProposal()) {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			// this is not a proposal we have seen before
			possibleProposals = append(possibleProposals, &possibleProposal{proposal: vd.GetInFlightProposal()})
		}
	}

	// fill out info on all possible proposals
	for _, prop := range proposalsAndMetadata {
		for _, possible := range possibleProposals {
			if prop.proposal == nil {
				possible.noArgument++
				continue
			}

			if prop.metadata.GetLatestSequence() != expectedSequence {
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
func maxLastDecisionSequence(messages map[uint64]*protos.ViewData) uint64 {
	max := uint64(0)
	for _, vd := range messages {
		if vd.GetLastDecision() == nil {
			panic("The last decision is not set")
		}
		if vd.LastDecision.Metadata == nil { // this is a genesis proposal
			continue
		}
		md := &protos.ViewMetadata{}
		if err := proto.Unmarshal(vd.GetLastDecision().GetMetadata(), md); err != nil {
			panic(fmt.Sprintf("Unable to unmarshal the last decision metadata, err: %v", err))
		}
		if md.GetLatestSequence() > max {
			max = md.GetLatestSequence()
		}
	}
	return max
}

// validateNewViewMsg validates a newView message and returns, in addition to its verdict,
// the view data it authenticated, keyed by the ID of the node that signed it.
// Only authenticated view data is counted when deciding on an in flight proposal.
func (v *ViewChanger) validateNewViewMsg(msg *protos.NewView) (valid bool, sync bool, deliver bool, authenticated map[uint64]*protos.ViewData) {
	signed := msg.GetSignedViewData()
	authenticated = make(map[uint64]*protos.ViewData, v.N)
	seenSigners := make(map[uint64]struct{}, v.N)
	mySequence, myLastDecision := v.extractCurrentSequence()
	for _, svd := range signed {
		if _, exist := seenSigners[svd.GetSigner()]; exist {
			// an honest leader never includes the same signer twice
			v.Logger.Warnf("Node %d is processing newView message, but %s is included more than once", v.SelfID, signedViewDataToString(svd))
			return false, false, false, nil
		}
		seenSigners[svd.GetSigner()] = struct{}{}

		if !slices.Contains(v.NodesList, svd.GetSigner()) {
			// the signer is not authenticated, hence its view data is not counted,
			// but the rest of the view data might still be
			v.Logger.Warnf("Node %d is processing newView message, but the signer of %s is not a member of the cluster, ignoring it", v.SelfID, signedViewDataToString(svd))
			continue
		}

		vd := &protos.ViewData{}
		if err := proto.Unmarshal(svd.GetRawViewData(), vd); err != nil {
			v.Logger.Errorf("Node %d was unable to unmarshal viewData from the newView message, error: %v", v.SelfID, err)
			return false, false, false, nil
		}

		if vd.GetNextView() != v.currView {
			v.Logger.Warnf("Node %d is processing newView message, but nextView of %s is %d, while the currView is %d", v.SelfID, signedViewDataToString(svd), vd.GetNextView(), v.currView)
			return false, false, false, nil
		}

		if vd.GetLastDecision() == nil {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is not set", v.SelfID, signedViewDataToString(svd))
			return false, false, false, nil
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
		//
		// The signature on the view data itself is verified in every branch, since the signer of a view data
		// is counted towards the quorum that decides on an in flight proposal.
		// On the branches where this node is ahead of the view data it might not have the right configuration
		// to verify the signature, for example when the last decision it is ahead by was a reconfiguration.
		// Such a view data is ignored rather than invalidating the whole message, and if that leaves too few
		// authenticated view data then this node syncs and expects to be able to process the new view afterwards.

		if vd.LastDecision.Metadata == nil { // this is a genesis proposal
			if err := v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, ignoring it, error: %v", v.SelfID, signedViewDataToString(svd), err)
				continue
			}
			if err := ValidateInFlight(vd.GetInFlightProposal(), 0); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false, nil
			}
			authenticated[svd.GetSigner()] = vd
			continue
		}

		lastDecisionMD := &protos.ViewMetadata{}
		if err := proto.Unmarshal(vd.GetLastDecision().GetMetadata(), lastDecisionMD); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but was unable to unmarshal the last decision of %s, err: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false, nil
		}
		if lastDecisionMD.GetViewId() >= vd.GetNextView() {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision view %d is greater or equal to requested next view %d of %s", v.SelfID, lastDecisionMD.GetViewId(), vd.GetNextView(), signedViewDataToString(svd))
			return false, false, false, nil
		}

		if lastDecisionMD.GetLatestSequence() > mySequence+1 { // this is a decision in the future, can't verify it and should sync
			v.Synchronizer.Sync() // TODO check if I managed to sync to latest decision, revalidate new view, and join the other nodes
			return true, true, false, nil
		}

		if lastDecisionMD.GetLatestSequence() < mySequence { // this is a decision in the past
			// the signature on the view data is verified as well, it does not depend on the last decision
			if err := v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, ignoring it, error: %v", v.SelfID, signedViewDataToString(svd), err)
				continue
			}
			if err := ValidateInFlight(vd.GetInFlightProposal(), lastDecisionMD.GetLatestSequence()); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false, nil
			}
			authenticated[svd.GetSigner()] = vd
			continue
		}

		if lastDecisionMD.GetLatestSequence() == mySequence { // just make sure that we have the same last decision, can't verify the signatures of this last decision since this might have been a reconfiguration
			// the signature on this message can be verified
			if err := v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false, nil
			}

			// compare the last decision itself
			if !proto.Equal(vd.GetLastDecision(), myLastDecision) {
				v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is with the same sequence but is not equal", v.SelfID, signedViewDataToString(svd))
				return false, false, false, nil
			}

			if err := ValidateInFlight(vd.GetInFlightProposal(), lastDecisionMD.GetLatestSequence()); err != nil {
				v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
				return false, false, false, nil
			}

			authenticated[svd.GetSigner()] = vd
			continue
		}

		if lastDecisionMD.GetLatestSequence() != mySequence+1 {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision sequence is not equal to this node's sequence + 1", v.SelfID)
			return false, false, false, nil
		}

		_, err := ValidateLastDecision(vd, v.quorum, v.N, v.Verifier)
		if err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but the last decision of %s is invalid, reason: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false, nil
		}

		proposal := types.Proposal{
			Header:               vd.GetLastDecision().GetHeader(),
			Metadata:             vd.GetLastDecision().GetMetadata(),
			Payload:              vd.GetLastDecision().GetPayload(),
			VerificationSequence: int64(vd.GetLastDecision().GetVerificationSequence()),
		}
		signatures := make([]types.Signature, 0)
		for _, sig := range vd.GetLastDecisionSignatures() {
			signature := types.Signature{
				ID:    sig.GetSigner(),
				Value: sig.GetValue(),
				Msg:   sig.GetMsg(),
			}
			signatures = append(signatures, signature)
		}
		v.deliverDecision(proposal, signatures)

		select { // if there was a delivery with a reconfig we need to stop here before verify signature
		case <-v.stopChan:
			return false, false, false, nil
		default:
		}

		if err = v.Verifier.VerifySignature(types.Signature{ID: svd.GetSigner(), Value: svd.GetSignature(), Msg: svd.GetRawViewData()}); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but signature of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false, nil
		}

		if err = ValidateInFlight(vd.GetInFlightProposal(), lastDecisionMD.GetLatestSequence()); err != nil {
			v.Logger.Warnf("Node %d is processing newView message, but the in flight proposal of %s is invalid, error: %v", v.SelfID, signedViewDataToString(svd), err)
			return false, false, false, nil
		}

		return true, false, true, nil
	}

	if len(authenticated) < v.quorum {
		v.Logger.Warnf("Node %d is processing newView message, but there were only %d authenticated view data messages while the quorum is %d", v.SelfID, len(authenticated), v.quorum)
		// this node might be missing the configuration needed to authenticate the view data,
		// hence it syncs and expects to be able to process the new view message afterwards
		v.syncForUnauthenticatedNewView()
		return true, true, false, nil
	}

	// The new leader is the node that collected the view data messages of the others,
	// so its own attestation must be part of the new view message. Without it the other
	// signers would be speaking on behalf of a view they never verified with the leader.
	// It must be authenticated, otherwise a view data of the leader that was ignored does not count.
	leader := v.getLeader()
	if _, exists := authenticated[leader]; !exists {
		v.Logger.Warnf("Node %d is processing newView message, but it does not contain an authenticated signed view data message of the new leader %d", v.SelfID, leader)
		return false, false, false, nil
	}

	v.Logger.Debugf("Node %d found a quorum of authenticated view data messages within the new view message", v.SelfID)
	return true, false, false, authenticated
}

// syncForUnauthenticatedNewView syncs when a new view message does not contain a quorum of view data
// this node is able to authenticate, since it might be missing the state or the configuration needed
// to authenticate the rest of them. It syncs at most once per view, so that a faulty leader cannot
// make this node sync over and over again.
func (v *ViewChanger) syncForUnauthenticatedNewView() {
	if v.syncedForNewView {
		v.Logger.Warnf("Node %d already synced in view %d because of a new view message it could not authenticate, not syncing again", v.SelfID, v.currView)
		return
	}
	v.syncedForNewView = true
	v.Synchronizer.Sync()
}

func (v *ViewChanger) processNewViewMsg(msg *protos.NewView) {
	valid, calledSync, calledDeliver, authenticated := v.validateNewViewMsg(msg)
	for calledDeliver {
		v.Logger.Debugf("Node %d is processing a newView message, and delivered a proposal", v.SelfID)
		valid, calledSync, calledDeliver, authenticated = v.validateNewViewMsg(msg)
	}
	if !valid {
		v.Logger.Warnf("Node %d is processing a newView message, but the message is invalid", v.SelfID)
		return
	}
	if calledSync {
		v.Logger.Debugf("Node %d is processing a newView message, and requested a sync", v.SelfID)
		return
	}

	// only the view data that was authenticated is counted when deciding on the in flight proposal
	ok, noInFlight, inFlightProposal, err := CheckInFlight(authenticated, v.f, v.quorum, v.N, v.Verifier)
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

	// The record of the new view replaces the state of the current view, so the view
	// has to be stopped first. Otherwise it can still append a commit after that
	// record and leave a sequence that recovery rejects. AbortViewAndHold waits for
	// the view to stop, keeps the sync path from restarting it, and keeps other views
	// from writing until ReleaseViewWrite below. If the controller has already moved
	// to a newer view, this new view message is stale.
	if !v.Controller.AbortViewAndHold(v.currView, v.stopChan) {
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
	// A new view record is not a truncation point: the in-flight proposal, if
	// there is one, must stay recoverable.
	if err = v.State.Save(newViewToSave, false); err != nil {
		v.Logger.Panicf("Failed to save message to state, error: %v", err)
	}
	v.Controller.ReleaseViewWrite()

	select { // if there was a delivery or sync with a reconfig when committing the in-flight proposal we should stop
	case <-v.stopChan:
		return
	default:
	}

	v.acceptedNewView = true
	v.realView = v.currView
	v.MetricsViewChange.RealView.Set(float64(v.realView))
	v.nvs.clear()
	v.Controller.ViewChanged(v.currView, mySequence+1)

	v.RequestsTimer.RestartTimers()
	v.checkTimeout = false
	v.backOffFactor = 1 // reset
}

func (v *ViewChanger) deliverDecision(proposal types.Proposal, signatures []types.Signature) {
	v.Logger.Debugf("Delivering to app from deliverDecision the last decision proposal")
	reconfig := v.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		v.close()
	}
	v.Logger.Debugf("Delivering end to app from deliverDecision the last decision proposal")

	requests := v.Verifier.RequestsFromProposal(proposal)
	for _, reqInfo := range requests {
		if err := v.RequestsTimer.RemoveRequest(reqInfo); err != nil {
			v.Logger.Warnf("Error during remove of request %s from the pool, err: %v", reqInfo, err)
		}
	}
	v.Pruner.MaybePruneRevokedRequests()
}

func (v *ViewChanger) commitInFlightProposal(proposal *protos.Proposal) (success bool) {
	myLastDecision, _ := v.Checkpoint.Get()
	if proposal == nil {
		v.Logger.Panicf("The in flight proposal is nil")
		return
	}
	proposalMD := &protos.ViewMetadata{}
	if err := proto.Unmarshal(proposal.GetMetadata(), proposalMD); err != nil {
		v.Logger.Panicf("Node %d is unable to unmarshal the in flight proposal metadata, err: %v", v.SelfID, err)
	}

	if myLastDecision.Metadata != nil { // if metadata is nil then I am at genesis proposal and I should commit the in flight proposal anyway
		lastDecisionMD := &protos.ViewMetadata{}
		if err := proto.Unmarshal(myLastDecision.GetMetadata(), lastDecisionMD); err != nil {
			v.Logger.Panicf("Node %d is unable to unmarshal its own last decision metadata from checkpoint, err: %v", v.SelfID, err)
		}
		if lastDecisionMD.GetLatestSequence() == proposalMD.GetLatestSequence() {
			v.Logger.Debugf("Node %d already decided on sequence %d and so it will not commit the in flight proposal with the same sequence", v.SelfID, lastDecisionMD.GetLatestSequence())
			v.Logger.Debugf("Node %d is comparing its last decision with the in flight proposal with the same sequence %d", v.SelfID, lastDecisionMD.GetLatestSequence())
			if !proto.Equal(myLastDecision, proposal) {
				v.Logger.Warnf("Node %d compared its last decision with the in flight proposal, which has the same sequence, but they are not equal", v.SelfID)
				return false
			}
			return true // I already decided on the in flight proposal
		}
		if lastDecisionMD.GetLatestSequence() != proposalMD.GetLatestSequence()-1 {
			v.Logger.Panicf("Node %d got an in flight proposal with sequence %d while its last decision was on sequence %d", v.SelfID, proposalMD.GetLatestSequence(), lastDecisionMD.GetLatestSequence())
		}
	}

	v.Logger.Debugf("Node %d is creating a view %d for the in flight proposal", v.SelfID, proposalMD.GetViewId())

	inFlightViewNum := proposalMD.GetViewId()
	inFlightViewLatestSeq := proposalMD.GetLatestSequence()

	v.inFlightViewLock.Lock()
	attempt := &inFlightAttempt{
		decideCh: make(chan struct{}, 1),
		syncCh:   make(chan struct{}, 1),
	}
	callbacks := &inFlightAttemptCallbacks{
		vc:      v,
		attempt: attempt,
	}
	inFlightView := &View{
		RetrieveCheckpoint: v.Checkpoint.Get,
		DecisionsPerLeader: v.DecisionsPerLeader,
		SelfID:             v.SelfID,
		N:                  v.N,
		NodesList:          v.NodesList,
		Number:             inFlightViewNum,
		LeaderID:           v.SelfID, // so that no byzantine leader will cause a complain
		Quorum:             v.quorum,
		Decider:            callbacks,
		FailureDetector:    v,
		Sync:               callbacks,
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
	attempt.viewRef = inFlightView
	inFlightView.MetricsView.ViewNumber.Set(float64(inFlightView.Number))
	inFlightView.MetricsView.LeaderID.Set(float64(inFlightView.LeaderID))
	inFlightView.MetricsView.ProposalSequence.Set(float64(inFlightView.ProposalSequence))
	inFlightView.MetricsView.DecisionsInView.Set(float64(inFlightView.DecisionsInView))
	inFlightView.MetricsView.Phase.Set(float64(inFlightView.Phase))

	v.inFlightView = inFlightView
	v.inFlightView.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(proposal.GetVerificationSequence()),
		Metadata:             proposal.GetMetadata(),
		Payload:              proposal.GetPayload(),
		Header:               proposal.GetHeader(),
	}
	// so that once the in flight proposal is decided its requests are removed from the pool
	v.inFlightView.inFlightRequests = v.Verifier.RequestsFromProposal(*v.inFlightView.inFlightProposal)
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

	inFlightView.beginPrePrepare = time.Now() // the latency of processing the in flight proposal is measured from here
	inFlightView.Start()
	defer func() {
		inFlightView.AbortWithCancel(v.stopChan)
		v.inFlightViewLock.Lock()
		if v.inFlightView == inFlightView {
			v.inFlightView = nil
		}
		v.inFlightViewLock.Unlock()
	}()

	v.inFlightViewLock.Unlock()

	v.Logger.Debugf("Node %d started a view %d for the in flight proposal", v.SelfID, inFlightViewNum)

	// wait for view to finish or time out
	for {
		select {
		case <-attempt.decideCh:
			v.Logger.Infof("In-flight view %d with latest sequence %d has committed a decision", inFlightViewNum, inFlightViewLatestSeq)
			return true
		case <-attempt.syncCh:
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

// decideInFlight delivers the decision of the in-flight proposal view to the application
// and informs the waiter of the given attempt after delivery.
func (v *ViewChanger) decideInFlight(attempt *inFlightAttempt, proposal types.Proposal, signatures []types.Signature, requests []types.RequestInfo) {
	attempt.viewRef.stop()
	v.Logger.Debugf("Delivering to app from Decide the last decision proposal")
	reconfig := v.Application.Deliver(proposal, signatures)
	if reconfig.InLatestDecision {
		v.close()
	}
	v.Logger.Debugf("Delivering end to app from Decide the last decision proposal")

	for _, reqInfo := range requests {
		if err := v.RequestsTimer.RemoveRequest(reqInfo); err != nil {
			v.Logger.Warnf("Error during remove of request %s from the pool, err: %v", reqInfo, err)
		}
	}
	v.Pruner.MaybePruneRevokedRequests()

	// The waiter may have already left (timed out or stopped), so never block on it.
	select {
	case attempt.decideCh <- struct{}{}:
	default:
	}
}

// Complain is called by the view of the in flight proposal, which then stops, and so it is only logged
func (v *ViewChanger) Complain(viewNum uint64, stopView bool) {
	v.Logger.Warnf("Node %d has complained while in the view for the in flight proposal", v.SelfID)
}

// syncInFlight calls the synchronizer and informs the waiter of the given attempt of the sync.
func (v *ViewChanger) syncInFlight(attempt *inFlightAttempt) {
	// the in flight proposal view asked to sync
	v.Logger.Debugf("Node %d is calling sync because the in flight proposal view has asked to sync", v.SelfID)
	v.Synchronizer.Sync()
	// The waiter may have already left (timed out or stopped), so never block on it.
	select {
	case attempt.syncCh <- struct{}{}:
	default:
	}
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
	if err := proto.Unmarshal(prop.GetMetadata(), md); err != nil {
		v.Logger.Panicf("Failed unmarshalling metadata: %v", err)
	}
	return md.GetBlackList()
}
