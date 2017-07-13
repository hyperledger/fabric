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
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

var timeout = time.Second * time.Duration(180)

var testWG = sync.WaitGroup{}

func init() {
	util.SetupTestLogging()
	rand.Seed(int64(time.Now().Second()))
	aliveTimeInterval := time.Duration(1000) * time.Millisecond
	discovery.SetAliveTimeInterval(aliveTimeInterval)
	discovery.SetAliveExpirationCheckInterval(aliveTimeInterval)
	discovery.SetAliveExpirationTimeout(aliveTimeInterval * 10)
	discovery.SetReconnectInterval(aliveTimeInterval)
	discovery.SetMaxConnAttempts(5)
	testWG.Add(6)
	factory.InitFactories(nil)
	identityExpirationCheckInterval = time.Second
}

var orgInChannelA = api.OrgIdentityType("ORG1")

func acceptData(m interface{}) bool {
	if dataMsg := m.(*proto.GossipMessage).GetDataMsg(); dataMsg != nil {
		return true
	}
	return false
}

func acceptLeadershp(message interface{}) bool {
	validMsg := message.(*proto.GossipMessage).Tag == proto.GossipMessage_CHAN_AND_ORG &&
		message.(*proto.GossipMessage).IsLeadershipMsg()

	return validMsg
}

type joinChanMsg struct {
	members2AnchorPeers map[string][]api.AnchorPeer
}

// SequenceNumber returns the sequence number of the block this joinChanMsg
// is derived from
func (*joinChanMsg) SequenceNumber() uint64 {
	return uint64(time.Now().UnixNano())
}

// Members returns the organizations of the channel
func (jcm *joinChanMsg) Members() []api.OrgIdentityType {
	if jcm.members2AnchorPeers == nil {
		return []api.OrgIdentityType{orgInChannelA}
	}
	members := make([]api.OrgIdentityType, len(jcm.members2AnchorPeers))
	i := 0
	for org := range jcm.members2AnchorPeers {
		members[i] = api.OrgIdentityType(org)
		i++
	}
	return members
}

// AnchorPeersOf returns the anchor peers of the given organization
func (jcm *joinChanMsg) AnchorPeersOf(org api.OrgIdentityType) []api.AnchorPeer {
	if jcm.members2AnchorPeers == nil {
		return []api.AnchorPeer{}
	}
	return jcm.members2AnchorPeers[string(org)]
}

type naiveCryptoService struct {
	sync.RWMutex
	allowedPkiIDS map[string]struct{}
	revokedPkiIDS map[string]struct{}
}

type orgCryptoService struct {
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity
func (*orgCryptoService) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return orgInChannelA
}

// Verify verifies a JoinChanMessage, returns nil on success,
// and an error on failure
func (*orgCryptoService) Verify(joinChanMsg api.JoinChannelMessage) error {
	return nil
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (cs *naiveCryptoService) VerifyByChannel(_ common.ChainID, identity api.PeerIdentityType, _, _ []byte) error {
	if cs.allowedPkiIDS == nil {
		return nil
	}
	if _, allowed := cs.allowedPkiIDS[string(identity)]; allowed {
		return nil
	}
	return errors.New("Forbidden")
}

func (cs *naiveCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	cs.RLock()
	defer cs.RUnlock()
	if cs.revokedPkiIDS == nil {
		return nil
	}
	if _, revoked := cs.revokedPkiIDS[string(cs.GetPKIidOfCert(peerIdentity))]; revoked {
		return errors.New("revoked")
	}
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveCryptoService) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	sig := make([]byte, len(msg))
	copy(sig, msg)
	return sig, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*naiveCryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong signature:%v, %v", signature, message)
	}
	return nil
}

func (cs *naiveCryptoService) revoke(pkiID common.PKIidType) {
	cs.Lock()
	defer cs.Unlock()
	if cs.revokedPkiIDS == nil {
		cs.revokedPkiIDS = map[string]struct{}{}
	}
	cs.revokedPkiIDS[string(pkiID)] = struct{}{}
}

func bootPeers(portPrefix int, ids ...int) []string {
	peers := []string{}
	for _, id := range ids {
		peers = append(peers, fmt.Sprintf("localhost:%d", (id+portPrefix)))
	}
	return peers
}

