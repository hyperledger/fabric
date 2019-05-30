/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package election

import (
	"bytes"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/util"
)

// Gossip leader election module
// Algorithm properties:
// - Peers break symmetry by comparing IDs
// - Each peer is either a leader or a follower,
//   and the aim is to have exactly 1 leader if the membership view
//   is the same for all peers
// - If the network is partitioned into 2 or more sets, the number of leaders
//   is the number of network partitions, but when the partition heals,
//   only 1 leader should be left eventually
// - Peers communicate by gossiping leadership proposal or declaration messages

// The Algorithm, in pseudo code:
//
//
// variables:
// 	leaderKnown = false
//
// Invariant:
//	Peer listens for messages from remote peers
//	and whenever it receives a leadership declaration,
//	leaderKnown is set to true
//
// Startup():
// 	wait for membership view to stabilize, or for a leadership declaration is received
//      or the startup timeout expires.
//	goto SteadyState()
//
// SteadyState():
// 	while true:
//		If leaderKnown is false:
// 			LeaderElection()
//		If you are the leader:
//			Broadcast leadership declaration
//			If a leadership declaration was received from
// 			a peer with a lower ID,
//			become a follower
//		Else, you're a follower:
//			If haven't received a leadership declaration within
// 			a time threshold:
//				set leaderKnown to false
//
// LeaderElection():
// 	Gossip leadership proposal message
//	Collect messages from other peers sent within a time period
//	If received a leadership declaration:
//		return
//	Iterate over all proposal messages collected.
// 	If a proposal message from a peer with an ID lower
// 	than yourself was received, return.
//	Else, declare yourself a leader

// LeaderElectionAdapter is used by the leader election module
// to send and receive messages and to get membership information
type LeaderElectionAdapter interface {
	// Gossip gossips a message to other peers
	Gossip(Msg)

	// Accept returns a channel that emits messages
	Accept() <-chan Msg

	// CreateProposalMessage
	CreateMessage(isDeclaration bool) Msg

	// Peers returns a list of peers considered alive
	Peers() []Peer

	// ReportMetrics sends a report to the metrics server about a leadership status
	ReportMetrics(isLeader bool)
}

type leadershipCallback func(isLeader bool)

// LeaderElectionService is the object that runs the leader election algorithm
type LeaderElectionService interface {
	// IsLeader returns whether this peer is a leader or not
	IsLeader() bool

	// Stop stops the LeaderElectionService
	Stop()

	// Yield relinquishes the leadership until a new leader is elected,
	// or a timeout expires
	Yield()
}

type peerID []byte

func (p peerID) String() string {
	if p == nil {
		return "<nil>"
	}
	return hex.EncodeToString(p)
}

// Peer describes a remote peer
type Peer interface {
	// ID returns the ID of the peer
	ID() peerID
}

// Msg describes a message sent from a remote peer
type Msg interface {
	// SenderID returns the ID of the peer sent the message
	SenderID() peerID
	// IsProposal returns whether this message is a leadership proposal
	IsProposal() bool
	// IsDeclaration returns whether this message is a leadership declaration
	IsDeclaration() bool
}

func noopCallback(_ bool) {
}

const (
	DefStartupGracePeriod       = time.Second * 15
	DefMembershipSampleInterval = time.Second
	DefLeaderAliveThreshold     = time.Second * 10
	DefLeaderElectionDuration   = time.Second * 5
)

type ElectionConfig struct {
	StartupGracePeriod       time.Duration
	MembershipSampleInterval time.Duration
	LeaderAliveThreshold     time.Duration
	LeaderElectionDuration   time.Duration
}

// NewLeaderElectionService returns a new LeaderElectionService
func NewLeaderElectionService(adapter LeaderElectionAdapter, id string, callback leadershipCallback, config ElectionConfig) LeaderElectionService {
	if len(id) == 0 {
		panic("Empty id")
	}
	le := &leaderElectionSvcImpl{
		id:            peerID(id),
		proposals:     util.NewSet(),
		adapter:       adapter,
		stopChan:      make(chan struct{}),
		interruptChan: make(chan struct{}, 1),
		logger:        util.GetLogger(util.ElectionLogger, ""),
		callback:      noopCallback,
		config:        config,
	}

	if callback != nil {
		le.callback = callback
	}

	go le.start()
	return le
}

