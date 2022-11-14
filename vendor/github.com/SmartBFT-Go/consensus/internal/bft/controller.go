// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"sync"
	"sync/atomic"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
)

// Decider delivers the proposal with signatures to the application
//go:generate mockery -dir . -name Decider -case underscore -output ./mocks/
type Decider interface {
	Decide(proposal types.Proposal, signatures []types.Signature, requests []types.RequestInfo)
}

// FailureDetector initiates a view change when there is a complaint
//go:generate mockery -dir . -name FailureDetector -case underscore -output ./mocks/
type FailureDetector interface {
	Complain(viewNum uint64, stopView bool)
}

// Batcher batches requests to eventually become a new proposal
//go:generate mockery -dir . -name Batcher -case underscore -output ./mocks/
type Batcher interface {
	NextBatch() [][]byte
	Close()
	Closed() bool
	Reset()
}

// RequestPool is a pool of client's requests
//go:generate mockery -dir . -name RequestPool -case underscore -output ./mocks/
type RequestPool interface {
	Prune(predicate func([]byte) error)
	Submit(request []byte) error
	Size() int
	NextRequests(maxCount int, maxSizeBytes uint64, check bool) (batch [][]byte, full bool)
	RemoveRequest(request types.RequestInfo) error
	StopTimers()
	RestartTimers()
	Close()
}

// LeaderMonitor monitors the heartbeat from the current leader
//go:generate mockery -dir . -name LeaderMonitor -case underscore -output ./mocks/
type LeaderMonitor interface {
	ChangeRole(role Role, view uint64, leaderID uint64)
	ProcessMsg(sender uint64, msg *protos.Message)
	InjectArtificialHeartbeat(sender uint64, msg *protos.Message)
	HeartbeatWasSent()
	Close()
}

// Proposer proposes a new proposal to be agreed on
type Proposer interface {
	Propose(proposal types.Proposal)
	Start()
	Abort()
	GetMetadata() []byte
	HandleMessage(sender uint64, m *protos.Message)
}

// ProposerBuilder builds a new Proposer
//go:generate mockery -dir . -name ProposerBuilder -case underscore -output ./mocks/
type ProposerBuilder interface {
	NewProposer(leader, proposalSequence, viewNum, decisionsInView uint64, quorumSize int) Proposer
}

// Controller controls the entire flow of the consensus
type Controller struct {
	api.Comm
	// configuration
	ID                 uint64
	N                  uint64
	NodesList          []uint64
	LeaderRotation     bool
	DecisionsPerLeader uint64
	RequestPool        RequestPool
	Batcher            Batcher
	LeaderMonitor      LeaderMonitor
	Verifier           api.Verifier
	Logger             api.Logger
	Assembler          api.Assembler
	Application        api.Application
	FailureDetector    FailureDetector
	Synchronizer       api.Synchronizer
	Signer             api.Signer
	RequestInspector   api.RequestInspector
	WAL                api.WriteAheadLog
	ProposerBuilder    ProposerBuilder
	Checkpoint         *types.Checkpoint
	ViewChanger        *ViewChanger
	Collector          *StateCollector
	State              State

	quorum int

	currView Proposer

	currViewLock   sync.RWMutex
	currViewNumber uint64

	currDecisionsInViewLock sync.RWMutex
	currDecisionsInView     uint64

	viewChange    chan viewInfo
	abortViewChan chan uint64

	stopOnce sync.Once
	stopChan chan struct{}

	syncChan             chan struct{}
	decisionChan         chan decision
	deliverChan          chan struct{}
	leaderToken          chan struct{}
	verificationSequence uint64

	controllerDone sync.WaitGroup

	ViewSequences *atomic.Value

	StartedWG *sync.WaitGroup
}

func (c *Controller) blacklist() []uint64 {
	prop, _ := c.Checkpoint.Get()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prop.Metadata, md); err != nil {
		c.Logger.Panicf("Failed unmarshalling metadata: %v", err)
	}

	return md.BlackList
}

