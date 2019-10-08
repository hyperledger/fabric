/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"bytes"
	"context"
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

	protoG "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip/msgstore"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

var timeout = time.Second * time.Duration(15)

var aliveTimeInterval = time.Duration(time.Millisecond * 300)
var defaultTestConfig = DiscoveryConfig{
	AliveTimeInterval:            aliveTimeInterval,
	AliveExpirationTimeout:       10 * aliveTimeInterval,
	AliveExpirationCheckInterval: aliveTimeInterval,
	ReconnectInterval:            10 * aliveTimeInterval,
}

func init() {
	util.SetupTestLogging()
	maxConnectionAttempts = 10000
}

type dummyReceivedMessage struct {
	msg  *proto.SignedGossipMessage
	info *proto.ConnectionInfo
}

func (*dummyReceivedMessage) Respond(msg *proto.GossipMessage) {
	panic("implement me")
}

func (rm *dummyReceivedMessage) GetGossipMessage() *proto.SignedGossipMessage {
	return rm.msg
}

func (*dummyReceivedMessage) GetSourceEnvelope() *proto.Envelope {
	panic("implement me")
}

func (rm *dummyReceivedMessage) GetConnectionInfo() *proto.ConnectionInfo {
	return rm.info
}

func (*dummyReceivedMessage) Ack(err error) {
	panic("implement me")
}

type dummyCommModule struct {
	validatedMessages chan *proto.SignedGossipMessage
	msgsReceived      uint32
	msgsSent          uint32
	id                string
	identitySwitch    chan common.PKIidType
	presumeDead       chan common.PKIidType
	detectedDead      chan string
	streams           map[string]proto.Gossip_GossipStreamClient
	conns             map[string]*grpc.ClientConn
	lock              *sync.RWMutex
	incMsgs           chan proto.ReceivedMessage
	lastSeqs          map[string]uint64
	shouldGossip      bool
	disableComm       bool
	mock              *mock.Mock
}

type gossipInstance struct {
	msgInterceptor func(*proto.SignedGossipMessage)
	comm           *dummyCommModule
	Discovery
	gRGCserv      *grpc.Server
	lsnr          net.Listener
	shouldGossip  bool
	syncInitiator *time.Ticker
	stopChan      chan struct{}
	port          int
}

func (comm *dummyCommModule) ValidateAliveMsg(am *proto.SignedGossipMessage) bool {
	comm.lock.RLock()
	c := comm.validatedMessages
	comm.lock.RUnlock()

	if c != nil {
		c <- am
	}
	return true
}

func (comm *dummyCommModule) IdentitySwitch() <-chan common.PKIidType {
	return comm.identitySwitch
}

func (comm *dummyCommModule) recordValidation(validatedMessages chan *proto.SignedGossipMessage) {
	comm.lock.Lock()
	defer comm.lock.Unlock()
	comm.validatedMessages = validatedMessages
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
	s, _ := am.NoopSign()
	env := s.Envelope
	env.SignSecret(signer, secret)
	return env
}

func (comm *dummyCommModule) Gossip(msg *proto.SignedGossipMessage) {
	if !comm.shouldGossip || comm.disableComm {
		return
	}
	comm.lock.Lock()
	defer comm.lock.Unlock()
	for _, conn := range comm.streams {
		conn.Send(msg.Envelope)
	}
}

func (comm *dummyCommModule) Forward(msg proto.ReceivedMessage) {
	if !comm.shouldGossip || comm.disableComm {
		return
	}
	comm.lock.Lock()
	defer comm.lock.Unlock()
	for _, conn := range comm.streams {
		conn.Send(msg.GetGossipMessage().Envelope)
	}
}

func (comm *dummyCommModule) SendToPeer(peer *NetworkMember, msg *proto.SignedGossipMessage) {
	if comm.disableComm {
		return
	}
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
	s, _ := msg.NoopSign()
	comm.streams[peer.Endpoint].Send(s.Envelope)
	comm.lock.Unlock()
	atomic.AddUint32(&comm.msgsSent, 1)
}

func (comm *dummyCommModule) Ping(peer *NetworkMember) bool {
	if comm.disableComm {
		return false
	}
	comm.lock.Lock()
	defer comm.lock.Unlock()

	if comm.mock != nil {
		comm.mock.Called()
	}

	_, alreadyExists := comm.streams[peer.Endpoint]
	conn := comm.conns[peer.Endpoint]
	if !alreadyExists || conn.GetState() == connectivity.Shutdown {
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
	if _, err := proto.NewGossipClient(conn).Ping(context.Background(), &proto.Empty{}); err != nil {
		return false
	}
	return true
}

func (comm *dummyCommModule) Accept() <-chan proto.ReceivedMessage {
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

func (g *gossipInstance) receivedMsgCount() int {
	return int(atomic.LoadUint32(&g.comm.msgsReceived))
}

func (g *gossipInstance) sentMsgCount() int {
	return int(atomic.LoadUint32(&g.comm.msgsSent))
}

func (g *gossipInstance) discoveryImpl() *gossipDiscoveryImpl {
	return g.Discovery.(*gossipDiscoveryImpl)
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
		g.msgInterceptor(gMsg)

		lgr.Debug(g.Discovery.Self().Endpoint, "Got message:", gMsg)
		g.comm.incMsgs <- &dummyReceivedMessage{
			msg: gMsg,
			info: &proto.ConnectionInfo{
				ID: common.PKIidType("testID"),
			},
		}
		atomic.AddUint32(&g.comm.msgsReceived, 1)

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
	g.comm.lock.Lock()
	for _, stream := range g.comm.streams {
		stream.CloseSend()
	}
	g.comm.lock.Unlock()
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
	return createDiscoveryInstanceCustomConfig(port, id, bootstrapPeers, defaultTestConfig)
}

func createDiscoveryInstanceCustomConfig(port int, id string, bootstrapPeers []string, config DiscoveryConfig) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, true, noopPolicy, config)
}

