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

package channel

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type msgMutator func(*proto.GossipMessage)

var conf = Config{
	ID: "test",
	PublishStateInfoInterval: time.Millisecond * 100,
	MaxBlockCountToStore:     100,
	PullPeerNum:              3,
	PullInterval:             time.Second,
	RequestStateInfoInterval: time.Millisecond * 100,
}

func init() {
	shortenedWaitTime := time.Millisecond * 300
	algo.SetDigestWaitTime(shortenedWaitTime / 2)
	algo.SetRequestWaitTime(shortenedWaitTime)
	algo.SetResponseWaitTime(shortenedWaitTime)
}

var (
	// Organizations: {ORG1, ORG2}
	// Channel A: {ORG1}
	channelA           = common.ChainID("A")
	orgInChannelA      = api.OrgIdentityType("ORG1")
	orgNotInChannelA   = api.OrgIdentityType("ORG2")
	anchorPeerIdentity = api.PeerIdentityType("identityInOrg1")
	pkiIDInOrg1        = common.PKIidType("pkiIDInOrg1")
	pkiIDinOrg2        = common.PKIidType("pkiIDinOrg2")
)

type joinChanMsg struct {
	getTS       func() time.Time
	anchorPeers func() []api.AnchorPeer
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

// AnchorPeers returns all the anchor peers that are in the channel
func (jcm *joinChanMsg) AnchorPeers() []api.AnchorPeer {
	if jcm.anchorPeers != nil {
		return jcm.anchorPeers()
	}
	return []api.AnchorPeer{{Cert: anchorPeerIdentity}}
}

type cryptoService struct {
	mock.Mock
}

func (cs *cryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	panic("Should not be called in this test")
}

func (cs *cryptoService) VerifyByChannel(_ common.ChainID, _ api.PeerIdentityType, _, _ []byte) error {
	panic("Should not be called in this test")
}

func (cs *cryptoService) VerifyBlock(chainID common.ChainID, signedBlock api.SignedBlock) error {
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
	msg   *proto.GossipMessage
	mock.Mock
}

func (m *receivedMsg) GetGossipMessage() *proto.GossipMessage {
	return m.msg
}

func (m *receivedMsg) Respond(msg *proto.GossipMessage) {
	m.Called(msg)
}

func (m *receivedMsg) GetPKIID() common.PKIidType {
	return m.PKIID
}

type gossipAdapterMock struct {
	mock.Mock
}

func (ga *gossipAdapterMock) GetConf() Config {
	args := ga.Called()
	return args.Get(0).(Config)
}

func (ga *gossipAdapterMock) Gossip(msg *proto.GossipMessage) {
	ga.Called(msg)
}

func (ga *gossipAdapterMock) DeMultiplex(msg interface{}) {
	ga.Called(msg)
}

func (ga *gossipAdapterMock) GetMembership() []discovery.NetworkMember {
	args := ga.Called()
	return args.Get(0).([]discovery.NetworkMember)
}

func (ga *gossipAdapterMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	// Ensure we have configured Send prior
	foundSend := false
	for _, ec := range ga.ExpectedCalls {
		if ec.Method == "Send" {
			foundSend = true
		}

	}
	if !foundSend {
		return
	}
	ga.Called(msg, peers)
}

func (ga *gossipAdapterMock) ValidateStateInfoMessage(msg *proto.GossipMessage) error {
	args := ga.Called(msg)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (ga *gossipAdapterMock) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	args := ga.Called(identity)
	return args.Get(0).(api.OrgIdentityType)
}

func (ga *gossipAdapterMock) GetOrgOfPeer(PKIIID common.PKIidType) api.OrgIdentityType {
	args := ga.Called(PKIIID)
	return args.Get(0).(api.OrgIdentityType)
}

func configureAdapter(adapter *gossipAdapterMock, members ...discovery.NetworkMember) {
	adapter.On("GetConf").Return(conf)
	adapter.On("GetMembership").Return(members)
	adapter.On("OrgByPeerIdentity", anchorPeerIdentity).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDInOrg1).Return(orgInChannelA)
	adapter.On("GetOrgOfPeer", pkiIDinOrg2).Return(orgNotInChannelA)
	adapter.On("GetOrgOfPeer", mock.Anything).Return(api.OrgIdentityType(nil))
}