// leaderElectionSvcImpl is an implementation of a LeaderElectionService
type leaderElectionSvcImpl struct {
	id        peerID
	proposals *util.Set
	sync.Mutex
	stopChan      chan struct{}
	interruptChan chan struct{}
	stopWG        sync.WaitGroup
	isLeader      int32
	leaderExists  int32
	yield         int32
	sleeping      bool
	adapter       LeaderElectionAdapter
	logger        util.Logger
	callback      leadershipCallback
	yieldTimer    *time.Timer
	config        ElectionConfig
}

func (le *leaderElectionSvcImpl) start() {
	le.stopWG.Add(2)
	go le.handleMessages()
	le.waitForMembershipStabilization(le.config.StartupGracePeriod)
	go le.run()
}

func (le *leaderElectionSvcImpl) handleMessages() {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting")
	defer le.stopWG.Done()
	msgChan := le.adapter.Accept()
	for {
		select {
		case <-le.stopChan:
			return
		case msg := <-msgChan:
			if !le.isAlive(msg.SenderID()) {
				le.logger.Debug(le.id, ": Got message from", msg.SenderID(), "but it is not in the view")
				break
			}
			le.handleMessage(msg)
		}
	}
}

func (le *leaderElectionSvcImpl) handleMessage(msg Msg) {
	msgType := "proposal"
	if msg.IsDeclaration() {
		msgType = "declaration"
	}
	le.logger.Debug(le.id, ":", msg.SenderID(), "sent us", msgType)
	le.Lock()
	defer le.Unlock()

	if msg.IsProposal() {
		le.proposals.Add(string(msg.SenderID()))
	} else if msg.IsDeclaration() {
		atomic.StoreInt32(&le.leaderExists, int32(1))
		if le.sleeping && len(le.interruptChan) == 0 {
			le.interruptChan <- struct{}{}
		}
		if bytes.Compare(msg.SenderID(), le.id) < 0 && le.IsLeader() {
			le.stopBeingLeader()
		}
	} else {
		// We shouldn't get here
		le.logger.Error("Got a message that's not a proposal and not a declaration")
	}
}

// waitForInterrupt sleeps until the interrupt channel is triggered
// or given timeout expires
func (le *leaderElectionSvcImpl) waitForInterrupt(timeout time.Duration) {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting")
	le.Lock()
	le.sleeping = true
	le.Unlock()

	select {
	case <-le.interruptChan:
	case <-le.stopChan:
	case <-time.After(timeout):
	}

	le.Lock()
	le.sleeping = false
	// We drain the interrupt channel
	// because we might get 2 leadership declarations messages
	// while sleeping, but we would only read 1 of them in the select block above
	le.drainInterruptChannel()
	le.Unlock()
}

func (le *leaderElectionSvcImpl) run() {
	defer le.stopWG.Done()
	for !le.shouldStop() {
		if !le.isLeaderExists() {
			le.leaderElection()
		}
		// If we are yielding and some leader has been elected,
		// stop yielding
		if le.isLeaderExists() && le.isYielding() {
			le.stopYielding()
		}
		if le.shouldStop() {
			return
		}
		if le.IsLeader() {
			le.leader()
		} else {
			le.follower()
		}
	}
}

func (le *leaderElectionSvcImpl) leaderElection() {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting")
	// If we're yielding to other peers, do not participate
	// in leader election
	if le.isYielding() {
		return
	}
	// Propose ourselves as a leader
	le.propose()
	// Collect other proposals
	le.waitForInterrupt(le.config.LeaderElectionDuration)
	// If someone declared itself as a leader, give up
	// on trying to become a leader too
	if le.isLeaderExists() {
		le.logger.Info(le.id, ": Some peer is already a leader")
		return
	}

	if le.isYielding() {
		le.logger.Debug(le.id, ": Aborting leader election because yielding")
		return
	}
	// Leader doesn't exist, let's see if there is a better candidate than us
	// for being a leader
	for _, o := range le.proposals.ToArray() {
		id := o.(string)
		if bytes.Compare(peerID(id), le.id) < 0 {
			return
		}
	}
	// If we got here, there is no one that proposed being a leader
	// that's a better candidate than us.
	le.beLeader()
	atomic.StoreInt32(&le.leaderExists, int32(1))
}

// propose sends a leadership proposal message to remote peers
func (le *leaderElectionSvcImpl) propose() {
	le.logger.Debug(le.id, ": Entering")
	le.logger.Debug(le.id, ": Exiting")
	leadershipProposal := le.adapter.CreateMessage(false)
	le.adapter.Gossip(leadershipProposal)
}