func newGossipInstanceWithCustomMCS(portPrefix int, id int, maxMsgCount int, mcs api.MessageCryptoService, boot ...int) Gossip {
	port := id + portPrefix
	conf := &Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(portPrefix, boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(500) * time.Millisecond,
		MaxPropagationBurstSize:    20,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Duration(2) * time.Second,
		PullPeerNum:                5,
		InternalEndpoint:           fmt.Sprintf("localhost:%d", port),
		ExternalEndpoint:           fmt.Sprintf("1.2.3.4:%d", port),
		PublishCertPeriod:          time.Duration(4) * time.Second,
		PublishStateInfoInterval:   time.Duration(1) * time.Second,
		RequestStateInfoInterval:   time.Duration(1) * time.Second,
	}
	selfId := api.PeerIdentityType(conf.InternalEndpoint)
	idMapper := identity.NewIdentityMapper(mcs, selfId)
	g := NewGossipServiceWithServer(conf, &orgCryptoService{}, mcs, idMapper,
		selfId, nil)

	return g
}

func newGossipInstance(portPrefix int, id int, maxMsgCount int, boot ...int) Gossip {
	return newGossipInstanceWithCustomMCS(portPrefix, id, maxMsgCount, &naiveCryptoService{}, boot...)
}

func newGossipInstanceWithOnlyPull(portPrefix int, id int, maxMsgCount int, boot ...int) Gossip {
	port := id + portPrefix
	conf := &Config{
		BindPort:                   port,
		BootstrapPeers:             bootPeers(portPrefix, boot...),
		ID:                         fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:       maxMsgCount,
		MaxPropagationBurstLatency: time.Duration(1000) * time.Millisecond,
		MaxPropagationBurstSize:    10,
		PropagateIterations:        0,
		PropagatePeerNum:           0,
		PullInterval:               time.Duration(1000) * time.Millisecond,
		PullPeerNum:                20,
		InternalEndpoint:           fmt.Sprintf("localhost:%d", port),
		ExternalEndpoint:           fmt.Sprintf("1.2.3.4:%d", port),
		PublishCertPeriod:          time.Duration(0) * time.Second,
		PublishStateInfoInterval:   time.Duration(1) * time.Second,
		RequestStateInfoInterval:   time.Duration(1) * time.Second,
	}

	cryptoService := &naiveCryptoService{}
	selfId := api.PeerIdentityType(conf.InternalEndpoint)
	idMapper := identity.NewIdentityMapper(cryptoService, selfId)

	g := NewGossipServiceWithServer(conf, &orgCryptoService{}, cryptoService, idMapper,
		selfId, nil)
	return g
}

func TestPull(t *testing.T) {
	t.Parallel()

	portPrefix := 5610
	t1 := time.Now()
	// Scenario: Turn off forwarding and use only pull-based gossip.
	// First phase: Ensure full membership view for all nodes
	// Second phase: Disseminate 10 messages and ensure all nodes got them

	shortenedWaitTime := time.Duration(200) * time.Millisecond
	algo.SetDigestWaitTime(shortenedWaitTime)
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

	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(i int) {
			defer wg.Done()
			pI := newGossipInstanceWithOnlyPull(portPrefix, i, 100, 0)
			pI.JoinChan(&joinChanMsg{}, common.ChainID("A"))
			pI.UpdateChannelMetadata([]byte{}, common.ChainID("A"))
			peers[i-1] = pI
		}(i)
	}
	wg.Wait()

	time.Sleep(time.Second)

	boot := newGossipInstanceWithOnlyPull(portPrefix, 0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChainID("A"))
	boot.UpdateChannelMetadata([]byte("bla bla"), common.ChainID("A"))

	knowAll := func() bool {
		for i := 1; i <= n; i++ {
			neighborCount := len(peers[i-1].Peers())
			if n != neighborCount {
				return false
			}
		}
		return true
	}

	receivedMessages := make([]int, n)
	wg = sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(i int) {
			acceptChan, _ := peers[i-1].Accept(acceptData, false)
			go func(index int, ch <-chan *proto.GossipMessage) {
				defer wg.Done()
				for j := 0; j < msgsCount2Send; j++ {
					<-ch
					receivedMessages[index]++
				}
			}(i-1, acceptChan)
		}(i)
	}

	for i := 1; i <= msgsCount2Send; i++ {
		boot.Gossip(createDataMsg(uint64(i), []byte{}, common.ChainID("A")))
	}

	waitUntilOrFail(t, knowAll)
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

	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestPull>>>")
	testWG.Done()
}

