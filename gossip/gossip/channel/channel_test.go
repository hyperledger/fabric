/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gproto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

type msgMutator func(message *proto.Envelope)

var conf = Config{
	ID: "test",
	PublishStateInfoInterval:    time.Millisecond * 100,
	MaxBlockCountToStore:        100,
	PullPeerNum:                 3,
	PullInterval:                time.Second,
	RequestStateInfoInterval:    time.Millisecond * 100,
	BlockExpirationInterval:     time.Second * 6,
	StateInfoCacheSweepInterval: time.Second,
}

func init() {
	util.SetupTestLogging()
	shortenedWaitTime := time.Millisecond * 300
	algo.SetDigestWaitTime(shortenedWaitTime / 2)
	algo.SetRequestWaitTime(shortenedWaitTime)
	algo.SetResponseWaitTime(shortenedWaitTime)
	factory.InitFactories(nil)
}

var (
	// Organizations: {ORG1, ORG2}
	// Channel A: {ORG1}
	channelA                  = common.ChainID("A")
	orgInChannelA             = api.OrgIdentityType("ORG1")
	orgNotInChannelA          = api.OrgIdentityType("ORG2")
	pkiIDInOrg1               = common.PKIidType("pkiIDInOrg1")
	pkiIDnilOrg               = common.PKIidType("pkIDnilOrg")
	pkiIDInOrg1ButNotEligible = common.PKIidType("pkiIDInOrg1ButNotEligible")
	pkiIDinOrg2               = common.PKIidType("pkiIDinOrg2")
)

type joinChanMsg struct {
	getTS               func() time.Time
	members2AnchorPeers map[string][]api.AnchorPeer
}

// SequenceNumber returns the sequence number of the block
// this joinChanMsg was derived from.
// I use timestamps here just for the test.
func (jcm *joinChanMsg) SequenceNumber() uint64 {
	if jcm.getTS != nil {
		return uint64(jcm.getTS().UnixNano())
	}
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

type cryptoService struct {
	mocked bool
	mock.Mock
}

func (cs *cryptoService) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (cs *cryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	panic("Should not be called in this test")
}

func (cs *cryptoService) VerifyByChannel(channel common.ChainID, identity api.PeerIdentityType, _, _ []byte) error {
	if !cs.mocked {
		return nil
	}
	args := cs.Called(identity)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (cs *cryptoService) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	args := cs.Called(signedBlock)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (cs *cryptoService) Sign(msg []byte) ([]byte, error) {
	panic("Should not be called in this test")
}

func (cs *cryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	panic("Should not be called in this test")
}

func (cs *cryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	panic("Should not be called in this test")
}

type receivedMsg struct {
	PKIID common.PKIidType
	msg   *proto.SignedGossipMessage
	mock.Mock
}

// GetSourceEnvelope Returns the Envelope the ReceivedMessage was
// constructed with
func (m *receivedMsg) GetSourceEnvelope() *proto.Envelope {
	return m.msg.Envelope
}

func (m *receivedMsg) GetGossipMessage() *proto.SignedGossipMessage {
	return m.msg
}

func (m *receivedMsg) Respond(msg *proto.GossipMessage) {
	m.Called(msg)
}

// Ack returns to the sender an acknowledgement for the message
func (m *receivedMsg) Ack(err error) {

}

func (m *receivedMsg) GetConnectionInfo() *proto.ConnectionInfo {
	return &proto.ConnectionInfo{
		ID: m.PKIID,
	}
}

type gossipAdapterMock struct {
	mock.Mock
}

func (ga *gossipAdapterMock) Sign(msg *proto.GossipMessage) (*proto.SignedGossipMessage, error) {
	return msg.NoopSign()
}

func (ga *gossipAdapterMock) GetConf() Config {
	args := ga.Called()
	return args.Get(0).(Config)
}

func (ga *gossipAdapterMock) Gossip(msg *proto.SignedGossipMessage) {
	ga.Called(msg)
}

func (ga *gossipAdapterMock) Forward(msg proto.ReceivedMessage) {
	ga.Called(msg)
}

func (ga *gossipAdapterMock) DeMultiplex(msg interface{}) {
	ga.Called(msg)
}

func (ga *gossipAdapterMock) GetMembership() []discovery.NetworkMember {
	args := ga.Called()
	members := args.Get(0).([]discovery.NetworkMember)
	return members
}

// Lookup returns a network member, or nil if not found
func (ga *gossipAdapterMock) Lookup(PKIID common.PKIidType) *discovery.NetworkMember {
	// Ensure we have configured Lookup prior
	if !ga.wasMocked("Lookup") {
		return &discovery.NetworkMember{}
	}
	args := ga.Called(PKIID)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*discovery.NetworkMember)
}

func (ga *gossipAdapterMock) Send(msg *proto.SignedGossipMessage, peers ...*comm.RemotePeer) {
	// Ensure we have configured Send prior
	if !ga.wasMocked("Send") {
		return
	}
	ga.Called(msg, peers)
}

func (ga *gossipAdapterMock) ValidateStateInfoMessage(msg *proto.SignedGossipMessage) error {
	args := ga.Called(msg)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (ga *gossipAdapterMock) GetOrgOfPeer(PKIIID common.PKIidType) api.OrgIdentityType {
	args := ga.Called(PKIIID)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(api.OrgIdentityType)
}

func (ga *gossipAdapterMock) GetIdentityByPKIID(pkiID common.PKIidType) api.PeerIdentityType {
	if ga.wasMocked("GetIdentityByPKIID") {
		return ga.Called(pkiID).Get(0).(api.PeerIdentityType)
	}
	return api.PeerIdentityType(pkiID)
}

func (ga *gossipAdapterMock) wasMocked(methodName string) bool {
	// The following On call is just to synchronize the ExpectedCalls
	// access with 'On' calls from the test goroutine
	ga.On("bla", mock.Anything)
	for _, ec := range ga.ExpectedCalls {
		if ec.Method == methodName {
			return true
		}
	}
	return false
}

func configureAdapter(adapter *gossipAdapterMock, members ...discovery.NetworkMember) {
	adapter.On("GetConf").Return(conf)
	adapter.On("GetMembership").Return(members)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1ButNotEligible).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDinOrg2).Return(orgNotInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDnilOrg).Return(nil)
	adapter.On("GetOrgOfPeer", mock.Anything).Return(api.OrgIdentityType(nil))
}

func TestBadInput(t *testing.T) {
	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{}).(*gossipChannel)
	assert.False(t, gc.verifyMsg(nil))
	assert.False(t, gc.verifyMsg(&receivedMsg{msg: nil, PKIID: nil}))
}

func TestSelf(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	pkiID1 := common.PKIidType("1")
	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgInChannelA): {},
		},
	}
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Gossip", mock.Anything)
	gc := NewGossipChannel(pkiID1, orgInChannelA, cs, channelA, adapter, jcm)
	gc.UpdateLedgerHeight(1)
	gMsg := gc.Self().GossipMessage
	env := gc.Self().Envelope
	sMsg, _ := env.ToGossipMessage()
	assert.True(t, gproto.Equal(gMsg, sMsg.GossipMessage))
	assert.Equal(t, gMsg.GetStateInfo().Properties.LedgerHeight, uint64(1))
	assert.Equal(t, gMsg.GetStateInfo().PkiId, []byte("1"))
}