func (c *Controller) getCurrentViewNumber() uint64 {
	c.currViewLock.RLock()
	defer c.currViewLock.RUnlock()

	return c.currViewNumber
}

func (c *Controller) setCurrentViewNumber(viewNumber uint64) {
	c.currViewLock.Lock()
	defer c.currViewLock.Unlock()

	c.currViewNumber = viewNumber
}

func (c *Controller) getCurrentDecisionsInView() uint64 {
	c.currDecisionsInViewLock.RLock()
	defer c.currDecisionsInViewLock.RUnlock()

	return c.currDecisionsInView
}

func (c *Controller) incrementCurrentDecisionsInView() {
	c.currDecisionsInViewLock.Lock()
	defer c.currDecisionsInViewLock.Unlock()

	c.currDecisionsInView++
}

func (c *Controller) setCurrentDecisionsInView(decisions uint64) {
	c.currDecisionsInViewLock.Lock()
	defer c.currDecisionsInViewLock.Unlock()

	c.currDecisionsInView = decisions
}

// thread safe
func (c *Controller) iAmTheLeader() (bool, uint64) {
	leader := c.leaderID()
	return leader == c.ID, leader
}

// thread safe
func (c *Controller) leaderID() uint64 {
	return getLeaderID(c.getCurrentViewNumber(), c.N, c.NodesList, c.LeaderRotation, c.getCurrentDecisionsInView(), c.DecisionsPerLeader, c.blacklist())
}

func (c *Controller) GetLeaderID() uint64 {
	return c.leaderID()
}

// HandleRequest handles a request from the client
func (c *Controller) HandleRequest(sender uint64, req []byte) {
	iAm, leaderID := c.iAmTheLeader()
	if !iAm {
		c.Logger.Warnf("Got request from %d but the leader is %d, dropping request", sender, leaderID)
		return
	}
	reqInfo, err := c.Verifier.VerifyRequest(req)
	if err != nil {
		c.Logger.Warnf("Got bad request from %d: %v", sender, err)
		return
	}
	c.Logger.Debugf("Got request from %d", sender)
	c.addRequest(reqInfo, req)
}

// SubmitRequest Submits a request to go through consensus.
func (c *Controller) SubmitRequest(request []byte) error {
	info := c.RequestInspector.RequestID(request)
	return c.addRequest(info, request)
}

func (c *Controller) addRequest(info types.RequestInfo, request []byte) error {
	err := c.RequestPool.Submit(request)
	if err != nil {
		c.Logger.Infof("Request %s was not submitted, error: %s", info, err)
		return err
	}

	c.Logger.Debugf("Request %s was submitted", info)

	return nil
}

// OnRequestTimeout is called when request-timeout expires and forwards the request to leader.
// Called by the request-pool timeout goroutine. Upon return, the leader-forward timeout is started.
func (c *Controller) OnRequestTimeout(request []byte, info types.RequestInfo) {
	iAm, leaderID := c.iAmTheLeader()
	if iAm {
		c.Logger.Infof("Request %s timeout expired, this node is the leader, nothing to do", info)
		return
	}

	c.Logger.Infof("Request %s timeout expired, forwarding request to leader: %d", info, leaderID)
	c.Comm.SendTransaction(leaderID, request)

	return
}

// OnLeaderFwdRequestTimeout is called when the leader-forward timeout expires, and complains about the leader.
// Called by the request-pool timeout goroutine. Upon return, the auto-remove timeout is started.
func (c *Controller) OnLeaderFwdRequestTimeout(request []byte, info types.RequestInfo) {
	iAm, leaderID := c.iAmTheLeader()
	if iAm {
		c.Logger.Infof("Request %s leader-forwarding timeout expired, this node is the leader, nothing to do", info)
		return
	}

	c.Logger.Warnf("Request %s leader-forwarding timeout expired, complaining about leader: %d", info, leaderID)
	c.FailureDetector.Complain(c.getCurrentViewNumber(), true)

	return
}

