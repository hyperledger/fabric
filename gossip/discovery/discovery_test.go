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

package discovery

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var timeout = time.Second * time.Duration(15)

func init() {
	aliveTimeInterval := time.Duration(time.Millisecond * 100)
	SetAliveTimeInterval(aliveTimeInterval)
	SetAliveExpirationTimeout(10 * aliveTimeInterval)
	SetAliveExpirationCheckInterval(aliveTimeInterval)
	SetReconnectInterval(10 * aliveTimeInterval)
	maxConnectionAttempts = 10000
}

type dummyCommModule struct {
	id           string
	presumeDead  chan common.PKIidType
	detectedDead chan string
	streams      map[string]proto.Gossip_GossipStreamClient
	conns        map[string]*grpc.ClientConn
	lock         *sync.RWMutex
	incMsgs      chan *proto.SignedGossipMessage
	lastSeqs     map[string]uint64
	shouldGossip bool
	mock         *mock.Mock
}

type gossipMsg struct {
	*proto.GossipMessage
}

func (m *gossipMsg) GetGossipMessage() *proto.GossipMessage {
	return m.GossipMessage
}

type gossipInstance struct {
	comm *dummyCommModule
	Discovery
	gRGCserv      *grpc.Server
	lsnr          net.Listener
	shouldGossip  bool
	syncInitiator *time.Ticker
	stopChan      chan struct{}
	port          int
}

func (comm *dummyCommModule) ValidateAliveMsg(am *proto.SignedGossipMessage) bool {
	return true
}

func (comm *dummyCommModule) SignMessage(am *proto.GossipMessage, internalEndpoint string) *proto.Envelope {
	am.NoopSign()

	secret := &proto.Secret{
		Content: &proto.Secret_InternalEndpoint{
			InternalEndpoint: internalEndpoint,
		},
	}
	signer := func(msg []byte) ([]byte, error) {
		return nil, nil
	}
	env := am.NoopSign().Envelope
	env.SignSecret(signer, secret)
	return env
}

func (comm *dummyCommModule) Gossip(msg *proto.SignedGossipMessage) {
	if !comm.shouldGossip {
		return
	}
	comm.lock.Lock()
	defer comm.lock.Unlock()
	for _, conn := range comm.streams {
		conn.Send(msg.Envelope)
	}
}

func (comm *dummyCommModule) SendToPeer(peer *NetworkMember, msg *proto.SignedGossipMessage) {
	comm.lock.RLock()
	_, exists := comm.streams[peer.Endpoint]
	mock := comm.mock
	comm.lock.RUnlock()

	if mock != nil {
		mock.Called(peer, msg)
	}

	if !exists {
		if comm.Ping(peer) == false {
			fmt.Printf("Ping to %v failed\n", peer.Endpoint)
			return
		}
	}
	comm.lock.Lock()
	comm.streams[peer.Endpoint].Send(msg.NoopSign().Envelope)
	comm.lock.Unlock()
}

func (comm *dummyCommModule) Ping(peer *NetworkMember) bool {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	if comm.mock != nil {
		comm.mock.Called()
	}

	_, alreadyExists := comm.streams[peer.Endpoint]
	if !alreadyExists {
		newConn, err := grpc.Dial(peer.Endpoint, grpc.WithInsecure())
		if err != nil {
			return false
		}
		if stream, err := proto.NewGossipClient(newConn).GossipStream(context.Background()); err == nil {
			comm.conns[peer.Endpoint] = newConn
			comm.streams[peer.Endpoint] = stream
			return true
		}
		return false
	}
	conn := comm.conns[peer.Endpoint]
	if _, err := proto.NewGossipClient(conn).Ping(context.Background(), &proto.Empty{}); err != nil {
		return false
	}
	return true
}

func (comm *dummyCommModule) Accept() <-chan *proto.SignedGossipMessage {
	return comm.incMsgs
}

