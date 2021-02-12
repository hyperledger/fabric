/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	corecomm "github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/require"
)

var timeout = time.Second * time.Duration(180)

func TestMain(m *testing.M) {
	util.SetupTestLogging()
	rand.Seed(int64(time.Now().Second()))
	factory.InitFactories(nil)
	os.Exit(m.Run())
}

var (
	aliveTimeInterval = 1000 * time.Millisecond
	discoveryConfig   = discovery.DiscoveryConfig{
		AliveTimeInterval:            aliveTimeInterval,
		AliveExpirationTimeout:       10 * aliveTimeInterval,
		AliveExpirationCheckInterval: aliveTimeInterval,
		ReconnectInterval:            aliveTimeInterval,
		MaxConnectionAttempts:        5,
		MsgExpirationFactor:          discovery.DefMsgExpirationFactor,
	}
)

var orgInChannelA = api.OrgIdentityType("ORG1")

func acceptData(m interface{}) bool {
	if dataMsg := m.(*proto.GossipMessage).GetDataMsg(); dataMsg != nil {
		return true
	}
	return false
}

func acceptLeadershp(message interface{}) bool {
	validMsg := message.(*proto.GossipMessage).Tag == proto.GossipMessage_CHAN_AND_ORG &&
		protoext.IsLeadershipMsg(message.(*proto.GossipMessage))

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
	allowedPkiIDS       map[string]struct{}
	revokedPkiIDS       map[string]struct{}
	expirationTimesLock *sync.RWMutex
	expirationTimes     map[string]time.Time
}

func (cs *naiveCryptoService) OrgByPeerIdentity(api.PeerIdentityType) api.OrgIdentityType {
	return nil
}

func (cs *naiveCryptoService) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	if cs.expirationTimesLock != nil {
		cs.expirationTimesLock.RLock()
		defer cs.expirationTimesLock.RUnlock()
	}
	if exp, exists := cs.expirationTimes[string(peerIdentity)]; exists {
		return exp, nil
	}
	return time.Now().Add(time.Hour), nil
}

type orgCryptoService struct{}

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
func (cs *naiveCryptoService) VerifyByChannel(_ common.ChannelID, identity api.PeerIdentityType, _, _ []byte) error {
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
func (*naiveCryptoService) VerifyBlock(channelID common.ChannelID, seqNum uint64, signedBlock *cb.Block) error {
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

func bootPeersWithPorts(ports ...int) []string {
	var peers []string
	for _, port := range ports {
		peers = append(peers, fmt.Sprintf("127.0.0.1:%d", port))
	}
	return peers
}

func newGossipInstanceWithGrpcMcsMetrics(id int, port int, gRPCServer *corecomm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, maxMsgCount int, mcs api.MessageCryptoService,
	metrics *metrics.GossipMetrics, bootPorts ...int) *gossipGRPC {
	conf := &Config{
		BootstrapPeers:               bootPeersWithPorts(bootPorts...),
		ID:                           fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:         maxMsgCount,
		MaxPropagationBurstLatency:   time.Duration(500) * time.Millisecond,
		MaxPropagationBurstSize:      20,
		PropagateIterations:          1,
		PropagatePeerNum:             3,
		PullInterval:                 time.Duration(4) * time.Second,
		PullPeerNum:                  5,
		InternalEndpoint:             fmt.Sprintf("127.0.0.1:%d", port),
		ExternalEndpoint:             fmt.Sprintf("1.2.3.4:%d", port),
		PublishCertPeriod:            time.Duration(4) * time.Second,
		PublishStateInfoInterval:     time.Duration(1) * time.Second,
		RequestStateInfoInterval:     time.Duration(1) * time.Second,
		TimeForMembershipTracker:     5 * time.Second,
		TLSCerts:                     certs,
		DigestWaitTime:               algo.DefDigestWaitTime,
		RequestWaitTime:              algo.DefRequestWaitTime,
		ResponseWaitTime:             algo.DefResponseWaitTime,
		DialTimeout:                  comm.DefDialTimeout,
		ConnTimeout:                  comm.DefConnTimeout,
		RecvBuffSize:                 comm.DefRecvBuffSize,
		SendBuffSize:                 comm.DefSendBuffSize,
		MsgExpirationTimeout:         channel.DefMsgExpirationTimeout,
		AliveTimeInterval:            discoveryConfig.AliveTimeInterval,
		AliveExpirationTimeout:       discoveryConfig.AliveExpirationTimeout,
		AliveExpirationCheckInterval: discoveryConfig.AliveExpirationCheckInterval,
		ReconnectInterval:            discoveryConfig.ReconnectInterval,
		MaxConnectionAttempts:        discoveryConfig.MaxConnectionAttempts,
		MsgExpirationFactor:          discoveryConfig.MsgExpirationFactor,
	}
	selfID := api.PeerIdentityType(conf.InternalEndpoint)
	g := New(conf, gRPCServer.Server(), &orgCryptoService{}, mcs, selfID,
		secureDialOpts, metrics, nil)
	go func() {
		gRPCServer.Start()
	}()
	return &gossipGRPC{Node: g, grpc: gRPCServer}
}

func newGossipInstanceWithGRPC(id int, port int, gRPCServer *corecomm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, maxMsgCount int, bootPorts ...int) *gossipGRPC {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{})
	mcs := &naiveCryptoService{}
	return newGossipInstanceWithGrpcMcsMetrics(id, port, gRPCServer, certs, secureDialOpts, maxMsgCount, mcs, metrics, bootPorts...)
}

func newGossipInstanceWithExpiration(expirations map[string]time.Time, lock *sync.RWMutex, id int, port int, gRPCServer *corecomm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, maxMsgCount int, bootPorts ...int) *gossipGRPC {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{})
	mcs := &naiveCryptoService{expirationTimesLock: lock, expirationTimes: expirations}
	return newGossipInstanceWithGrpcMcsMetrics(id, port, gRPCServer, certs, secureDialOpts, maxMsgCount, mcs, metrics, bootPorts...)
}