func TestChannelPeriodicalPublishStateInfo(t *testing.T) {
	t.Parallel()
	ledgerHeight := 5
	receivedMsg := int32(0)
	stateInfoReceptionChan := make(chan *proto.GossipMessage, 1)

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)

	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Send", mock.AnythingOfType("*proto.GossipMessage"), mock.Anything)
	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage")).Run(func(arg mock.Arguments) {
		if atomic.LoadInt32(&receivedMsg) == int32(1) {
			return
		}

		atomic.StoreInt32(&receivedMsg, int32(1))
		msg := arg.Get(0).(*proto.GossipMessage)
		stateInfoReceptionChan <- msg
	})

	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	gc.UpdateStateInfo(createStateInfoMsg(ledgerHeight, pkiIDInOrg1, channelA))

	var msg *proto.GossipMessage
	select {
	case <-time.After(time.Second * 5):
		t.Fatalf("Haven't sent stateInfo on time")
	case m := <-stateInfoReceptionChan:
		msg = m
	}

	md := msg.GetStateInfo().Metadata
	height, err := strconv.ParseInt(string(md), 10, 64)
	assert.NoError(t, err, "ReceivedMetadata is invalid")
	assert.Equal(t, ledgerHeight, int(height), "Received different ledger height than expected")
}

func TestChannelPull(t *testing.T) {
	t.Parallel()
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	receivedBlocksChan := make(chan *proto.GossipMessage)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage"))
	adapter.On("DeMultiplex", mock.AnythingOfType("*proto.GossipMessage")).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		if !msg.IsDataMsg() {
			return
		}
		// The peer is supposed to de-multiplex 2 ledger blocks
		assert.True(t, msg.IsDataMsg())
		receivedBlocksChan <- msg
	})
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	go gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(100, pkiIDInOrg1, channelA)})

	var wg sync.WaitGroup
	pullPhase := simulatePullPhase(gc, t, &wg, func(*proto.GossipMessage) {})
	adapter.On("Send", mock.AnythingOfType("*proto.GossipMessage"), mock.Anything).Run(pullPhase)

	wg.Wait()
	for expectedSeq := 10; expectedSeq < 11; expectedSeq++ {
		select {
		case <-time.After(time.Second * 5):
			t.Fatalf("Haven't received blocks on time")
		case msg := <-receivedBlocksChan:
			assert.Equal(t, uint64(expectedSeq), msg.GetDataMsg().Payload.SeqNum)
		}
	}
}