func (comm *dummyCommModule) PresumedDead() <-chan common.PKIidType {
	return comm.presumeDead
}

func (comm *dummyCommModule) CloseConn(peer *NetworkMember) {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	if _, exists := comm.streams[peer.Endpoint]; !exists {
		return
	}

	comm.streams[peer.Endpoint].CloseSend()
	comm.conns[peer.Endpoint].Close()
}

func (g *gossipInstance) initiateSync(frequency time.Duration, peerNum int) {
	g.syncInitiator = time.NewTicker(frequency)
	g.stopChan = make(chan struct{})
	go func() {
		for {
			select {
			case <-g.syncInitiator.C:
				g.Discovery.InitiateSync(peerNum)
			case <-g.stopChan:
				g.syncInitiator.Stop()
				return
			}
		}
	}()
}

func (g *gossipInstance) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	for {
		envelope, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		lgr := g.Discovery.(*gossipDiscoveryImpl).logger
		gMsg, err := envelope.ToGossipMessage()
		if err != nil {
			lgr.Warning("Failed deserializing GossipMessage from envelope:", err)
			continue
		}

		lgr.Debug(g.Discovery.Self().Endpoint, "Got message:", gMsg)
		g.comm.incMsgs <- gMsg

		if aliveMsg := gMsg.GetAliveMsg(); aliveMsg != nil {
			g.tryForwardMessage(gMsg)
		}
	}
}

func (g *gossipInstance) tryForwardMessage(msg *proto.SignedGossipMessage) {
	g.comm.lock.Lock()

	aliveMsg := msg.GetAliveMsg()

	forward := false
	id := string(aliveMsg.Membership.PkiId)
	seqNum := aliveMsg.Timestamp.SeqNum
	if last, exists := g.comm.lastSeqs[id]; exists {
		if last < seqNum {
			g.comm.lastSeqs[id] = seqNum
			forward = true
		}
	} else {
		g.comm.lastSeqs[id] = seqNum
		forward = true
	}

	g.comm.lock.Unlock()

	if forward {
		g.comm.Gossip(msg)
	}
}

func (g *gossipInstance) Stop() {
	if g.syncInitiator != nil {
		g.stopChan <- struct{}{}
	}
	g.gRGCserv.Stop()
	g.lsnr.Close()
	for _, stream := range g.comm.streams {
		stream.CloseSend()
	}
	for _, conn := range g.comm.conns {
		conn.Close()
	}
	g.Discovery.Stop()
}

func (g *gossipInstance) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

var noopPolicy = func(remotePeer *NetworkMember) (Sieve, EnvelopeFilter) {
	return func(msg *proto.SignedGossipMessage) bool {
			return true
		}, func(message *proto.SignedGossipMessage) *proto.Envelope {
			return message.Envelope
		}
}

func createDiscoveryInstance(port int, id string, bootstrapPeers []string) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, true, noopPolicy)
}

func createDiscoveryInstanceWithNoGossip(port int, id string, bootstrapPeers []string) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, false, noopPolicy)
}

func createDiscoveryInstanceWithNoGossipWithDisclosurePolicy(port int, id string, bootstrapPeers []string, pol DisclosurePolicy) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, false, pol)
}