func newGossipInstanceWithGRPCWithOnlyPull(id int, port int, gRPCServer *corecomm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, maxMsgCount int, mcs api.MessageCryptoService,
	metrics *metrics.GossipMetrics, bootPorts ...int) *gossipGRPC {
	shortenedWaitTime := time.Duration(200) * time.Millisecond
	conf := &Config{
		BootstrapPeers:               bootPeersWithPorts(bootPorts...),
		ID:                           fmt.Sprintf("p%d", id),
		MaxBlockCountToStore:         maxMsgCount,
		MaxPropagationBurstLatency:   time.Duration(1000) * time.Millisecond,
		MaxPropagationBurstSize:      10,
		PropagateIterations:          0,
		PropagatePeerNum:             0,
		PullInterval:                 time.Duration(1000) * time.Millisecond,
		PullPeerNum:                  20,
		InternalEndpoint:             fmt.Sprintf("127.0.0.1:%d", port),
		ExternalEndpoint:             fmt.Sprintf("1.2.3.4:%d", port),
		PublishCertPeriod:            time.Duration(0) * time.Second,
		PublishStateInfoInterval:     time.Duration(1) * time.Second,
		RequestStateInfoInterval:     time.Duration(1) * time.Second,
		TimeForMembershipTracker:     5 * time.Second,
		TLSCerts:                     certs,
		DigestWaitTime:               shortenedWaitTime,
		RequestWaitTime:              shortenedWaitTime,
		ResponseWaitTime:             shortenedWaitTime,
		DialTimeout:                  comm.DefDialTimeout,
		ConnTimeout:                  comm.DefConnTimeout,
		RecvBuffSize:                 comm.DefRecvBuffSize,
		SendBuffSize:                 comm.DefSendBuffSize,
		MsgExpirationTimeout:         channel.DefMsgExpirationTimeout,
		AliveTimeInterval:            discoveryConfig.AliveTimeInterval,
		AliveExpirationTimeout:       discoveryConfig.AliveExpirationTimeout,
		AliveExpirationCheckInterval: discoveryConfig.AliveExpirationCheckInterval,
		ReconnectInterval:            discoveryConfig.ReconnectInterval,
		MaxConnectionAttempts:        discoveryConfig.MaxConnectionAttempts,
		MsgExpirationFactor:          discoveryConfig.MsgExpirationFactor,
	}
	selfID := api.PeerIdentityType(conf.InternalEndpoint)
	g := New(conf, gRPCServer.Server(), &orgCryptoService{}, mcs, selfID,
		secureDialOpts, metrics, nil)
	go func() {
		gRPCServer.Start()
	}()
	return &gossipGRPC{Node: g, grpc: gRPCServer}
}

func newGossipInstanceCreateGRPCWithMCSWithMetrics(id int, maxMsgCount int, mcs api.MessageCryptoService,
	metrics *metrics.GossipMetrics, bootPorts ...int) *gossipGRPC {
	p, g, c, s, _ := util.CreateGRPCLayer()
	return newGossipInstanceWithGrpcMcsMetrics(id, p, g, c, s, maxMsgCount, mcs, metrics, bootPorts...)
}

func newGossipInstanceCreateGRPC(id int, maxMsgCount int, bootPorts ...int) *gossipGRPC {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{})
	mcs := &naiveCryptoService{}
	return newGossipInstanceCreateGRPCWithMCSWithMetrics(id, maxMsgCount, mcs, metrics, bootPorts...)
}

func newGossipInstanceCreateGRPCWithOnlyPull(id int, maxMsgCount int, mcs api.MessageCryptoService,
	metrics *metrics.GossipMetrics, bootPorts ...int) *gossipGRPC {
	p, g, c, s, _ := util.CreateGRPCLayer()
	return newGossipInstanceWithGRPCWithOnlyPull(id, p, g, c, s, maxMsgCount, mcs, metrics, bootPorts...)
}

type gossipGRPC struct {
	*Node
	grpc *corecomm.GRPCServer
}

func (g *gossipGRPC) Stop() {
	g.Node.Stop()
	g.grpc.Stop()
}

func TestLeaveChannel(t *testing.T) {
	// Scenario: Have 3 peers in a channel and make one of them leave it.
	// Ensure the peers don't recognize the other peer when it left the channel

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	port2, grpc2, certs2, secDialOpts2, _ := util.CreateGRPCLayer()

	p0 := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100, port2)
	p0.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	p0.UpdateLedgerHeight(1, common.ChannelID("A"))
	defer p0.Stop()

	p1 := newGossipInstanceWithGRPC(1, port1, grpc1, certs1, secDialOpts1, 100, port0)
	p1.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	p1.UpdateLedgerHeight(1, common.ChannelID("A"))
	defer p1.Stop()

	p2 := newGossipInstanceWithGRPC(2, port2, grpc2, certs2, secDialOpts2, 100, port1)
	p2.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	p2.UpdateLedgerHeight(1, common.ChannelID("A"))
	defer p2.Stop()

	countMembership := func(g *gossipGRPC, expected int) func() bool {
		return func() bool {
			peers := g.PeersOfChannel(common.ChannelID("A"))
			return len(peers) == expected
		}
	}

	// Wait until everyone sees each other in the channel
	waitUntilOrFail(t, countMembership(p0, 2), "waiting for p0 to form membership")
	waitUntilOrFail(t, countMembership(p1, 2), "waiting for p1 to form membership")
	waitUntilOrFail(t, countMembership(p2, 2), "waiting for p2 to form membership")

	// Now p2 leaves the channel
	p2.LeaveChan(common.ChannelID("A"))

	// Ensure channel membership is adjusted accordingly
	waitUntilOrFail(t, countMembership(p0, 1), "waiting for p0 to update membership view")
	waitUntilOrFail(t, countMembership(p1, 1), "waiting for p1 to update membership view")
	waitUntilOrFail(t, countMembership(p2, 0), "waiting for p2 to update membership view")
}