func TestMsgStoreNotExpire(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}

	pkiID1 := common.PKIidType("1")
	pkiID2 := common.PKIidType("2")
	pkiID3 := common.PKIidType("3")

	peer1 := discovery.NetworkMember{PKIid: pkiID2, InternalEndpoint: "1", Endpoint: "1"}
	peer2 := discovery.NetworkMember{PKIid: pkiID2, InternalEndpoint: "2", Endpoint: "2"}
	peer3 := discovery.NetworkMember{PKIid: pkiID3, InternalEndpoint: "3", Endpoint: "3"}

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgInChannelA): {},
		},
	}

	adapter := new(gossipAdapterMock)
	adapter.On("GetOrgOfPeer", pkiID1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiID2).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiID3).Return(orgInChannelA)

	adapter.On("ValidateStateInfoMessage", mock.Anything).Return(nil)
	adapter.On("GetMembership").Return([]discovery.NetworkMember{peer2, peer3})
	adapter.On("DeMultiplex", mock.Anything)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("GetConf").Return(conf)

	gc := NewGossipChannel(pkiID1, orgInChannelA, cs, channelA, adapter, jcm)
	gc.UpdateLedgerHeight(1)
	// Receive StateInfo messages from other peers
	gc.HandleMessage(&receivedMsg{PKIID: pkiID2, msg: createStateInfoMsg(1, pkiID2, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID3, msg: createStateInfoMsg(1, pkiID3, channelA)})

	simulateStateInfoRequest := func(pkiID []byte, outChan chan *proto.SignedGossipMessage) {
		sentMessages := make(chan *proto.GossipMessage, 1)
		// Ensure we respond to stateInfoSnapshot requests with valid MAC
		s, _ := (&proto.GossipMessage{
			Tag: proto.GossipMessage_CHAN_OR_ORG,
			Content: &proto.GossipMessage_StateInfoPullReq{
				StateInfoPullReq: &proto.StateInfoPullRequest{
					Channel_MAC: GenerateMAC(pkiID, channelA),
				},
			},
		}).NoopSign()
		snapshotReq := &receivedMsg{
			PKIID: pkiID,
			msg:   s,
		}
		snapshotReq.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
			sentMessages <- args.Get(0).(*proto.GossipMessage)
		})

		go gc.HandleMessage(snapshotReq)
		select {
		case <-time.After(time.Second):
			t.Fatal("Haven't received a state info snapshot on time")
		case msg := <-sentMessages:
			for _, el := range msg.GetStateSnapshot().Elements {
				sMsg, err := el.ToGossipMessage()
				assert.NoError(t, err)
				outChan <- sMsg
			}
		}
	}

	c := make(chan *proto.SignedGossipMessage, 3)
	simulateStateInfoRequest(pkiID2, c)
	assert.Len(t, c, 3)

	c = make(chan *proto.SignedGossipMessage, 3)
	simulateStateInfoRequest(pkiID3, c)
	assert.Len(t, c, 3)

	// Now simulate an expiration of peer 3 in the membership view
	adapter.On("Lookup", pkiID1).Return(&peer1)
	adapter.On("Lookup", pkiID2).Return(&peer2)
	adapter.On("Lookup", pkiID3).Return(nil)
	// Ensure that we got at least 1 sweep before continuing
	// the test
	time.Sleep(conf.StateInfoCacheSweepInterval * 2)

	c = make(chan *proto.SignedGossipMessage, 3)
	simulateStateInfoRequest(pkiID2, c)
	assert.Len(t, c, 2)

	c = make(chan *proto.SignedGossipMessage, 3)
	simulateStateInfoRequest(pkiID3, c)
	assert.Len(t, c, 2)
}

func TestLeaveChannel(t *testing.T) {
	// Scenario: Have our peer receive a stateInfo message
	// from a peer that has left the channel, and ensure that it skips it
	// when returning membership.
	// Next, have our own peer leave the channel and ensure:
	// 1) It doesn't return any members of the channel when queried
	// 2) It doesn't send anymore pull for blocks
	// 3) When asked for pull for blocks, it ignores the request
	t.Parallel()

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			"ORG1": {},
			"ORG2": {},
		},
	}

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	members := []discovery.NetworkMember{
		{PKIid: pkiIDInOrg1},
		{PKIid: pkiIDinOrg2},
	}
	var helloPullWG sync.WaitGroup
	helloPullWG.Add(1)
	configureAdapter(adapter, members...)
	gc := NewGossipChannel(common.PKIidType("p0"), orgInChannelA, cs, channelA, adapter, jcm)
	adapter.On("Send", mock.Anything, mock.Anything).Run(func(arguments mock.Arguments) {
		msg := arguments.Get(0).(*proto.SignedGossipMessage)
		if msg.IsPullMsg() {
			helloPullWG.Done()
			assert.False(t, gc.(*gossipChannel).hasLeftChannel())
		}
	})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDinOrg2, msg: createStateInfoMsg(1, pkiIDinOrg2, channelA)})
	// Have some peer send a block to us, so we can send some peer a digest when hello is sent to us
	gc.HandleMessage(&receivedMsg{msg: createDataMsg(2, channelA), PKIID: pkiIDInOrg1})
	assert.Len(t, gc.GetPeers(), 2)
	// Now, have peer in org2 "leave the channel" by publishing is an update
	stateInfoMsg := &receivedMsg{PKIID: pkiIDinOrg2, msg: createStateInfoMsg(0, pkiIDinOrg2, channelA)}
	stateInfoMsg.GetGossipMessage().GetStateInfo().Properties.LeftChannel = true
	gc.HandleMessage(stateInfoMsg)
	assert.Len(t, gc.GetPeers(), 1)
	// Ensure peer in org1 remained and peer in org2 is skipped
	assert.Equal(t, pkiIDInOrg1, gc.GetPeers()[0].PKIid)
	var digestSendTime int32
	var DigestSentWg sync.WaitGroup
	DigestSentWg.Add(1)
	hello := createHelloMsg(pkiIDInOrg1)
	hello.On("Respond", mock.Anything).Run(func(arguments mock.Arguments) {
		atomic.AddInt32(&digestSendTime, 1)
		// Ensure we only respond with digest before we leave the channel
		assert.Equal(t, int32(1), atomic.LoadInt32(&digestSendTime))
		DigestSentWg.Done()
	})
	// Wait until we send a hello pull message
	helloPullWG.Wait()
	go gc.HandleMessage(hello)
	DigestSentWg.Wait()
	// Make the peer leave the channel
	gc.LeaveChannel()
	// Send another hello. Shouldn't respond
	go gc.HandleMessage(hello)
	// Ensure it doesn't know now any other peer
	assert.Len(t, gc.GetPeers(), 0)
	// Sleep 3 times the pull interval.
	// we're not supposed to send a pull during this time.
	time.Sleep(conf.PullInterval * 3)

}

func TestChannelPeriodicalPublishStateInfo(t *testing.T) {
	t.Parallel()
	ledgerHeight := 5
	receivedMsg := int32(0)
	stateInfoReceptionChan := make(chan *proto.SignedGossipMessage, 1)

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)

	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("Gossip", mock.Anything).Run(func(arg mock.Arguments) {
		if atomic.LoadInt32(&receivedMsg) == int32(1) {
			return
		}

		atomic.StoreInt32(&receivedMsg, int32(1))
		msg := arg.Get(0).(*proto.SignedGossipMessage)
		stateInfoReceptionChan <- msg
	})

	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.UpdateLedgerHeight(uint64(ledgerHeight))
	defer gc.Stop()

	var msg *proto.SignedGossipMessage
	select {
	case <-time.After(time.Second * 5):
		t.Fatal("Haven't sent stateInfo on time")
	case m := <-stateInfoReceptionChan:
		msg = m
	}

	assert.Equal(t, ledgerHeight, int(msg.GetStateInfo().Properties.LedgerHeight))
}

