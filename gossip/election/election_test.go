/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package election

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	testTimeout                       = 5 * time.Second
	testPollInterval                  = time.Millisecond * 300
	testStartupGracePeriod            = time.Millisecond * 500
	testMembershipSampleInterval      = time.Millisecond * 100
	testLeaderAliveThreshold          = time.Millisecond * 500
	testLeaderElectionDuration        = time.Millisecond * 500
	testLeadershipDeclarationInterval = testLeaderAliveThreshold / 2
)

func init() {
	util.SetupTestLogging()
}

type msg struct {
	sender   string
	proposal bool
}

func (m *msg) SenderID() peerID {
	return peerID(m.sender)
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
	id                 string
	peers              map[string]*peer
	sharedLock         *sync.RWMutex
	msgChan            chan Msg
	leaderFromCallback bool
	callbackInvoked    bool
	lock               sync.RWMutex
	LeaderElectionService
}

func (p *peer) On(methodName string, arguments ...interface{}) *mock.Call {
	p.sharedLock.Lock()
	defer p.sharedLock.Unlock()
	p.mockedMethods[methodName] = struct{}{}
	return p.Mock.On(methodName, arguments...)
}

func (p *peer) ID() peerID {
	return peerID(p.id)
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

func (p *peer) ReportMetrics(isLeader bool) {
	p.Mock.Called(isLeader)
}

func (p *peer) leaderCallback(isLeader bool) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.leaderFromCallback = isLeader
	p.callbackInvoked = true
}

func (p *peer) isLeaderFromCallback() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.leaderFromCallback
}