func TestPull(t *testing.T) {
	t1 := time.Now()
	// Scenario: Turn off forwarding and use only pull-based gossip.
	// First phase: Ensure full membership view for all nodes
	// Second phase: Disseminate 10 messages and ensure all nodes got them

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 5
	msgsCount2Send := 10

	metrics := metrics.NewGossipMetrics(&disabled.Provider{})
	mcs := &naiveCryptoService{}
	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()

	peers := make([]*gossipGRPC, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func(i int) {
			defer wg.Done()
			pI := newGossipInstanceCreateGRPCWithOnlyPull(i, 100, mcs, metrics, port0)
			pI.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
			pI.UpdateLedgerHeight(1, common.ChannelID("A"))
			peers[i-1] = pI
		}(i)
	}
	wg.Wait()

	time.Sleep(time.Second)

	boot := newGossipInstanceWithGRPCWithOnlyPull(0, port0, grpc0, certs0, secDialOpts0, 100, mcs, metrics)
	boot.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	boot.UpdateLedgerHeight(1, common.ChannelID("A"))

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
		boot.Gossip(createDataMsg(uint64(i), []byte{}, common.ChannelID("A")))
	}

	waitUntilOrFail(t, knowAll, "waiting to form membership among all peers")
	waitUntilOrFailBlocking(t, wg.Wait, "waiting peers to register for gossip messages")

	receivedAll := func() bool {
		for i := 0; i < n; i++ {
			if msgsCount2Send != receivedMessages[i] {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, receivedAll, "waiting for all messages to be received by all peers")

	stop := func() {
		stopPeers(append(peers, boot))
	}

	waitUntilOrFailBlocking(t, stop, "waiting to stop all peers")

	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestPull>>>")
}

func TestConnectToAnchorPeers(t *testing.T) {
	// Scenario: spawn 10 peers, and have them join a channel
	// of 3 anchor peers that don't exist yet.
	// Wait 5 seconds, and then spawn a random anchor peer out of the 3.
	// Ensure that all peers successfully see each other in the channel

	// Scenario: Spawn 5 peers, and make each of them connect to
	// the other 2 using join channel.
	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)
	n := 10
	anchorPeercount := 3

	var ports []int
	var grpcs []*corecomm.GRPCServer
	var certs []*common.TLSCertificates
	var secDialOpts []api.PeerSecureDialOpts

	jcm := &joinChanMsg{members2AnchorPeers: map[string][]api.AnchorPeer{string(orgInChannelA): {}}}
	for i := 0; i < anchorPeercount; i++ {
		port, grpc, cert, secDialOpt, _ := util.CreateGRPCLayer()
		ports = append(ports, port)
		grpcs = append(grpcs, grpc)
		certs = append(certs, cert)
		secDialOpts = append(secDialOpts, secDialOpt)
		ap := api.AnchorPeer{
			Port: port,
			Host: "127.0.0.1",
		}
		jcm.members2AnchorPeers[string(orgInChannelA)] = append(jcm.members2AnchorPeers[string(orgInChannelA)], ap)
	}

	// Start peers
	peers := make([]*gossipGRPC, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			peers[i] = newGossipInstanceCreateGRPC(i+anchorPeercount, 100)
			peers[i].JoinChan(jcm, common.ChannelID("A"))
			peers[i].UpdateLedgerHeight(1, common.ChannelID("A"))
			wg.Done()
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait, "waiting until all peers join the channel")

	// Now start a random anchor peer
	index := rand.Intn(anchorPeercount)
	anchorPeer := newGossipInstanceWithGRPC(index, ports[index], grpcs[index], certs[index], secDialOpts[index], 100)
	anchorPeer.JoinChan(jcm, common.ChannelID("A"))
	anchorPeer.UpdateLedgerHeight(1, common.ChannelID("A"))

	defer anchorPeer.Stop()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n), "waiting for peers to form membership view")

	channelMembership := func() bool {
		for _, peer := range peers {
			if len(peer.PeersOfChannel(common.ChannelID("A"))) != n {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, channelMembership, "waiting for peers to form channel membership view")

	stop := func() {
		stopPeers(peers)
	}
	waitUntilOrFailBlocking(t, stop, "waiting for gossip instances to stop")

	fmt.Println("<<<TestConnectToAnchorPeers>>>")
	atomic.StoreInt32(&stopped, int32(1))
}

func TestMembership(t *testing.T) {
	t1 := time.Now()
	// Scenario: spawn 20 nodes and a single bootstrap node and then:
	// 1) Check full membership views for all nodes but the bootstrap node.
	// 2) Update metadata of last peer and ensure it propagates to all peers

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 10

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	boot := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	boot.UpdateLedgerHeight(1, common.ChannelID("A"))

	peers := make([]*gossipGRPC, n)
	wg := sync.WaitGroup{}
	wg.Add(n - 1)
	for i := 1; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			pI := newGossipInstanceCreateGRPC(i, 100, port0)
			peers[i-1] = pI
			pI.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
			pI.UpdateLedgerHeight(1, common.ChannelID("A"))
		}(i)
	}

	portn, grpcn, certsn, secDialOptsn, _ := util.CreateGRPCLayer()
	lastPeer := fmt.Sprintf("127.0.0.1:%d", portn)
	pI := newGossipInstanceWithGRPC(0, portn, grpcn, certsn, secDialOptsn, 100, port0)
	peers[n-1] = pI
	pI.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	pI.UpdateLedgerHeight(1, common.ChannelID("A"))

	waitUntilOrFailBlocking(t, wg.Wait, "waiting for all peers to join the channel")
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
	waitUntilOrFail(t, seeAllNeighbors, "waiting for all peer to form the membership")
	t.Log("membership established in", time.Since(membershipEstablishTime))

	t.Log("Updating metadata...")
	// Change metadata in last node
	peers[len(peers)-1].UpdateMetadata([]byte("bla bla"))

	metaDataUpdated := func() bool {
		if !bytes.Equal([]byte("bla bla"), metadataOfPeer(boot.Peers(), lastPeer)) {
			return false
		}
		for i := 0; i < n-1; i++ {
			if !bytes.Equal([]byte("bla bla"), metadataOfPeer(peers[i].Peers(), lastPeer)) {
				return false
			}
		}
		return true
	}
	metadataDisseminationTime := time.Now()
	waitUntilOrFail(t, metaDataUpdated, "wait until metadata update is got propagated")
	fmt.Println("Metadata updated")
	t.Log("Metadata dissemination took", time.Since(metadataDisseminationTime))

	stop := func() {
		stopPeers(append(peers, boot))
	}

	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop, "waiting for all instances to stop")
	t.Log("Stop took", time.Since(stopTime))

	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestMembership>>>")
}