func TestChannelMsgStoreEviction(t *testing.T) {
	t.Parallel()
	// Scenario: Create 4 phases in which the pull mediator of the channel would receive blocks
	// via pull.
	// The total amount of blocks should be restricted by the capacity of the message store.
	// After the pull phases end, we ensure that only the latest blocks are preserved in the pull
	// mediator, and the old blocks were evicted.
	// We test this by sending a hello message to the pull mediator and inspecting the digest message
	// returned as a response.

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything).Run(func(arg mock.Arguments) {
	})

	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	defer gc.Stop()
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(100, pkiIDInOrg1, channelA)})

	var wg sync.WaitGroup

	msgsPerPhase := uint64(50)
	lastPullPhase := make(chan uint64, msgsPerPhase)
	totalPhases := uint64(4)
	phaseNum := uint64(0)
	wg.Add(int(totalPhases))

	adapter.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		msg := args.Get(0).(*proto.SignedGossipMessage)
		// Ignore all other messages sent like StateInfo messages
		if !msg.IsPullMsg() {
			return
		}
		// Stop the pull when we reach the final phase
		if atomic.LoadUint64(&phaseNum) == totalPhases && msg.IsHelloMsg() {
			return
		}

		start := atomic.LoadUint64(&phaseNum) * msgsPerPhase
		end := start + msgsPerPhase
		if msg.IsHelloMsg() {
			// Advance phase
			atomic.AddUint64(&phaseNum, uint64(1))
		}

		// Create and execute the current phase of pull
		currSeq := sequence(start, end)
		pullPhase := simulatePullPhase(gc, t, &wg, func(envelope *proto.Envelope) {}, currSeq...)
		pullPhase(args)

		// If we finished the last phase, save the sequence to be used later for inspection
		if msg.IsDataReq() && atomic.LoadUint64(&phaseNum) == totalPhases {
			for _, seq := range currSeq {
				lastPullPhase <- seq
			}
			close(lastPullPhase)
		}
	})
	// Wait for all pull phases to end
	wg.Wait()

	msgSentFromPullMediator := make(chan *proto.GossipMessage, 1)

	helloMsg := createHelloMsg(pkiIDInOrg1)
	helloMsg.On("Respond", mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		if !msg.IsDigestMsg() {
			return
		}
		msgSentFromPullMediator <- msg
	})
	gc.HandleMessage(helloMsg)
	select {
	case msg := <-msgSentFromPullMediator:
		// This is just to check that we responded with a digest on time.
		// Put message back into the channel for further inspection
		msgSentFromPullMediator <- msg
	case <-time.After(time.Second * 5):
		t.Fatal("Didn't reply with a digest on time")
	}
	// Only 1 digest sent
	assert.Len(t, msgSentFromPullMediator, 1)
	msg := <-msgSentFromPullMediator
	// It's a digest and not anything else, like an update
	assert.True(t, msg.IsDigestMsg())
	assert.Len(t, msg.GetDataDig().Digests, adapter.GetConf().MaxBlockCountToStore+1)
	// Check that the last sequences are kept.
	// Since we checked the length, it proves that the old blocks were discarded, since we had much more
	// total blocks overall than our capacity
	for seq := range lastPullPhase {
		assert.Contains(t, msg.GetDataDig().Digests, []byte(fmt.Sprintf("%d", seq)))
	}
}

func TestChannelPull(t *testing.T) {
	t.Parallel()
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	receivedBlocksChan := make(chan *proto.SignedGossipMessage, 2)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.SignedGossipMessage)
		if !msg.IsDataMsg() {
			return
		}
		// The peer is supposed to de-multiplex 2 ledger blocks
		assert.True(t, msg.IsDataMsg())
		receivedBlocksChan <- msg
	})
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	go gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(100, pkiIDInOrg1, channelA)})

	var wg sync.WaitGroup
	wg.Add(1)
	pullPhase := simulatePullPhase(gc, t, &wg, func(envelope *proto.Envelope) {}, 10, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase)

	wg.Wait()
	for expectedSeq := 10; expectedSeq <= 11; expectedSeq++ {
		select {
		case <-time.After(time.Second * 5):
			t.Fatal("Haven't received blocks on time")
		case msg := <-receivedBlocksChan:
			assert.Equal(t, uint64(expectedSeq), msg.GetDataMsg().Payload.SeqNum)
		}
	}
}

func TestChannelPullAccessControl(t *testing.T) {
	t.Parallel()
	// Scenario: We have 2 organizations in the channel: ORG1, ORG2
	// The "acting peer" is from ORG1 and peers "1", "2", "3" are from
	// the following organizations:
	// ORG1: "1"
	// ORG2: "2", "3"
	// We test 2 cases:
	// 1) We don't respond for Hello messages from peers in foreign organizations
	// 2) We don't select peers from foreign organizations when doing pull

	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	cs.Mock = mock.Mock{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)

	pkiID1 := common.PKIidType("1")
	pkiID2 := common.PKIidType("2")
	pkiID3 := common.PKIidType("3")

	peer1 := discovery.NetworkMember{PKIid: pkiID1, InternalEndpoint: "1", Endpoint: "1"}
	peer2 := discovery.NetworkMember{PKIid: pkiID2, InternalEndpoint: "2", Endpoint: "2"}
	peer3 := discovery.NetworkMember{PKIid: pkiID3, InternalEndpoint: "3", Endpoint: "3"}

	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(api.OrgIdentityType("ORG1"))
	adapter.On("GetOrgOfPeer", pkiID1).Return(api.OrgIdentityType("ORG1"))
	adapter.On("GetOrgOfPeer", pkiID2).Return(api.OrgIdentityType("ORG2"))
	adapter.On("GetOrgOfPeer", pkiID3).Return(api.OrgIdentityType("ORG2"))

	adapter.On("GetMembership").Return([]discovery.NetworkMember{peer1, peer2, peer3})
	adapter.On("DeMultiplex", mock.Anything)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("GetConf").Return(conf)

	sentHello := int32(0)
	adapter.On("Send", mock.Anything, mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.SignedGossipMessage)
		if !msg.IsHelloMsg() {
			return
		}
		atomic.StoreInt32(&sentHello, int32(1))
		peerID := string(arg.Get(1).([]*comm.RemotePeer)[0].PKIID)
		assert.Equal(t, "1", peerID)
		assert.NotEqual(t, "2", peerID, "Sent hello to peer 2 but it's in a different org")
		assert.NotEqual(t, "3", peerID, "Sent hello to peer 3 but it's in a different org")
	})

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			"ORG1": {},
			"ORG2": {},
		},
	}
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, jcm)
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(100, pkiIDInOrg1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID1, msg: createStateInfoMsg(100, pkiID1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID2, msg: createStateInfoMsg(100, pkiID2, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID3, msg: createStateInfoMsg(100, pkiID3, channelA)})

	respondedChan := make(chan *proto.GossipMessage, 1)
	messageRelayer := func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		respondedChan <- msg
	}

	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(5, channelA), PKIID: pkiIDInOrg1})

	helloMsg := createHelloMsg(pkiID1)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	go gc.HandleMessage(helloMsg)
	select {
	case <-respondedChan:
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't reply to a hello within a timely manner")
	}

	helloMsg = createHelloMsg(pkiID2)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	go gc.HandleMessage(helloMsg)
	select {
	case <-respondedChan:
		assert.Fail(t, "Shouldn't have replied to a hello, because the peer is from a foreign org")
	case <-time.After(time.Second):
	}

	// Sleep a bit to let the gossip channel send out its hello messages
	time.Sleep(time.Second * 3)
	// Make sure we sent at least 1 hello message, otherwise the test passed vacuously
	assert.Equal(t, int32(1), atomic.LoadInt32(&sentHello))
}