func TestChannelPeerNotInChannel(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	gossipMessagesSentFromChannel := make(chan *proto.GossipMessage, 1)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage"))
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})

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
	helloMsg := createHelloMsg(pkiIDInOrg1)
	helloMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-gossipMessagesSentFromChannel:
	case <-time.After(time.Second * 5):
		t.Fatalf("Didn't reply with a digest on time")
	}
	// And now for peers that are not in the channel (should not send back a message)
	helloMsg = createHelloMsg(pkiIDinOrg2)
	helloMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(helloMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatalf("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}

	// Ensure we respond to a valid StateInfoRequest
	req := gc.(*gossipChannel).createStateInfoRequest()
	validReceivedMsg := &receivedMsg{
		msg:   req,
		PKIID: pkiIDInOrg1,
	}
	validReceivedMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(validReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
	case <-time.After(time.Second * 5):
		t.Fatalf("Didn't reply with a digest on time")
	}

	// Ensure we don't respond to a StateInfoRequest from a peer in the wrong org
	invalidReceivedMsg := &receivedMsg{
		msg:   req,
		PKIID: pkiIDinOrg2,
	}
	invalidReceivedMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatalf("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}

	// Ensure we don't respond to a StateInfoRequest in the wrong channel from a peer in the right org
	req2 := gc.(*gossipChannel).createStateInfoRequest()
	req2.Channel = []byte("B") // Not channelA
	invalidReceivedMsg2 := &receivedMsg{
		msg:   req2,
		PKIID: pkiIDInOrg1,
	}
	invalidReceivedMsg2.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg2)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatalf("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}
}

func TestChannelIsInChannel(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage"))
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
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage"))
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1, channelA), PKIID: pkiIDInOrg1})
	assert.True(t, gc.IsSubscribed(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
}

func TestChannelAddToMessageStore(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	demuxedMsgs := make(chan *proto.GossipMessage, 1)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter)
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("DeMultiplex", mock.AnythingOfType("*proto.GossipMessage")).Run(func(arg mock.Arguments) {
		demuxedMsgs <- arg.Get(0).(*proto.GossipMessage)
	})

	// Check that adding a message of a bad type doesn't crash the program
	gc.AddToMsgStore(createHelloMsg(pkiIDInOrg1).GetGossipMessage())

	// We make sure that if we get a new message it is de-multiplexed,
	// but if we put such a message in the message store, it isn't demultiplexed when we
	// receive that message again
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(11, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
		t.Fatalf("Haven't detected a demultiplexing within a time period")
	case <-demuxedMsgs:
	}
	gc.AddToMsgStore(dataMsgOfChannel(12, channelA))
	gc.HandleMessage(&receivedMsg{msg: dataMsgOfChannel(12, channelA), PKIID: pkiIDInOrg1})
	select {
	case <-time.After(time.Second):
	case <-demuxedMsgs:
		t.Fatalf("Demultiplexing detected, even though it wasn't supposed to happen")
	}

	gc.AddToMsgStore(createStateInfoMsg(10, pkiIDInOrg1, channelA))
	helloMsg := createHelloMsg(pkiIDInOrg1)
	respondedChan := make(chan struct{}, 1)
	helloMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(func(arg mock.Arguments) {
		respondedChan <- struct{}{}
	})
	gc.HandleMessage(helloMsg)
	select {
	case <-time.After(time.Second):
		t.Fatalf("Haven't responded to hello message within a time period")
	case <-respondedChan:
	}

	gc.HandleMessage(&receivedMsg{msg: createStateInfoMsg(10, pkiIDInOrg1, channelA), PKIID: pkiIDInOrg1})
	assert.True(t, gc.IsSubscribed(discovery.NetworkMember{PKIid: pkiIDInOrg1}))
}

func TestChannelBadBlocks(t *testing.T) {
	t.Parallel()
	receivedMessages := make(chan *proto.GossipMessage, 1)
	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	adapter.On("Gossip", mock.Anything)
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})

	adapter.On("DeMultiplex", mock.Anything).Run(func(args mock.Arguments) {
		receivedMessages <- args.Get(0).(*proto.GossipMessage)
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
	cs.On("VerifyBlock", mock.Anything).Return(fmt.Errorf("Bad signature"))
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
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg sync.WaitGroup
	wg.Add(1)

	changeChan := func(msg *proto.GossipMessage) {
		msg.Channel = []byte("B")
	}

	pullPhase1 := simulatePullPhase(gc, t, &wg, changeChan)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase1)
	adapter.On("DeMultiplex", mock.Anything)
	wg.Wait()
	gc.Stop()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with a badly signed block
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(fmt.Errorf("Bad block"))
	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg2 sync.WaitGroup
	wg2.Add(1)
	noop := func(msg *proto.GossipMessage) {

	}
	pullPhase2 := simulatePullPhase(gc, t, &wg2, noop)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase2)
	wg2.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with an empty block
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg3 sync.WaitGroup
	wg3.Add(1)
	emptyBlock := func(msg *proto.GossipMessage) {
		msg.GetDataMsg().Payload = nil
	}
	pullPhase3 := simulatePullPhase(gc, t, &wg3, emptyBlock)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase3)
	wg3.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())

	// Test a pull with a non-block message
	cs = &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)

	adapter = new(gossipAdapterMock)
	adapter.On("Gossip", mock.Anything)
	adapter.On("DeMultiplex", mock.Anything)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc = NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: createStateInfoMsg(1, pkiIDInOrg1, channelA)})

	var wg4 sync.WaitGroup
	wg4.Add(1)
	nonBlockMsg := func(msg *proto.GossipMessage) {
		msg.Content = createHelloMsg(pkiIDInOrg1).GetGossipMessage().Content
	}
	pullPhase4 := simulatePullPhase(gc, t, &wg4, nonBlockMsg)
	adapter.On("Send", mock.Anything, mock.Anything).Run(pullPhase4)
	wg4.Wait()
	assert.Equal(t, 0, gc.(*gossipChannel).blockMsgStore.Size())
}