func TestNoMessagesSelfLoop(t *testing.T) {
	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	boot := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	boot.UpdateLedgerHeight(1, common.ChannelID("A"))

	peer := newGossipInstanceCreateGRPC(1, 100, port0)
	peer.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	peer.UpdateLedgerHeight(1, common.ChannelID("A"))

	// Wait until both peers get connected
	waitUntilOrFail(t, checkPeersMembership(t, []*gossipGRPC{peer}, 1), "waiting for peers to form membership view")
	_, commCh := boot.Accept(func(msg interface{}) bool {
		return protoext.IsDataMsg(msg.(protoext.ReceivedMessage).GetGossipMessage().GossipMessage)
	}, true)

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Make sure sending peer is not getting his own
	// message back
	go func(ch <-chan protoext.ReceivedMessage) {
		defer wg.Done()
		for {
			select {
			case msg := <-ch:
				{
					if protoext.IsDataMsg(msg.GetGossipMessage().GossipMessage) {
						t.Errorf("Should not receive data message back, got %s", msg)
					}
				}
				// Waiting for 2 seconds to make sure we won't
				// get message back w.h.p.
			case <-time.After(2 * time.Second):
				{
					return
				}
			}
		}
	}(commCh)

	peerCh, _ := peer.Accept(acceptData, false)

	// Ensure recipient gets his message
	go func(ch <-chan *proto.GossipMessage) {
		defer wg.Done()
		<-ch
	}(peerCh)

	boot.Gossip(createDataMsg(uint64(2), []byte{}, common.ChannelID("A")))
	waitUntilOrFailBlocking(t, wg.Wait, "waiting for everyone to get the message")

	stop := func() {
		stopPeers([]*gossipGRPC{peer, boot})
	}

	waitUntilOrFailBlocking(t, stop, "waiting for all instances to stop")
}