func TestConnectToAnchorPeers(t *testing.T) {
	t.Parallel()
	// Scenario: spawn 10 peers, and have them join a channel
	// of 3 anchor peers that don't exist yet.
	// Wait 5 seconds, and then spawn a random anchor peer out of the 3.
	// Ensure that all peers successfully see each other in the channel

	portPrefix := 8610
	// Scenario: Spawn 5 peers, and make each of them connect to
	// the other 2 using join channel.
	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)
	n := 10
	anchorPeercount := 3

	jcm := &joinChanMsg{members2AnchorPeers: map[string][]api.AnchorPeer{string(orgInChannelA): {}}}
	for i := 0; i < anchorPeercount; i++ {
		ap := api.AnchorPeer{
			Port: portPrefix + i,
			Host: "localhost",
		}
		jcm.members2AnchorPeers[string(orgInChannelA)] = append(jcm.members2AnchorPeers[string(orgInChannelA)], ap)
	}

	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			peers[i] = newGossipInstance(portPrefix, i+anchorPeercount, 100)
			peers[i].JoinChan(jcm, common.ChainID("A"))
			peers[i].UpdateChannelMetadata([]byte("bla bla"), common.ChainID("A"))
			wg.Done()
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait)

	time.Sleep(time.Second * 5)

	// Now start a random anchor peer
	anchorPeer := newGossipInstance(portPrefix, rand.Intn(anchorPeercount), 100)
	anchorPeer.JoinChan(jcm, common.ChainID("A"))
	anchorPeer.UpdateChannelMetadata([]byte("bla bla"), common.ChainID("A"))

	defer anchorPeer.Stop()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n))

	channelMembership := func() bool {
		for _, peer := range peers {
			if len(peer.PeersOfChannel(common.ChainID("A"))) != n {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, channelMembership)

	stop := func() {
		stopPeers(peers)
	}
	waitUntilOrFailBlocking(t, stop)

	fmt.Println("<<<TestConnectToAnchorPeers>>>")
	atomic.StoreInt32(&stopped, int32(1))
	testWG.Done()
}

func TestMembership(t *testing.T) {
	t.Parallel()
	portPrefix := 4610
	t1 := time.Now()
	// Scenario: spawn 20 nodes and a single bootstrap node and then:
	// 1) Check full membership views for all nodes but the bootstrap node.
	// 2) Update metadata of last peer and ensure it propagates to all peers

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 10
	var lastPeer = fmt.Sprintf("localhost:%d", (n + portPrefix))
	boot := newGossipInstance(portPrefix, 0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChainID("A"))
	boot.UpdateChannelMetadata([]byte{}, common.ChainID("A"))

	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(i int) {
			defer wg.Done()
			pI := newGossipInstance(portPrefix, i, 100, 0)
			peers[i-1] = pI
			pI.JoinChan(&joinChanMsg{}, common.ChainID("A"))
			pI.UpdateChannelMetadata([]byte{}, common.ChainID("A"))
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait)
	t.Log("Peers started")

	seeAllNeighbors := func() bool {
		for i := 1; i <= n; i++ {
			neighborCount := len(peers[i-1].Peers())
			if neighborCount != n {
				return false
			}
		}
		return true
	}

	membershipEstablishTime := time.Now()
	waitUntilOrFail(t, seeAllNeighbors)
	t.Log("membership established in", time.Since(membershipEstablishTime))

	t.Log("Updating metadata...")
	// Change metadata in last node
	peers[len(peers)-1].UpdateMetadata([]byte("bla bla"))

	metaDataUpdated := func() bool {
		if "bla bla" != string(metadataOfPeer(boot.Peers(), lastPeer)) {
			return false
		}
		for i := 0; i < n-1; i++ {
			if "bla bla" != string(metadataOfPeer(peers[i].Peers(), lastPeer)) {
				return false
			}
		}
		return true
	}
	metadataDisseminationTime := time.Now()
	waitUntilOrFail(t, metaDataUpdated)
	fmt.Println("Metadata updated")
	t.Log("Metadata dissemination took", time.Since(metadataDisseminationTime))

	stop := func() {
		stopPeers(append(peers, boot))
	}

	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop)
	t.Log("Stop took", time.Since(stopTime))

	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestMembership>>>")
	testWG.Done()
}