func TestChannelStateInfoSnapshot(t *testing.T) {
	t.Parallel()

	cs := &cryptoService{}
	cs.On("VerifyBlock", mock.Anything).Return(nil)
	adapter := new(gossipAdapterMock)
	configureAdapter(adapter, discovery.NetworkMember{PKIid: pkiIDInOrg1})
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
	adapter.On("Gossip", mock.Anything)
	sentMessages := make(chan *proto.GossipMessage, 10)
	adapter.On("Send", mock.Anything, mock.Anything)
	adapter.On("ValidateStateInfoMessage", mock.AnythingOfType("*proto.GossipMessage")).Return(nil)

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

	// Ensure we process stateInfo snapshots that are OK
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))})
	assert.NotEmpty(t, gc.GetPeers())
	assert.Equal(t, "4", string(gc.GetPeers()[0].Metadata))

	// Check we can respond to stateInfoSnapshot requests
	snapshotReq := &receivedMsg{
		PKIID: pkiIDInOrg1,
		msg: &proto.GossipMessage{
			Channel: channelA,
			Tag:     proto.GossipMessage_CHAN_OR_ORG,
			Content: &proto.GossipMessage_StateInfoPullReq{
				StateInfoPullReq: &proto.StateInfoPullRequest{},
			},
		},
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
		assert.Equal(t, []byte("4"), elements[0].GetStateInfo().Metadata)
	}

	// Ensure we don't crash if we got an invalid state info message
	invalidStateInfoSnapshot := stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, pkiIDInOrg1, channelA))
	invalidStateInfoSnapshot.GetStateSnapshot().Elements = []*proto.GossipMessage{createHelloMsg(pkiIDInOrg1).GetGossipMessage()}
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: invalidStateInfoSnapshot})

	// Ensure we don't crash if we got a stateInfoMessage from a peer that its org isn't known
	invalidStateInfoSnapshot = stateInfoSnapshotForChannel(channelA, createStateInfoMsg(4, common.PKIidType("unknown"), channelA))
	gc.HandleMessage(&receivedMsg{PKIID: pkiIDInOrg1, msg: invalidStateInfoSnapshot})

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
	gc := NewGossipChannel(cs, channelA, adapter, &joinChanMsg{})
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
		anchorPeers: func() []api.AnchorPeer {
			return []api.AnchorPeer{{Cert: api.PeerIdentityType(orgNotInChannelA)}}
		},
		getTS: func() time.Time {
			return time.Now()
		},
	}

	newJoinChanMsg := &joinChanMsg{
		anchorPeers: func() []api.AnchorPeer {
			return []api.AnchorPeer{{Cert: api.PeerIdentityType(orgInChannelA)}}
		},
		getTS: func() time.Time {
			return time.Now().Add(time.Millisecond * 100)
		},
	}

	updatedJoinChanMsg := &joinChanMsg{
		anchorPeers: func() []api.AnchorPeer {
			return []api.AnchorPeer{{Cert: api.PeerIdentityType(orgNotInChannelA)}}
		},
		getTS: func() time.Time {
			return time.Now().Add(time.Millisecond * 200)
		},
	}

	gc := NewGossipChannel(cs, channelA, adapter, api.JoinChannelMessage(newJoinChanMsg))

	// Just call it again, to make sure stuff don't crash
	gc.ConfigureChannel(api.JoinChannelMessage(newJoinChanMsg))

	adapter.On("Gossip", mock.AnythingOfType("*proto.GossipMessage"))
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
	invalidReceivedMsg := &receivedMsg{
		msg:   gc.(*gossipChannel).createStateInfoRequest(),
		PKIID: pkiIDInOrg1,
	}
	gossipMessagesSentFromChannel := make(chan *proto.GossipMessage, 1)
	messageRelayer := func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		gossipMessagesSentFromChannel <- msg
	}
	invalidReceivedMsg.On("Respond", mock.AnythingOfType("*proto.GossipMessage")).Run(messageRelayer)
	gc.HandleMessage(invalidReceivedMsg)
	select {
	case <-gossipMessagesSentFromChannel:
		t.Fatalf("Responded with digest, but shouldn't have since peer is in ORG2 and its not in the channel")
	case <-time.After(time.Second * 1):
	}

}

