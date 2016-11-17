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

package gossip

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bytes"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
)

var individualTimeout = time.Second * time.Duration(60)
var perTestTimeout = time.Second * time.Duration(180)

func init() {
	aliveTimeInterval := time.Duration(1000) * time.Millisecond
	discovery.SetAliveTimeInternal(aliveTimeInterval)
	discovery.SetAliveExpirationCheckInterval(aliveTimeInterval)
	discovery.SetExpirationTimeout(aliveTimeInterval * 10)
	discovery.SetReconnectInterval(aliveTimeInterval * 5)
}

var portPrefix = 5610
var testLock = sync.RWMutex{}

func acceptData(m interface{}) bool {
	if dataMsg := m.(*proto.GossipMessage).GetDataMsg(); dataMsg != nil {
		return true
	}
	return false
}

type naiveCryptoService struct {
}

func (*naiveCryptoService) ValidateAliveMsg(am *proto.AliveMessage) bool {
	return true
}

func (*naiveCryptoService) SignMessage(am *proto.AliveMessage) *proto.AliveMessage {
	return am
}

func (*naiveCryptoService) IsEnabled() bool {
	return true
}

func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (*naiveCryptoService) Verify(vkID, signature, message []byte) error {
	if bytes.Equal(signature, message) {
		return nil
	}
	return fmt.Errorf("Failed verifying")
}

func bootPeers(ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", (id+portPrefix)))
	}
	return peers
}

func newGossipInstance(id int, maxMsgCount int, boot ...int) Gossip {
	port := id + portPrefix
	conf := &Config{
		BindPort:       port,
		BootstrapPeers: bootPeers(boot...),
		ID:             fmt.Sprintf("p%d", id),
		MaxMessageCountToStore:     maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(10) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(4) * time.Second,
		PullPeerNum:                5,
		SelfEndpoint:               fmt.Sprintf("localhost:%d", port),
	}
	comm, err := comm.NewCommInstanceWithServer(port, &naiveCryptoService{}, []byte(conf.SelfEndpoint))
	if err != nil {
		panic(err)
	}
	return NewGossipService(conf, comm, &naiveCryptoService{})
}

func newGossipInstanceWithOnlyPull(id int, maxMsgCount int, boot ...int) Gossip {
	port := id + portPrefix
	conf := &Config{
		BindPort:       port,
		BootstrapPeers: bootPeers(boot...),
		ID:             fmt.Sprintf("p%d", id),
		MaxMessageCountToStore:     maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(10) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        0,
		PropagatePeerNum:           0,
		PullInterval:               time.Duration(1000) * time.Millisecond,
		PullPeerNum:                20,
		SelfEndpoint:               fmt.Sprintf("localhost:%d", port),
	}
	comm, err := comm.NewCommInstanceWithServer(port, &naiveCryptoService{}, []byte(conf.SelfEndpoint))
	if err != nil {
		panic(err)
	}
	return NewGossipService(conf, comm, &naiveCryptoService{})
}