// OnAutoRemoveTimeout is called when the auto-remove timeout expires.
// Called by the request-pool timeout goroutine.
func (c *Controller) OnAutoRemoveTimeout(requestInfo types.RequestInfo) {
	c.Logger.Debugf("Request %s auto-remove timeout expired, removed from the request pool", requestInfo)
}

// OnHeartbeatTimeout is called when the heartbeat timeout expires.
// Called by the HeartbeatMonitor goroutine.
func (c *Controller) OnHeartbeatTimeout(view uint64, leaderID uint64) {
	c.Logger.Debugf("Heartbeat timeout expired, reported-view: %d, reported-leader: %d", view, leaderID)

	iAm, currentLeaderID := c.iAmTheLeader()
	if iAm {
		c.Logger.Debugf("Heartbeat timeout expired, this node is the leader, nothing to do; current-view: %d, current-leader: %d",
			c.getCurrentViewNumber(), currentLeaderID)
		return
	}

	if leaderID != currentLeaderID {
		c.Logger.Warnf("Heartbeat timeout expired, but current leader: %d, differs from reported leader: %d; ignoring", currentLeaderID, leaderID)
		return
	}

	c.Logger.Warnf("Heartbeat timeout expired, complaining about leader: %d", leaderID)
	c.FailureDetector.Complain(c.getCurrentViewNumber(), true)
}

// ProcessMessages dispatches the incoming message to the required component
func (c *Controller) ProcessMessages(sender uint64, m *protos.Message) {
	c.Logger.Debugf("%d got message from %d: %s", c.ID, sender, MsgToString(m))
	switch m.GetContent().(type) {
	case *protos.Message_PrePrepare, *protos.Message_Prepare, *protos.Message_Commit:
		c.currViewLock.RLock()
		view := c.currView
		c.currViewLock.RUnlock()
		view.HandleMessage(sender, m)
		c.ViewChanger.HandleViewMessage(sender, m)
		if sender == c.leaderID() {
			c.LeaderMonitor.InjectArtificialHeartbeat(sender, c.convertViewMessageToHeartbeat(m))
		}
	case *protos.Message_ViewChange, *protos.Message_ViewData, *protos.Message_NewView:
		c.ViewChanger.HandleMessage(sender, m)
	case *protos.Message_HeartBeat, *protos.Message_HeartBeatResponse:
		c.LeaderMonitor.ProcessMsg(sender, m)
	case *protos.Message_StateTransferRequest:
		c.respondToStateTransferRequest(sender)
	case *protos.Message_StateTransferResponse:
		c.Collector.HandleMessage(sender, m)
	default:
		c.Logger.Warnf("Unexpected message type, ignoring")
	}
}

func (c *Controller) respondToStateTransferRequest(sender uint64) {
	vs := c.ViewSequences.Load()
	if vs == nil {
		c.Logger.Panicf("ViewSequences is nil")
	}
	msg := &protos.Message{
		Content: &protos.Message_StateTransferResponse{
			StateTransferResponse: &protos.StateTransferResponse{
				ViewNum:  c.getCurrentViewNumber(),
				Sequence: vs.(ViewSequence).ProposalSeq,
			},
		},
	}
	c.Comm.SendConsensus(sender, msg)
}

func (c *Controller) convertViewMessageToHeartbeat(m *protos.Message) *protos.Message {
	view := viewNumber(m)
	seq := proposalSequence(m)
	return &protos.Message{
		Content: &protos.Message_HeartBeat{
			HeartBeat: &protos.HeartBeat{
				View: view,
				Seq:  seq,
			},
		},
	}
}

func (c *Controller) startView(proposalSequence uint64) {
	view := c.ProposerBuilder.NewProposer(c.leaderID(), proposalSequence, c.currViewNumber, c.currDecisionsInView, c.quorum)

	c.currViewLock.Lock()
	c.currView = view
	c.currView.Start()
	c.currViewLock.Unlock()

	role := Follower
	leader, _ := c.iAmTheLeader()
	if leader {
		role = Leader
	}
	c.LeaderMonitor.ChangeRole(role, c.currViewNumber, c.leaderID())
	c.Logger.Infof("Starting view with number %d, sequence %d, and decisions %d", c.currViewNumber, proposalSequence, c.currDecisionsInView)
}