func TestDissemination(t *testing.T) {
	t.Parallel()
	portPrefix := 3610
	t1 := time.Now()
	// Scenario: 20 nodes and a bootstrap node.
	// The bootstrap node sends 10 messages and we count
	// that each node got 10 messages after a few seconds

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 10
	msgsCount2Send := 10
	boot := newGossipInstance(portPrefix, 0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChainID("A"))
	boot.UpdateChannelMetadata([]byte{}, common.ChainID("A"))

	peers := make([]Gossip, n)
	receivedMessages := make([]int, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		pI := newGossipInstance(portPrefix, i, 100, 0)
		peers[i-1] = pI
		pI.JoinChan(&joinChanMsg{}, common.ChainID("A"))
		pI.UpdateChannelMetadata([]byte{}, common.ChainID("A"))
		acceptChan, _ := pI.Accept(acceptData, false)
		go func(index int, ch <-chan *proto.GossipMessage) {
			defer wg.Done()
			for j := 0; j < msgsCount2Send; j++ {
				<-ch
				receivedMessages[index]++
			}
		}(i-1, acceptChan)
		// Change metadata in last node
		if i == n {
			pI.UpdateChannelMetadata([]byte("bla bla"), common.ChainID("A"))
		}
	}
	var lastPeer = fmt.Sprintf("localhost:%d", (n + portPrefix))
	metaDataUpdated := func() bool {
		if "bla bla" != string(metadataOfPeer(boot.PeersOfChannel(common.ChainID("A")), lastPeer)) {
			return false
		}
		for i := 0; i < n-1; i++ {
			if "bla bla" != string(metadataOfPeer(peers[i].PeersOfChannel(common.ChainID("A")), lastPeer)) {
				return false
			}
		}
		return true
	}

	membershipTime := time.Now()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n))
	t.Log("Membership establishment took", time.Since(membershipTime))

	for i := 1; i <= msgsCount2Send; i++ {
		boot.Gossip(createDataMsg(uint64(i), []byte{}, common.ChainID("A")))
	}

	t2 := time.Now()
	waitUntilOrFailBlocking(t, wg.Wait)
	t.Log("Block dissemination took", time.Since(t2))
	t2 = time.Now()
	waitUntilOrFail(t, metaDataUpdated)
	t.Log("Metadata dissemination took", time.Since(t2))

	for i := 0; i < n; i++ {
		assert.Equal(t, msgsCount2Send, receivedMessages[i])
	}

	//Sending leadership messages
	receivedLeadershipMessages := make([]int, n)
	wgLeadership := sync.WaitGroup{}
	wgLeadership.Add(n)
	for i := 1; i <= n; i++ {
		leadershipChan, _ := peers[i-1].Accept(acceptLeadershp, false)
		go func(index int, ch <-chan *proto.GossipMessage) {
			defer wgLeadership.Done()
			msg := <-ch
			if bytes.Equal(msg.Channel, common.ChainID("A")) {
				receivedLeadershipMessages[index]++
			}
		}(i-1, leadershipChan)
	}

	seqNum := 0
	incTime := uint64(time.Now().UnixNano())
	t3 := time.Now()

	leadershipMsg := createLeadershipMsg(true, common.ChainID("A"), incTime, uint64(seqNum), boot.(*gossipServiceImpl).conf.InternalEndpoint, boot.(*gossipServiceImpl).comm.GetPKIid())
	boot.Gossip(leadershipMsg)

	waitUntilOrFailBlocking(t, wgLeadership.Wait)
	t.Log("Leadership message dissemination took", time.Since(t3))

	for i := 0; i < n; i++ {
		assert.Equal(t, 1, receivedLeadershipMessages[i])
	}

	t.Log("Stopping peers")

	stop := func() {
		stopPeers(append(peers, boot))
	}

	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop)
	t.Log("Stop took", time.Since(stopTime))
	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDissemination>>>")
	testWG.Done()
}