func TestDissemination(t *testing.T) {
	t1 := time.Now()
	// Scenario: 20 nodes and a bootstrap node.
	// The bootstrap node sends 10 messages and we count
	// that each node got 10 messages after a few seconds

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	n := 10
	msgsCount2Send := 10

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	boot := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	boot.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	boot.UpdateLedgerHeight(1, common.ChannelID("A"))
	boot.UpdateChaincodes([]*proto.Chaincode{{Name: "exampleCC", Version: "1.2"}}, common.ChannelID("A"))

	peers := make([]*gossipGRPC, n)
	receivedMessages := make([]int, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	portn, grpcn, certsn, secDialOptsn, _ := util.CreateGRPCLayer()
	for i := 1; i <= n; i++ {
		var pI *gossipGRPC
		if i == n {
			pI = newGossipInstanceWithGRPC(i, portn, grpcn, certsn, secDialOptsn, 100, port0)
		} else {
			pI = newGossipInstanceCreateGRPC(i, 100, port0)
		}
		peers[i-1] = pI
		pI.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
		pI.UpdateLedgerHeight(1, common.ChannelID("A"))
		pI.UpdateChaincodes([]*proto.Chaincode{{Name: "exampleCC", Version: "1.2"}}, common.ChannelID("A"))
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
			pI.UpdateLedgerHeight(2, common.ChannelID("A"))
		}
	}
	lastPeer := fmt.Sprintf("127.0.0.1:%d", portn)
	metaDataUpdated := func() bool {
		if heightOfPeer(boot.PeersOfChannel(common.ChannelID("A")), lastPeer) != 2 {
			return false
		}
		for i := 0; i < n-1; i++ {
			if heightOfPeer(peers[i].PeersOfChannel(common.ChannelID("A")), lastPeer) != 2 {
				return false
			}
			for _, p := range peers[i].PeersOfChannel(common.ChannelID("A")) {
				if len(p.Properties.Chaincodes) != 1 {
					return false
				}

				if !reflect.DeepEqual(p.Properties.Chaincodes, []*proto.Chaincode{{Name: "exampleCC", Version: "1.2"}}) {
					return false
				}
			}
		}
		return true
	}

	membershipTime := time.Now()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n), "waiting for all peers to form membership view")
	t.Log("Membership establishment took", time.Since(membershipTime))

	for i := 2; i <= msgsCount2Send+1; i++ {
		boot.Gossip(createDataMsg(uint64(i), []byte{}, common.ChannelID("A")))
	}

	t2 := time.Now()
	waitUntilOrFailBlocking(t, wg.Wait, "waiting to receive all messages")
	t.Log("Block dissemination took", time.Since(t2))
	t2 = time.Now()
	waitUntilOrFail(t, metaDataUpdated, "wa")
	t.Log("Metadata dissemination took", time.Since(t2))

	for i := 0; i < n; i++ {
		require.Equal(t, msgsCount2Send, receivedMessages[i])
	}

	// Sending leadership messages
	receivedLeadershipMessages := make([]int, n)
	wgLeadership := sync.WaitGroup{}
	wgLeadership.Add(n)
	for i := 1; i <= n; i++ {
		leadershipChan, _ := peers[i-1].Accept(acceptLeadershp, false)
		go func(index int, ch <-chan *proto.GossipMessage) {
			defer wgLeadership.Done()
			msg := <-ch
			if bytes.Equal(msg.Channel, common.ChannelID("A")) {
				receivedLeadershipMessages[index]++
			}
		}(i-1, leadershipChan)
	}

	seqNum := 0
	incTime := uint64(time.Now().UnixNano())
	t3 := time.Now()

	leadershipMsg := createLeadershipMsg(true, common.ChannelID("A"), incTime, uint64(seqNum), boot.Node.comm.GetPKIid())
	boot.Gossip(leadershipMsg)

	waitUntilOrFailBlocking(t, wgLeadership.Wait, "waiting to get all leadership messages")
	t.Log("Leadership message dissemination took", time.Since(t3))

	for i := 0; i < n; i++ {
		require.Equal(t, 1, receivedLeadershipMessages[i])
	}

	t.Log("Stopping peers")

	stop := func() {
		stopPeers(append(peers, boot))
	}

	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop, "waiting for all instances to stop")
	t.Log("Stop took", time.Since(stopTime))
	t.Log("Took", time.Since(t1))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDissemination>>>")
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

	t1 := time.Now()

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	port2, grpc2, certs2, secDialOpts2, _ := util.CreateGRPCLayer()
	boot0 := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	boot1 := newGossipInstanceWithGRPC(1, port1, grpc1, certs1, secDialOpts1, 100)
	boot2 := newGossipInstanceWithGRPC(2, port2, grpc2, certs2, secDialOpts2, 100)
	ports := []int{port0, port1, port2}

	peers := []*gossipGRPC{boot0, boot1, boot2}
	// 0: {3, 6, 9, 12}
	// 1: {4, 7, 10, 13}
	// 2: {5, 8, 11, 14}
	for i := 3; i < 15; i++ {
		pI := newGossipInstanceCreateGRPC(i, 100, ports[i%3])
		peers = append(peers, pI)
	}

	waitUntilOrFail(t, checkPeersMembership(t, peers, 4), "waiting for all instance to form membership")
	t.Log("Sets of peers connected successfully")

	port15, grpc15, certs15, secDialOpts15, _ := util.CreateGRPCLayer()
	connectorPeer := newGossipInstanceWithGRPC(15, port15, grpc15, certs15, secDialOpts15, 100, ports...)
	endpoint15 := fmt.Sprintf("127.0.0.1:%d", port15)
	connectorPeer.UpdateMetadata([]byte("Connector"))

	fullKnowledge := func() bool {
		for i := 0; i < 15; i++ {
			if len(peers[i].Peers()) != 15 {
				return false
			}
			if string(metadataOfPeer(peers[i].Peers(), endpoint15)) != "Connector" {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, fullKnowledge, "waiting for all instances to form membership view")

	t.Log("Stopping connector...")
	waitUntilOrFailBlocking(t, connectorPeer.Stop, "waiting for connector to stop")
	t.Log("Stopped")
	time.Sleep(time.Duration(15) * time.Second)

	ensureForget := func() bool {
		for i := 0; i < 15; i++ {
			if len(peers[i].Peers()) != 14 {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureForget, "waiting to ensure we evicted stopped connector")

	port15, grpc15, certs15, secDialOpts15, _ = util.CreateGRPCLayer()
	connectorPeer = newGossipInstanceWithGRPC(15, port15, grpc15, certs15, secDialOpts15, 100, ports...)
	endpoint15 = fmt.Sprintf("127.0.0.1:%d", port15)
	connectorPeer.UpdateMetadata([]byte("Connector2"))
	t.Log("Started connector")

	ensureResync := func() bool {
		for i := 0; i < 15; i++ {
			if len(peers[i].Peers()) != 15 {
				return false
			}
			if string(metadataOfPeer(peers[i].Peers(), endpoint15)) != "Connector2" {
				return false
			}
		}
		return true
	}

	waitUntilOrFail(t, ensureResync, "waiting for connector 2 to become part of membership view")

	waitUntilOrFailBlocking(t, connectorPeer.Stop, "waiting for connector 2 to stop")

	t.Log("Stopping peers")
	stop := func() {
		stopPeers(peers)
	}

	waitUntilOrFailBlocking(t, stop, "waiting for instances to stop")
	atomic.StoreInt32(&stopped, int32(1))
	t.Log("Took", time.Since(t1))
	fmt.Println("<<<TestMembershipConvergence>>>")
}

func TestMembershipRequestSpoofing(t *testing.T) {
	// Scenario: g1, g2, g3 are peers, and g2 is malicious, and wants
	// to impersonate g3 when sending a membership request to g1.
	// Expected output: g1 should *NOT* respond to g2,
	// However, g1 should respond to g3 when it sends the message itself.

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	port2, grpc2, certs2, secDialOpts2, _ := util.CreateGRPCLayer()
	g1 := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	g2 := newGossipInstanceWithGRPC(1, port1, grpc1, certs1, secDialOpts1, 100, port2)
	g3 := newGossipInstanceWithGRPC(2, port2, grpc2, certs2, secDialOpts2, 100, port1)
	defer g1.Stop()
	defer g2.Stop()
	defer g3.Stop()

	endpoint0 := fmt.Sprintf("127.0.0.1:%d", port0)
	endpoint2 := fmt.Sprintf("127.0.0.1:%d", port2)

	// Wait for g2 and g3 to know about each other
	waitUntilOrFail(t, checkPeersMembership(t, []*gossipGRPC{g2, g3}, 1), "wait for g2 and g3 to know about each other")
	// Obtain an alive message from p3
	_, aliveMsgChan := g2.Accept(func(o interface{}) bool {
		msg := o.(protoext.ReceivedMessage).GetGossipMessage()
		// Make sure we get an AliveMessage and it's about g3
		return protoext.IsAliveMsg(msg.GossipMessage) && bytes.Equal(msg.GetAliveMsg().Membership.PkiId, []byte(endpoint2))
	}, true)
	aliveMsg := <-aliveMsgChan

	// Obtain channel for messages from g1 to g2
	_, g1ToG2 := g2.Accept(func(o interface{}) bool {
		connInfo := o.(protoext.ReceivedMessage).GetConnectionInfo()
		return bytes.Equal([]byte(endpoint0), connInfo.ID)
	}, true)

	// Obtain channel for messages from g1 to g3
	_, g1ToG3 := g3.Accept(func(o interface{}) bool {
		connInfo := o.(protoext.ReceivedMessage).GetConnectionInfo()
		return bytes.Equal([]byte(endpoint0), connInfo.ID)
	}, true)

	// Now, create a membership request message
	memRequestSpoofFactory := func(aliveMsgEnv *proto.Envelope) *protoext.SignedGossipMessage {
		sMsg, _ := protoext.NoopSign(&proto.GossipMessage{
			Tag:   proto.GossipMessage_EMPTY,
			Nonce: uint64(0),
			Content: &proto.GossipMessage_MemReq{
				MemReq: &proto.MembershipRequest{
					SelfInformation: aliveMsgEnv,
					Known:           [][]byte{},
				},
			},
		})
		return sMsg
	}
	spoofedMemReq := memRequestSpoofFactory(aliveMsg.GetSourceEnvelope())
	g2.Send(spoofedMemReq.GossipMessage, &comm.RemotePeer{Endpoint: endpoint0, PKIID: common.PKIidType(endpoint0)})
	select {
	case <-time.After(time.Second):
		break
	case <-g1ToG2:
		require.Fail(t, "Received response from g1 but shouldn't have")
	}

	// Now send the same message from g3 to g1
	g3.Send(spoofedMemReq.GossipMessage, &comm.RemotePeer{Endpoint: endpoint0, PKIID: common.PKIidType(endpoint0)})
	select {
	case <-time.After(time.Second):
		require.Fail(t, "Didn't receive a message back from g1 on time")
	case <-g1ToG3:
		break
	}
}

func TestDataLeakage(t *testing.T) {
	// Scenario: spawn some nodes and let them all
	// establish full membership.
	// Then, have half be in channel A and half be in channel B.
	// However, make it so that only the first 3 from each channel
	// are eligible to obtain blocks from the channels they're in.
	// Ensure nodes only get messages of their channels and in case they
	// are eligible for the channels.

	totalPeers := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} // THIS MUST BE EVEN AND NOT ODD
	n := len(totalPeers)
	// Peer0 and Peer5 disseminate blocks
	// only 1,2 and 6,7 should get blocks.

	var ports []int
	var grpcs []*corecomm.GRPCServer
	var certs []*common.TLSCertificates
	var secDialOpts []api.PeerSecureDialOpts
	var endpoints []string

	for i := 0; i < n; i++ {
		port, grpc, cert, secDialOpt, _ := util.CreateGRPCLayer()
		ports = append(ports, port)
		grpcs = append(grpcs, grpc)
		certs = append(certs, cert)
		secDialOpts = append(secDialOpts, secDialOpt)
		endpoints = append(endpoints, fmt.Sprintf("127.0.0.1:%d", port))
	}

	metrics := metrics.NewGossipMetrics(&disabled.Provider{})
	mcs := &naiveCryptoService{
		allowedPkiIDS: map[string]struct{}{
			// Channel A
			endpoints[0]: {},
			endpoints[1]: {},
			endpoints[2]: {},
			// Channel B
			endpoints[5]: {},
			endpoints[6]: {},
			endpoints[7]: {},
		},
	}

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	peers := make([]*gossipGRPC, n)
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			totPeers := append([]int(nil), ports[:i]...)
			bootPeers := append(totPeers, ports[i+1:]...)
			peers[i] = newGossipInstanceWithGrpcMcsMetrics(i, ports[i], grpcs[i], certs[i], secDialOpts[i], 100, mcs, metrics, bootPeers...)
			wg.Done()
		}(i)
	}

	waitUntilOrFailBlocking(t, wg.Wait, "waiting to create all instances")
	waitUntilOrFail(t, checkPeersMembership(t, peers, n-1), "waiting for all instance to form membership view")

	channels := []common.ChannelID{common.ChannelID("A"), common.ChannelID("B")}

	height := uint64(1)

	for i, channel := range channels {
		for j := 0; j < (n / 2); j++ {
			instanceIndex := (n/2)*i + j
			peers[instanceIndex].JoinChan(&joinChanMsg{}, channel)
			if i != 0 {
				height = uint64(2)
			}
			peers[instanceIndex].UpdateLedgerHeight(height, channel)
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
	waitUntilOrFail(t, seeChannelMetadata, "waiting for all peers to build per channel view")

	t.Log("Metadata sync took", time.Since(t1))
	for i, channel := range channels {
		for j := 0; j < 3; j++ {
			instanceIndex := (n/2)*i + j
			require.Len(t, peers[instanceIndex].PeersOfChannel(channel), 2)
			if i == 0 {
				require.Equal(t, uint64(1), peers[instanceIndex].PeersOfChannel(channel)[0].Properties.LedgerHeight)
			} else {
				require.Equal(t, uint64(2), peers[instanceIndex].PeersOfChannel(channel)[0].Properties.LedgerHeight)
			}
		}
	}

	gotMessages := func() {
		var wg sync.WaitGroup
		wg.Add(4)
		for i, channel := range channels {
			for j := 1; j < 3; j++ {
				instanceIndex := (n/2)*i + j
				go func(instanceIndex int, channel common.ChannelID) {
					incMsgChan, _ := peers[instanceIndex].Accept(acceptData, false)
					msg := <-incMsgChan
					require.Equal(t, []byte(channel), []byte(msg.Channel))
					wg.Done()
				}(instanceIndex, channel)
			}
		}
		wg.Wait()
	}

	t1 = time.Now()
	peers[0].Gossip(createDataMsg(2, []byte{}, channels[0]))
	peers[n/2].Gossip(createDataMsg(3, []byte{}, channels[1]))
	waitUntilOrFailBlocking(t, gotMessages, "waiting to get messages")
	t.Log("Dissemination took", time.Since(t1))
	stop := func() {
		stopPeers(peers)
	}
	stopTime := time.Now()
	waitUntilOrFailBlocking(t, stop, "waiting for all instances to stop")
	t.Log("Stop took", time.Since(stopTime))
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDataLeakage>>>")
}

func TestDisseminateAll2All(t *testing.T) {
	t.Skip()

	// Scenario: spawn some nodes, have each node
	// disseminate a block to all nodes.
	// Ensure all blocks are received

	stopped := int32(0)
	go waitForTestCompletion(&stopped, t)

	totalPeers := []int{0, 1, 2, 3, 4, 5, 6}
	n := len(totalPeers)
	peers := make([]*gossipGRPC, n)
	wg := sync.WaitGroup{}

	var ports []int
	var grpcs []*corecomm.GRPCServer
	var certs []*common.TLSCertificates
	var secDialOpts []api.PeerSecureDialOpts

	for i := 0; i < n; i++ {
		port, grpc, cert, secDialOpt, _ := util.CreateGRPCLayer()
		ports = append(ports, port)
		grpcs = append(grpcs, grpc)
		certs = append(certs, cert)
		secDialOpts = append(secDialOpts, secDialOpt)
	}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			totPeers := append([]int(nil), ports[:i]...)
			bootPeers := append(totPeers, ports[i+1:]...)
			pI := newGossipInstanceWithGRPC(i, ports[i], grpcs[i], certs[i], secDialOpts[i], 100, bootPeers...)
			pI.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
			pI.UpdateLedgerHeight(1, common.ChannelID("A"))
			peers[i] = pI
			wg.Done()
		}(i)
	}
	wg.Wait()
	waitUntilOrFail(t, checkPeersMembership(t, peers, n-1), "waiting for instances to form membership view")

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
				peers[i].Gossip(createDataMsg(blockSeq, []byte{}, common.ChannelID("A")))
			}
		}(i)
	}
	waitUntilOrFailBlocking(t, bMutex.Wait, "waiting for all message been distributed among all instances")

	stop := func() {
		stopPeers(peers)
	}
	waitUntilOrFailBlocking(t, stop, "waiting for all instance to stop")
	atomic.StoreInt32(&stopped, int32(1))
	fmt.Println("<<<TestDisseminateAll2All>>>")
}

