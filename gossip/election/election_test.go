/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	testTimeout      = 5 * time.Second
	testPollInterval = time.Millisecond * 300
)

func init() {
	startupGracePeriod = time.Millisecond * 500
	membershipSampleInterval = time.Millisecond * 100
	leaderAliveThreshold = time.Millisecond * 500
	leadershipDeclarationInterval = leaderAliveThreshold / 2
	leaderElectionDuration = time.Millisecond * 500
}

type msg struct {
	sender   string
	proposal bool
}

func (m *msg) SenderID() string {
	return m.sender
}

func (m *msg) IsProposal() bool {
	return m.proposal
}

func (m *msg) IsDeclaration() bool {
	return !m.proposal
}

type peer struct {
	mockedMethods map[string]struct{}
	mock.Mock
	id                   string
	peers                map[string]*peer
	sharedLock           *sync.RWMutex
	msgChan              chan Msg
	isLeaderFromCallback bool
	callbackInvoked      bool
	LeaderElectionService
}

func (p *peer) On(methodName string, arguments ...interface{}) *mock.Call {
	p.sharedLock.Lock()
	defer p.sharedLock.Unlock()
	p.mockedMethods[methodName] = struct{}{}
	return p.Mock.On(methodName, arguments...)
}

func (p *peer) ID() string {
	return p.id
}

func (p *peer) Gossip(m Msg) {
	p.sharedLock.RLock()
	defer p.sharedLock.RUnlock()

	if _, isMocked := p.mockedMethods["Gossip"]; isMocked {
		p.Called(m)
		return
	}

	for _, peer := range p.peers {
		if peer.id == p.id {
			continue
		}
		peer.msgChan <- m.(*msg)
	}
}

func (p *peer) Accept() <-chan Msg {
	p.sharedLock.RLock()
	defer p.sharedLock.RUnlock()

	if _, isMocked := p.mockedMethods["Accept"]; isMocked {
		args := p.Called()
		return args.Get(0).(<-chan Msg)
	}
	return (<-chan Msg)(p.msgChan)
}

func (p *peer) CreateMessage(isDeclaration bool) Msg {
	return &msg{proposal: !isDeclaration, sender: p.id}
}

func (p *peer) Peers() []Peer {
	p.sharedLock.RLock()
	defer p.sharedLock.RUnlock()

	if _, isMocked := p.mockedMethods["Peers"]; isMocked {
		args := p.Called()
		return args.Get(0).([]Peer)
	}

	var peers []Peer
	for id := range p.peers {
		peers = append(peers, &peer{id: id})
	}
	return peers
}

func (p *peer) leaderCallback(isLeader bool) {
	p.isLeaderFromCallback = isLeader
	p.callbackInvoked = true
}

func createPeers(spawnInterval time.Duration, ids ...int) []*peer {
	peers := make([]*peer, len(ids))
	peerMap := make(map[string]*peer)
	l := &sync.RWMutex{}
	for i, id := range ids {
		p := createPeer(id, peerMap, l)
		if spawnInterval != 0 {
			time.Sleep(spawnInterval)
		}
		peers[i] = p
	}
	return peers
}

func createPeer(id int, peerMap map[string]*peer, l *sync.RWMutex) *peer {
	idStr := fmt.Sprintf("p%d", id)
	c := make(chan Msg, 100)
	p := &peer{id: idStr, peers: peerMap, sharedLock: l, msgChan: c, mockedMethods: make(map[string]struct{}), isLeaderFromCallback: false, callbackInvoked: false}
	p.LeaderElectionService = NewLeaderElectionService(p, idStr, p.leaderCallback)
	l.Lock()
	peerMap[idStr] = p
	l.Unlock()
	return p

}

func waitForMultipleLeadersElection(t *testing.T, peers []*peer, leadersNum int) []string {
	end := time.Now().Add(testTimeout)
	for time.Now().Before(end) {
		var leaders []string
		for _, p := range peers {
			if p.IsLeader() {
				leaders = append(leaders, p.id)
			}
		}
		if len(leaders) >= leadersNum {
			return leaders
		}
		time.Sleep(testPollInterval)
	}
	t.Fatal("No leader detected")
	return nil
}

func waitForLeaderElection(t *testing.T, peers []*peer) []string {
	return waitForMultipleLeadersElection(t, peers, 1)
}

func TestInitPeersAtSameTime(t *testing.T) {
	t.Parallel()
	// Scenario: Peers are spawned at the same time
	// expected outcome: the peer that has the lowest ID is the leader
	peers := createPeers(0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0)
	time.Sleep(startupGracePeriod + leaderElectionDuration)
	leaders := waitForLeaderElection(t, peers)
	isP0leader := peers[len(peers)-1].IsLeader()
	assert.True(t, isP0leader, "p0 isn't a leader. Leaders are: %v", leaders)
	assert.True(t, peers[len(peers)-1].isLeaderFromCallback, "p0 didn't got leaderhip change callback invoked")
	assert.Len(t, leaders, 1, "More than 1 leader elected")
}

func TestInitPeersStartAtIntervals(t *testing.T) {
	t.Parallel()
	// Scenario: Peers are spawned one by one in a slow rate
	// expected outcome: the first peer is the leader although its ID is lowest
	peers := createPeers(startupGracePeriod+leadershipDeclarationInterval, 3, 2, 1, 0)
	waitForLeaderElection(t, peers)
	assert.True(t, peers[0].IsLeader())
}