func TestMembershipConvergence(t *testing.T) {
	t.Parallel()
	portPrefix := 2610
	// Scenario: Spawn 12 nodes and 3 bootstrap peers
	// but assign each node to its bootstrap peer group modulo 3.
	// Then:
	// 1) Check all groups know only themselves in the view and not others.
	// 2) Bring up a node that will connect to all bootstrap peers.
	// 3) Wait a few seconds and check that all views converged to a single one
	// 4) Kill that last node, wait a while and:
	// 4)a) Ensure all nodes consider it as dead
	// 4)b) Ensure all node still know each other

	t1 := time.Now()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)
	boot0 := newGossipInstance(portPrefix, 0, 100)
	boot1 := newGossipInstance(portPrefix, 1, 100)
	boot2 := newGossipInstance(portPrefix, 2, 100)

	peers := []Gossip{boot0, boot1, boot2}
	// 0: {3, 6, 9, 12}
	// 1: {4, 7, 10, 13}
	// 2: {5, 8, 11, 14}
	for i := 3; i < 15; i++ {
		pI := newGossipInstance(portPrefix, i, 100, i%3)
		peers = append(peers, pI)
	}

	waitUntilOrFail(t, checkPeersMembership(t, peers, 4))
	t.Log("Sets of peers connected successfully")

	connectorPeer := newGossipInstance(portPrefix, 15, 100, 0, 1, 2)
	connectorPeer.UpdateMetadata([]byte("Connector"))

	fullKnowledge := func() bool {
		for i := 0; i < 15; i++ {
			if 15 != len(peers[i].Peers()) {
				return false
			}
			if "Connector" != string(metadataOfPeer(peers[i].Peers(), "localhost:2625")) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, fullKnowledge)

	t.Log("Stopping connector...")
	waitUntilOrFailBlocking(t, connectorPeer.Stop)
	t.Log("Stopped")
	time.Sleep(time.Duration(15) * time.Second)

	ensureForget := func() bool {
		for i := 0; i < 15; i++ {
			if 14 != len(peers[i].Peers()) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureForget)

	connectorPeer = newGossipInstance(portPrefix, 15, 100)
	connectorPeer.UpdateMetadata([]byte("Connector2"))
	t.Log("Started connector")

	ensureResync := func() bool {
		for i := 0; i < 15; i++ {
			if 15 != len(peers[i].Peers()) {
				return false
			}
			if "Connector2" != string(metadataOfPeer(peers[i].Peers(), "localhost:2625")) {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureResync)

	waitUntilOrFailBlocking(t, connectorPeer.Stop)

	t.Log("Stopping peers")
	stop := func() {
		stopPeers(peers)
	}

	waitUntilOrFailBlocking(t, stop)
	atomic.StoreInt32(&stopped, int32(1))
	t.Log("Took", time.Since(t1))
	fmt.Println("<<<TestMembershipConvergence>>>")
	testWG.Done()
}

func TestMembershipRequestSpoofing(t *testing.T) {
	t.Parallel()
	// Scenario: g1, g2, g3 are peers, and g2 is malicious, and wants
	// to impersonate g3 when sending a membership request to g1.
	// Expected output: g1 should *NOT* respond to g2,
	// However, g1 should respond to g3 when it sends the message itself.

	portPrefix := 2000
	g1 := newGossipInstance(portPrefix, 0, 100)
	g2 := newGossipInstance(portPrefix, 1, 100, 2)
	g3 := newGossipInstance(portPrefix, 2, 100, 1)
	defer g1.Stop()
	defer g2.Stop()
	defer g3.Stop()

	// Wait for g2 and g3 to know about each other
	waitUntilOrFail(t, checkPeersMembership(t, []Gossip{g2, g3}, 1))
	// Obtain an alive message from p3
	_, aliveMsgChan := g2.Accept(func(o interface{}) bool {
		msg := o.(proto.ReceivedMessage).GetGossipMessage()
		// Make sure we get an AliveMessage and it's about g3
		return msg.IsAliveMsg() && bytes.Equal(msg.GetAliveMsg().Membership.PkiId, []byte("localhost:2002"))
	}, true)
	aliveMsg := <-aliveMsgChan

	// Obtain channel for messages from g1 to g2
	_, g1ToG2 := g2.Accept(func(o interface{}) bool {
		connInfo := o.(proto.ReceivedMessage).GetConnectionInfo()
		return bytes.Equal([]byte("localhost:2000"), connInfo.ID)
	}, true)

	// Obtain channel for messages from g1 to g3
	_, g1ToG3 := g3.Accept(func(o interface{}) bool {
		connInfo := o.(proto.ReceivedMessage).GetConnectionInfo()
		return bytes.Equal([]byte("localhost:2000"), connInfo.ID)
	}, true)

	// Now, create a membership request message
	memRequestSpoofFactory := func(aliveMsgEnv *proto.Envelope) *proto.SignedGossipMessage {
		sMsg, _ := (&proto.GossipMessage{
			Tag:   proto.GossipMessage_EMPTY,
			Nonce: uint64(0),
			Content: &proto.GossipMessage_MemReq{
				MemReq: &proto.MembershipRequest{
					SelfInformation: aliveMsgEnv,
					Known:           [][]byte{},
				},
			},
		}).NoopSign()
		return sMsg
	}
	spoofedMemReq := memRequestSpoofFactory(aliveMsg.GetSourceEnvelope())
	g2.Send(spoofedMemReq.GossipMessage, &comm.RemotePeer{Endpoint: "localhost:2000", PKIID: common.PKIidType("localhost:2000")})
	select {
	case <-time.After(time.Second):
		break
	case <-g1ToG2:
		assert.Fail(t, "Received response from g1 but shouldn't have")
	}

	// Now send the same message from g3 to g1
	g3.Send(spoofedMemReq.GossipMessage, &comm.RemotePeer{Endpoint: "localhost:2000", PKIID: common.PKIidType("localhost:2000")})
	select {
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't receive a message back from g1 on time")
	case <-g1ToG3:
		break
	}
}

func TestDataLeakage(t *testing.T) {
	t.Parallel()
	portPrefix := 1610
	// Scenario: spawn some nodes and let them all
	// establish full membership.
	// Then, have half be in channel A and half be in channel B.
	// However, make it so that only the first 3 from each channel
	// are eligible to obtain blocks from the channels they're in.
	// Ensure nodes only get messages of their channels and in case they
	// are eligible for the channels.

	totalPeers := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} // THIS MUST BE EVEN AND NOT ODD
	// Peer0 and Peer5 disseminate blocks
	// only 1,2 and 6,7 should get blocks.

	mcs := &naiveCryptoService{
		allowedPkiIDS: map[string]struct{}{
			// Channel A
			"localhost:1610": {},
			"localhost:1611": {},
			"localhost:1612": {},
			// Channel B
			"localhost:1615": {},
			"localhost:1616": {},
			"localhost:1617": {},
		},
	}

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := len(totalPeers)
	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			totPeers := append([]int(nil), totalPeers[:i]...)
			bootPeers := append(totPeers, totalPeers[i+1:]...)
			peers[i] = newGossipInstanceWithCustomMCS(portPrefix, i, 100, mcs, bootPeers...)
			wg.Done()
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait)
	waitUntilOrFail(t, checkPeersMembership(t, peers, n-1))

	channels := []common.ChainID{common.ChainID("A"), common.ChainID("B")}

	channelAmetadata := []byte("some metadata of channel A")
	channelBmetadata := []byte("some metadata on channel B")

	for i, channel := range channels {
		for j := 0; j < (n / 2); j++ {
			instanceIndex := (n/2)*i + j
			peers[instanceIndex].JoinChan(&joinChanMsg{}, channel)
			var metadata []byte
			if i == 0 {
				metadata = channelAmetadata
			} else {
				metadata = channelBmetadata
			}
			peers[instanceIndex].UpdateChannelMetadata(metadata, channel)
			t.Log(instanceIndex, "joined", string(channel))
		}
	}

	// Wait until all peers have other peers in the per-channel view
	seeChannelMetadata := func() bool {
		for i, channel := range channels {
			for j := 0; j < 3; j++ {
				instanceIndex := (n/2)*i + j
				if len(peers[instanceIndex].PeersOfChannel(channel)) < 2 {
					return false
				}
			}
		}
		return true
	}
	t1 := time.Now()
	waitUntilOrFail(t, seeChannelMetadata)

	t.Log("Metadata sync took", time.Since(t1))
	for i, channel := range channels {
		for j := 0; j < 3; j++ {
			instanceIndex := (n/2)*i + j
			assert.Len(t, peers[instanceIndex].PeersOfChannel(channel), 2)
			if i == 0 {
				assert.Equal(t, channelAmetadata, peers[instanceIndex].PeersOfChannel(channel)[0].Metadata)
			} else {
				assert.Equal(t, channelBmetadata, peers[instanceIndex].PeersOfChannel(channel)[0].Metadata)
			}
		}
	}

	gotMessages := func() {
		var wg sync.WaitGroup
		wg.Add(4)
		for i, channel := range channels {
			for j := 1; j < 3; j++ {
				instanceIndex := (n/2)*i + j
				go func(instanceIndex int, channel common.ChainID) {
					incMsgChan, _ := peers[instanceIndex].Accept(acceptData, false)
					msg := <-incMsgChan
					assert.Equal(t, []byte(channel), []byte(msg.Channel))
					wg.Done()
				}(instanceIndex, channel)
			}
		}
		wg.Wait()
	}

	t1 = time.Now()
	peers[0].Gossip(createDataMsg(1, []byte{}, channels[0]))
	peers[n/2].Gossip(createDataMsg(2, []byte{}, channels[1]))
	waitUntilOrFailBlocking(t, gotMessages)
	t.Log("Dissemination took", time.Since(t1))
	stop := func() {
		stopPeers(peers)
	}
	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop)
	t.Log("Stop took", time.Since(stopTime))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDataLeakage>>>")
	testWG.Done()
}