func (c *Controller) changeView(newViewNumber uint64, newProposalSequence uint64, newDecisionsInView uint64) {

	latestView := c.getCurrentViewNumber()
	if latestView > newViewNumber {
		c.Logger.Debugf("Got view change to %d but already at %d", newViewNumber, latestView)
		return
	}

	if !c.abortView(latestView) {
		return
	}

	c.setCurrentViewNumber(newViewNumber)
	c.setCurrentDecisionsInView(newDecisionsInView)
	c.Logger.Debugf("Starting view after setting decisions in view to %d", newDecisionsInView)
	c.startView(newProposalSequence)

	// If I'm the leader, I can claim the leader token.
	if iAm, _ := c.iAmTheLeader(); iAm {
		c.Batcher.Reset()
		c.acquireLeaderToken()
	}
}

func (c *Controller) abortView(view uint64) bool {
	currView := c.getCurrentViewNumber()
	if view < currView {
		c.Logger.Debugf("Was asked to abort view %d but the current view with number %d", view, currView)
		return false
	}

	// Drain the leader token in case we held it,
	// so we won't start proposing after view change.
	c.relinquishLeaderToken()

	// Kill current view
	c.Logger.Debugf("Aborting current view with number %d", c.currViewNumber)
	c.currView.Abort()

	return true
}

// Sync initiates a synchronization
func (c *Controller) Sync() {
	if iAmLeader, _ := c.iAmTheLeader(); iAmLeader {
		c.Batcher.Close()
	}
	c.grabSyncToken()
}

// AbortView makes the controller abort the current view
func (c *Controller) AbortView(view uint64) {
	c.Logger.Debugf("AbortView, the current view num is %d", c.getCurrentViewNumber())

	c.Batcher.Close()

	c.abortViewChan <- view
}

// ViewChanged makes the controller abort the current view and start a new one with the given numbers
func (c *Controller) ViewChanged(newViewNumber uint64, newProposalSequence uint64) {
	c.Logger.Debugf("ViewChanged, the new view is %d", newViewNumber)
	amILeader, _ := c.iAmTheLeader()
	if amILeader {
		c.Batcher.Close()
	}
	c.viewChange <- viewInfo{proposalSeq: newProposalSequence, viewNumber: newViewNumber}
}

func (c *Controller) getNextBatch() [][]byte {
	var validRequests [][]byte
	for len(validRequests) == 0 { // no valid requests in this batch
		requests := c.Batcher.NextBatch()
		if c.stopped() || c.Batcher.Closed() {
			return nil
		}
		for _, req := range requests {
			validRequests = append(validRequests, req)
		}
	}
	return validRequests
}

func (c *Controller) propose() {
	nextBatch := c.getNextBatch()
	if len(nextBatch) == 0 {
		// If our next batch is empty,
		// it can only be because
		// the batcher is stopped and so are we.
		return
	}
	metadata := c.currView.GetMetadata()
	proposal := c.Assembler.AssembleProposal(metadata, nextBatch)
	c.currView.Propose(proposal)
}

func (c *Controller) run() {
	// At exit, always make sure to kill current view
	// and wait for it to finish.
	defer func() {
		c.Logger.Infof("Exiting")
		c.currView.Abort()
	}()

	for {
		select {
		case d := <-c.decisionChan:
			c.decide(d)
		case newView := <-c.viewChange:
			c.changeView(newView.viewNumber, newView.proposalSeq, 0)
		case view := <-c.abortViewChan:
			c.abortView(view)
		case <-c.stopChan:
			return
		case <-c.leaderToken:
			c.propose()
		case <-c.syncChan:
			view, seq, dec := c.sync()
			c.MaybePruneRevokedRequests()
			if view > 0 || seq > 0 {
				c.changeView(view, seq, dec)
			} else {
				vs := c.ViewSequences.Load()
				if vs == nil {
					c.Logger.Panicf("ViewSequences is nil")
				}
				c.changeView(c.getCurrentViewNumber(), vs.(ViewSequence).ProposalSeq, c.getCurrentDecisionsInView())
			}
		}
	}
}