func TestStop(t *testing.T) {
	t.Parallel()
	// Scenario: peers are spawned at the same time
	// and then are stopped. We count the number of Gossip() invocations they invoke
	// after they stop, and it should not increase after they are stopped
	peers := createPeers(0, 3, 2, 1, 0)
	var gossipCounter int32
	for i, p := range peers {
		p.On("Gossip", mock.Anything).Run(func(args mock.Arguments) {
			msg := args.Get(0).(Msg)
			atomic.AddInt32(&gossipCounter, int32(1))
			for j := range peers {
				if i == j {
					continue
				}
				peers[j].msgChan <- msg
			}
		})
	}
	waitForLeaderElection(t, peers)
	for _, p := range peers {
		p.Stop()
	}
	time.Sleep(leaderAliveThreshold)
	gossipCounterAfterStop := atomic.LoadInt32(&gossipCounter)
	time.Sleep(leaderAliveThreshold * 5)
	assert.Equal(t, gossipCounterAfterStop, atomic.LoadInt32(&gossipCounter))
}

func TestConvergence(t *testing.T) {
	// Scenario: 2 peer group converge their views
	// expected outcome: only 1 leader is left out of the 2
	// and that leader is the leader with the lowest ID
	t.Parallel()
	peers1 := createPeers(0, 3, 2, 1, 0)
	peers2 := createPeers(0, 4, 5, 6, 7)
	leaders1 := waitForLeaderElection(t, peers1)
	leaders2 := waitForLeaderElection(t, peers2)
	assert.Len(t, leaders1, 1, "Peer group 1 was suppose to have 1 leader exactly")
	assert.Len(t, leaders2, 1, "Peer group 2 was suppose to have 1 leader exactly")
	combinedPeers := append(peers1, peers2...)

	var allPeerIds []Peer
	for _, p := range combinedPeers {
		allPeerIds = append(allPeerIds, &peer{id: p.id})
	}

	for i, p := range combinedPeers {
		index := i
		gossipFunc := func(args mock.Arguments) {
			msg := args.Get(0).(Msg)
			for j := range combinedPeers {
				if index == j {
					continue
				}
				combinedPeers[j].msgChan <- msg
			}
		}
		p.On("Gossip", mock.Anything).Run(gossipFunc)
		p.On("Peers").Return(allPeerIds)
	}

	time.Sleep(leaderAliveThreshold * 5)
	finalLeaders := waitForLeaderElection(t, combinedPeers)
	assert.Len(t, finalLeaders, 1, "Combined peer group was suppose to have 1 leader exactly")
	assert.Equal(t, leaders1[0], finalLeaders[0], "Combined peer group has different leader than expected:")

	for _, p := range combinedPeers {
		if p.id == finalLeaders[0] {
			assert.True(t, p.isLeaderFromCallback, "Leadership callback result is wrong for ", p.id)
			assert.True(t, p.callbackInvoked, "Leadership callback wasn't invoked for ", p.id)
		} else {
			assert.False(t, p.isLeaderFromCallback, "Leadership callback result is wrong for ", p.id)
			if p.id == leaders2[0] {
				assert.True(t, p.callbackInvoked, "Leadership callback wasn't invoked for ", p.id)
			}
		}
	}
}

func TestLeadershipTakeover(t *testing.T) {
	t.Parallel()
	// Scenario: Peers spawn one by one in descending order.
	// After a while, the leader peer stops.
	// expected outcome: the peer that takes over is the peer with lowest ID
	peers := createPeers(startupGracePeriod+leadershipDeclarationInterval, 5, 4, 3, 2)
	leaders := waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p5", leaders[0])
	peers[0].Stop()
	time.Sleep(leadershipDeclarationInterval + leaderAliveThreshold*3)
	leaders = waitForLeaderElection(t, peers[1:])
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p2", leaders[0])
}

func TestPartition(t *testing.T) {
	t.Parallel()
	// Scenario: peers spawn together, and then after a while a network partition occurs
	// and no peer can communicate with another peer
	// Expected outcome 1: each peer is a leader
	// After this, we heal the partition to be a unified view again
	// Expected outcome 2: p0 is the leader once again
	peers := createPeers(0, 5, 4, 3, 2, 1, 0)
	leaders := waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
	assert.True(t, peers[len(peers)-1].isLeaderFromCallback, "Leadership callback result is wrong for %s", peers[len(peers)-1].id)

	for _, p := range peers {
		p.On("Peers").Return([]Peer{})
		p.On("Gossip", mock.Anything)
	}
	time.Sleep(leadershipDeclarationInterval + leaderAliveThreshold*2)
	leaders = waitForMultipleLeadersElection(t, peers, 6)
	assert.Len(t, leaders, 6)
	for _, p := range peers {
		assert.True(t, p.isLeaderFromCallback, "Leadership callback result is wrong for %s", p.id)
	}

	for _, p := range peers {
		p.sharedLock.Lock()
		p.mockedMethods = make(map[string]struct{})
		p.callbackInvoked = false
		p.sharedLock.Unlock()
	}
	time.Sleep(leadershipDeclarationInterval + leaderAliveThreshold*2)
	leaders = waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
	for _, p := range peers {
		if p.id == leaders[0] {
			assert.True(t, p.isLeaderFromCallback, "Leadership callback result is wrong for %", p.id)
		} else {
			assert.False(t, p.isLeaderFromCallback, "Leadership callback result is wrong for %s", p.id)
			assert.True(t, p.callbackInvoked, "Leadership callback wasn't invoked for %s", p.id)
		}
	}

}