func createDiscoveryInstanceWithNoGossip(port int, id string, bootstrapPeers []string) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, false, noopPolicy, defaultTestConfig)
}

func createDiscoveryInstanceWithNoGossipWithDisclosurePolicy(port int, id string, bootstrapPeers []string, pol DisclosurePolicy) *gossipInstance {
	return createDiscoveryInstanceThatGossips(port, id, bootstrapPeers, false, pol, defaultTestConfig)
}

func createDiscoveryInstanceThatGossips(port int, id string, bootstrapPeers []string, shouldGossip bool, pol DisclosurePolicy, config DiscoveryConfig) *gossipInstance {
	return createDiscoveryInstanceThatGossipsWithInterceptors(port, id, bootstrapPeers, shouldGossip, pol, func(_ *proto.SignedGossipMessage) {}, config)
}

func createDiscoveryInstanceThatGossipsWithInterceptors(port int, id string, bootstrapPeers []string, shouldGossip bool, pol DisclosurePolicy, f func(*proto.SignedGossipMessage), config DiscoveryConfig) *gossipInstance {
	comm := &dummyCommModule{
		conns:          make(map[string]*grpc.ClientConn),
		streams:        make(map[string]proto.Gossip_GossipStreamClient),
		incMsgs:        make(chan proto.ReceivedMessage, 1000),
		presumeDead:    make(chan common.PKIidType, 10000),
		id:             id,
		detectedDead:   make(chan string, 10000),
		identitySwitch: make(chan common.PKIidType),
		lock:           &sync.RWMutex{},
		lastSeqs:       make(map[string]uint64),
		shouldGossip:   shouldGossip,
		disableComm:    false,
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

	config.BootstrapPeers = bootstrapPeers
	discSvc := NewDiscoveryService(self, comm, comm, pol, config)
	for _, bootPeer := range bootstrapPeers {
		bp := bootPeer
		discSvc.Connect(NetworkMember{Endpoint: bp, InternalEndpoint: bootPeer}, func() (*PeerIdentification, error) {
			return &PeerIdentification{SelfOrg: true, ID: common.PKIidType(bp)}, nil
		})
	}

	gossInst := &gossipInstance{comm: comm, gRGCserv: s, Discovery: discSvc, lsnr: ll, shouldGossip: shouldGossip, port: port, msgInterceptor: f}

	proto.RegisterGossipServer(s, gossInst)
	go s.Serve(ll)

	return gossInst
}

func bootPeer(port int) string {
	return fmt.Sprintf("localhost:%d", port)
}

func TestHasExternalEndpoints(t *testing.T) {
	memberWithEndpoint := NetworkMember{Endpoint: "foo"}
	memberWithoutEndpoint := NetworkMember{}

	assert.True(t, HasExternalEndpoint(memberWithEndpoint))
	assert.False(t, HasExternalEndpoint(memberWithoutEndpoint))
}

func TestToString(t *testing.T) {
	nm := NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
	}
	assert.Equal(t, "b", nm.PreferredEndpoint())
	nm = NetworkMember{
		Endpoint: "a",
	}
	assert.Equal(t, "a", nm.PreferredEndpoint())

	now := time.Now()
	ts := &timestamp{
		incTime: now,
		seqNum:  uint64(42),
	}
	assert.Equal(t, fmt.Sprintf("%d, %d", now.UnixNano(), 42), fmt.Sprint(ts))
}

func TestNetworkMemberString(t *testing.T) {
	tests := []struct {
		input    NetworkMember
		expected string
	}{
		{
			input:    NetworkMember{Endpoint: "endpoint", InternalEndpoint: "internal-endpoint", PKIid: common.PKIidType{0, 1, 2, 3}, Metadata: nil},
			expected: "Endpoint: endpoint, InternalEndpoint: internal-endpoint, PKI-ID: 00010203, Metadata: ",
		},
		{
			input:    NetworkMember{Endpoint: "endpoint", InternalEndpoint: "internal-endpoint", PKIid: common.PKIidType{0, 1, 2, 3}, Metadata: []byte{4, 5, 6, 7}},
			expected: "Endpoint: endpoint, InternalEndpoint: internal-endpoint, PKI-ID: 00010203, Metadata: 04050607",
		},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.String())
	}
}

func TestBadInput(t *testing.T) {
	inst := createDiscoveryInstance(2048, fmt.Sprintf("d%d", 0), []string{})
	inst.Discovery.(*gossipDiscoveryImpl).handleMsgFromComm(nil)
	s, _ := (&proto.GossipMessage{
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{},
		},
	}).NoopSign()
	inst.Discovery.(*gossipDiscoveryImpl).handleMsgFromComm(&dummyReceivedMessage{
		msg: s,
		info: &proto.ConnectionInfo{
			ID: common.PKIidType("testID"),
		},
	})
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
		inst.Connect(netMember2Connect2, func() (identification *PeerIdentification, err error) {
			return &PeerIdentification{SelfOrg: false, ID: nil}, nil
		})
	}

	time.Sleep(time.Second * 3)
	fullMembership := func() bool {
		return nodeNum-1 == len(instances[nodeNum-1].GetMembership())
	}
	waitUntilOrFail(t, fullMembership)

	discInst := instances[rand.Intn(len(instances))].Discovery.(*gossipDiscoveryImpl)
	mr, _ := discInst.createMembershipRequest(true)
	am, _ := mr.GetMemReq().SelfInformation.ToGossipMessage()
	assert.NotNil(t, am.SecretEnvelope)
	mr2, _ := discInst.createMembershipRequest(false)
	am, _ = mr2.GetMemReq().SelfInformation.ToGossipMessage()
	assert.Nil(t, am.SecretEnvelope)
	stopInstances(t, instances)
	assert.Len(t, firstSentMemReqMsgs, 10)
	close(firstSentMemReqMsgs)
	for firstSentSelfMsg := range firstSentMemReqMsgs {
		assert.Nil(t, firstSentSelfMsg.Envelope.SecretEnvelope)
	}
}