func (c *Controller) decide(d decision) {
	reconfig := c.Application.Deliver(d.proposal, d.signatures)
	if reconfig.InLatestDecision {
		c.close()
	}
	c.Checkpoint.Set(d.proposal, d.signatures)
	c.Logger.Debugf("Node %d delivered proposal", c.ID)
	c.removeDeliveredFromPool(d)
	select {
	case c.deliverChan <- struct{}{}:
	case <-c.stopChan:
		return
	}
	c.incrementCurrentDecisionsInView()

	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(d.proposal.Metadata, md); err != nil {
		c.Logger.Panicf("Failed to unmarshal proposal metadata, error: %v", err)
	}

	if c.checkIfRotate(md.BlackList) {
		c.Logger.Debugf("Restarting view to rotate the leader")
		c.changeView(c.getCurrentViewNumber(), md.LatestSequence+1, c.getCurrentDecisionsInView())
	}
	c.MaybePruneRevokedRequests()
	if iAm, _ := c.iAmTheLeader(); iAm {
		c.acquireLeaderToken()
	}
}

func (c *Controller) checkIfRotate(blacklist []uint64) bool {
	view := c.getCurrentViewNumber()
	decisionsInView := c.getCurrentDecisionsInView()
	c.Logger.Debugf("view(%d) + (decisionsInView(%d) / decisionsPerLeader(%d)), N(%d), blacklist(%v)",
		view, decisionsInView, c.DecisionsPerLeader, c.N, blacklist)
	// called after increment
	currLeader := getLeaderID(view, c.N, c.NodesList, c.LeaderRotation, decisionsInView-1, c.DecisionsPerLeader, blacklist)
	nextLeader := getLeaderID(view, c.N, c.NodesList, c.LeaderRotation, decisionsInView, c.DecisionsPerLeader, blacklist)
	shouldWeRotate := currLeader != nextLeader
	if shouldWeRotate {
		c.Logger.Infof("Rotating leader from %d to %d", currLeader, nextLeader)
	}

	return shouldWeRotate
}

func (c *Controller) sync() (viewNum uint64, seq uint64, decisions uint64) {
	// Block any concurrent sync attempt.
	c.grabSyncToken()
	// At exit, enable sync once more, but ignore
	// all synchronization attempts done while
	// we were syncing.
	defer c.relinquishSyncToken()

	syncResponse := c.Synchronizer.Sync()
	if syncResponse.Reconfig.InReplicatedDecisions {
		c.close()
		c.ViewChanger.close()
	}
	decision := syncResponse.Latest
	if decision.Proposal.Metadata == nil {
		c.Logger.Infof("Synchronizer returned with proposal metadata nil")
		response := c.fetchState()
		if response == nil {
			return 0, 0, 0
		}
		if response.View > 0 && response.Seq == 1 {
			c.Logger.Infof("The collected state is with view %d and sequence %d", response.View, response.Seq)
			newViewToSave := &protos.SavedMessage{
				Content: &protos.SavedMessage_NewView{
					NewView: &protos.ViewMetadata{
						ViewId:          response.View,
						LatestSequence:  0,
						DecisionsInView: 0,
					},
				},
			}
			if err := c.State.Save(newViewToSave); err != nil {
				c.Logger.Panicf("Failed to save message to state, error: %v", err)
			}
			c.ViewChanger.InformNewView(response.View)
			return response.View, 1, 0
		}
		return 0, 0, 0
	}
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(decision.Proposal.Metadata, md); err != nil {
		c.Logger.Panicf("Controller was unable to unmarshal the proposal metadata returned by the Synchronizer")
	}
	if md.ViewId < c.currViewNumber {
		c.Logger.Infof("Synchronizer returned with view number %d but the controller is at view number %d", md.ViewId, c.currViewNumber)
		return 0, 0, 0
	}
	c.Logger.Infof("Synchronized to view %d and sequence %d with verification sequence %d", md.ViewId, md.LatestSequence, decision.Proposal.VerificationSequence)

	view := md.ViewId
	newView := false

	response := c.fetchState()
	if response != nil {
		if response.View > md.ViewId && response.Seq == md.LatestSequence+1 {
			c.Logger.Infof("The collected state is with view %d and sequence %d", response.View, response.Seq)
			view = response.View
			newViewToSave := &protos.SavedMessage{
				Content: &protos.SavedMessage_NewView{
					NewView: &protos.ViewMetadata{
						ViewId:          view,
						LatestSequence:  md.LatestSequence,
						DecisionsInView: 0,
					},
				},
			}
			if err := c.State.Save(newViewToSave); err != nil {
				c.Logger.Panicf("Failed to save message to state, error: %v", err)
			}
			newView = true
		}
	}

	c.Logger.Debugf("Node %d is setting the checkpoint after sync to view %d and seq %d", c.ID, md.ViewId, md.LatestSequence)
	c.Checkpoint.Set(decision.Proposal, decision.Signatures)
	c.verificationSequence = uint64(decision.Proposal.VerificationSequence)
	c.Logger.Debugf("Node %d is informing the view changer after sync of view %d and seq %d", c.ID, md.ViewId, md.LatestSequence)
	c.ViewChanger.InformNewView(view)
	if md.LatestSequence == 0 || newView {
		return view, md.LatestSequence + 1, 0
	}
	return view, md.LatestSequence + 1, md.DecisionsInView + 1
}