func TestChannelPeerNotInChannel(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	gossipMessagesSentFromChannel := make(chan *proto.GossipMessage, 1)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})

	// First thing, we test that blocks can only be received from peers that are in an org that's in the channel
	// Empty PKI-ID, should drop the block
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(5, channelA)})
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Known PKI-ID but not in channel, should drop the block
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(5, channelA), PKIID: pkiIDinOrg2})
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())
	// Known PKI-ID, and in channel, should add the block
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(5, channelA), PKIID: pkiIDInOrg1})
	assert.Equal(t, 1, gc.(*gossipChannel).blockMsgStore.Size())

	// Next, we make sure that the channel doesn't respond to pull messages (hello or requests) from peers that're not in the channel
	messageRelayer := func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		gossipMessagesSentFromChannel <- msg
	}
	// First, ensure it does that for pull messages from peers that are in the channel
	// Let the peer first publish it is in the channel
	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1, channelA), PKIID: pkiIDInOrg1})
	helloMsg := createHelloMsg(pkiIDInOrg1)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-gossipMessagesSentFromChannel:
	case <-time.After(time.Second * 5):
		t.Fatal("Didn't reply with a digest on time")
	}
	// And now for peers that are not in the channel (should not send back a message)
	helloMsg = createHelloMsg(pkiIDinOrg2)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatal("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}

	// Now for a more advanced scenario- the peer claims to be in the right org, and also claims to be in the channel
	// but the MSP declares it is not eligible for the channel
	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1ButNotEligible, channelA), PKIID: pkiIDInOrg1ButNotEligible})
	// configure MSP
	cs.On("VerifyByChannel", mock.Anything).Return(errors.New("Not eligible"))
	cs.mocked = true
	// Simulate a config update
	gc.ConfigureChannel(&joinChanMsg{})
	helloMsg = createHelloMsg(pkiIDInOrg1ButNotEligible)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatal("Responded with digest, but shouldn't have since peer is not eligible for the channel")
	case <-time.After(time.Second * 1):
	}

	cs.Mock = mock.Mock{}

	// Ensure we respond to a valid StateInfoRequest
	req, _ := gc.(*gossipChannel).createStateInfoRequest()
	validReceivedMsg := &receivedMsg{
		msg:   req,
		PKIID: pkiIDInOrg1,
	}
	validReceivedMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(validReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
	case <-time.After(time.Second * 5):
		t.Fatal("Didn't reply with a digest on time")
	}

	// Ensure we don't respond to a StateInfoRequest from a peer in the wrong org
	invalidReceivedMsg := &receivedMsg{
		msg:   req,
		PKIID: pkiIDinOrg2,
	}
	invalidReceivedMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatal("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}

	// Ensure we don't respond to a StateInfoRequest in the wrong channel from a peer in the right org
	req2, _ := gc.(*gossipChannel).createStateInfoRequest()
	req2.GetStateInfoPullReq().Channel_MAC = GenerateMAC(pkiIDInOrg1, common.ChainID("B"))
	invalidReceivedMsg2 := &receivedMsg{
		msg:   req2,
		PKIID: pkiIDInOrg1,
	}
	invalidReceivedMsg2.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg2)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatal("Responded with stateInfo request, but shouldn't have since it has the wrong MAC")
	case <-time.After(time.Second * 1):
	}
}

func TestChannelIsInChannel(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)

	assert.False(t, gc.IsOrgInChannel(nil))
	assert.True(t, gc.IsOrgInChannel(orgInChannelA))
	assert.False(t, gc.IsOrgInChannel(orgNotInChannelA))
	assert.True(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
}

func TestChannelIsSubscribed(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1, channelA), PKIID: pkiIDInOrg1})
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
}

func TestChannelAddToMessageStore(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	demuxedMsgs := make(chan *proto.SignedGossipMessage, 1)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything).Run(func(arg mock.Arguments) {
		demuxedMsgs <- arg.Get(0).(*proto.SignedGossipMessage)
	})

	// Check that adding a message of a bad type doesn't crash the program
	gc.AddToMsgStore(createHelloMsg(pkiIDInOrg1).GetGossipMessage())

	// We make sure that if we get a new message it is de-multiplexed,
	// but if we put such a message in the message store, it isn't demultiplexed when we
	// receive that message again
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(11, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't detected a demultiplexing within a time period")
	case <-demuxedMsgs:
	}
	gc.AddToMsgStore(dataMsgOfChannel(12, channelA))
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(12, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
	case <-demuxedMsgs:
		t.Fatal("Demultiplexing detected, even though it wasn't supposed to happen")
	}

	gc.AddToMsgStore(createStateInfoMsg(10, pkiIDInOrg1, channelA))
	helloMsg := createHelloMsg(pkiIDInOrg1)
	respondedChan := make(chan struct{}, 1)
	helloMsg.On("Respond", mock.Anything).Run(func(arg mock.Arguments) {
		respondedChan <- struct{}{}
	})
	gc.HandleMessage(helloMsg)
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't responded to hello message within a time period")
	case <-respondedChan:
	}

	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1, channelA), PKIID: pkiIDInOrg1})
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
}

func TestChannelBlockExpiration(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	demuxedMsgs := make(chan *proto.SignedGossipMessage, 1)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything).Run(func(arg mock.Arguments) {
		demuxedMsgs <- arg.Get(0).(*proto.SignedGossipMessage)
	})
	respondedChan := make(chan *proto.GossipMessage, 1)
	messageRelayer := func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		respondedChan <- msg
	}

	// We make sure that if we get a new message it is de-multiplexed,
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(11, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't detected a demultiplexing within a time period")
	case <-demuxedMsgs:
	}

	// Lets check digests and state info store
	stateInfoMsg := createStateInfoMsg(10, pkiIDInOrg1, channelA)
	gc.AddToMsgStore(stateInfoMsg)
	helloMsg := createHelloMsg(pkiIDInOrg1)
	helloMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't responded to hello message within a time period")
	case msg := <-respondedChan:
		if msg.IsDigestMsg() {
			assert.Equal(t, 1, len(msg.GetDataDig().Digests), "Number of digests returned by channel blockPuller incorrect")
		} else {
			t.Fatal("Not correct pull msg type in response - expect digest")
		}
	}

	time.Sleep(gc.(*gossipChannel).GetConf().BlockExpirationInterval + time.Second)

	// message expired in store, but still isn't demultiplexed when we
	// receive that message again
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(11, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
	case <-demuxedMsgs:
		t.Fatal("Demultiplexing detected, even though it wasn't supposed to happen")
	}

	// Lets check digests and state info store - state info expired, its add will do nothing and digest should not be sent
	gc.AddToMsgStore(stateInfoMsg)
	gc.HandleMessage(helloMsg)
	select {
	case <-time.After(time.Second):
	case <-respondedChan:
		t.Fatal("No digest should be sent")
	}

	time.Sleep(gc.(*gossipChannel).GetConf().BlockExpirationInterval + time.Second)
	// message removed from store, so it will be demultiplexed when we
	// receive that message again
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(11, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't detected a demultiplexing within a time period")
	case <-demuxedMsgs:
	}

	// Lets check digests and state info store - state info removed as well, so it will be added back and digest will be created
	gc.AddToMsgStore(stateInfoMsg)
	gc.HandleMessage(helloMsg)
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't responded to hello message within a time period")
	case msg := <-respondedChan:
		if msg.IsDigestMsg() {
			assert.Equal(t, 1, len(msg.GetDataDig().Digests), "Number of digests returned by channel blockPuller incorrect")
		} else {
			t.Fatal("Not correct pull msg type in response - expect digest")
		}
	}

	gc.Stop()
}