func TestValidation(t *testing.T) {
	t.Parallel()

	// Scenarios: This test contains the following sub-tests:
	// 1) alive message validation: a message is validated <==> it entered the message store
	// 2) request/response message validation:
	//   2.1) alive messages from membership requests/responses are validated.
	//   2.2) once alive messages enter the message store, reception of them via membership responses
	//        doesn't trigger validation, but via membership requests - do.

	wrapReceivedMessage := func(msg *proto.SignedGossipMessage) proto.ReceivedMessage {
		return &dummyReceivedMessage{
			msg: msg,
			info: &proto.ConnectionInfo{
				ID: common.PKIidType("testID"),
			},
		}
	}

	requestMessagesReceived := make(chan *proto.SignedGossipMessage, 100)
	responseMessagesReceived := make(chan *proto.SignedGossipMessage, 100)
	aliveMessagesReceived := make(chan *proto.SignedGossipMessage, 5000)

	var membershipRequest atomic.Value
	var membershipResponseWithAlivePeers atomic.Value
	var membershipResponseWithDeadPeers atomic.Value

	recordMembershipRequest := func(req *proto.SignedGossipMessage) {
		msg, _ := req.GetMemReq().SelfInformation.ToGossipMessage()
		membershipRequest.Store(req)
		requestMessagesReceived <- msg
	}

	recordMembershipResponse := func(res *proto.SignedGossipMessage) {
		memRes := res.GetMemRes()
		if len(memRes.GetAlive()) > 0 {
			membershipResponseWithAlivePeers.Store(res)
		}
		if len(memRes.GetDead()) > 0 {
			membershipResponseWithDeadPeers.Store(res)
		}
		responseMessagesReceived <- res
	}

	interceptor := func(msg *proto.SignedGossipMessage) {
		if memReq := msg.GetMemReq(); memReq != nil {
			recordMembershipRequest(msg)
			return
		}

		if memRes := msg.GetMemRes(); memRes != nil {
			recordMembershipResponse(msg)
			return
		}
		// Else, it's an alive message
		aliveMessagesReceived <- msg
	}

	// p3 is the boot peer of p1, and p1 is the boot peer of p2.
	// p1 sends a (membership) request to p3, and receives a (membership) response back.
	// p2 sends a (membership) request to p1.
	// Therefore, p1 receives both a membership request and a response.
	p1 := createDiscoveryInstanceThatGossipsWithInterceptors(4675, "p1", []string{bootPeer(4677)}, true, noopPolicy, interceptor, defaultTestConfig)
	p2 := createDiscoveryInstance(4676, "p2", []string{bootPeer(4675)})
	p3 := createDiscoveryInstance(4677, "p3", nil)
	instances := []*gossipInstance{p1, p2, p3}

	assertMembership(t, instances, 2)

	instances = []*gossipInstance{p1, p2}
	// Stop p3 and wait until its death is detected
	p3.Stop()
	assertMembership(t, instances, 1)
	// Force p1 to send a membership request so it can receive back a response
	// with dead peers.
	p1.InitiateSync(1)

	// Wait until a response with a dead peer is received
	waitUntilOrFail(t, func() bool {
		return membershipResponseWithDeadPeers.Load() != nil
	})

	p1.Stop()
	p2.Stop()

	close(aliveMessagesReceived)
	t.Log("Recorded", len(aliveMessagesReceived), "alive messages")
	t.Log("Recorded", len(requestMessagesReceived), "request messages")
	t.Log("Recorded", len(responseMessagesReceived), "response messages")

	// Ensure we got alive messages from membership requests and from membership responses
	assert.NotNil(t, membershipResponseWithAlivePeers.Load())
	assert.NotNil(t, membershipRequest.Load())

	t.Run("alive message", func(t *testing.T) {
		t.Parallel()
		// Spawn a new peer - p4
		p4 := createDiscoveryInstance(4678, "p1", nil)
		defer p4.Stop()
		// Record messages validated
		validatedMessages := make(chan *proto.SignedGossipMessage, 5000)
		p4.comm.recordValidation(validatedMessages)
		tmpMsgs := make(chan *proto.SignedGossipMessage, 5000)
		// Replay the messages sent to p1 into p4, and also save them into a temporary channel
		for msg := range aliveMessagesReceived {
			p4.comm.incMsgs <- wrapReceivedMessage(msg)
			tmpMsgs <- msg
		}

		// Simulate the messages received by p4 into the message store
		policy := proto.NewGossipMessageComparator(0)
		msgStore := msgstore.NewMessageStore(policy, func(_ interface{}) {})
		close(tmpMsgs)
		for msg := range tmpMsgs {
			if msgStore.Add(msg) {
				// Ensure the message was verified if it can be added into the message store
				expectedMessage := <-validatedMessages
				assert.Equal(t, expectedMessage, msg)
			}
		}
		// Ensure we didn't validate any other messages.
		assert.Empty(t, validatedMessages)
	})

	req := membershipRequest.Load().(*proto.SignedGossipMessage)
	res := membershipResponseWithDeadPeers.Load().(*proto.SignedGossipMessage)
	// Ensure the membership response contains both alive and dead peers
	assert.Len(t, res.GetMemRes().GetAlive(), 2)
	assert.Len(t, res.GetMemRes().GetDead(), 1)

	for _, testCase := range []struct {
		name                  string
		expectedAliveMessages int
		port                  int
		message               *proto.SignedGossipMessage
		shouldBeReValidated   bool
	}{
		{
			name:                  "membership request",
			expectedAliveMessages: 1,
			message:               req,
			port:                  4679,
			shouldBeReValidated:   true,
		},
		{
			name:                  "membership response",
			expectedAliveMessages: 3,
			message:               res,
			port:                  4680,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			p := createDiscoveryInstance(testCase.port, "p", nil)
			defer p.Stop()
			// Record messages validated
			validatedMessages := make(chan *proto.SignedGossipMessage, testCase.expectedAliveMessages)
			p.comm.recordValidation(validatedMessages)

			p.comm.incMsgs <- wrapReceivedMessage(testCase.message)
			// Ensure all messages were validated
			for i := 0; i < testCase.expectedAliveMessages; i++ {
				validatedMsg := <-validatedMessages
				// send the message directly to be included in the message store
				p.comm.incMsgs <- wrapReceivedMessage(validatedMsg)
			}
			// Wait for the messages to be validated
			for i := 0; i < testCase.expectedAliveMessages; i++ {
				<-validatedMessages
			}
			// Not more than testCase.expectedAliveMessages should have been validated
			assert.Empty(t, validatedMessages)

			if !testCase.shouldBeReValidated {
				// Re-submit the message twice and ensure it wasn't validated.
				// If it is validated, panic would occur because an enqueue to the validatesMessages channel
				// would be attempted and the channel is closed.
				close(validatedMessages)
			}
			p.comm.incMsgs <- wrapReceivedMessage(testCase.message)
			p.comm.incMsgs <- wrapReceivedMessage(testCase.message)
			// Wait until the size of the channel is zero. It means at least one message was processed.
			waitUntilOrFail(t, func() bool {
				return len(p.comm.incMsgs) == 0
			})
		})
	}
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
				time.Sleep(defaultTestConfig.AliveExpirationTimeout / 3)
				inst.InitiateSync(9)
			}
		}()
	}
	time.Sleep(defaultTestConfig.AliveExpirationTimeout * 4)
	assertMembership(t, instances, nodeNum-1)
	atomic.StoreInt32(&toDie, int32(1))
	stopInstances(t, instances)
}

