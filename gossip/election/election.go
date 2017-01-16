/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package election

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/util"
	"github.com/op/go-logging"
)

var (
	startupGracePeriod            = time.Second * 15
	membershipSampleInterval      = time.Second
	leaderAliveThreshold          = time.Second * 10
	leadershipDeclarationInterval = leaderAliveThreshold / 2
	leaderElectionDuration        = time.Second * 5
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
}

type leadershipCallback func(isLeader bool)

// LeaderElectionService is the object that runs the leader election algorithm
type LeaderElectionService interface {
	// IsLeader returns whether this peer is a leader or not
	IsLeader() bool

	// Stop stops the LeaderElectionService
	Stop()
}

// Peer describes a remote peer
type Peer interface {
	// ID returns the ID of the peer
	ID() string
}

// Msg describes a message sent from a remote peer
type Msg interface {
	// SenderID returns the ID of the peer sent the message
	SenderID() string
	// IsProposal returns whether this message is a leadership proposal
	IsProposal() bool
	// IsDeclaration returns whether this message is a leadership declaration
	IsDeclaration() bool
}

func noopCallback(_ bool) {
}

// NewLeaderElectionService returns a new LeaderElectionService
func NewLeaderElectionService(adapter LeaderElectionAdapter, id string, callback leadershipCallback) LeaderElectionService {
	if len(id) == 0 {
		panic(fmt.Errorf("Empty id"))
	}
	le := &leaderElectionSvcImpl{
		id:            id,
		proposals:     util.NewSet(),
		adapter:       adapter,
		stopChan:      make(chan struct{}, 1),
		interruptChan: make(chan struct{}, 1),
		logger:        util.GetLogger(util.LoggingElectionModule, ""),
		callback:      noopCallback,
	}

	if callback != nil {
		le.callback = callback
	}

	go le.start()
	return le
}

// leaderElectionSvcImpl is an implementation of a LeaderElectionService
type leaderElectionSvcImpl struct {
	id        string
	proposals *util.Set
	sync.Mutex
	stopChan      chan struct{}
	interruptChan chan struct{}
	stopWG        sync.WaitGroup
	isLeader      int32
	toDie         int32
	leaderExists  int32
	sleeping      bool
	adapter       LeaderElectionAdapter
	logger        *logging.Logger
	callback      leadershipCallback
}

func (le *leaderElectionSvcImpl) start() {
	le.stopWG.Add(2)
	go le.handleMessages()
	le.waitForMembershipStabilization(startupGracePeriod)
	go le.run()
}

func (le *leaderElectionSvcImpl) handleMessages() {
	le.logger.Info(le.id, ": Entering")
	defer le.logger.Info(le.id, ": Exiting")
	defer le.stopWG.Done()
	msgChan := le.adapter.Accept()
	for {
		select {
		case <-le.stopChan:
			le.stopChan <- struct{}{}
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
		le.proposals.Add(msg.SenderID())
	} else if msg.IsDeclaration() {
		atomic.StoreInt32(&le.leaderExists, int32(1))
		if le.sleeping && len(le.interruptChan) == 0 {
			le.interruptChan <- struct{}{}
		}
		if msg.SenderID() < le.id && le.IsLeader() {
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
		le.stopChan <- struct{}{}
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
	le.logger.Info(le.id, ": Entering")
	defer le.logger.Info(le.id, ": Exiting")
	le.propose()
	le.waitForInterrupt(leaderElectionDuration)
	// If someone declared itself as a leader, give up
	// on trying to become a leader too
	if le.isLeaderExists() {
		le.logger.Info(le.id, ": Some peer is already a leader")
		return
	}
	// Leader doesn't exist, let's see if there is a better candidate than us
	// for being a leader
	for _, o := range le.proposals.ToArray() {
		id := o.(string)
		if id < le.id {
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
	le.logger.Info(le.id, ": Entering")
	le.logger.Info(le.id, ": Exiting")
	leadershipProposal := le.adapter.CreateMessage(false)
	le.adapter.Gossip(leadershipProposal)
}

func (le *leaderElectionSvcImpl) follower() {
	le.logger.Debug(le.id, ": Entering")
	defer le.logger.Debug(le.id, ": Exiting")

	le.proposals.Clear()
	atomic.StoreInt32(&le.leaderExists, int32(0))
	select {
	case <-time.After(leaderAliveThreshold):
	case <-le.stopChan:
		le.stopChan <- struct{}{}
	}
}

func (le *leaderElectionSvcImpl) leader() {
	leaderDeclaration := le.adapter.CreateMessage(true)
	le.adapter.Gossip(leaderDeclaration)
	le.waitForInterrupt(leadershipDeclarationInterval)
}

// waitForMembershipStabilization waits for membership view to stabilize
// or until a time limit expires, or until a peer declares itself as a leader
func (le *leaderElectionSvcImpl) waitForMembershipStabilization(timeLimit time.Duration) {
	le.logger.Info(le.id, ": Entering")
	defer le.logger.Info(le.id, ": Exiting")
	endTime := time.Now().Add(timeLimit)
	viewSize := len(le.adapter.Peers())
	for !le.shouldStop() {
		time.Sleep(membershipSampleInterval)
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
func (le *leaderElectionSvcImpl) isAlive(id string) bool {
	for _, p := range le.adapter.Peers() {
		if p.ID() == id {
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
	return atomic.LoadInt32(&le.toDie) == int32(1)
}

// Stop stops the LeaderElectionService
func (le *leaderElectionSvcImpl) Stop() {
	le.logger.Info(le.id, ": Entering")
	defer le.logger.Info(le.id, ": Exiting")
	atomic.StoreInt32(&le.toDie, int32(1))
	le.stopChan <- struct{}{}
	le.stopWG.Wait()
}