func TestChannelBadBlocks(t *testing.T) {
	t.Parallel()
	receivedMessages := make(chan *proto.SignedGossipMessage, 1)
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})

	adapter.On("DeMultiplex", mock.Anything).Run(func(args mock.Arguments) {
		receivedMessages <- args.Get(0).(*proto.SignedGossipMessage)
	})

	// Send a valid block
	gc.HandleMessage(&receivedMsg{msg: createDataMsg(1, channelA), PKIID: pkiIDInOrg1})
	assert.Len(t, receivedMessages, 1)
	<-receivedMessages // drain

	// Send a block with wrong channel
	gc.HandleMessage(&receivedMsg{msg: createDataMsg(2, common.ChainID("B")), PKIID: pkiIDInOrg1})
	assert.Len(t, receivedMessages, 0)

	// Send a block with empty payload
	dataMsg := createDataMsg(3, channelA)
	dataMsg.GetDataMsg().Payload = nil
	gc.HandleMessage(&receivedMsg{msg: dataMsg, PKIID: pkiIDInOrg1})
	assert.Len(t, receivedMessages, 0)

	// Send a block with a bad signature
	cs.Mock = mock.Mock{}
	cs.On("VerifyBlock", mock.Anything).Return(errors.New("Bad signature"))
	gc.HandleMessage(&receivedMsg{msg: createDataMsg(4, channelA), PKIID: pkiIDInOrg1})
	assert.Len(t, receivedMessages, 0)
}

func TestChannelPulledBadBlocks(t *testing.T) {
	t.Parallel()

	// Test a pull with a block of a bad channel
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("DeMultiplex", mock.Anything)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg sync.WaitGroup
	wg.Add(1)

	changeChan := func(env *proto.Envelope) {
		sMsg, _ := env.ToGossipMessage()
		sMsg.Channel = []byte("B")
		sMsg, _ = sMsg.NoopSign()
		env.Payload = sMsg.Payload
	}

	pullPhase1 := simulatePullPhase(gc, t, &wg, changeChan, 10, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase1)
	adapter.On("DeMultiplex", mock.Anything)
	wg.Wait()
	gc.Stop()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with a badly signed block
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(errors.New("Bad block"))
	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg2 sync.WaitGroup
	wg2.Add(1)
	noop := func(env *proto.Envelope) {

	}
	pullPhase2 := simulatePullPhase(gc, t, &wg2, noop, 10, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase2)
	wg2.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with an empty block
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg3 sync.WaitGroup
	wg3.Add(1)
	emptyBlock := func(env *proto.Envelope) {
		sMsg, _ := env.ToGossipMessage()
		sMsg.GossipMessage.GetDataMsg().Payload = nil
		sMsg, _ = sMsg.NoopSign()
		env.Payload = sMsg.Payload
	}
	pullPhase3 := simulatePullPhase(gc, t, &wg3, emptyBlock, 10, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase3)
	wg3.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with a non-block message
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)

	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg4 sync.WaitGroup
	wg4.Add(1)
	nonBlockMsg := func(env *proto.Envelope) {
		sMsg, _ := env.ToGossipMessage()
		sMsg.Content = createHelloMsg(pkiIDInOrg1).GetGossipMessage().Content
		sMsg, _ = sMsg.NoopSign()
		env.Payload = sMsg.Payload
	}
	pullPhase4 := simulatePullPhase(gc, t, &wg4, nonBlockMsg, 10, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase4)
	wg4.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())
}

func TestChannelStateInfoSnapshot(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	adapter.On("Lookup", mock.Anything).Return(&discovery.NetworkMember{Endpoint: "localhost:5000"})
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	sentMessages := make(chan *proto.GossipMessage, 10)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("ValidateStateInfoMessage", mock.Anything).Return(nil)

	// Ensure we ignore stateInfo snapshots from peers not in the channel
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(common.ChainID("B"), createStateInfoMsg(4, pkiIDInOrg1, channelA))})
	assert.Empty(t, gc.GetPeers())
	// Ensure we ignore invalid stateInfo snapshots
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, common.ChainID("B")))})
	assert.Empty(t, gc.GetPeers())

	// Ensure we ignore stateInfo messages from peers not in the channel
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDinOrg2, channelA))})
	assert.Empty(t, gc.GetPeers())

	// Ensure we ignore stateInfo snapshots from peers not in the org
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDinOrg2, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))})
	assert.Empty(t, gc.GetPeers())

	// Ensure we ignore stateInfo snapshots with StateInfo messages with wrong MACs
	sim := createStateInfoMsg(4, pkiIDInOrg1, channelA)
	sim.GetStateInfo().Channel_MAC = append(sim.GetStateInfo().Channel_MAC, 1)
	sim, _ = sim.NoopSign()
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, sim)})
	assert.Empty(t, gc.GetPeers())

	// Ensure we ignore stateInfo snapshots with correct StateInfo messages, BUT with wrong MACs
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))})

	// Ensure we process stateInfo snapshots that are OK
	stateInfoMsg := &receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))}
	gc.HandleMessage(stateInfoMsg)
	assert.NotEmpty(t, gc.GetPeers())
	assert.Equal(t, 4, int(gc.GetPeers()[0].Properties.LedgerHeight))

	// Check we don't respond to stateInfoSnapshot requests with wrong MAC
	sMsg, _ := (&proto.GossipMessage{
		Tag: proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{
				Channel_MAC: append(GenerateMAC(pkiIDInOrg1, channelA), 1),
			},
		},
	}).NoopSign()
	snapshotReq := &receivedMsg{
		PKIID: pkiIDInOrg1,
		msg:   sMsg,
	}
	snapshotReq.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		sentMessages <- args.Get(0).(*proto.GossipMessage)
	})

	go gc.HandleMessage(snapshotReq)
	select {
	case <-time.After(time.Second):
	case <-sentMessages:
		assert.Fail(t, "Shouldn't have responded to this StateInfoSnapshot request because of bad MAC")
	}

	// Ensure we respond to stateInfoSnapshot requests with valid MAC
	sMsg, _ = (&proto.GossipMessage{
		Tag: proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{
				Channel_MAC: GenerateMAC(pkiIDInOrg1, channelA),
			},
		},
	}).NoopSign()
	snapshotReq = &receivedMsg{
		PKIID: pkiIDInOrg1,
		msg:   sMsg,
	}
	snapshotReq.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		sentMessages <- args.Get(0).(*proto.GossipMessage)
	})

	go gc.HandleMessage(snapshotReq)
	select {
	case <-time.After(time.Second):
		t.Fatal("Haven't received a state info snapshot on time")
	case msg := <-sentMessages:
		elements := msg.GetStateSnapshot().Elements
		assert.Len(t, elements, 1)
		sMsg, err := elements[0].ToGossipMessage()
		assert.NoError(t, err)
		assert.Equal(t, 4, int(sMsg.GetStateInfo().Properties.LedgerHeight))
	}

	// Ensure we don't crash if we got an invalid state info message
	invalidStateInfoSnapshot := stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))
	invalidStateInfoSnapshot.GetStateSnapshot().Elements = []*proto.Envelope{createHelloMsg(pkiIDInOrg1).GetSourceEnvelope()}
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: invalidStateInfoSnapshot})

	// Ensure we don't crash if we got a stateInfoMessage from a peer that its org isn't known
	invalidStateInfoSnapshot = stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, common.PKIidType("unknown"), channelA))
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: invalidStateInfoSnapshot})
}