func TestSelf(t *testing.T) {
	t.Parallel()
	inst := createDiscoveryInstance(13463, "d1", []string{})
	defer inst.Stop()
	env := inst.Self().Envelope
	sMsg, err := env.ToGossipMessage()
	assert.NoError(t, err)
	member := sMsg.GetAliveMsg().Membership
	assert.Equal(t, "localhost:13463", member.Endpoint)
	assert.Equal(t, []byte("localhost:13463"), member.PkiId)

	assert.Equal(t, "localhost:13463", inst.Self().Endpoint)
	assert.Equal(t, common.PKIidType("localhost:13463"), inst.Self().PKIid)
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

	assertMembership(t, instances[:len(instances)-2], nodeNum-3)

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
	inst.comm.mock.On("SendToPeer", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
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
	time.Sleep(time.Second * 6)
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
			inst.initiateSync(defaultTestConfig.AliveExpirationTimeout/3, 10)
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
				envelope := protoG.Clone(msg.Envelope).(*proto.Envelope)
				if selfPort < 8615 && targetPort >= 8615 {
					envelope.SecretEnvelope = nil
				}

				if selfPort >= 8615 && targetPort < 8615 {
					envelope.SecretEnvelope = nil
				}

				return envelope
			}
	}
}

func TestExpirationNoSecretEnvelope(t *testing.T) {
	t.Parallel()

	l, err := zap.NewDevelopment()
	assert.NoError(t, err)

	removed := make(chan struct{})
	logger := flogging.NewFabricLogger(l, zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Removing member: Endpoint: foo") {
			removed <- struct{}{}
		}
		return nil
	}))

	msgStore := newAliveMsgStore(&gossipDiscoveryImpl{
		aliveExpirationTimeout: time.Millisecond,
		lock:                   &sync.RWMutex{},
		aliveMembership:        util.NewMembershipStore(),
		deadMembership:         util.NewMembershipStore(),
		logger:                 logger,
	})

	msg := &proto.GossipMessage{
		Content: &proto.GossipMessage_AliveMsg{
			AliveMsg: &proto.AliveMessage{Membership: &proto.Member{
				Endpoint: "foo",
			}},
		},
	}

	sMsg, err := msg.NoopSign()
	assert.NoError(t, err)

	msgStore.Add(sMsg)
	select {
	case <-removed:
	case <-time.After(time.Second * 10):
		t.Fatalf("timed out")
	}
}