func createDataUpdateMsg(nonce uint64) *proto.GossipMessage {
	return &proto.GossipMessage{
		Nonce:   0,
		Channel: []byte(channelA),
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				MsgType: proto.PullMsgType_BlockMessage,
				Nonce:   nonce,
				Data:    []*proto.GossipMessage{createDataMsg(10, channelA), createDataMsg(11, channelA)},
			},
		},
	}
}

func createHelloMsg(PKIID common.PKIidType) *receivedMsg {
	msg := &proto.GossipMessage{
		Channel: []byte(channelA),
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce:    500,
				Metadata: nil,
				MsgType:  proto.PullMsgType_BlockMessage,
			},
		},
	}
	return &receivedMsg{msg: msg, PKIID: PKIID}
}

func dataMsgOfChannel(seqnum uint64, channel common.ChainID) *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: []byte(channel),
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   []byte{},
					Hash:   "",
					SeqNum: seqnum,
				},
			},
		},
	}
}

func createStateInfoMsg(ledgerHeight int, pkiID common.PKIidType, channel common.ChainID) *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: channel,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfo{
			StateInfo: &proto.StateInfo{
				Timestamp: &proto.PeerTime{IncNumber: uint64(time.Now().UnixNano()), SeqNum: 1},
				Metadata:  []byte(fmt.Sprintf("%d", ledgerHeight)),
				PkiID:     []byte(pkiID),
			},
		},
	}
}

func stateInfoSnapshotForChannel(chainID common.ChainID, stateInfoMsgs ...*proto.GossipMessage) *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: chainID,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Nonce:   0,
		Content: &proto.GossipMessage_StateSnapshot{
			StateSnapshot: &proto.StateInfoSnapshot{
				Elements: stateInfoMsgs,
			},
		},
	}
}

func createDataMsg(seqnum uint64, channel common.ChainID) *proto.GossipMessage {
	return &proto.GossipMessage{
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Channel: []byte(channel),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					Data:   []byte{},
					Hash:   "",
					SeqNum: seqnum,
				},
			},
		},
	}
}

func simulatePullPhase(gc GossipChannel, t *testing.T, wg *sync.WaitGroup, mutator msgMutator) func(args mock.Arguments) {
	var l sync.Mutex
	var sentHello bool
	var sentReq bool
	return func(args mock.Arguments) {
		msg := args.Get(0).(*proto.GossipMessage)
		l.Lock()
		defer l.Unlock()

		if msg.IsHelloMsg() && !sentHello {
			sentHello = true
			// Simulate a digest message an imaginary peer responds to the hello message sent
			digestMsg := &receivedMsg{
				PKIID: pkiIDInOrg1,
				msg: &proto.GossipMessage{
					Tag:     proto.GossipMessage_CHAN_AND_ORG,
					Channel: []byte(channelA),
					Content: &proto.GossipMessage_DataDig{
						DataDig: &proto.DataDigest{
							MsgType: proto.PullMsgType_BlockMessage,
							Digests: []string{"10", "11"},
							Nonce:   msg.GetHello().Nonce,
						},
					},
				},
			}
			go gc.HandleMessage(digestMsg)
		}
		if msg.IsDataReq() && !sentReq {
			sentReq = true
			dataReq := msg.GetDataReq()
			for _, expectedDigest := range []string{"10", "11"} {
				assert.Contains(t, dataReq.Digests, expectedDigest)
			}
			assert.Equal(t, 2, len(dataReq.Digests))
			// When we send a data request, simulate a response of a data update
			// from the imaginary peer that got the request
			dataUpdateMsg := new(receivedMsg)
			dataUpdateMsg.PKIID = pkiIDInOrg1
			dataUpdateMsg.msg = createDataUpdateMsg(dataReq.Nonce)
			mutator(dataUpdateMsg.msg.GetDataUpdate().Data[0])
			gc.HandleMessage(dataUpdateMsg)
			wg.Done()
		}
	}

}
