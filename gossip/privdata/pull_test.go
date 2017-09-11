/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"crypto/rand"
	"testing"

	"sync"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/util"
	fcommon "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logging.SetLevel(logging.DEBUG, util.LoggingPrivModule)
	policy2Filter = make(map[privdata.SerializedPolicy]privdata.Filter)
}

var policyLock sync.Mutex
var policy2Filter map[privdata.SerializedPolicy]privdata.Filter

type mockSerializedPolicy struct {
	ps *mockPolicyStore
}

func (*mockSerializedPolicy) Channel() string {
	panic("implement me")
}

func (*mockSerializedPolicy) Raw() []byte {
	panic("implement me")
}

func (sp *mockSerializedPolicy) thatMapsTo(peers ...string) *mockPolicyStore {
	policyLock.Lock()
	defer policyLock.Unlock()
	policy2Filter[sp] = func(sd fcommon.SignedData) bool {
		for _, peer := range peers {
			if bytes.Equal(sd.Identity, []byte(peer)) {
				return true
			}
		}
		return false
	}
	return sp.ps
}

type mockPolicyStore struct {
	m map[string]*mockSerializedPolicy
}

func newPolicyStore() *mockPolicyStore {
	return &mockPolicyStore{
		m: make(map[string]*mockSerializedPolicy),
	}
}

func (ps *mockPolicyStore) withPolicy(collection string) *mockSerializedPolicy {
	sp := &mockSerializedPolicy{ps: ps}
	ps.m[collection] = sp
	return sp
}

func (ps mockPolicyStore) CollectionPolicy(cc rwset.CollectionCriteria) privdata.SerializedPolicy {
	return ps.m[cc.Collection]
}

type mockPolicyParser struct {
}

func (pp *mockPolicyParser) Parse(sp privdata.SerializedPolicy) privdata.Filter {
	policyLock.Lock()
	defer policyLock.Unlock()
	return policy2Filter[sp]
}

type dataRetrieverMock struct {
	mock.Mock
}

func (dr *dataRetrieverMock) CollectionRWSet(txID, collection, namespace string) []util.PrivateRWSet {
	return dr.Called(txID, collection, namespace).Get(0).([]util.PrivateRWSet)
}

type receivedMsg struct {
	responseChan chan proto.ReceivedMessage
	*comm.RemotePeer
	*proto.SignedGossipMessage
}

func (msg *receivedMsg) Ack(_ error) {

}

func (msg *receivedMsg) Respond(message *proto.GossipMessage) {
	m, _ := message.NoopSign()
	msg.responseChan <- &receivedMsg{SignedGossipMessage: m, RemotePeer: &comm.RemotePeer{}}
}

func (msg *receivedMsg) GetGossipMessage() *proto.SignedGossipMessage {
	return msg.SignedGossipMessage
}

func (msg *receivedMsg) GetSourceEnvelope() *proto.Envelope {
	panic("implement me")
}

func (msg *receivedMsg) GetConnectionInfo() *proto.ConnectionInfo {
	return &proto.ConnectionInfo{
		Identity: api.PeerIdentityType(msg.RemotePeer.PKIID),
		Auth: &proto.AuthInfo{
			SignedData: []byte{},
			Signature:  []byte{},
		},
	}
}

type mockGossip struct {
	mock.Mock
	msgChan chan proto.ReceivedMessage
	id      *comm.RemotePeer
	network *gossipNetwork
}

func newMockGossip(id *comm.RemotePeer) *mockGossip {
	return &mockGossip{
		msgChan: make(chan proto.ReceivedMessage),
		id:      id,
	}
}

func (g *mockGossip) PeerFilter(channel common.ChainID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	for _, call := range g.Mock.ExpectedCalls {
		if call.Method == "PeerFilter" {
			args := g.Called(channel, messagePredicate)
			if args.Get(1) != nil {
				return nil, args.Get(1).(error)
			} else {
				return args.Get(0).(filter.RoutingFilter), nil
			}
		}
	}
	return func(member discovery.NetworkMember) bool {
		return messagePredicate(api.PeerSignature{
			PeerIdentity: api.PeerIdentityType(member.PKIid),
		})
	}, nil
}

func (g *mockGossip) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	sMsg, _ := msg.NoopSign()
	for _, peer := range g.network.peers {
		if bytes.Equal(peer.id.PKIID, peers[0].PKIID) {
			peer.msgChan <- &receivedMsg{
				RemotePeer:          g.id,
				SignedGossipMessage: sMsg,
				responseChan:        g.msgChan,
			}
			return
		}
	}
}

func (g *mockGossip) PeersOfChannel(common.ChainID) []discovery.NetworkMember {
	return g.Called().Get(0).([]discovery.NetworkMember)
}

func (g *mockGossip) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage) {
	return nil, g.msgChan
}

type gossipNetwork struct {
	peers []*mockGossip
}

func (gn *gossipNetwork) newPuller(id string, ps privdata.PolicyStore, knownMembers ...string) *puller {
	g := newMockGossip(&comm.RemotePeer{PKIID: common.PKIidType(id), Endpoint: id})
	g.network = gn
	var peers []discovery.NetworkMember
	for _, member := range knownMembers {
		peers = append(peers, discovery.NetworkMember{Endpoint: member, PKIid: common.PKIidType(member)})
	}
	g.On("PeersOfChannel", mock.Anything).Return(peers)
	dr := &dataRetrieverMock{}
	p := NewPuller(ps, &mockPolicyParser{}, g, dr, "A")
	gn.peers = append(gn.peers, g)
	return p
}