func TestPull(t *testing.T) {
	t1 := time.Now()
	// Scenario: Turn off forwarding and use only pull-based gossip.
	// First phase: Ensure full membership view for all nodes
	// Second phase: Disseminate 10 messages and ensure all nodes got them

	testLock.Lock()
	defer testLock.Unlock()

	shortenedWaitTime := time.Duration(500) * time.Millisecond
	algo.SetDigestWaitTime(shortenedWaitTime / 5)
	algo.SetRequestWaitTime(shortenedWaitTime)
	algo.SetResponseWaitTime(shortenedWaitTime)

	defer func() {
		algo.SetDigestWaitTime(time.Duration(1) * time.Second)
		algo.SetRequestWaitTime(time.Duration(1) * time.Second)
		algo.SetResponseWaitTime(time.Duration(2) * time.Second)
	}()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 5
	msgsCount2Send := 10
	boot := newGossipInstanceWithOnlyPull(0, 100)
	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			pI := newGossipInstanceWithOnlyPull(i, 100, 0)
			peers[i-1] = pI
			wg.Done()
		}(i)
	}
	wg.Wait()

	knowAll := func() bool {
		for i := 1; i <= n; i++ {
			neighborCount := len(peers[i-1].GetPeers())
			if n != neighborCount {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, knowAll)

	receivedMessages := make([]int, n)
	wg = sync.WaitGroup{}
	for i := 1; i <= n; i++ {
		go func(i int) {
			go func(index int, ch <-chan *proto.GossipMessage) {
				wg.Add(1)
				defer wg.Done()
				for j := 0; j < msgsCount2Send; j++ {
					<-ch
					receivedMessages[index]++
				}
			}(i-1, peers[i-1].Accept(acceptData))
		}(i)
	}

	for i := 1; i <= msgsCount2Send; i++ {
		boot.Gossip(createDataMsg(uint64(i), []byte{}, ""))
	}
	time.Sleep(time.Duration(3) * time.Second)

	waitUntilOrFailBlocking(t, wg.Wait)

	receivedAll := func() bool {
		for i := 0; i < n; i++ {
			if msgsCount2Send != receivedMessages[i] {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, receivedAll)

	stop := func() {
		stopPeers(append(peers, boot))
	}

	waitUntilOrFailBlocking(t, stop)

	fmt.Println("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	ensureGoroutineExit(t)
}

func TestMembership(t *testing.T) {
	t1 := time.Now()
	// Scenario: spawn 20 nodes and a single bootstrap node and then:
	// 1) Check full membership views for all nodes but the bootstrap node.
	// 2) Update metadata of last peer and ensure it propagates to all peers
	testLock.Lock()
	defer testLock.Unlock()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 20
	var lastPeer = fmt.Sprintf("localhost:%d", (n + portPrefix))
	boot := newGossipInstance(0, 100)

	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			pI := newGossipInstance(i, 100, 0)
			peers[i-1] = pI
			wg.Done()
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait)

	seeAllNeighbors := func() bool {
		for i := 1; i <= n; i++ {
			neighborCount := len(peers[i-1].GetPeers())
			if neighborCount != n {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, seeAllNeighbors)

	fmt.Println("Updating metadata...")

	// Change metadata in last node
	peers[len(peers)-1].UpdateMetadata([]byte("bla bla"))

	metaDataUpdated := func() bool {
		if "bla bla" != string(metadataOfPeer(boot.GetPeers(), lastPeer)) {
			return false
		}
		for i := 0; i < n-1; i++ {
			if "bla bla" != string(metadataOfPeer(peers[i].GetPeers(), lastPeer)) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, metaDataUpdated)

	stop := func() {
		stopPeers(append(peers, boot))
	}

	waitUntilOrFailBlocking(t, stop)

	fmt.Println("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))

	ensureGoroutineExit(t)
}

func TestDissemination(t *testing.T) {
	t1 := time.Now()
	// Scenario: 20 nodes and a bootstrap node.
	// The bootstrap node sends 10 messages and we count
	// that each node got 10 messages after a few seconds
	testLock.Lock()
	defer testLock.Unlock()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 20
	msgsCount2Send := 10
	boot := newGossipInstance(0, 100)

	peers := make([]Gossip, n)
	receivedMessages := make([]int, n)
	wg := sync.WaitGroup{}

	for i := 1; i <= n; i++ {
		pI := newGossipInstance(i, 100, 0)
		peers[i-1] = pI

		wg.Add(1)
		go func(index int, ch <-chan *proto.GossipMessage) {
			defer wg.Done()
			for j := 0; j < msgsCount2Send; j++ {
				<-ch
				receivedMessages[index]++
			}
		}(i-1, pI.Accept(acceptData))
	}

	waitUntilOrFail(t, checkPeersMembership(peers, n))

	for i := 1; i <= msgsCount2Send; i++ {
		boot.Gossip(createDataMsg(uint64(i), []byte{}, ""))
	}

	waitUntilOrFailBlocking(t, wg.Wait)

	for i := 0; i < n; i++ {
		assert.Equal(t, msgsCount2Send, receivedMessages[i])
	}
	fmt.Println("Stopping peers")

	stop := func() {
		stopPeers(append(peers, boot))
	}

	waitUntilOrFailBlocking(t, stop)

	fmt.Println("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	ensureGoroutineExit(t)
}

func TestMembershipConvergence(t *testing.T) {
	// Scenario: Spawn 12 nodes and 3 bootstrap peers
	// but assign each node to its bootstrap peer group modulo 3.
	// Then:
	// 1) Check all groups know only themselves in the view and not others.
	// 2) Bring up a node that will connect to all bootstrap peers.
	// 3) Wait a few seconds and check that all views converged to a single one
	// 4) Kill that last node, wait a while and:
	// 4)a) Ensure all nodes consider it as dead
	// 4)b) Ensure all node still know each other

	testLock.Lock()
	defer testLock.Unlock()

	t1 := time.Now()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)
	boot0 := newGossipInstance(0, 100)
	boot1 := newGossipInstance(1, 100)
	boot2 := newGossipInstance(2, 100)

	time.Sleep(time.Duration(4) * time.Second)

	peers := []Gossip{boot0, boot1, boot2}
	// 0: {3, 6, 9, 12}
	// 1: {4, 7, 10, 13}
	// 2: {5, 8, 11, 14}
	for i := 3; i < 15; i++ {
		pI := newGossipInstance(i, 100, i%3)
		peers = append(peers, pI)
	}

	waitUntilOrFail(t, checkPeersMembership(peers, 4))

	connectorPeer := newGossipInstance(15, 100, 0, 1, 2)
	connectorPeer.UpdateMetadata([]byte("Connector"))

	fullKnowledge := func() bool {
		for i := 0; i < 15; i++ {
			if 15 != len(peers[i].GetPeers()) {
				return false
			}
			if "Connector" != string(metadataOfPeer(peers[i].GetPeers(), "localhost:5625")) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, fullKnowledge)

	fmt.Println("Stopping connector...")
	waitUntilOrFailBlocking(t, connectorPeer.Stop)
	fmt.Println("Stopped")
	time.Sleep(time.Duration(15) * time.Second)

	ensureForget := func() bool {
		for i := 0; i < 15; i++ {
			if 14 != len(peers[i].GetPeers()) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureForget)

	connectorPeer = newGossipInstance(15, 100)
	connectorPeer.UpdateMetadata([]byte("Connector"))
	fmt.Println("Started connector")

	ensureResync := func() bool {
		for i := 0; i < 15; i++ {
			if 15 != len(peers[i].GetPeers()) {
				return false
			}
			if "Connector" != string(metadataOfPeer(peers[i].GetPeers(), "localhost:5625")) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureResync)

	waitUntilOrFailBlocking(t, connectorPeer.Stop)

	fmt.Println("Stopping peers")
	stop := func() {
		stopPeers(peers)
	}

	waitUntilOrFailBlocking(t, stop)
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("Took", time.Since(t1))
	ensureGoroutineExit(t)
}

func createDataMsg(seqnum uint64, data []byte, hash string) *proto.GossipMessage {
	return &proto.GossipMessage{
		Nonce: 0,
		Tag:   proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   data,
					Hash:   hash,
					SeqNum: seqnum,
				},
			},
		},
	}
}

var runTests = func(g goroutine) bool {
	return searchInStackTrace("testing.RunTests", g.stack)
}

var waitForTestCompl = func(g goroutine) bool {
	return searchInStackTrace("waitForTestCompletion", g.stack)
}

var goExit = func(g goroutine) bool {
	return searchInStackTrace("runtime.goexit", g.stack)
}

var clientConn = func(g goroutine) bool {
	return searchInStackTrace("resetTransport", g.stack)
}

var testingg = func(g goroutine) bool {
	return strings.Index(g.stack[len(g.stack)-1], "testing.go") != -1
}

func shouldNotBeRunningAtEnd(gr goroutine) bool {
	return !runTests(gr) && !goExit(gr) && !testingg(gr) && !clientConn(gr) && !waitForTestCompl(gr)
}

func ensureGoroutineExit(t *testing.T) {
	for i := 0; i <= 20; i++ {
		time.Sleep(time.Second)
		allEnded := true
		for _, gr := range getGoRoutines() {
			if shouldNotBeRunningAtEnd(gr) {
				allEnded = false
				continue
			}

			if shouldNotBeRunningAtEnd(gr) && i == 20 {
				assert.Fail(t, "Goroutine(s) haven't ended:", fmt.Sprintf("%v", gr.stack))
				for _, gr2 := range getGoRoutines() {
					for _, ste := range gr2.stack {
						t.Log(ste)
					}
					t.Log("")
				}
				break
			}
		}

		if allEnded {
			return
		}
	}
}

func metadataOfPeer(members []discovery.NetworkMember, endpoint string) []byte {
	for _, member := range members {
		if member.Endpoint == endpoint {
			return member.Metadata
		}
	}
	return nil
}

func waitForTestCompletion(stopFlag *int32, t *testing.T) {
	time.Sleep(perTestTimeout)
	if atomic.LoadInt32(stopFlag) == int32(1) {
		return
	}
	util.PrintStackTrace()
	assert.Fail(t, "Didn't stop within a timely manner")
}

func stopPeers(peers []Gossip) {
	stoppingWg := sync.WaitGroup{}
	for i, pI := range peers {
		stoppingWg.Add(1)
		go func(i int, p_i Gossip) {
			defer stoppingWg.Done()
			p_i.Stop()
		}(i, pI)
	}
	stoppingWg.Wait()
	time.Sleep(time.Second * time.Duration(2))
}

func getGoroutineRawText() string {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	return string(buf)
}

func getGoRoutines() []goroutine {
	goroutines := []goroutine{}
	s := getGoroutineRawText()
	a := strings.Split(s, "goroutine ")
	for _, s := range a {
		gr := strings.Split(s, "\n")
		idStr := bytes.TrimPrefix([]byte(gr[0]), []byte("goroutine "))
		i := (strings.Index(string(idStr), " "))
		if i == -1 {
			continue
		}
		id, _ := strconv.ParseUint(string(string(idStr[:i])), 10, 64)
		stack := []string{}
		for i := 1; i < len(gr); i++ {
			if len([]byte(gr[i])) != 0 {
				stack = append(stack, gr[i])
			}
		}
		goroutines = append(goroutines, goroutine{id: id, stack: stack})
	}
	return goroutines
}

type goroutine struct {
	id    uint64
	stack []string
}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	start := time.Now()
	limit := start.UnixNano() + individualTimeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(individualTimeout / 60)
	}
	util.PrintStackTrace()
	assert.Fail(t, "Timeout expired!")
}

func waitUntilOrFailBlocking(t *testing.T, f func()) {
	successChan := make(chan struct{}, 1)
	go func() {
		f()
		successChan <- struct{}{}
	}()
	select {
	case <-time.NewTimer(individualTimeout).C:
		break
	case <-successChan:
		return
	}
	util.PrintStackTrace()
	assert.Fail(t, "Timeout expired!")
}

func searchInStackTrace(searchTerm string, stack []string) bool {
	for _, ste := range stack {
		if strings.Index(ste, searchTerm) != -1 {
			return true
		}
	}
	return false
}

func checkPeersMembership(peers []Gossip, n int) func() bool {
	return func() bool {
		for _, peer := range peers {
			if len(peer.GetPeers()) != n {
				return false
			}
		}
		return true
	}
}