func TestInterOrgExternalEndpointDisclosure(t *testing.T) {
	t.Parallel()
	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	pkiID1 := common.PKIidType("withExternalEndpoint")
	pkiID2 := common.PKIidType("noExternalEndpoint")
	pkiID3 := common.PKIidType("pkiIDinOrg2")
	adapter.On("Lookup", pkiID1).Return(&discovery.NetworkMember{Endpoint: "localhost:5000"})
	adapter.On("Lookup", pkiID2).Return(&discovery.NetworkMember{})
	adapter.On("Lookup", pkiID3).Return(&discovery.NetworkMember{})
	adapter.On("GetOrgOfPeer", pkiID1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiID2).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiID3).Return(api.OrgIdentityType("ORG2"))
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter)
	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgInChannelA): {},
			"ORG2":                {},
		},
	}
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, jcm)
	gc.HandleMessage(&receivedMsg{PKIID: pkiID1, msg: createStateInfoMsg(0, pkiID1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID2, msg: createStateInfoMsg(0, pkiID2, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiID2, msg: createStateInfoMsg(0, pkiID3, channelA)})

	sentMessages := make(chan *proto.GossipMessage, 10)

	// Check that we only return StateInfo messages of peers with external endpoints
	// to peers of other orgs
	sMsg, _ := (&proto.GossipMessage{
		Tag: proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{
				Channel_MAC: GenerateMAC(pkiID3, channelA),
			},
		},
	}).NoopSign()
	snapshotReq := &receivedMsg{
		PKIID: pkiID3,
		msg:   sMsg,
	}
	snapshotReq.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		sentMessages <- args.Get(0).(*proto.GossipMessage)
	})

	go gc.HandleMessage(snapshotReq)
	select {
	case <-time.After(time.Second):
		assert.Fail(t, "Should have responded to this StateInfoSnapshot, but didn't")
	case msg := <-sentMessages:
		elements := msg.GetStateSnapshot().Elements
		assert.Len(t, elements, 2)
		m1, _ := elements[0].ToGossipMessage()
		m2, _ := elements[1].ToGossipMessage()
		pkiIDs := [][]byte{m1.GetStateInfo().PkiId, m2.GetStateInfo().PkiId}
		assert.Contains(t, pkiIDs, []byte(pkiID1))
		assert.Contains(t, pkiIDs, []byte(pkiID3))
	}

	// Check that we return all StateInfo messages to peers in our organization, regardless
	// if the peers from foreign organizations have external endpoints or not
	sMsg, _ = (&proto.GossipMessage{
		Tag: proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{
				Channel_MAC: GenerateMAC(pkiID2, channelA),
			},
		},
	}).NoopSign()
	snapshotReq = &receivedMsg{
		PKIID: pkiID2,
		msg:   sMsg,
	}
	snapshotReq.On("Respond", mock.Anything).Run(func(args mock.Arguments) {
		sentMessages <- args.Get(0).(*proto.GossipMessage)
	})

	go gc.HandleMessage(snapshotReq)
	select {
	case <-time.After(time.Second):
		assert.Fail(t, "Should have responded to this StateInfoSnapshot, but didn't")
	case msg := <-sentMessages:
		elements := msg.GetStateSnapshot().Elements
		assert.Len(t, elements, 3)
		m1, _ := elements[0].ToGossipMessage()
		m2, _ := elements[1].ToGossipMessage()
		m3, _ := elements[2].ToGossipMessage()
		pkiIDs := [][]byte{m1.GetStateInfo().PkiId, m2.GetStateInfo().PkiId, m3.GetStateInfo().PkiId}
		assert.Contains(t, pkiIDs, []byte(pkiID1))
		assert.Contains(t, pkiIDs, []byte(pkiID2))
		assert.Contains(t, pkiIDs, []byte(pkiID3))
	}
}

func TestChannelStop(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	var sendCount int32
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Send", mock.Anything, mock.Anything).Run(func(mock.Arguments) {
		atomic.AddInt32(&sendCount, int32(1))
	})
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	time.Sleep(time.Second)
	gc.Stop()
	oldCount := atomic.LoadInt32(&sendCount)
	t1 := time.Now()
	for {
		if time.Since(t1).Nanoseconds() > (time.Second * 15).Nanoseconds() {
			t.Fatal("Stop failed")
		}
		time.Sleep(time.Second)
		newCount := atomic.LoadInt32(&sendCount)
		if newCount == oldCount {
			break
		}
		oldCount = newCount
	}
}

func TestChannelReconfigureChannel(t *testing.T) {
	t.Parallel()

	// Scenario: We test the following things:
	// Updating a channel with an outdated JoinChannel message doesn't work
	// Removing an organization from a channel is indeed reflected in that
	// the GossipChannel doesn't consider peers from that organization as
	// peers in the channel, and refuses to have any channel-related contact
	// with peers of that channel

	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})

	adapter.On("GetConf").Return(conf)
	adapter.On("GetMembership").Return([]discovery.NetworkMember{})
	adapter.On("OrgByPeerIdentity", api.PeerIdentityType(orgInChannelA)).Return(orgInChannelA)
	adapter.On("OrgByPeerIdentity", api.PeerIdentityType(orgNotInChannelA)).Return(orgNotInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDinOrg2).Return(orgNotInChannelA)

	outdatedJoinChanMsg := &joinChanMsg{
		getTS: func() time.Time {
			return time.Now()
		},
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgNotInChannelA): {},
		},
	}

	newJoinChanMsg := &joinChanMsg{
		getTS: func() time.Time {
			return time.Now().Add(time.Millisecond * 100)
		},
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgInChannelA): {},
		},
	}

	updatedJoinChanMsg := &joinChanMsg{
		getTS: func() time.Time {
			return time.Now().Add(time.Millisecond * 200)
		},
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgNotInChannelA): {},
		},
	}

	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, api.JoinChannelMessage(newJoinChanMsg))

	// Just call it again, to make sure stuff don't crash
	gc.ConfigureChannel(api.JoinChannelMessage(newJoinChanMsg))

	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)

	assert.True(t, gc.IsOrgInChannel(orgInChannelA))
	assert.False(t, gc.IsOrgInChannel(orgNotInChannelA))
	assert.True(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDinOrg2}))

	gc.ConfigureChannel(outdatedJoinChanMsg)
	assert.True(t, gc.IsOrgInChannel(orgInChannelA))
	assert.False(t, gc.IsOrgInChannel(orgNotInChannelA))
	assert.True(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDinOrg2}))

	gc.ConfigureChannel(updatedJoinChanMsg)
	gc.ConfigureChannel(updatedJoinChanMsg)
	assert.False(t, gc.IsOrgInChannel(orgInChannelA))
	assert.True(t, gc.IsOrgInChannel(orgNotInChannelA))
	assert.False(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.True(t, gc.IsMemberInChan(discovery.NetworkMember{PKIid: pkiIDinOrg2}))

	// Ensure we don't respond to a StateInfoRequest from a peer in the wrong org
	sMsg, _ := gc.(*gossipChannel).createStateInfoRequest()
	invalidReceivedMsg := &receivedMsg{
		msg:   sMsg,
		PKIID: pkiIDInOrg1,
	}
	gossipMessagesSentFromChannel := make(chan *proto.GossipMessage, 1)
	messageRelayer := func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		gossipMessagesSentFromChannel <- msg
	}
	invalidReceivedMsg.On("Respond", mock.Anything).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatal("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}
}

func TestChannelNoAnchorPeers(t *testing.T) {
	t.Parallel()

	// Scenario: We got a join channel message with no anchor peers
	// In this case, we should be in the channel

	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})

	adapter.On("GetConf").Return(conf)
	adapter.On("GetMembership").Return([]discovery.NetworkMember{})
	adapter.On("OrgByPeerIdentity", api.PeerIdentityType(orgInChannelA)).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDinOrg2).Return(orgNotInChannelA)

	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(orgInChannelA): {},
		},
	}

	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, api.JoinChannelMessage(jcm))
	assert.True(t, gc.IsOrgInChannel(orgInChannelA))
}