func TestSendByCriteria(t *testing.T) {
	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	g1 := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 100)
	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	g2 := newGossipInstanceWithGRPC(1, port1, grpc1, certs1, secDialOpts1, 100, port0)
	port2, grpc2, certs2, secDialOpts2, _ := util.CreateGRPCLayer()
	g3 := newGossipInstanceWithGRPC(2, port2, grpc2, certs2, secDialOpts2, 100, port0)
	port3, grpc3, certs3, secDialOpts3, _ := util.CreateGRPCLayer()
	g4 := newGossipInstanceWithGRPC(3, port3, grpc3, certs3, secDialOpts3, 100, port0)

	peers := []*gossipGRPC{g1, g2, g3, g4}
	for _, p := range peers {
		p.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
		p.UpdateLedgerHeight(1, common.ChannelID("A"))
	}
	defer stopPeers(peers)
	msg, _ := protoext.NoopSign(createDataMsg(1, []byte{}, common.ChannelID("A")))

	// We send without specifying maximum peers,
	// which sets it to the zero value, and
	// this is a no-op.
	criteria := SendCriteria{
		IsEligible: func(discovery.NetworkMember) bool {
			t.Fatal("Shouldn't have called, because when max peers is 0, the operation is a no-op")
			return false
		},
		Timeout: time.Second * 1,
		MinAck:  1,
	}
	require.NoError(t, g1.SendByCriteria(msg, criteria))

	// We send without specifying a timeout
	criteria = SendCriteria{
		MaxPeers: 100,
	}
	err := g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Equal(t, "Timeout should be specified", err.Error())

	// We send without specifying a minimum acknowledge threshold
	criteria.Timeout = time.Second * 3
	err = g1.SendByCriteria(msg, criteria)
	// Should work, because minAck is 0 (not specified)
	require.NoError(t, err)

	// We send without specifying a channel
	criteria.Channel = common.ChannelID("B")
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "but no such channel exists")

	// We send to peers from the channel, but we expect 10 acknowledgements.
	// It should immediately return because we don't know about 10 peers so no point in even trying
	criteria.Channel = common.ChannelID("A")
	criteria.MinAck = 10
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requested to send to at least 10 peers, but know only of")

	// We send to a minimum of 3 peers with acknowledgement, while no peer acknowledges the messages.
	// Wait until g1 sees the rest of the peers in the channel
	waitUntilOrFail(t, func() bool {
		return len(g1.PeersOfChannel(common.ChannelID("A"))) > 2
	}, "waiting until g1 sees the rest of the peers in the channel")
	criteria.MinAck = 3
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
	require.Contains(t, err.Error(), "3")

	// We retry the test above, but this time the peers acknowledge
	// Peers now ack
	acceptDataMsgs := func(m interface{}) bool {
		return protoext.IsDataMsg(m.(protoext.ReceivedMessage).GetGossipMessage().GossipMessage)
	}
	_, ackChan2 := g2.Accept(acceptDataMsgs, true)
	_, ackChan3 := g3.Accept(acceptDataMsgs, true)
	_, ackChan4 := g4.Accept(acceptDataMsgs, true)
	ack := func(c <-chan protoext.ReceivedMessage) {
		msg := <-c
		msg.Ack(nil)
	}

	go ack(ackChan2)
	go ack(ackChan3)
	go ack(ackChan4)
	err = g1.SendByCriteria(msg, criteria)
	require.NoError(t, err)

	// We send to 3 peers, but 2 out of 3 peers acknowledge with an error
	nack := func(c <-chan protoext.ReceivedMessage) {
		msg := <-c
		msg.Ack(fmt.Errorf("uh oh"))
	}
	go ack(ackChan2)
	go nack(ackChan3)
	go nack(ackChan4)
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "uh oh")

	// We try to send to either g2 or g3, but neither would ack us, so we would fail.
	// However - what we actually check in this test is that we send to peers according to the
	// filter passed in the criteria
	failOnAckRequest := func(c <-chan protoext.ReceivedMessage, peerId int) {
		msg := <-c
		if msg == nil {
			return
		}
		t.Errorf("%d got a message, but shouldn't have!", peerId)
	}
	g2Endpoint := fmt.Sprintf("127.0.0.1:%d", port1)
	g3Endpoint := fmt.Sprintf("127.0.0.1:%d", port2)
	criteria.IsEligible = func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == g2Endpoint || nm.InternalEndpoint == g3Endpoint
	}
	criteria.MinAck = 1
	go failOnAckRequest(ackChan4, 3)
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
	require.Contains(t, err.Error(), "2")
	// Finally, ack the lost messages, to cleanup for the next test
	ack(ackChan2)
	ack(ackChan3)

	// We may send to 2 peers, but we check that if we specify a max criteria.MaxPeers,
	// this property is respected - and only 1 peer receives a message, and not both
	criteria.MaxPeers = 1
	// invoke f() in case message has been received
	waitForMessage := func(c <-chan protoext.ReceivedMessage, f func()) {
		select {
		case msg := <-c:
			if msg == nil {
				return
			}
		case <-time.After(time.Second * 5):
			return
		}
		f()
	}
	var messagesSent uint32
	go waitForMessage(ackChan2, func() {
		atomic.AddUint32(&messagesSent, 1)
	})
	go waitForMessage(ackChan3, func() {
		atomic.AddUint32(&messagesSent, 1)
	})
	err = g1.SendByCriteria(msg, criteria)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
	// Check how many messages were sent.
	// Only 1 should have been sent
	require.Equal(t, uint32(1), atomic.LoadUint32(&messagesSent))
}