func TestDisseminateAll2All(t *testing.T) {
	// Scenario: spawn some nodes, have each node
	// disseminate a block to all nodes.
	// Ensure all blocks are received

	t.Skip()
	t.Parallel()
	portPrefix := 6610
	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	totalPeers := []int{0, 1, 2, 3, 4, 5, 6}
	n := len(totalPeers)
	peers := make([]Gossip, n)
	wg := sync.WaitGroup{}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			totPeers := append([]int(nil), totalPeers[:i]...)
			bootPeers := append(totPeers, totalPeers[i+1:]...)
			pI := newGossipInstance(portPrefix, i, 100, bootPeers...)
			pI.JoinChan(&joinChanMsg{}, common.ChainID("A"))
			pI.UpdateChannelMetadata([]byte{}, common.ChainID("A"))
			peers[i] = pI
			wg.Done()
		}(i)
	}
	wg.Wait()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n-1))

	bMutex := sync.WaitGroup{}
	bMutex.Add(10 * n * (n - 1))

	wg = sync.WaitGroup{}
	wg.Add(n)

	reader := func(msgChan <-chan *proto.GossipMessage, i int) {
		wg.Done()
		for range msgChan {
			bMutex.Done()
		}
	}

	for i := 0; i < n; i++ {
		msgChan, _ := peers[i].Accept(acceptData, false)
		go reader(msgChan, i)
	}

	wg.Wait()

	for i := 0; i < n; i++ {
		go func(i int) {
			blockStartIndex := i * 10
			for j := 0; j < 10; j++ {
				blockSeq := uint64(j + blockStartIndex)
				peers[i].Gossip(createDataMsg(blockSeq, []byte{}, common.ChainID("A")))
			}
		}(i)
	}
	waitUntilOrFailBlocking(t, bMutex.Wait)

	stop := func() {
		stopPeers(peers)
	}
	waitUntilOrFailBlocking(t, stop)
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDisseminateAll2All>>>")
	testWG.Done()
}