func TestGossipChannelEligibility(t *testing.T) {
	t.Parallel()

	// Scenario: We have a peer in an org that joins a channel with org1 and org2.
	// and it receives StateInfo messages of other peers and the eligibility
	// of these peers of being in the channel is checked.
	// During the test, the channel is reconfigured, and the expiration
	// of the peer identities is simulated.

	cs := &cryptoService{}
	selfPKIID := common.PKIidType("p")
	adapter := new(gossipAdapterMock)
	pkiIDinOrg3 := common.PKIidType("pkiIDinOrg3")
	members := []discovery.NetworkMember{
		{PKIid: pkiIDInOrg1},
		{PKIid: pkiIDInOrg1ButNotEligible},
		{PKIid: pkiIDinOrg2},
		{PKIid: pkiIDinOrg3},
	}
	adapter.On("GetMembership").Return(members)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	adapter.On("GetConf").Return(conf)

	// At first, all peers are in the channel except pkiIDinOrg3
	org1 := api.OrgIdentityType("ORG1")
	org2 := api.OrgIdentityType("ORG2")
	org3 := api.OrgIdentityType("ORG3")

	adapter.On("GetOrgOfPeer", selfPKIID).Return(org1)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(org1)
	adapter.On("GetOrgOfPeer", pkiIDinOrg2).Return(org2)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1ButNotEligible).Return(org1)
	adapter.On("GetOrgOfPeer", pkiIDinOrg3).Return(org3)

	gc := NewGossipChannel(selfPKIID, orgInChannelA, cs, channelA, adapter, &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(org1): {},
			string(org2): {},
		},
	})
	// Every peer sends a StateInfo message
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDinOrg2, msg: createStateInfoMsg(1, pkiIDinOrg2, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1ButNotEligible, msg: createStateInfoMsg(1, pkiIDInOrg1ButNotEligible, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDinOrg3, msg: createStateInfoMsg(1, pkiIDinOrg3, channelA)})

	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1ButNotEligible}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg3}))

	// Ensure peers from the channel are returned
	assert.True(t, gc.PeerFilter(func(signature api.PeerSignature) bool {
		return true
	})(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.True(t, gc.PeerFilter(func(signature api.PeerSignature) bool {
		return true
	})(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	// But not peers which aren't in the channel
	assert.False(t, gc.PeerFilter(func(signature api.PeerSignature) bool {
		return true
	})(discovery.NetworkMember{PKIid: pkiIDinOrg3}))

	// Ensure the given predicate is considered
	assert.True(t, gc.PeerFilter(func(signature api.PeerSignature) bool {
		return bytes.Equal(signature.PeerIdentity, []byte("pkiIDinOrg2"))
	})(discovery.NetworkMember{PKIid: pkiIDinOrg2}))

	assert.False(t, gc.PeerFilter(func(signature api.PeerSignature) bool {
		return bytes.Equal(signature.PeerIdentity, []byte("pkiIDinOrg2"))
	})(discovery.NetworkMember{PKIid: pkiIDInOrg1}))

	// Remove org2 from the channel
	gc.ConfigureChannel(&joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(org1): {},
		},
	})

	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1ButNotEligible}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg3}))

	// Now simulate a config update that removed pkiIDInOrg1ButNotEligible from the channel readers
	cs.mocked = true
	cs.On("VerifyByChannel", api.PeerIdentityType(pkiIDInOrg1ButNotEligible)).Return(errors.New("Not a channel reader"))
	cs.On("VerifyByChannel", mock.Anything).Return(nil)
	gc.ConfigureChannel(&joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			string(org1): {},
		},
	})
	assert.True(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1ButNotEligible}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg3}))

	// Now Simulate a certificate expiration of pkiIDInOrg1.
	// This is done by asking the adapter to lookup the identity by PKI-ID, but if the certificate
	// is expired, the mapping is deleted and hence the lookup yields nothing.
	adapter.On("GetIdentityByPKIID", pkiIDInOrg1).Return(api.PeerIdentityType(nil))
	adapter.On("GetIdentityByPKIID", pkiIDinOrg2).Return(api.PeerIdentityType(pkiIDinOrg2))
	adapter.On("GetIdentityByPKIID", pkiIDInOrg1ButNotEligible).Return(api.PeerIdentityType(pkiIDInOrg1ButNotEligible))
	adapter.On("GetIdentityByPKIID", pkiIDinOrg3).Return(api.PeerIdentityType(pkiIDinOrg3))

	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1ButNotEligible}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg3}))

	// Now make another update of StateInfo messages, this time with updated ledger height (to overwrite earlier messages)
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(2, pkiIDInOrg1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(2, pkiIDinOrg2, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(2, pkiIDInOrg1ButNotEligible, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(2, pkiIDinOrg3, channelA)})

	// Ensure the access control resolution hasn't changed
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg2}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDInOrg1ButNotEligible}))
	assert.False(t, gc.EligibleForChannel(discovery.NetworkMember{PKIid: pkiIDinOrg3}))
}

func TestChannelGetPeers(t *testing.T) {
	t.Parallel()

	// Scenario: We have a peer in an org, and the peer is notified that several peers
	// exist, and some of them:
	// (1) Join its channel, and are eligible for receiving blocks.
	// (2) Join its channel, but are not eligible for receiving blocks (MSP doesn't allow this).
	// (3) Say they join its channel, but are actually from an org that is not in the channel.
	// The GetPeers query should only return peers that belong to the first group.
	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	members := []discovery.NetworkMember{
		{PKIid: pkiIDInOrg1},
		{PKIid: pkiIDInOrg1ButNotEligible},
		{PKIid: pkiIDinOrg2},
	}
	configureAdapter(adapter, members...)
	gc := NewGossipChannel(common.PKIidType("p0"), orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDinOrg2, channelA)})
	assert.Len(t, gc.GetPeers(), 1)
	assert.Equal(t, pkiIDInOrg1, gc.GetPeers()[0].PKIid)

	// Ensure envelope from GetPeers is valid
	gMsg, _ := gc.GetPeers()[0].Envelope.ToGossipMessage()
	assert.Equal(t, []byte(pkiIDInOrg1), gMsg.GetStateInfo().PkiId)

	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1ButNotEligible, channelA), PKIID: pkiIDInOrg1ButNotEligible})
	cs.On("VerifyByChannel", mock.Anything).Return(errors.New("Not eligible"))
	cs.mocked = true
	// Simulate a config update
	gc.ConfigureChannel(&joinChanMsg{})
	assert.Len(t, gc.GetPeers(), 0)

	// Now recreate gc and corrupt the MAC
	// and ensure that the StateInfo message doesn't count
	gc = NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	msg := &receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)}
	msg.GetGossipMessage().GetStateInfo().Channel_MAC = GenerateMAC(pkiIDinOrg2, channelA)
	gc.HandleMessage(msg)
	assert.Len(t, gc.GetPeers(), 0)
}

func TestOnDemandGossip(t *testing.T) {
	t.Parallel()

	// Scenario: update the metadata and ensure only 1 dissemination
	// takes place when membership is not empty

	cs := &cryptoService{}
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)

	gossipedEvents := make(chan struct{})

	conf := conf
	conf.PublishStateInfoInterval = time.Millisecond * 200
	adapter.On("GetConf").Return(conf)
	adapter.On("GetMembership").Return([]discovery.NetworkMember{})
	adapter.On("Gossip", mock.Anything).Run(func(mock.Arguments) {
		gossipedEvents <- struct{}{}
	})
	adapter.On("Forward", mock.Anything)
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, api.JoinChannelMessage(&joinChanMsg{}))
	defer gc.Stop()
	select {
	case <-gossipedEvents:
		assert.Fail(t, "Should not have gossiped because metadata has not been updated yet")
	case <-time.After(time.Millisecond * 500):
	}
	gc.UpdateLedgerHeight(0)
	select {
	case <-gossipedEvents:
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't gossip within a timely manner")
	}
	select {
	case <-gossipedEvents:
	case <-time.After(time.Second):
		assert.Fail(t, "Should have gossiped a second time, because membership is empty")
	}
	adapter = new(gossipAdapterMock)
	configureAdapter(adapter, []discovery.NetworkMember{{}}...)
	adapter.On("Gossip", mock.Anything).Run(func(mock.Arguments) {
		gossipedEvents <- struct{}{}
	})
	adapter.On("Forward", mock.Anything)
	gc.(*gossipChannel).Adapter = adapter
	select {
	case <-gossipedEvents:
	case <-time.After(time.Second):
		assert.Fail(t, "Should have gossiped a third time")
	}
	select {
	case <-gossipedEvents:
		assert.Fail(t, "Should not have gossiped a fourth time, because dirty flag should have been turned off")
	case <-time.After(time.Millisecond * 500):
	}
	gc.UpdateLedgerHeight(1)
	select {
	case <-gossipedEvents:
	case <-time.After(time.Second):
		assert.Fail(t, "Should have gossiped a block now, because got a new StateInfo message")
	}
}