func (c *Controller) fetchState() *types.ViewAndSeq {
	msg := &protos.Message{
		Content: &protos.Message_StateTransferRequest{
			StateTransferRequest: &protos.StateTransferRequest{},
		},
	}
	c.Collector.ClearCollected()
	c.BroadcastConsensus(msg)
	return c.Collector.CollectStateResponses()
}

func (c *Controller) grabSyncToken() {
	select {
	case c.syncChan <- struct{}{}:
	default:
	}
}

func (c *Controller) relinquishSyncToken() {
	select {
	case <-c.syncChan:
	default:
	}
}

// MaybePruneRevokedRequests prunes requests with different verification sequence
func (c *Controller) MaybePruneRevokedRequests() {
	oldVerSqn := c.verificationSequence
	newVerSqn := c.Verifier.VerificationSequence()
	if newVerSqn == oldVerSqn {
		return
	}
	c.verificationSequence = newVerSqn

	c.Logger.Infof("Verification sequence changed: %d --> %d", oldVerSqn, newVerSqn)
	c.RequestPool.Prune(func(req []byte) error {
		_, err := c.Verifier.VerifyRequest(req)
		return err
	})
}

func (c *Controller) acquireLeaderToken() {
	select {
	case c.leaderToken <- struct{}{}:
	default:
		// No room, seems we're already a leader.
	}
}

func (c *Controller) relinquishLeaderToken() {
	select {
	case <-c.leaderToken:
	default:

	}
}

func (c *Controller) syncOnStart(startViewNumber uint64, startProposalSequence uint64, startDecisionsInView uint64) (viewNum uint64, seq uint64, decisions uint64) {
	syncView, syncSeq, syncDecsions := c.sync()
	c.MaybePruneRevokedRequests()
	viewNum = startViewNumber
	seq = startProposalSequence
	decisions = startDecisionsInView
	if syncView > startViewNumber {
		viewNum = syncView
		decisions = syncDecsions
	}
	if syncSeq > startProposalSequence {
		seq = syncSeq
		decisions = syncDecsions
	}
	return viewNum, seq, decisions
}