func TestIdentityExpiration(t *testing.T) {
	// Scenario: spawn 5 peers and make the MessageCryptoService revoke one of the first 4.
	// The last peer's certificate expires after a few seconds.
	// Eventually, the rest of the peers should not be able to communicate with
	// the revoked peer at all because its identity would seem to them as expired

	var expirationTimesLock sync.RWMutex
	expirationTimes := map[string]time.Time{}

	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	g1 := newGossipInstanceWithExpiration(expirationTimes, &expirationTimesLock, 1, port1, grpc1, certs1, secDialOpts1, 100)
	port2, grpc2, certs2, secDialOpts2, _ := util.CreateGRPCLayer()
	g2 := newGossipInstanceWithExpiration(expirationTimes, &expirationTimesLock, 2, port2, grpc2, certs2, secDialOpts2, 100, port1)
	port3, grpc3, certs3, secDialOpts3, _ := util.CreateGRPCLayer()
	g3 := newGossipInstanceWithExpiration(expirationTimes, &expirationTimesLock, 3, port3, grpc3, certs3, secDialOpts3, 100, port1)
	port4, grpc4, certs4, secDialOpts4, _ := util.CreateGRPCLayer()
	g4 := newGossipInstanceWithExpiration(expirationTimes, &expirationTimesLock, 4, port4, grpc4, certs4, secDialOpts4, 100, port1)
	port5, grpc5, certs5, secDialOpts5, _ := util.CreateGRPCLayer()
	g5 := newGossipInstanceWithExpiration(expirationTimes, &expirationTimesLock, 5, port5, grpc5, certs5, secDialOpts5, 100, port1)

	// Set expiration of the last peer to 5 seconds from now
	endpointLast := fmt.Sprintf("127.0.0.1:%d", port5)
	expirationTimesLock.Lock()
	expirationTimes[endpointLast] = time.Now().Add(time.Second * 5)
	expirationTimesLock.Unlock()

	peers := []*gossipGRPC{g1, g2, g3, g4}

	// Make the last peer be revoked in 5 seconds from now
	time.AfterFunc(time.Second*5, func() {
		for _, p := range peers {
			p.Node.mcs.(*naiveCryptoService).revoke(common.PKIidType(endpointLast))
		}
	})

	seeAllNeighbors := func() bool {
		for i := 0; i < 4; i++ {
			neighborCount := len(peers[i].Peers())
			if neighborCount != 3 {
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, seeAllNeighbors, "waiting for all instances to form uniform membership view")
	// Now revoke some peer
	var ports []int
	ports = append(ports, port1, port2, port3, port4)
	revokedPeerIndex := rand.Intn(4)
	revokedPkiID := common.PKIidType(fmt.Sprintf("127.0.0.1:%d", ports[revokedPeerIndex]))
	for i, p := range peers {
		if i == revokedPeerIndex {
			continue
		}
		p.Node.mcs.(*naiveCryptoService).revoke(revokedPkiID)
	}
	// Trigger a config update to the rest of the peers
	for i := 0; i < 4; i++ {
		if i == revokedPeerIndex {
			continue
		}
		peers[i].SuspectPeers(func(_ api.PeerIdentityType) bool {
			return true
		})
	}
	// Ensure that no one talks to the peer that is revoked
	ensureRevokedPeerIsIgnored := func() bool {
		for i := 0; i < 4; i++ {
			neighborCount := len(peers[i].Peers())
			expectedNeighborCount := 2
			// If it's the revoked peer, or the last peer who's certificate
			// has expired
			if i == revokedPeerIndex || i == 4 {
				expectedNeighborCount = 0
			}
			if neighborCount != expectedNeighborCount {
				fmt.Println("neighbor count of", i, "is", neighborCount)
				return false
			}
		}
		return true
	}
	waitUntilOrFail(t, ensureRevokedPeerIsIgnored, "waiting to make sure revoked peers are ignored")
	stopPeers(peers)
	g5.Stop()
}

func createDataMsg(seqnum uint64, data []byte, channel common.ChannelID) *proto.GossipMessage {
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

func createLeadershipMsg(isDeclaration bool, channel common.ChannelID, incTime uint64, seqNum uint64, pkiid []byte) *proto.GossipMessage {
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

func metadataOfPeer(members []discovery.NetworkMember, endpoint string) []byte {
	for _, member := range members {
		if member.InternalEndpoint == endpoint {
			return member.Metadata
		}
	}
	return nil
}

func heightOfPeer(members []discovery.NetworkMember, endpoint string) int {
	for _, member := range members {
		if member.InternalEndpoint == endpoint {
			return int(member.Properties.LedgerHeight)
		}
	}
	return -1
}

func waitForTestCompletion(stopFlag *int32, t *testing.T) {
	time.Sleep(timeout)
	if atomic.LoadInt32(stopFlag) == int32(1) {
		return
	}
	util.PrintStackTrace()
	require.Fail(t, "Didn't stop within a timely manner")
}

func stopPeers(peers []*gossipGRPC) {
	stoppingWg := sync.WaitGroup{}
	stoppingWg.Add(len(peers))
	for i, pI := range peers {
		go func(i int, p_i *gossipGRPC) {
			defer stoppingWg.Done()
			p_i.Stop()
		}(i, pI)
	}
	stoppingWg.Wait()
}

func waitUntilOrFail(t *testing.T, pred func() bool, context string) {
	start := time.Now()
	limit := start.UnixNano() + timeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeout / 1000)
	}
	util.PrintStackTrace()
	require.Failf(t, "Timeout expired, while %s", context)
}

func waitUntilOrFailBlocking(t *testing.T, f func(), context string) {
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
	require.Failf(t, "Timeout expired, while %s", context)
}

func checkPeersMembership(t *testing.T, peers []*gossipGRPC, n int) func() bool {
	return func() bool {
		for _, peer := range peers {
			if len(peer.Peers()) != n {
				return false
			}
			for _, p := range peer.Peers() {
				require.NotNil(t, p.InternalEndpoint)
				require.NotEmpty(t, p.Endpoint)
			}
		}
		return true
	}
}

func TestMembershipMetrics(t *testing.T) {
	wg0 := sync.WaitGroup{}
	wg0.Add(1)
	once0 := sync.Once{}
	wg1 := sync.WaitGroup{}
	wg1.Add(1)
	once1 := sync.Once{}

	testMetricProvider := mocks.TestUtilConstructMetricProvider()

	testMetricProvider.FakeTotalGauge.SetStub = func(delta float64) {
		if delta == 0 {
			once0.Do(func() {
				wg0.Done()
			})
		}
		if delta == 1 {
			once1.Do(func() {
				wg1.Done()
			})
		}
	}

	gmetrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider)

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	pI0 := newGossipInstanceWithGrpcMcsMetrics(0, port0, grpc0, certs0, secDialOpts0, 100, &naiveCryptoService{}, gmetrics)
	pI0.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	pI0.UpdateLedgerHeight(1, common.ChannelID("A"))

	// assert channel membership metrics reported with 0 as value
	wg0.Wait()
	require.Equal(t,
		[]string{"channel", "A"},
		testMetricProvider.FakeTotalGauge.WithArgsForCall(0),
	)
	require.EqualValues(t, 0,
		testMetricProvider.FakeTotalGauge.SetArgsForCall(0),
	)

	pI1 := newGossipInstanceCreateGRPC(1, 100, port0)

	pI1.JoinChan(&joinChanMsg{}, common.ChannelID("A"))
	pI1.UpdateLedgerHeight(1, common.ChannelID("A"))

	waitForMembership := func(n int) func() bool {
		return func() bool {
			if len(pI0.PeersOfChannel(common.ChannelID("A"))) != n || len(pI1.PeersOfChannel(common.ChannelID("A"))) != n {
				return false
			}
			return true
		}
	}
	waitUntilOrFail(t, waitForMembership(1), "waiting for metrics membership of 1")

	// assert channel membership metrics reported with 1 as value
	wg1.Wait()

	pI1.Stop()
	waitUntilOrFail(t, waitForMembership(0), "waiting for metrics membership of 0")
	pI0.Stop()
}