func TestCertificateChange(t *testing.T) {
	t.Parallel()

	bootPeers := []string{bootPeer(42611), bootPeer(42612), bootPeer(42613)}
	p1 := createDiscoveryInstance(42611, "d1", bootPeers)
	p2 := createDiscoveryInstance(42612, "d2", bootPeers)
	p3 := createDiscoveryInstance(42613, "d3", bootPeers)

	// Wait for membership establishment
	assertMembership(t, []*gossipInstance{p1, p2, p3}, 2)

	// Shutdown the second peer
	waitUntilOrFailBlocking(t, p2.Stop)

	var pingCountFrom1 uint32
	var pingCountFrom3 uint32
	// Program mocks to increment ping counters
	p1.comm.lock.Lock()
	p1.comm.mock = &mock.Mock{}
	p1.comm.mock.On("SendToPeer", mock.Anything, mock.Anything)
	p1.comm.mock.On("Ping").Run(func(arguments mock.Arguments) {
		atomic.AddUint32(&pingCountFrom1, 1)
	})
	p1.comm.lock.Unlock()

	p3.comm.lock.Lock()
	p3.comm.mock = &mock.Mock{}
	p3.comm.mock.On("SendToPeer", mock.Anything, mock.Anything)
	p3.comm.mock.On("Ping").Run(func(arguments mock.Arguments) {
		atomic.AddUint32(&pingCountFrom3, 1)
	})
	p3.comm.lock.Unlock()

	pingCount1 := func() uint32 {
		return atomic.LoadUint32(&pingCountFrom1)
	}

	pingCount3 := func() uint32 {
		return atomic.LoadUint32(&pingCountFrom3)
	}

	c1 := pingCount1()
	c3 := pingCount3()

	// Ensure the first peer and third peer try to reconnect to it
	waitUntilTimeoutOrFail(t, func() bool {
		return pingCount1() > c1 && pingCount3() > c3
	}, timeout)

	// Tell the first peer that the second peer's PKI-ID has changed
	// So that it will purge it from the membership entirely
	p1.comm.identitySwitch <- common.PKIidType("localhost:42612")

	c1 = pingCount1()
	c3 = pingCount3()
	// Ensure third peer tries to reconnect to it
	waitUntilTimeoutOrFail(t, func() bool {
		return pingCount3() > c3
	}, timeout)

	// Ensure the first peer ceases from trying
	assert.Equal(t, c1, pingCount1())

	waitUntilOrFailBlocking(t, p1.Stop)
	waitUntilOrFailBlocking(t, p3.Stop)
}