// Start the controller
func (c *Controller) Start(startViewNumber uint64, startProposalSequence uint64, startDecisionsInView uint64, syncOnStart bool) {
	c.Logger.Debugf("Starting controller with view %d, sequence %d, and decisions %d", startViewNumber, startProposalSequence, startDecisionsInView)
	c.controllerDone.Add(1)
	c.stopOnce = sync.Once{}
	c.syncChan = make(chan struct{}, 1)
	c.stopChan = make(chan struct{})
	c.leaderToken = make(chan struct{}, 1)
	c.decisionChan = make(chan decision)
	c.deliverChan = make(chan struct{})
	c.viewChange = make(chan viewInfo, 1)
	c.abortViewChan = make(chan uint64, 1)

	Q, F := computeQuorum(c.N)
	c.Logger.Debugf("The number of nodes (N) is %d, F is %d, and the quorum size is %d", c.N, F, Q)
	c.quorum = Q

	c.verificationSequence = c.Verifier.VerificationSequence()

	if syncOnStart {
		startViewNumber, startProposalSequence, startDecisionsInView = c.syncOnStart(startViewNumber, startProposalSequence, startDecisionsInView)
		c.Logger.Debugf("After sync starting controller with view %d, sequence %d, and decisions %d", startViewNumber, startProposalSequence, startDecisionsInView)
	}

	c.currViewNumber = startViewNumber
	c.currDecisionsInView = startDecisionsInView
	c.startView(startProposalSequence)
	if iAm, _ := c.iAmTheLeader(); iAm {
		c.acquireLeaderToken()
	}

	go func() {
		defer c.controllerDone.Done()
		c.run()
	}()

	c.StartedWG.Done()
}

func (c *Controller) close() {
	c.stopOnce.Do(
		func() {
			select {
			case <-c.stopChan:
				return
			default:
				close(c.stopChan)
			}
		},
	)
}

// Stop the controller
func (c *Controller) Stop() {
	c.close()
	c.Batcher.Close()
	c.RequestPool.Close()
	c.LeaderMonitor.Close()

	// Drain the leader token if we hold it.
	select {
	case <-c.leaderToken:
	default:
		// Do nothing
	}

	c.controllerDone.Wait()
}

// Stop the controller but only stop the requests pool timers
func (c *Controller) StopWithPoolPause() {
	c.close()
	c.Batcher.Close()
	c.RequestPool.StopTimers()
	c.LeaderMonitor.Close()

	// Drain the leader token if we hold it.
	select {
	case <-c.leaderToken:
	default:
		// Do nothing
	}

	c.controllerDone.Wait()
}

func (c *Controller) stopped() bool {
	select {
	case <-c.stopChan:
		return true
	default:
		return false
	}
}

// Decide delivers the decision to the application
func (c *Controller) Decide(proposal types.Proposal, signatures []types.Signature, requests []types.RequestInfo) {
	select {
	case c.decisionChan <- decision{
		proposal:   proposal,
		requests:   requests,
		signatures: signatures,
	}:
	case <-c.stopChan:
		// In case we are in the middle of shutting down,
		// abort deciding.
		return
	}

	select {
	case <-c.deliverChan: // wait for the delivery of the decision to the application
	case <-c.stopChan: // If we stopped the controller, abort delivery
	}

}

func (c *Controller) removeDeliveredFromPool(d decision) {
	for _, reqInfo := range d.requests {
		if err := c.RequestPool.RemoveRequest(reqInfo); err != nil {
			c.Logger.Debugf("Request %s wasn't found in the pool : %s", reqInfo, err)
		}
	}
}

type viewInfo struct {
	viewNumber  uint64
	proposalSeq uint64
}

type decision struct {
	proposal   types.Proposal
	signatures []types.Signature
	requests   []types.RequestInfo
}

//BroadcastConsensus broadcasts the message and informs the heartbeat monitor if necessary
func (c *Controller) BroadcastConsensus(m *protos.Message) {
	for _, node := range c.NodesList {
		// Do not send to yourself
		if c.ID == node {
			continue
		}
		c.Comm.SendConsensus(node, m)
	}

	if m.GetPrePrepare() != nil || m.GetPrepare() != nil || m.GetCommit() != nil {
		if leader, _ := c.iAmTheLeader(); leader {
			c.LeaderMonitor.HeartbeatWasSent()
		}
	}
}