func TestIdentityExpiration(t *testing.T) {
	t.Parallel()
	// Scenario: spawn 4 peers and make the MessageCryptoService revoke one of them.
	// Eventually, the rest of the peers should not be able to communicate with
	// the revoked peer at all because its identity would seem to them as expired

	portPrefix := 7000
	g1 := newGossipInstance(portPrefix, 0, 100)
	g2 := newGossipInstance(portPrefix, 1, 100, 0)
	g3 := newGossipInstance(portPrefix, 2, 100, 0)
	g4 := newGossipInstance(portPrefix, 3, 100, 0)

	peers := []Gossip{g1, g2, g3, g4}

	seeAllNeighbors := func() bool {
		for i := 0; i < 4; i++ {
			neighborCount := len(peers[i].Peers())
			if neighborCount != 3 {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, seeAllNeighbors)
	// Now revoke some peer
	revokedPeerIndex := rand.Intn(4)
	revokedPkiID := common.PKIidType(fmt.Sprintf("localhost:%d", portPrefix+int(revokedPeerIndex)))
	for i, p := range peers {
		if i == revokedPeerIndex {
			continue
		}
		p.(*gossipServiceImpl).mcs.(*naiveCryptoService).revoke(revokedPkiID)
	}
	// Ensure that no one talks to the peer that is revoked
	ensureRevokedPeerIsIgnored := func() bool {
		for i := 0; i < 4; i++ {
			neighborCount := len(peers[i].Peers())
			expectedNeighborCount := 2
			if i == revokedPeerIndex {
				expectedNeighborCount = 0
			}
			if neighborCount != expectedNeighborCount {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, ensureRevokedPeerIsIgnored)
	stopPeers(peers)
}

func TestEndedGoroutines(t *testing.T) {
	t.Parallel()
	testWG.Wait()
	ensureGoroutineExit(t)
}

func createDataMsg(seqnum uint64, data []byte, channel common.ChainID) *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: []byte(channel),
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   data,
					SeqNum: seqnum,
				},
			},
		},
	}
}