func TestChannelPullWithDigestsFilter(t *testing.T) {
	t.Parallel()
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	receivedBlocksChan := make(chan *proto.SignedGossipMessage, 2)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Forward", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.SignedGossipMessage)
		if !msg.IsDataMsg() {
			return
		}
		// The peer is supposed to de-multiplex 1 ledger block
		assert.True(t, msg.IsDataMsg())
		receivedBlocksChan <- msg
	})
	gc := NewGossipChannel(pkiIDInOrg1, orgInChannelA, cs, channelA, adapter, &joinChanMsg{})
	go gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(100, pkiIDInOrg1, channelA)})

	gc.UpdateLedgerHeight(11)

	var wg sync.WaitGroup
	wg.Add(1)

	pullPhase := simulatePullPhaseWithVariableDigest(gc, t, &wg, func(envelope *proto.Envelope) {}, [][]byte{[]byte("10"), []byte("11")}, []string{"11"}, 11)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase)
	wg.Wait()

	select {
	case <-time.After(time.Second * 5):
		t.Fatal("Haven't received blocks on time")
	case msg := <-receivedBlocksChan:
		assert.Equal(t, uint64(11), msg.GetDataMsg().Payload.SeqNum)
	}

}

func createDataUpdateMsg(nonce uint64, seqs ...uint64) *proto.SignedGossipMessage {
	msg := &proto.GossipMessage{
		Nonce:   0,
		Channel: []byte(channelA),
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				MsgType: proto.PullMsgType_BLOCK_MSG,
				Nonce:   nonce,
				Data:    []*proto.Envelope{},
			},
		},
	}
	for _, seq := range seqs {
		msg.GetDataUpdate().Data = append(msg.GetDataUpdate().Data, createDataMsg(seq, channelA).Envelope)
	}
	sMsg, _ := msg.NoopSign()
	return sMsg
}

func createHelloMsg(PKIID common.PKIidType) *receivedMsg {
	msg := &proto.GossipMessage{
		Channel: []byte(channelA),
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce:    500,
				Metadata: nil,
				MsgType:  proto.PullMsgType_BLOCK_MSG,
			},
		},
	}
	sMsg, _ := msg.NoopSign()
	return &receivedMsg{msg: sMsg, PKIID: PKIID}
}

func dataMsgOfChannel(seqnum uint64, channel common.ChainID) *proto.SignedGossipMessage {
	sMsg, _ := (&proto.GossipMessage{
		Channel: []byte(channel),
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   []byte{},
					SeqNum: seqnum,
				},
			},
		},
	}).NoopSign()
	return sMsg
}

func createStateInfoMsg(ledgerHeight int, pkiID common.PKIidType, channel common.ChainID) *proto.SignedGossipMessage {
	sMsg, _ := (&proto.GossipMessage{
		Tag: proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfo{
			StateInfo: &proto.StateInfo{
				Channel_MAC: GenerateMAC(pkiID, channel),
				Timestamp:   &proto.PeerTime{IncNum: uint64(time.Now().UnixNano()), SeqNum: 1},
				PkiId:       []byte(pkiID),
				Properties: &proto.Properties{
					LedgerHeight: uint64(ledgerHeight),
				},
			},
		},
	}).NoopSign()
	return sMsg
}

func stateInfoSnapshotForChannel(chainID common.ChainID, stateInfoMsgs ...*proto.SignedGossipMessage) *proto.SignedGossipMessage {
	envelopes := make([]*proto.Envelope, len(stateInfoMsgs))
	for i, sim := range stateInfoMsgs {
		envelopes[i] = sim.Envelope
	}
	sMsg, _ := (&proto.GossipMessage{
		Channel: chainID,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Nonce:   0,
		Content: &proto.GossipMessage_StateSnapshot{
			StateSnapshot: &proto.StateInfoSnapshot{
				Elements: envelopes,
			},
		},
	}).NoopSign()
	return sMsg
}

func createDataMsg(seqnum uint64, channel common.ChainID) *proto.SignedGossipMessage {
	sMsg, _ := (&proto.GossipMessage{
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Channel: []byte(channel),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   []byte{},
					SeqNum: seqnum,
				},
			},
		},
	}).NoopSign()
	return sMsg
}

func simulatePullPhase(gc GossipChannel, t *testing.T, wg *sync.WaitGroup, mutator msgMutator, seqs ...uint64) func(args mock.Arguments) {
	return simulatePullPhaseWithVariableDigest(gc, t, wg, mutator, [][]byte{[]byte("10"), []byte("11")}, []string{"10", "11"}, seqs...)
}

func simulatePullPhaseWithVariableDigest(gc GossipChannel, t *testing.T, wg *sync.WaitGroup, mutator msgMutator, proposedDigestSeqs [][]byte, resultDigestSeqs []string, seqs ...uint64) func(args mock.Arguments) {
	var l sync.Mutex
	var sentHello bool
	var sentReq bool
	return func(args mock.Arguments) {
		msg := args.Get(0).(*proto.SignedGossipMessage)
		l.Lock()
		defer l.Unlock()
		if msg.IsHelloMsg() && !sentHello {
			sentHello = true
			// Simulate a digest message an imaginary peer responds to the hello message sent
			sMsg, _ := (&proto.GossipMessage{
				Tag:     proto.GossipMessage_CHAN_AND_ORG,
				Channel: []byte(channelA),
				Content: &proto.GossipMessage_DataDig{
					DataDig: &proto.DataDigest{
						MsgType: proto.PullMsgType_BLOCK_MSG,
						Digests: proposedDigestSeqs,
						Nonce:   msg.GetHello().Nonce,
					},
				},
			}).NoopSign()
			digestMsg := &receivedMsg{
				PKIID: pkiIDInOrg1,
				msg:   sMsg,
			}
			go gc.HandleMessage(digestMsg)
		}
		if msg.IsDataReq() && !sentReq {
			sentReq = true
			dataReq := msg.GetDataReq()
			for _, expectedDigest := range util.StringsToBytes(resultDigestSeqs) {
				assert.Contains(t, dataReq.Digests, expectedDigest)
			}
			assert.Equal(t, len(resultDigestSeqs), len(dataReq.Digests))
			// When we send a data request, simulate a response of a data update
			// from the imaginary peer that got the request
			dataUpdateMsg := new(receivedMsg)
			dataUpdateMsg.PKIID = pkiIDInOrg1
			dataUpdateMsg.msg = createDataUpdateMsg(dataReq.Nonce, seqs...)
			mutator(dataUpdateMsg.msg.GetDataUpdate().Data[0])
			gc.HandleMessage(dataUpdateMsg)
			wg.Done()
		}
	}
}

func sequence(start uint64, end uint64) []uint64 {
	sequence := make([]uint64, end-start+1)
	i := 0
	for n := start; n <= end; n++ {
		sequence[i] = n
		i++
	}
	return sequence

}