func (le *leaderElectionSvcImpl) follower() {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting")

	le.proposals.Clear()
	atomic.StoreInt32(&le.leaderExists, int32(0))
	le.adapter.ReportMetrics(false)
	select {
	case <-time.After(le.config.LeaderAliveThreshold):
	case <-le.stopChan:
	}
}

func (le *leaderElectionSvcImpl) leader() {
	leaderDeclaration := le.adapter.CreateMessage(true)
	le.adapter.Gossip(leaderDeclaration)
	le.adapter.ReportMetrics(true)
	le.waitForInterrupt(le.config.LeaderAliveThreshold / 2)
}

// waitForMembershipStabilization waits for membership view to stabilize
// or until a time limit expires, or until a peer declares itself as a leader
func (le *leaderElectionSvcImpl) waitForMembershipStabilization(timeLimit time.Duration) {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting, peers found", len(le.adapter.Peers()))
	endTime := time.Now().Add(timeLimit)
	viewSize := len(le.adapter.Peers())
	for !le.shouldStop() {
		time.Sleep(le.config.MembershipSampleInterval)
		newSize := len(le.adapter.Peers())
		if newSize == viewSize || time.Now().After(endTime) || le.isLeaderExists() {
			return
		}
		viewSize = newSize
	}
}

// drainInterruptChannel clears the interruptChannel
// if needed
func (le *leaderElectionSvcImpl) drainInterruptChannel() {
	if len(le.interruptChan) == 1 {
		<-le.interruptChan
	}
}

// isAlive returns whether peer of given id is considered alive
func (le *leaderElectionSvcImpl) isAlive(id peerID) bool {
	for _, p := range le.adapter.Peers() {
		if bytes.Equal(p.ID(), id) {
			return true
		}
	}
	return false
}

func (le *leaderElectionSvcImpl) isLeaderExists() bool {
	return atomic.LoadInt32(&le.leaderExists) == int32(1)
}

// IsLeader returns whether this peer is a leader
func (le *leaderElectionSvcImpl) IsLeader() bool {
	isLeader := atomic.LoadInt32(&le.isLeader) == int32(1)
	le.logger.Debug(le.id, ": Returning", isLeader)
	return isLeader
}

func (le *leaderElectionSvcImpl) beLeader() {
	le.logger.Info(le.id, ": Becoming a leader")
	atomic.StoreInt32(&le.isLeader, int32(1))
	le.callback(true)
}

func (le *leaderElectionSvcImpl) stopBeingLeader() {
	le.logger.Info(le.id, "Stopped being a leader")
	atomic.StoreInt32(&le.isLeader, int32(0))
	le.callback(false)
}

func (le *leaderElectionSvcImpl) shouldStop() bool {
	select {
	case <-le.stopChan:
		return true
	default:
		return false
	}
}

func (le *leaderElectionSvcImpl) isYielding() bool {
	return atomic.LoadInt32(&le.yield) == int32(1)
}

func (le *leaderElectionSvcImpl) stopYielding() {
	le.logger.Debug("Stopped yielding")
	le.Lock()
	defer le.Unlock()
	atomic.StoreInt32(&le.yield, int32(0))
	le.yieldTimer.Stop()
}

// Yield relinquishes the leadership until a new leader is elected,
// or a timeout expires
func (le *leaderElectionSvcImpl) Yield() {
	le.Lock()
	defer le.Unlock()
	if !le.IsLeader() || le.isYielding() {
		return
	}
	// Turn on the yield flag
	atomic.StoreInt32(&le.yield, int32(1))
	// Stop being a leader
	le.stopBeingLeader()
	// Clear the leader exists flag since it could be that we are the leader
	atomic.StoreInt32(&le.leaderExists, int32(0))
	// Clear the yield flag in any case afterwards
	le.yieldTimer = time.AfterFunc(le.config.LeaderAliveThreshold*6, func() {
		atomic.StoreInt32(&le.yield, int32(0))
	})
}

// Stop stops the LeaderElectionService
func (le *leaderElectionSvcImpl) Stop() {
	select {
	case <-le.stopChan:
	default:
		close(le.stopChan)
		le.logger.Debug(le.id, ": Entering")
		defer le.logger.Debug(le.id, ": Exiting")
		le.stopWG.Wait()
	}
}