func newPRWSet() []util.PrivateRWSet {
	b1 := make([]byte, 10)
	b2 := make([]byte, 10)
	rand.Read(b1)
	rand.Read(b2)
	return []util.PrivateRWSet{util.PrivateRWSet(b1), util.PrivateRWSet(b2)}
}

func TestPullerFromOnly1Peer(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and not from p3
	// and succeeds - p1 asks from p2 (and not from p3!) for the
	// expected digest
	gn := &gossipNetwork{}
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	p2TransientStore := newPRWSet()
	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return(p2TransientStore)

	p3 := gn.newPuller("p3", newPolicyStore())
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1", Namespace: "ns1"}},
	})
	rws1 := util.PrivateRWSet(fetchedMessages[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, p2TransientStore, fetched)
}

func TestPullerDataNotAvailable(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and not from p3
	// but the data in p2 doesn't exist
	gn := &gossipNetwork{}
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return([]util.PrivateRWSet{})

	p3 := gn.newPuller("p3", newPolicyStore())
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1", Namespace: "ns1"}},
	})
	assert.Empty(t, fetchedMessages)
	assert.NoError(t, err)
}

func TestPullerNoPeersKnown(t *testing.T) {
	t.Parallel()
	// Scenario: p1 doesn't know any peer and therefore fails fetching
	gn := &gossipNetwork{}
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2").withPolicy("col1").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore)
	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1", Namespace: "ns1"}},
	})
	assert.Empty(t, fetchedMessages)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Empty membership")
}

func TestPullPeerFilterError(t *testing.T) {
	t.Parallel()
	// Scenario: p1 attempts to fetch for the wrong channel
	gn := &gossipNetwork{}
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore)
	gn.peers[0].On("PeerFilter", mock.Anything, mock.Anything).Return(nil, errors.New("Failed obtaining filter"))
	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1", Namespace: "ns1"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed obtaining filter")
	assert.Empty(t, fetchedMessages)
}

func TestPullerPeerNotEligible(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 or from p3
	// but it's not eligible for pulling data from p2 or from p3
	gn := &gossipNetwork{}
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2").withPolicy("col1").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p2")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Run(func(_ mock.Arguments) {
		t.Fatal("p2 shouldn't have approved the pull")
	})

	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p3")
	p3 := gn.newPuller("p3", policyStore)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have approved the pull")
	})

	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1"}},
	})
	assert.Empty(t, fetchedMessages)
	assert.NoError(t, err)
}

func TestPullerDifferentPeersDifferentCollections(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and from p3
	// and each has different collections
	gn := &gossipNetwork{}

	policyStore := newPolicyStore().withPolicy("col2").thatMapsTo("p2").withPolicy("col3").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	p2TransientStore := newPRWSet()
	policyStore = newPolicyStore().withPolicy("col2").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col2", "ns1").Return(p2TransientStore)

	p3TransientStore := newPRWSet()
	policyStore = newPolicyStore().withPolicy("col3").thatMapsTo("p1")
	p3 := gn.newPuller("p3", policyStore)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col3", "ns1").Return(p3TransientStore)

	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{
			{Collection: "col2", TxId: "txID1", Namespace: "ns1"},
			{Collection: "col3", TxId: "txID1", Namespace: "ns1"}},
	})
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages[0].Payload[1])
	rws3 := util.PrivateRWSet(fetchedMessages[1].Payload[0])
	rws4 := util.PrivateRWSet(fetchedMessages[1].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2, rws3, rws4}
	assert.Contains(t, fetched, p2TransientStore[0])
	assert.Contains(t, fetched, p2TransientStore[1])
	assert.Contains(t, fetched, p3TransientStore[0])
	assert.Contains(t, fetched, p3TransientStore[1])
}

func TestPullerRetries(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2, p3, p4 and p5.
	// Only p3 considers p1 to be eligible to receive the data.
	// The rest consider p1 as not eligible.
	gn := &gossipNetwork{}

	// p1
	policyStore := newPolicyStore().withPolicy("col1").thatMapsTo("p2", "p3", "p4", "p5")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3", "p4", "p5")

	// p2, p3, p4, and p5 have the same transient store
	transientStore := newPRWSet()

	// p2
	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p2")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return(transientStore)

	// p3
	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p1")
	p3 := gn.newPuller("p3", policyStore)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return(transientStore)

	// p4
	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p4")
	p4 := gn.newPuller("p4", policyStore)
	p4.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return(transientStore)

	// p5
	policyStore = newPolicyStore().withPolicy("col1").thatMapsTo("p5")
	p5 := gn.newPuller("p5", policyStore)
	p5.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", "txID1", "col1", "ns1").Return(transientStore)

	// Fetch from someone
	fetchedMessages, err := p1.fetch(&proto.RemotePvtDataRequest{
		Digests: []*proto.PvtDataDigest{{Collection: "col1", TxId: "txID1", Namespace: "ns1"}},
	})
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, transientStore, fetched)
}