func (p *peer) isCallbackInvoked() bool {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.callbackInvoked
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

func createPeerWithCostumeMetrics(id int, peerMap map[string]*peer, l *sync.RWMutex, f func(mock.Arguments)) *peer {
	idStr := fmt.Sprintf("p%d", id)
	c := make(chan Msg, 100)
	p := &peer{id: idStr, peers: peerMap, sharedLock: l, msgChan: c, mockedMethods: make(map[string]struct{}), leaderFromCallback: false, callbackInvoked: false}
	p.On("ReportMetrics", mock.Anything).Run(f)
	config := ElectionConfig{
		StartupGracePeriod:       testStartupGracePeriod,
		MembershipSampleInterval: testMembershipSampleInterval,
		LeaderAliveThreshold:     testLeaderAliveThreshold,
		LeaderElectionDuration:   testLeaderElectionDuration,
	}
	p.LeaderElectionService = NewLeaderElectionService(p, idStr, p.leaderCallback, config)
	l.Lock()
	peerMap[idStr] = p
	l.Unlock()
	return p

}

func createPeer(id int, peerMap map[string]*peer, l *sync.RWMutex) *peer {
	return createPeerWithCostumeMetrics(id, peerMap, l, func(mock.Arguments) {})
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

func TestMetrics(t *testing.T) {
	t.Parallel()
	// Scenario: spawn a single peer and ensure it reports being a leader after some time.
	// Then, make it relinquish its leadership and then ensure it reports not being a leader.
	var wgLeader sync.WaitGroup
	var wgFollower sync.WaitGroup
	wgLeader.Add(1)
	wgFollower.Add(1)
	var once sync.Once
	var once2 sync.Once
	f := func(args mock.Arguments) {
		if args[0] == true {
			once.Do(func() {
				wgLeader.Done()
			})
		} else {
			once2.Do(func() {
				wgFollower.Done()
			})
		}
	}

	p := createPeerWithCostumeMetrics(0, make(map[string]*peer), &sync.RWMutex{}, f)
	waitForLeaderElection(t, []*peer{p})

	// Ensure we sent a leadership declaration during the time of leadership acquisition
	wgLeader.Wait()
	p.AssertCalled(t, "ReportMetrics", true)

	p.Yield()
	assert.False(t, p.IsLeader())

	// Ensure declaration for not being a leader was sent
	wgFollower.Wait()
	p.AssertCalled(t, "ReportMetrics", false)

	waitForLeaderElection(t, []*peer{p})
}

func TestInitPeersAtSameTime(t *testing.T) {
	t.Parallel()
	// Scenario: Peers are spawned at the same time
	// expected outcome: the peer that has the lowest ID is the leader
	peers := createPeers(0, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0)
	time.Sleep(testStartupGracePeriod + testLeaderElectionDuration)
	leaders := waitForLeaderElection(t, peers)
	isP0leader := peers[len(peers)-1].IsLeader()
	assert.True(t, isP0leader, "p0 isn't a leader. Leaders are: %v", leaders)
	assert.Len(t, leaders, 1, "More than 1 leader elected")
	waitForBoolFunc(t, peers[len(peers)-1].isLeaderFromCallback, true, "Leadership callback result is wrong for ", peers[len(peers)-1].id)
}

func TestInitPeersStartAtIntervals(t *testing.T) {
	t.Parallel()
	// Scenario: Peers are spawned one by one in a slow rate
	// expected outcome: the first peer is the leader although its ID is highest
	peers := createPeers(testStartupGracePeriod+testLeadershipDeclarationInterval, 3, 2, 1, 0)
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
	time.Sleep(testLeaderAliveThreshold)
	gossipCounterAfterStop := atomic.LoadInt32(&gossipCounter)
	time.Sleep(testLeaderAliveThreshold * 5)
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

	time.Sleep(testLeaderAliveThreshold * 5)
	finalLeaders := waitForLeaderElection(t, combinedPeers)
	assert.Len(t, finalLeaders, 1, "Combined peer group was suppose to have 1 leader exactly")
	assert.Equal(t, leaders1[0], finalLeaders[0], "Combined peer group has different leader than expected:")

	for _, p := range combinedPeers {
		if p.id == finalLeaders[0] {
			waitForBoolFunc(t, p.isLeaderFromCallback, true, "Leadership callback result is wrong for ", p.id)
			waitForBoolFunc(t, p.isCallbackInvoked, true, "Leadership callback wasn't invoked for ", p.id)
		} else {
			waitForBoolFunc(t, p.isLeaderFromCallback, false, "Leadership callback result is wrong for ", p.id)
			if p.id == leaders2[0] {
				waitForBoolFunc(t, p.isCallbackInvoked, true, "Leadership callback wasn't invoked for ", p.id)
			}
		}
	}
}

func TestLeadershipTakeover(t *testing.T) {
	t.Parallel()
	// Scenario: Peers spawn one by one in descending order.
	// After a while, the leader peer stops.
	// expected outcome: the peer that takes over is the peer with lowest ID
	peers := createPeers(testStartupGracePeriod+testLeadershipDeclarationInterval, 5, 4, 3, 2)
	leaders := waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p5", leaders[0])
	peers[0].Stop()
	time.Sleep(testLeadershipDeclarationInterval + testLeaderAliveThreshold*3)
	leaders = waitForLeaderElection(t, peers[1:])
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p2", leaders[0])
}

func TestYield(t *testing.T) {
	t.Parallel()
	// Scenario: Peers spawn and a leader is elected.
	// After a while, the leader yields.
	// (Call yield twice to ensure only one callback is called)
	// Expected outcome:
	// (1) A new leader is elected
	// (2) The old leader doesn't take back its leadership
	peers := createPeers(0, 0, 1, 2, 3, 4, 5)
	leaders := waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
	peers[0].Yield()
	// Ensure the callback was called with 'false'
	assert.True(t, peers[0].isCallbackInvoked())
	assert.False(t, peers[0].isLeaderFromCallback())
	// Clear the callback invoked flag
	peers[0].lock.Lock()
	peers[0].callbackInvoked = false
	peers[0].lock.Unlock()
	// Yield again and ensure it isn't called again
	peers[0].Yield()
	assert.False(t, peers[0].isCallbackInvoked())

	ensureP0isNotAleader := func() bool {
		leaders := waitForLeaderElection(t, peers)
		return len(leaders) == 1 && leaders[0] != "p0"
	}
	// A new leader is elected, and it is not p0
	waitForBoolFunc(t, ensureP0isNotAleader, true)
	time.Sleep(testLeaderAliveThreshold * 2)
	// After a while, p0 doesn't restore its leadership status
	waitForBoolFunc(t, ensureP0isNotAleader, true)
}

func TestYieldSinglePeer(t *testing.T) {
	t.Parallel()
	// Scenario: spawn a single peer and have it yield.
	// Ensure it recovers its leadership after a while.
	peers := createPeers(0, 0)
	waitForLeaderElection(t, peers)
	peers[0].Yield()
	assert.False(t, peers[0].IsLeader())
	waitForLeaderElection(t, peers)
}

func TestYieldAllPeers(t *testing.T) {
	t.Parallel()
	// Scenario: spawn 2 peers and have them all yield after regaining leadership.
	// Ensure the first peer is the leader in the end after both peers yield
	peers := createPeers(0, 0, 1)
	leaders := waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
	peers[0].Yield()
	leaders = waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p1", leaders[0])
	peers[1].Yield()
	leaders = waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
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
	waitForBoolFunc(t, peers[len(peers)-1].isLeaderFromCallback, true, "Leadership callback result is wrong for %s", peers[len(peers)-1].id)

	for _, p := range peers {
		p.On("Peers").Return([]Peer{})
		p.On("Gossip", mock.Anything)
	}
	time.Sleep(testLeadershipDeclarationInterval + testLeaderAliveThreshold*2)
	leaders = waitForMultipleLeadersElection(t, peers, 6)
	assert.Len(t, leaders, 6)
	for _, p := range peers {
		waitForBoolFunc(t, p.isLeaderFromCallback, true, "Leadership callback result is wrong for %s", p.id)
	}

	for _, p := range peers {
		p.sharedLock.Lock()
		p.mockedMethods = make(map[string]struct{})
		p.callbackInvoked = false
		p.sharedLock.Unlock()
	}
	time.Sleep(testLeadershipDeclarationInterval + testLeaderAliveThreshold*2)
	leaders = waitForLeaderElection(t, peers)
	assert.Len(t, leaders, 1, "Only 1 leader should have been elected")
	assert.Equal(t, "p0", leaders[0])
	for _, p := range peers {
		if p.id == leaders[0] {
			waitForBoolFunc(t, p.isLeaderFromCallback, true, "Leadership callback result is wrong for %s", p.id)
		} else {
			waitForBoolFunc(t, p.isLeaderFromCallback, false, "Leadership callback result is wrong for %s", p.id)
			waitForBoolFunc(t, p.isCallbackInvoked, true, "Leadership callback wasn't invoked for %s", p.id)
		}
	}
}

func Test_peerIDString(t *testing.T) {
	tests := []struct {
		input    peerID
		expected string
	}{
		{nil, "<nil>"},
		{peerID{}, ""},
		{peerID{0, 1, 2, 3}, "00010203"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.String())
	}
}

func waitForBoolFunc(t *testing.T, f func() bool, expectedValue bool, msgAndArgs ...interface{}) {
	end := time.Now().Add(testTimeout)
	for time.Now().Before(end) {
		if f() == expectedValue {
			return
		}
		time.Sleep(testPollInterval)
	}
	assert.Fail(t, fmt.Sprintf("Should be %t", expectedValue), msgAndArgs...)
}