func createLeadershipMsg(isDeclaration bool, channel common.ChainID, incTime uint64, seqNum uint64, endpoint string, pkiid []byte) *proto.GossipMessage {

	leadershipMsg := &proto.LeadershipMessage{
		IsDeclaration: isDeclaration,
		PkiId:         pkiid,
		Timestamp: &proto.PeerTime{
			IncNum: incTime,
			SeqNum: seqNum,
		},
	}

	msg := &proto.GossipMessage{
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_LeadershipMsg{LeadershipMsg: leadershipMsg},
		Channel: channel,
	}
	return msg
}

type goroutinePredicate func(g goroutine) bool

var connectionLeak = func(g goroutine) bool {
	return searchInStackTrace("comm.(*connection).writeToStream", g.stack)
}

var connectionLeak2 = func(g goroutine) bool {
	return searchInStackTrace("comm.(*connection).readFromStream", g.stack)
}

var runTests = func(g goroutine) bool {
	return searchInStackTrace("testing.RunTests", g.stack)
}

var waitForTestCompl = func(g goroutine) bool {
	return searchInStackTrace("waitForTestCompletion", g.stack)
}

var gossipTest = func(g goroutine) bool {
	return searchInStackTrace("gossip_test.go", g.stack)
}

var goExit = func(g goroutine) bool {
	return searchInStackTrace("runtime.goexit", g.stack)
}

var clientConn = func(g goroutine) bool {
	return searchInStackTrace("resetTransport", g.stack)
}

var testingg = func(g goroutine) bool {
	if len(g.stack) == 0 {
		return false
	}
	return strings.Index(g.stack[len(g.stack)-1], "testing.go") != -1
}

func anyOfPredicates(predicates ...goroutinePredicate) goroutinePredicate {
	return func(g goroutine) bool {
		for _, pred := range predicates {
			if pred(g) {
				return true
			}
		}
		return false
	}
}

func shouldNotBeRunningAtEnd(gr goroutine) bool {
	return !anyOfPredicates(runTests, goExit, testingg, waitForTestCompl, gossipTest,
		clientConn, connectionLeak, connectionLeak2)(gr)
}

func ensureGoroutineExit(t *testing.T) {
	for i := 0; i <= 20; i++ {
		time.Sleep(time.Second)
		allEnded := true
		for _, gr := range getGoRoutines() {
			if shouldNotBeRunningAtEnd(gr) {
				allEnded = false
			}

			if shouldNotBeRunningAtEnd(gr) && i == 20 {
				assert.Fail(t, "Goroutine(s) haven't ended:", fmt.Sprintf("%v", gr.stack))
				util.PrintStackTrace()
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
		if member.InternalEndpoint == endpoint {
			return member.Metadata
		}
	}
	return nil
}

func waitForTestCompletion(stopFlag *int32, t *testing.T) {
	time.Sleep(timeout)
	if atomic.LoadInt32(stopFlag) == int32(1) {
		return
	}
	util.PrintStackTrace()
	assert.Fail(t, "Didn't stop within a timely manner")
}

func stopPeers(peers []Gossip) {
	stoppingWg := sync.WaitGroup{}
	stoppingWg.Add(len(peers))
	for i, pI := range peers {
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
	limit := start.UnixNano() + timeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeout / 60)
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
	case <-time.NewTimer(timeout).C:
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

func checkPeersMembership(t *testing.T, peers []Gossip, n int) func() bool {
	return func() bool {
		for _, peer := range peers {
			if len(peer.Peers()) != n {
				return false
			}
			for _, p := range peer.Peers() {
				assert.NotNil(t, p.InternalEndpoint)
				assert.NotEmpty(t, p.Endpoint)
			}
		}
		return true
	}
}