func TestMsgStoreExpiration(t *testing.T) {
	// Starts 4 instances, wait for membership to build, stop 2 instances
	// Check that membership in 2 running instances become 2
	// Wait for expiration and check that alive messages and related entities in maps are removed in running instances
	t.Parallel()
	nodeNum := 4
	bootPeers := []string{bootPeer(12611), bootPeer(12612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(12611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(12612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(12610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	assertMembership(t, instances, nodeNum-1)

	waitUntilOrFailBlocking(t, instances[nodeNum-1].Stop)
	waitUntilOrFailBlocking(t, instances[nodeNum-2].Stop)

	assertMembership(t, instances[:len(instances)-2], nodeNum-3)

	checkMessages := func() bool {
		for _, inst := range instances[:len(instances)-2] {
			for _, downInst := range instances[len(instances)-2:] {
				downCastInst := inst.discoveryImpl()
				downCastInst.lock.RLock()
				if _, exist := downCastInst.aliveLastTS[string(downInst.discoveryImpl().self.PKIid)]; exist {
					downCastInst.lock.RUnlock()
					return false
				}
				if _, exist := downCastInst.deadLastTS[string(downInst.discoveryImpl().self.PKIid)]; exist {
					downCastInst.lock.RUnlock()
					return false
				}
				if _, exist := downCastInst.id2Member[string(downInst.discoveryImpl().self.PKIid)]; exist {
					downCastInst.lock.RUnlock()
					return false
				}
				if downCastInst.aliveMembership.MsgByID(downInst.discoveryImpl().self.PKIid) != nil {
					downCastInst.lock.RUnlock()
					return false
				}
				if downCastInst.deadMembership.MsgByID(downInst.discoveryImpl().self.PKIid) != nil {
					downCastInst.lock.RUnlock()
					return false
				}
				for _, am := range downCastInst.msgStore.Get() {
					m := am.(*proto.SignedGossipMessage).GetAliveMsg()
					if bytes.Equal(m.Membership.PkiId, downInst.discoveryImpl().self.PKIid) {
						downCastInst.lock.RUnlock()
						return false
					}
				}
				downCastInst.lock.RUnlock()
			}
		}
		return true
	}

	waitUntilTimeoutOrFail(t, checkMessages, defaultTestConfig.AliveExpirationTimeout*(msgExpirationFactor+5))

	assertMembership(t, instances[:len(instances)-2], nodeNum-3)

	stopInstances(t, instances[:len(instances)-2])
}

func TestMsgStoreExpirationWithMembershipMessages(t *testing.T) {
	// Creates 3 discovery instances without gossip communication
	// Generates MembershipRequest msg for each instance using createMembershipRequest
	// Generates Alive msg for each instance using createAliveMessage
	// Builds membership using Alive msgs
	// Checks msgStore and related maps
	// Generates MembershipResponse msgs for each instance using createMembershipResponse
	// Generates new set of Alive msgs and processes them
	// Checks msgStore and related maps
	// Waits for expiration and checks msgStore and related maps
	// Processes stored MembershipRequest msg and checks msgStore and related maps
	// Processes stored MembershipResponse msg and checks msgStore and related maps

	t.Parallel()
	bootPeers := []string{}
	peersNum := 3
	instances := []*gossipInstance{}
	aliveMsgs := []*proto.SignedGossipMessage{}
	newAliveMsgs := []*proto.SignedGossipMessage{}
	memReqMsgs := []*proto.SignedGossipMessage{}
	memRespMsgs := make(map[int][]*proto.MembershipResponse)

	for i := 0; i < peersNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst := createDiscoveryInstanceWithNoGossip(22610+i, id, bootPeers)
		inst.comm.disableComm = true
		instances = append(instances, inst)
	}

	// Creating MembershipRequest messages
	for i := 0; i < peersNum; i++ {
		memReqMsg, _ := instances[i].discoveryImpl().createMembershipRequest(true)
		sMsg, _ := memReqMsg.NoopSign()
		memReqMsgs = append(memReqMsgs, sMsg)
	}
	// Creating Alive messages
	for i := 0; i < peersNum; i++ {
		aliveMsg, _ := instances[i].discoveryImpl().createSignedAliveMessage(true)
		aliveMsgs = append(aliveMsgs, aliveMsg)
	}

	repeatForFiltered := func(n int, filter func(i int) bool, action func(i int)) {
		for i := 0; i < n; i++ {
			if filter(i) {
				continue
			}
			action(i)
		}
	}

	// Handling Alive
	for i := 0; i < peersNum; i++ {
		for k := 0; k < peersNum; k++ {
			instances[i].discoveryImpl().handleMsgFromComm(&dummyReceivedMessage{
				msg: aliveMsgs[k],
				info: &proto.ConnectionInfo{
					ID: common.PKIidType(fmt.Sprintf("d%d", i)),
				},
			})
		}
	}

	checkExistence := func(instances []*gossipInstance, msgs []*proto.SignedGossipMessage, index int, i int, step string) {
		_, exist := instances[index].discoveryImpl().aliveLastTS[string(instances[i].discoveryImpl().self.PKIid)]
		assert.True(t, exist, fmt.Sprint(step, " Data from alive msg ", i, " doesn't exist in aliveLastTS of discovery inst ", index))

		_, exist = instances[index].discoveryImpl().id2Member[string(string(instances[i].discoveryImpl().self.PKIid))]
		assert.True(t, exist, fmt.Sprint(step, " id2Member mapping doesn't exist for alive msg ", i, " of discovery inst ", index))

		assert.NotNil(t, instances[index].discoveryImpl().aliveMembership.MsgByID(instances[i].discoveryImpl().self.PKIid), fmt.Sprint(step, " Alive msg", i, " not exist in aliveMembership of discovery inst ", index))

		assert.Contains(t, instances[index].discoveryImpl().msgStore.Get(), msgs[i], fmt.Sprint(step, " Alive msg ", i, "not stored in store of discovery inst ", index))
	}

	checkAliveMsgExist := func(instances []*gossipInstance, msgs []*proto.SignedGossipMessage, index int, step string) {
		instances[index].discoveryImpl().lock.RLock()
		defer instances[index].discoveryImpl().lock.RUnlock()
		repeatForFiltered(peersNum,
			func(k int) bool {
				return k == index
			},
			func(k int) {
				checkExistence(instances, msgs, index, k, step)
			})
	}

	// Checking is Alive was processed
	for i := 0; i < peersNum; i++ {
		checkAliveMsgExist(instances, aliveMsgs, i, "[Step 1 - processing aliveMsg]")
	}

	// Creating MembershipResponse while all instances have full membership
	for i := 0; i < peersNum; i++ {
		peerToResponse := &NetworkMember{
			Metadata:         []byte{},
			PKIid:            []byte(fmt.Sprintf("localhost:%d", 22610+i)),
			Endpoint:         fmt.Sprintf("localhost:%d", 22610+i),
			InternalEndpoint: fmt.Sprintf("localhost:%d", 22610+i),
		}
		memRespMsgs[i] = []*proto.MembershipResponse{}
		repeatForFiltered(peersNum,
			func(k int) bool {
				return k == i
			},
			func(k int) {
				aliveMsg, _ := instances[k].discoveryImpl().createSignedAliveMessage(true)
				memResp := instances[k].discoveryImpl().createMembershipResponse(aliveMsg, peerToResponse)
				memRespMsgs[i] = append(memRespMsgs[i], memResp)
			})
	}

	// Re-creating Alive msgs with highest seq_num, to make sure Alive msgs in memReq and memResp are older
	for i := 0; i < peersNum; i++ {
		aliveMsg, _ := instances[i].discoveryImpl().createSignedAliveMessage(true)
		newAliveMsgs = append(newAliveMsgs, aliveMsg)
	}

	// Handling new Alive set
	for i := 0; i < peersNum; i++ {
		for k := 0; k < peersNum; k++ {
			instances[i].discoveryImpl().handleMsgFromComm(&dummyReceivedMessage{
				msg: newAliveMsgs[k],
				info: &proto.ConnectionInfo{
					ID: common.PKIidType(fmt.Sprintf("d%d", i)),
				},
			})
		}
	}

	// Checking is new Alive was processed
	for i := 0; i < peersNum; i++ {
		checkAliveMsgExist(instances, newAliveMsgs, i, "[Step 2 - proccesing aliveMsg]")
	}

	checkAliveMsgNotExist := func(instances []*gossipInstance, msgs []*proto.SignedGossipMessage, index int, step string) {
		instances[index].discoveryImpl().lock.RLock()
		defer instances[index].discoveryImpl().lock.RUnlock()
		assert.Empty(t, instances[index].discoveryImpl().aliveLastTS, fmt.Sprint(step, " Data from alive msg still exists in aliveLastTS of discovery inst ", index))
		assert.Empty(t, instances[index].discoveryImpl().deadLastTS, fmt.Sprint(step, " Data from alive msg still exists in deadLastTS of discovery inst ", index))
		assert.Empty(t, instances[index].discoveryImpl().id2Member, fmt.Sprint(step, " id2Member mapping still still contains data related to Alive msg: discovery inst ", index))
		assert.Empty(t, instances[index].discoveryImpl().msgStore.Get(), fmt.Sprint(step, " Expired Alive msg still stored in store of discovery inst ", index))
		assert.Zero(t, instances[index].discoveryImpl().aliveMembership.Size(), fmt.Sprint(step, " Alive membership list is not empty, discovery instance", index))
		assert.Zero(t, instances[index].discoveryImpl().deadMembership.Size(), fmt.Sprint(step, " Dead membership list is not empty, discovery instance", index))
	}

	// Sleep until expire
	time.Sleep(defaultTestConfig.AliveExpirationTimeout * (msgExpirationFactor + 5))

	// Checking Alive expired
	for i := 0; i < peersNum; i++ {
		checkAliveMsgNotExist(instances, newAliveMsgs, i, "[Step 3 - expiration in msg store]")
	}

	// Processing old MembershipRequest
	for i := 0; i < peersNum; i++ {
		repeatForFiltered(peersNum,
			func(k int) bool {
				return k == i
			},
			func(k int) {
				instances[i].discoveryImpl().handleMsgFromComm(&dummyReceivedMessage{
					msg: memReqMsgs[k],
					info: &proto.ConnectionInfo{
						ID: common.PKIidType(fmt.Sprintf("d%d", i)),
					},
				})
			})
	}

	// MembershipRequest processing didn't change anything
	for i := 0; i < peersNum; i++ {
		checkAliveMsgNotExist(instances, aliveMsgs, i, "[Step 4 - memReq processing after expiration]")
	}

	// Processing old (later) Alive messages
	for i := 0; i < peersNum; i++ {
		for k := 0; k < peersNum; k++ {
			instances[i].discoveryImpl().handleMsgFromComm(&dummyReceivedMessage{
				msg: aliveMsgs[k],
				info: &proto.ConnectionInfo{
					ID: common.PKIidType(fmt.Sprintf("d%d", i)),
				},
			})
		}
	}

	// Alive msg processing didn't change anything
	for i := 0; i < peersNum; i++ {
		checkAliveMsgNotExist(instances, aliveMsgs, i, "[Step 5.1 - after lost old aliveMsg process]")
		checkAliveMsgNotExist(instances, newAliveMsgs, i, "[Step 5.2 - after lost new aliveMsg process]")
	}

	// Handling old MembershipResponse messages
	for i := 0; i < peersNum; i++ {
		respForPeer := memRespMsgs[i]
		for _, msg := range respForPeer {
			sMsg, _ := (&proto.GossipMessage{
				Tag:   proto.GossipMessage_EMPTY,
				Nonce: uint64(0),
				Content: &proto.GossipMessage_MemRes{
					MemRes: msg,
				},
			}).NoopSign()
			instances[i].discoveryImpl().handleMsgFromComm(&dummyReceivedMessage{
				msg: sMsg,
				info: &proto.ConnectionInfo{
					ID: common.PKIidType(fmt.Sprintf("d%d", i)),
				},
			})
		}
	}

	// MembershipResponse msg processing didn't change anything
	for i := 0; i < peersNum; i++ {
		checkAliveMsgNotExist(instances, aliveMsgs, i, "[Step 6 - after lost MembershipResp process]")
	}

	for i := 0; i < peersNum; i++ {
		instances[i].Stop()
	}

}

func TestAliveMsgStore(t *testing.T) {
	t.Parallel()

	bootPeers := []string{}
	peersNum := 2
	instances := []*gossipInstance{}
	aliveMsgs := []*proto.SignedGossipMessage{}
	memReqMsgs := []*proto.SignedGossipMessage{}

	for i := 0; i < peersNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst := createDiscoveryInstanceWithNoGossip(32610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	// Creating MembershipRequest messages
	for i := 0; i < peersNum; i++ {
		memReqMsg, _ := instances[i].discoveryImpl().createMembershipRequest(true)
		sMsg, _ := memReqMsg.NoopSign()
		memReqMsgs = append(memReqMsgs, sMsg)
	}
	// Creating Alive messages
	for i := 0; i < peersNum; i++ {
		aliveMsg, _ := instances[i].discoveryImpl().createSignedAliveMessage(true)
		aliveMsgs = append(aliveMsgs, aliveMsg)
	}

	//Check new alive msgs
	for _, msg := range aliveMsgs {
		assert.True(t, instances[0].discoveryImpl().msgStore.CheckValid(msg), "aliveMsgStore CheckValid returns false on new AliveMsg")
	}

	// Add new alive msgs
	for _, msg := range aliveMsgs {
		assert.True(t, instances[0].discoveryImpl().msgStore.Add(msg), "aliveMsgStore Add returns false on new AliveMsg")
	}

	// Check exist alive msgs
	for _, msg := range aliveMsgs {
		assert.False(t, instances[0].discoveryImpl().msgStore.CheckValid(msg), "aliveMsgStore CheckValid returns true on existing AliveMsg")
	}

	// Check non-alive msgs
	for _, msg := range memReqMsgs {
		assert.Panics(t, func() { instances[1].discoveryImpl().msgStore.CheckValid(msg) }, "aliveMsgStore CheckValid should panic on new MembershipRequest msg")
		assert.Panics(t, func() { instances[1].discoveryImpl().msgStore.Add(msg) }, "aliveMsgStore Add should panic on new MembershipRequest msg")
	}
}

func TestMemRespDisclosurePol(t *testing.T) {
	t.Parallel()
	pol := func(remotePeer *NetworkMember) (Sieve, EnvelopeFilter) {
		return func(_ *proto.SignedGossipMessage) bool {
				return remotePeer.Endpoint == "localhost:7880"
			}, func(m *proto.SignedGossipMessage) *proto.Envelope {
				return m.Envelope
			}
	}
	d1 := createDiscoveryInstanceThatGossips(7878, "d1", []string{}, true, pol, defaultTestConfig)
	defer d1.Stop()
	d2 := createDiscoveryInstanceThatGossips(7879, "d2", []string{"localhost:7878"}, true, noopPolicy, defaultTestConfig)
	defer d2.Stop()
	d3 := createDiscoveryInstanceThatGossips(7880, "d3", []string{"localhost:7878"}, true, noopPolicy, defaultTestConfig)
	defer d3.Stop()
	// Both d1 and d3 know each other, and also about d2
	assertMembership(t, []*gossipInstance{d1, d3}, 2)
	// d2 doesn't know about any one because the bootstrap peer is ignoring it due to custom policy
	assertMembership(t, []*gossipInstance{d2}, 0)
	assert.Zero(t, d2.receivedMsgCount())
	assert.NotZero(t, d2.sentMsgCount())
}

func TestMembersByID(t *testing.T) {
	members := Members{
		{PKIid: common.PKIidType("p0"), Endpoint: "p0"},
		{PKIid: common.PKIidType("p1"), Endpoint: "p1"},
	}
	byID := members.ByID()
	assert.Len(t, byID, 2)
	assert.Equal(t, "p0", byID["p0"].Endpoint)
	assert.Equal(t, "p1", byID["p1"].Endpoint)
}

func TestFilter(t *testing.T) {
	members := Members{
		{PKIid: common.PKIidType("p0"), Endpoint: "p0", Properties: &proto.Properties{
			Chaincodes: []*proto.Chaincode{{Name: "cc", Version: "1.0"}},
		}},
		{PKIid: common.PKIidType("p1"), Endpoint: "p1", Properties: &proto.Properties{
			Chaincodes: []*proto.Chaincode{{Name: "cc", Version: "2.0"}},
		}},
	}
	res := members.Filter(func(member NetworkMember) bool {
		cc := member.Properties.Chaincodes[0]
		return cc.Version == "2.0" && cc.Name == "cc"
	})
	assert.Equal(t, Members{members[1]}, res)
}

func TestMap(t *testing.T) {
	members := Members{
		{PKIid: common.PKIidType("p0"), Endpoint: "p0"},
		{PKIid: common.PKIidType("p1"), Endpoint: "p1"},
	}
	expectedMembers := Members{
		{PKIid: common.PKIidType("p0"), Endpoint: "p0", Properties: &proto.Properties{LedgerHeight: 2}},
		{PKIid: common.PKIidType("p1"), Endpoint: "p1", Properties: &proto.Properties{LedgerHeight: 2}},
	}

	addProperty := func(member NetworkMember) NetworkMember {
		member.Properties = &proto.Properties{
			LedgerHeight: 2,
		}
		return member
	}

	assert.Equal(t, expectedMembers, members.Map(addProperty))
	// Ensure original members didn't change
	assert.Nil(t, members[0].Properties)
	assert.Nil(t, members[1].Properties)
}

func TestMembersIntersect(t *testing.T) {
	members1 := Members{
		{PKIid: common.PKIidType("p0"), Endpoint: "p0"},
		{PKIid: common.PKIidType("p1"), Endpoint: "p1"},
	}
	members2 := Members{
		{PKIid: common.PKIidType("p1"), Endpoint: "p1"},
		{PKIid: common.PKIidType("p2"), Endpoint: "p2"},
	}
	assert.Equal(t, Members{{PKIid: common.PKIidType("p1"), Endpoint: "p1"}}, members1.Intersect(members2))
}

func TestPeerIsolation(t *testing.T) {
	t.Parallel()

	// Scenario:
	// Start 3 peers (peer0, peer1, peer2). Set peer1 as the bootstrap peer for all.
	// Stop peer0 and peer1 for a while, start them again and test if peer2 still gets full membership

	config := defaultTestConfig
	// Use a smaller AliveExpirationTimeout than the default to reduce the running time of the test.
	config.AliveExpirationTimeout = 2 * config.AliveTimeInterval

	peersNum := 3
	bootPeers := []string{bootPeer(7121)}
	instances := []*gossipInstance{}
	var inst *gossipInstance

	// Start all peers and wait for full membership
	for i := 0; i < peersNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstanceCustomConfig(7120+i, id, bootPeers, config)
		instances = append(instances, inst)
	}
	assertMembership(t, instances, peersNum-1)

	// Stop the first 2 peers so the third peer would stay alone
	stopInstances(t, instances[:peersNum-1])
	assertMembership(t, instances[peersNum-1:], 0)

	// Sleep the same amount of time as it takes to remove a message from the aliveMsgStore (aliveMsgTTL)
	// Add a second as buffer
	time.Sleep(config.AliveExpirationTimeout*msgExpirationFactor + time.Second)

	// Start again the first 2 peers and wait for all the peers to get full membership.
	// Especially, we want to test that peer2 won't be isolated
	for i := 0; i < peersNum-1; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstanceCustomConfig(7120+i, id, bootPeers, config)
		instances[i] = inst
	}
	assertMembership(t, instances, peersNum-1)
}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	waitUntilTimeoutOrFail(t, pred, timeout)
}

func waitUntilTimeoutOrFail(t *testing.T, pred func() bool, timeout time.Duration) {
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
	wg := sync.WaitGroup{}
	wg.Add(len(instances))

	ctx, cancelation := context.WithTimeout(context.Background(), timeout)
	defer cancelation()

	for _, inst := range instances {
		go func(ctx context.Context, i *gossipInstance) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(timeout / 10):
					if len(i.GetMembership()) == expectedNum {
						return
					}
				}
			}
		}(ctx, inst)
	}

	wg.Wait()
	assert.NoError(t, ctx.Err(), "Timeout expired!")
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