func createDiscoveryInstanceThatGossips(port int, id string, bootstrapPeers []string, shouldGossip bool, pol DisclosurePolicy) *gossipInstance {
	comm := &dummyCommModule{
		conns:        make(map[string]*grpc.ClientConn),
		streams:      make(map[string]proto.Gossip_GossipStreamClient),
		incMsgs:      make(chan *proto.SignedGossipMessage, 1000),
		presumeDead:  make(chan common.PKIidType, 10000),
		id:           id,
		detectedDead: make(chan string, 10000),
		lock:         &sync.RWMutex{},
		lastSeqs:     make(map[string]uint64),
		shouldGossip: shouldGossip,
	}

	endpoint := fmt.Sprintf("localhost:%d", port)
	self := NetworkMember{
		Metadata:         []byte{},
		PKIid:            []byte(endpoint),
		Endpoint:         endpoint,
		InternalEndpoint: endpoint,
	}

	listenAddress := fmt.Sprintf("%s:%d", "", port)
	ll, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Printf("Error listening on %v, %v", listenAddress, err)
	}
	s := grpc.NewServer()

	discSvc := NewDiscoveryService(bootstrapPeers, self, comm, comm, pol)
	gossInst := &gossipInstance{comm: comm, gRGCserv: s, Discovery: discSvc, lsnr: ll, shouldGossip: shouldGossip, port: port}

	proto.RegisterGossipServer(s, gossInst)
	go s.Serve(ll)

	return gossInst
}

func bootPeer(port int) string {
	return fmt.Sprintf("localhost:%d", port)
}

func TestConnect(t *testing.T) {
	t.Parallel()
	nodeNum := 10
	instances := []*gossipInstance{}
	firstSentMemReqMsgs := make(chan *proto.SignedGossipMessage, nodeNum)
	for i := 0; i < nodeNum; i++ {
		inst := createDiscoveryInstance(7611+i, fmt.Sprintf("d%d", i), []string{})

		inst.comm.lock.Lock()
		inst.comm.mock = &mock.Mock{}
		inst.comm.mock.On("SendToPeer", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
			inst := inst
			msg := arguments.Get(1).(*proto.SignedGossipMessage)
			if req := msg.GetMemReq(); req != nil {
				selfMsg, _ := req.SelfInformation.ToGossipMessage()
				firstSentMemReqMsgs <- selfMsg
				inst.comm.lock.Lock()
				inst.comm.mock = nil
				inst.comm.lock.Unlock()
			}
		})
		inst.comm.mock.On("Ping", mock.Anything)
		inst.comm.lock.Unlock()

		instances = append(instances, inst)
		j := (i + 1) % 10
		endpoint := fmt.Sprintf("localhost:%d", 7611+j)
		netMember2Connect2 := NetworkMember{Endpoint: endpoint, PKIid: []byte(endpoint)}
		inst.Connect(netMember2Connect2, func() bool { return false })
	}

	time.Sleep(time.Second * 3)
	assert.Len(t, firstSentMemReqMsgs, 10)
	close(firstSentMemReqMsgs)
	for firstSentSelfMsg := range firstSentMemReqMsgs {
		assert.Nil(t, firstSentSelfMsg.Envelope.SecretEnvelope)
	}

	fullMembership := func() bool {
		return nodeNum-1 == len(instances[nodeNum-1].GetMembership())
	}
	waitUntilOrFail(t, fullMembership)

	discInst := instances[rand.Intn(len(instances))].Discovery.(*gossipDiscoveryImpl)
	am, _ := discInst.createMembershipRequest(true).GetMemReq().SelfInformation.ToGossipMessage()
	assert.NotNil(t, am.SecretEnvelope)
	am, _ = discInst.createMembershipRequest(false).GetMemReq().SelfInformation.ToGossipMessage()
	assert.Nil(t, am.SecretEnvelope)
	stopInstances(t, instances)
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	nodeNum := 5
	bootPeers := []string{bootPeer(6611), bootPeer(6612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(6611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(6612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(6610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	fullMembership := func() bool {
		return nodeNum-1 == len(instances[nodeNum-1].GetMembership())
	}

	waitUntilOrFail(t, fullMembership)

	instances[0].UpdateMetadata([]byte("bla bla"))
	instances[nodeNum-1].UpdateEndpoint("localhost:5511")

	checkMembership := func() bool {
		for _, member := range instances[nodeNum-1].GetMembership() {
			if string(member.PKIid) == instances[0].comm.id {
				if "bla bla" != string(member.Metadata) {
					return false
				}
			}
		}

		for _, member := range instances[0].GetMembership() {
			if string(member.PKIid) == instances[nodeNum-1].comm.id {
				if "localhost:5511" != string(member.Endpoint) {
					return false
				}
			}
		}
		return true
	}

	waitUntilOrFail(t, checkMembership)
	stopInstances(t, instances)
}

func TestInitiateSync(t *testing.T) {
	t.Parallel()
	nodeNum := 10
	bootPeers := []string{bootPeer(3611), bootPeer(3612)}
	instances := []*gossipInstance{}

	toDie := int32(0)
	for i := 1; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst := createDiscoveryInstanceWithNoGossip(3610+i, id, bootPeers)
		instances = append(instances, inst)
		go func() {
			for {
				if atomic.LoadInt32(&toDie) == int32(1) {
					return
				}
				time.Sleep(getAliveExpirationTimeout() / 3)
				inst.InitiateSync(9)
			}
		}()
	}
	time.Sleep(getAliveExpirationTimeout() * 4)
	assertMembership(t, instances, nodeNum-1)
	atomic.StoreInt32(&toDie, int32(1))
	stopInstances(t, instances)
}

func TestExpiration(t *testing.T) {
	t.Parallel()
	nodeNum := 5
	bootPeers := []string{bootPeer(2611), bootPeer(2612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(2611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(2612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(2610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	assertMembership(t, instances, nodeNum-1)

	waitUntilOrFailBlocking(t, instances[nodeNum-1].Stop)
	waitUntilOrFailBlocking(t, instances[nodeNum-2].Stop)

	assertMembership(t, instances, nodeNum-3)

	stopAction := &sync.WaitGroup{}
	for i, inst := range instances {
		if i+2 == nodeNum {
			break
		}
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}

	waitUntilOrFailBlocking(t, stopAction.Wait)
}

func TestGetFullMembership(t *testing.T) {
	t.Parallel()
	nodeNum := 15
	bootPeers := []string{bootPeer(5511), bootPeer(5512)}
	instances := []*gossipInstance{}
	var inst *gossipInstance

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(5510+i, id, bootPeers)
		instances = append(instances, inst)
	}

	time.Sleep(time.Second)

	inst = createDiscoveryInstance(5511, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(5512, "d2", bootPeers)
	instances = append(instances, inst)

	assertMembership(t, instances, nodeNum-1)

	// Ensure that internal endpoint was propagated to everyone
	for _, inst := range instances {
		for _, member := range inst.GetMembership() {
			assert.NotEmpty(t, member.InternalEndpoint)
			assert.NotEmpty(t, member.Endpoint)
		}
	}

	// Check that Lookup() is valid
	for _, inst := range instances {
		for _, member := range inst.GetMembership() {
			assert.Equal(t, string(member.PKIid), inst.Lookup(member.PKIid).Endpoint)
			assert.Equal(t, member.PKIid, inst.Lookup(member.PKIid).PKIid)
		}
	}

	stopInstances(t, instances)
}

func TestGossipDiscoveryStopping(t *testing.T) {
	t.Parallel()
	inst := createDiscoveryInstance(9611, "d1", []string{bootPeer(9611)})
	time.Sleep(time.Second)
	waitUntilOrFailBlocking(t, inst.Stop)
}

func TestGossipDiscoverySkipConnectingToLocalhostBootstrap(t *testing.T) {
	t.Parallel()
	inst := createDiscoveryInstance(11611, "d1", []string{"localhost:11611", "127.0.0.1:11611"})
	inst.comm.lock.Lock()
	inst.comm.mock = &mock.Mock{}
	inst.comm.mock.On("SendToPeer", mock.Anything).Run(func(mock.Arguments) {
		t.Fatal("Should not have connected to any peer")
	})
	inst.comm.mock.On("Ping", mock.Anything).Run(func(mock.Arguments) {
		t.Fatal("Should not have connected to any peer")
	})
	inst.comm.lock.Unlock()
	time.Sleep(time.Second * 3)
	waitUntilOrFailBlocking(t, inst.Stop)
}

func TestConvergence(t *testing.T) {
	t.Parallel()
	// scenario:
	// {boot peer: [peer list]}
	// {d1: d2, d3, d4}
	// {d5: d6, d7, d8}
	// {d9: d10, d11, d12}
	// connect all boot peers with d13
	// take down d13
	// ensure still full membership
	instances := []*gossipInstance{}
	for _, i := range []int{1, 5, 9} {
		bootPort := 4610 + i
		id := fmt.Sprintf("d%d", i)
		leader := createDiscoveryInstance(bootPort, id, []string{})
		instances = append(instances, leader)
		for minionIndex := 1; minionIndex <= 3; minionIndex++ {
			id := fmt.Sprintf("d%d", i+minionIndex)
			minion := createDiscoveryInstance(4610+minionIndex+i, id, []string{bootPeer(bootPort)})
			instances = append(instances, minion)
		}
	}

	assertMembership(t, instances, 3)
	connector := createDiscoveryInstance(4623, "d13", []string{bootPeer(4611), bootPeer(4615), bootPeer(4619)})
	instances = append(instances, connector)
	assertMembership(t, instances, 12)
	connector.Stop()
	instances = instances[:len(instances)-1]
	assertMembership(t, instances, 11)
	stopInstances(t, instances)
}

func TestDisclosurePolicyWithPull(t *testing.T) {
	t.Parallel()
	// Scenario: run 2 groups of peers that simulate 2 organizations:
	// {p0, p1, p2, p3, p4}
	// {p5, p6, p7, p8, p9}
	// Only peers that have an even id have external addresses
	// and only these peers should be published to peers of the other group,
	// while the only ones that need to know about them are peers
	// that have an even id themselves.
	// Furthermore, peers in different sets, should not know about internal addresses of
	// other peers.

	// This is a bootstrap map that matches for each peer its own bootstrap peer.
	// In practice (production) peers should only use peers of their orgs as bootstrap peers,
	// but the discovery layer is ignorant of organizations.
	bootPeerMap := map[int]int{
		8610: 8616,
		8611: 8610,
		8612: 8610,
		8613: 8610,
		8614: 8610,
		8615: 8616,
		8616: 8610,
		8617: 8616,
		8618: 8616,
		8619: 8616,
	}

	// This map matches each peer, the peers it should know about in the test scenario.
	peersThatShouldBeKnownToPeers := map[int][]int{
		8610: {8611, 8612, 8613, 8614, 8616, 8618},
		8611: {8610, 8612, 8613, 8614},
		8612: {8610, 8611, 8613, 8614, 8616, 8618},
		8613: {8610, 8611, 8612, 8614},
		8614: {8610, 8611, 8612, 8613, 8616, 8618},
		8615: {8616, 8617, 8618, 8619},
		8616: {8610, 8612, 8614, 8615, 8617, 8618, 8619},
		8617: {8615, 8616, 8618, 8619},
		8618: {8610, 8612, 8614, 8615, 8616, 8617, 8619},
		8619: {8615, 8616, 8617, 8618},
	}
	// Create the peers in the two groups
	instances1, instances2 := createDisjointPeerGroupsWithNoGossip(bootPeerMap)
	// Sleep a while to let them establish membership. This time should be more than enough
	// because the instances are configured to pull membership in very high frequency from
	// up to 10 peers (which results in - pulling from everyone)
	waitUntilOrFail(t, func() bool {
		for _, inst := range append(instances1, instances2...) {
			// Ensure the expected membership is equal in size to the actual membership
			// of each peer.
			portsOfKnownMembers := portsOfMembers(inst.GetMembership())
			if len(peersThatShouldBeKnownToPeers[inst.port]) != len(portsOfKnownMembers) {
				return false
			}
		}
		return true
	})
	for _, inst := range append(instances1, instances2...) {
		portsOfKnownMembers := portsOfMembers(inst.GetMembership())
		// Ensure the expected membership is equal to the actual membership
		// of each peer. the portsOfMembers returns a sorted slice so assert.Equal does the job.
		assert.Equal(t, peersThatShouldBeKnownToPeers[inst.port], portsOfKnownMembers)
		// Next, check that internal endpoints aren't leaked across groups,
		for _, knownPeer := range inst.GetMembership() {
			// If internal endpoint is known, ensure the peers are in the same group
			// unless the peer in question is a peer that has a public address.
			// We cannot control what we disclose about ourselves when we send a membership request
			if len(knownPeer.InternalEndpoint) > 0 && inst.port%2 != 0 {
				bothInGroup1 := portOfEndpoint(knownPeer.Endpoint) < 8615 && inst.port < 8615
				bothInGroup2 := portOfEndpoint(knownPeer.Endpoint) >= 8615 && inst.port >= 8615
				assert.True(t, bothInGroup1 || bothInGroup2, "%v knows about %v's internal endpoint", inst.port, knownPeer.InternalEndpoint)
			}
		}
	}

	t.Log("Shutting down instance 0...")
	// Now, we shutdown instance 0 and ensure that peers that shouldn't know it,
	// do not know it via membership requests
	stopInstances(t, []*gossipInstance{instances1[0]})
	time.Sleep(time.Second * 3)
	for _, inst := range append(instances1[1:], instances2...) {
		if peersThatShouldBeKnownToPeers[inst.port][0] == 8610 {
			assert.Equal(t, 1, inst.Discovery.(*gossipDiscoveryImpl).deadMembership.Size())
		} else {
			assert.Equal(t, 0, inst.Discovery.(*gossipDiscoveryImpl).deadMembership.Size())
		}
	}
	stopInstances(t, instances1[1:])
	stopInstances(t, instances2)
}

func createDisjointPeerGroupsWithNoGossip(bootPeerMap map[int]int) ([]*gossipInstance, []*gossipInstance) {
	instances1 := []*gossipInstance{}
	instances2 := []*gossipInstance{}
	for group := 0; group < 2; group++ {
		for i := 0; i < 5; i++ {
			group := group
			id := fmt.Sprintf("id%d", group*5+i)
			port := 8610 + group*5 + i
			bootPeers := []string{bootPeer(bootPeerMap[port])}
			pol := discPolForPeer(port)
			inst := createDiscoveryInstanceWithNoGossipWithDisclosurePolicy(8610+group*5+i, id, bootPeers, pol)
			inst.initiateSync(getAliveExpirationTimeout()/3, 10)
			if group == 0 {
				instances1 = append(instances1, inst)
			} else {
				instances2 = append(instances2, inst)
			}
		}
	}
	return instances1, instances2
}

func discPolForPeer(selfPort int) DisclosurePolicy {
	return func(remotePeer *NetworkMember) (Sieve, EnvelopeFilter) {
		targetPortStr := strings.Split(remotePeer.Endpoint, ":")[1]
		targetPort, _ := strconv.ParseInt(targetPortStr, 10, 64)
		return func(msg *proto.SignedGossipMessage) bool {
				portOfAliveMsgStr := strings.Split(msg.GetAliveMsg().Membership.Endpoint, ":")[1]
				portOfAliveMsg, _ := strconv.ParseInt(portOfAliveMsgStr, 10, 64)

				if portOfAliveMsg < 8615 && targetPort < 8615 {
					return true
				}
				if portOfAliveMsg >= 8615 && targetPort >= 8615 {
					return true
				}

				// Else, expose peers with even ids to other peers with even ids
				return portOfAliveMsg%2 == 0 && targetPort%2 == 0
			}, func(msg *proto.SignedGossipMessage) *proto.Envelope {
				if selfPort < 8615 && targetPort >= 8615 {
					msg.Envelope.SecretEnvelope = nil
				}

				if selfPort >= 8615 && targetPort < 8615 {
					msg.Envelope.SecretEnvelope = nil
				}

				return msg.Envelope
			}
	}
}

func TestConfigFromFile(t *testing.T) {
	preAliveTimeInterval := getAliveTimeInterval()
	preAliveExpirationTimeout := getAliveExpirationTimeout()
	preAliveExpirationCheckInterval := getAliveExpirationCheckInterval()
	preReconnectInterval := getReconnectInterval()

	// Recover the config values in order to avoid impacting other tests
	defer func() {
		SetAliveTimeInterval(preAliveTimeInterval)
		SetAliveExpirationTimeout(preAliveExpirationTimeout)
		SetAliveExpirationCheckInterval(preAliveExpirationCheckInterval)
		SetReconnectInterval(preReconnectInterval)
	}()

	// Verify if using default values when config is missing
	viper.Reset()
	aliveExpirationCheckInterval = 0 * time.Second
	assert.Equal(t, time.Duration(5)*time.Second, getAliveTimeInterval())
	assert.Equal(t, time.Duration(25)*time.Second, getAliveExpirationTimeout())
	assert.Equal(t, time.Duration(25)*time.Second/10, getAliveExpirationCheckInterval())
	assert.Equal(t, time.Duration(25)*time.Second, getReconnectInterval())

	//Verify reading the values from config file
	viper.Reset()
	aliveExpirationCheckInterval = 0 * time.Second
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	viper.AddConfigPath("./../../peer")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(5)*time.Second, getAliveTimeInterval())
	assert.Equal(t, time.Duration(25)*time.Second, getAliveExpirationTimeout())
	assert.Equal(t, time.Duration(25)*time.Second/10, getAliveExpirationCheckInterval())
	assert.Equal(t, time.Duration(25)*time.Second, getReconnectInterval())
}

func TestFilterOutLocalhost(t *testing.T) {
	t.Parallel()
	endpoints := []string{"localhost:5611", "127.0.0.1:5611", "1.2.3.4:5611"}
	assert.Len(t, filterOutLocalhost(endpoints, 5611), 1)
	endpoints = []string{"1.2.3.4:5611"}
	assert.Len(t, filterOutLocalhost(endpoints, 5611), 1)
	endpoints = []string{"localhost:5611", "127.0.0.1:5611"}
	assert.Len(t, filterOutLocalhost(endpoints, 5611), 0)
	// Check slice returned is a copy
	endpoints = []string{"localhost:5611", "127.0.0.1:5611", "1.2.3.4:5611"}
	endpoints2 := filterOutLocalhost(endpoints, 5611)
	endpoints2[0] = "bla bla"
	assert.NotEqual(t, endpoints[2], endpoints[0])
}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	start := time.Now()
	limit := start.UnixNano() + timeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeout / 10)
	}
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
	assert.Fail(t, "Timeout expired!")
}

func stopInstances(t *testing.T, instances []*gossipInstance) {
	stopAction := &sync.WaitGroup{}
	for _, inst := range instances {
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}

	waitUntilOrFailBlocking(t, stopAction.Wait)
}

func assertMembership(t *testing.T, instances []*gossipInstance, expectedNum int) {
	fullMembership := func() bool {
		for _, inst := range instances {
			if len(inst.GetMembership()) == expectedNum {
				return true
			}
		}
		return false
	}
	waitUntilOrFail(t, fullMembership)
}

func portsOfMembers(members []NetworkMember) []int {
	ports := make([]int, len(members))
	for i := range members {
		ports[i] = portOfEndpoint(members[i].Endpoint)
	}
	sort.Ints(ports)
	return ports
}

func portOfEndpoint(endpoint string) int {
	port, _ := strconv.ParseInt(strings.Split(endpoint, ":")[1], 10, 64)
	return int(port)
}
