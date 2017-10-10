/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/util"
	fcommon "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	logging.SetLevel(logging.DEBUG, util.LoggingPrivModule)
	policy2Filter = make(map[privdata.CollectionAccessPolicy]privdata.Filter)
}

var policyLock sync.Mutex
var policy2Filter map[privdata.CollectionAccessPolicy]privdata.Filter

type mockCollectionStore struct {
	m map[string]*mockCollectionAccess
}

func newCollectionStore() *mockCollectionStore {
	return &mockCollectionStore{
		m: make(map[string]*mockCollectionAccess),
	}
}

func (cs *mockCollectionStore) withPolicy(collection string) *mockCollectionAccess {
	coll := &mockCollectionAccess{cs: cs}
	cs.m[collection] = coll
	return coll
}

func (cs mockCollectionStore) RetrieveCollectionAccessPolicy(cc fcommon.CollectionCriteria) (privdata.CollectionAccessPolicy, error) {
	return cs.m[cc.Collection], nil
}

func (cs mockCollectionStore) RetrieveCollection(fcommon.CollectionCriteria) (privdata.Collection, error) {
	panic("implement me")
}

func (cs mockCollectionStore) RetrieveCollectionConfigPackage(fcommon.CollectionCriteria) (*fcommon.CollectionConfigPackage, error) {
	panic("implement me")
}

type mockCollectionAccess struct {
	cs *mockCollectionStore
}

func (mc *mockCollectionAccess) thatMapsTo(peers ...string) *mockCollectionStore {
	policyLock.Lock()
	defer policyLock.Unlock()
	policy2Filter[mc] = func(sd fcommon.SignedData) bool {
		for _, peer := range peers {
			if bytes.Equal(sd.Identity, []byte(peer)) {
				return true
			}
		}
		return false
	}
	return mc.cs
}

func (mc *mockCollectionAccess) MemberOrgs() []string {
	return nil
}

func (mc *mockCollectionAccess) AccessFilter() privdata.Filter {
	policyLock.Lock()
	defer policyLock.Unlock()
	return policy2Filter[mc]
}

func (mc *mockCollectionAccess) RequiredPeerCount() int {
	return 0
}

func (mc *mockCollectionAccess) MaximumPeerCount() int {
	return 0
}

type dataRetrieverMock struct {
	mock.Mock
}

func (dr *dataRetrieverMock) CollectionRWSet(dig *proto.PvtDataDigest) []util.PrivateRWSet {
	return dr.Called(dig).Get(0).([]util.PrivateRWSet)
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
			}
			return args.Get(0).(filter.RoutingFilter), nil
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

func (gn *gossipNetwork) newPuller(id string, ps privdata.CollectionStore, knownMembers ...string) *puller {
	g := newMockGossip(&comm.RemotePeer{PKIID: common.PKIidType(id), Endpoint: id})
	g.network = gn
	var peers []discovery.NetworkMember
	for _, member := range knownMembers {
		peers = append(peers, discovery.NetworkMember{Endpoint: member, PKIid: common.PKIidType(member)})
	}
	g.On("PeersOfChannel", mock.Anything).Return(peers)
	dr := &dataRetrieverMock{}
	p := NewPuller(ps, g, dr, "A")
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
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	p2TransientStore := newPRWSet()
	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return(p2TransientStore)

	p3 := gn.newPuller("p3", newCollectionStore())
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	dasf := &digestsAndSourceFactory{}

	fetchedMessages, err := p1.fetch(dasf.mapDigest(dig).toSources().create())
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
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return([]util.PrivateRWSet{})

	p3 := gn.newPuller("p3", newCollectionStore())
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(dig).toSources().create())
	assert.Empty(t, fetchedMessages)
	assert.NoError(t, err)
}

func TestPullerNoPeersKnown(t *testing.T) {
	t.Parallel()
	// Scenario: p1 doesn't know any peer and therefore fails fetching
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2").withPolicy("col1").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore)
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&proto.PvtDataDigest{Collection: "col1", TxId: "txID1", Namespace: "ns1"}).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.Empty(t, fetchedMessages)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Empty membership")
}

func TestPullPeerFilterError(t *testing.T) {
	t.Parallel()
	// Scenario: p1 attempts to fetch for the wrong channel
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2")
	p1 := gn.newPuller("p1", policyStore)
	gn.peers[0].On("PeerFilter", mock.Anything, mock.Anything).Return(nil, errors.New("Failed obtaining filter"))
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&proto.PvtDataDigest{Collection: "col1", TxId: "txID1", Namespace: "ns1"}).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed obtaining filter")
	assert.Empty(t, fetchedMessages)
}

func TestPullerPeerNotEligible(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 or from p3
	// but it's not eligible for pulling data from p2 or from p3
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2").withPolicy("col1").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p2")
	p2 := gn.newPuller("p2", policyStore)

	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Run(func(_ mock.Arguments) {
		t.Fatal("p2 shouldn't have approved the pull")
	})

	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p3")
	p3 := gn.newPuller("p3", policyStore)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have approved the pull")
	})
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&proto.PvtDataDigest{Collection: "col1", TxId: "txID1"}).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.Empty(t, fetchedMessages)
	assert.NoError(t, err)
}

func TestPullerDifferentPeersDifferentCollections(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and from p3
	// and each has different collections
	gn := &gossipNetwork{}

	policyStore := newCollectionStore().withPolicy("col2").thatMapsTo("p2").withPolicy("col3").thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3")

	p2TransientStore := newPRWSet()
	policyStore = newCollectionStore().withPolicy("col2").thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore)
	dig1 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col2",
		Namespace:  "ns1",
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig1).Return(p2TransientStore)

	p3TransientStore := newPRWSet()
	policyStore = newCollectionStore().withPolicy("col3").thatMapsTo("p1")
	p3 := gn.newPuller("p3", policyStore)
	dig2 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col3",
		Namespace:  "ns1",
	}

	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig2).Return(p3TransientStore)

	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(dig1).toSources().mapDigest(dig2).toSources().create())
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
	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p2", "p3", "p4", "p5")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3", "p4", "p5")

	// p2, p3, p4, and p5 have the same transient store
	transientStore := newPRWSet()
	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	// p2
	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p2")
	p2 := gn.newPuller("p2", policyStore)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return(transientStore)

	// p3
	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p1")
	p3 := gn.newPuller("p3", policyStore)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return(transientStore)

	// p4
	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p4")
	p4 := gn.newPuller("p4", policyStore)
	p4.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return(transientStore)

	// p5
	policyStore = newCollectionStore().withPolicy("col1").thatMapsTo("p5")
	p5 := gn.newPuller("p5", policyStore)
	p5.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig).Return(transientStore)

	// Fetch from someone
	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(dig).toSources().create())
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, transientStore, fetched)
}

func TestPullerPreferEndorsers(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2, p3, p4, p5
	// and the only endorser for col1 is p3, so it should be selected
	// at the top priority for col1.
	// for col2, only p2 should have the data, but its not an endorser of the data.
	gn := &gossipNetwork{}

	policyStore := newCollectionStore().withPolicy("col1").thatMapsTo("p1", "p2", "p3", "p4", "p5").withPolicy("col2").thatMapsTo("p1", "p2")
	p1 := gn.newPuller("p1", policyStore, "p2", "p3", "p4", "p5")

	p3TransientStore := newPRWSet()
	p2TransientStore := newPRWSet()

	p2 := gn.newPuller("p2", policyStore)
	p3 := gn.newPuller("p3", policyStore)
	gn.newPuller("p4", policyStore)
	gn.newPuller("p5", policyStore)

	dig1 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	dig2 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col2",
		Namespace:  "ns1",
	}

	// We only define an action for dig2 on p2, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig2
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig2).Return(p2TransientStore)

	// We only define an action for dig1 on p3, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig1
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", dig1).Return(p3TransientStore)

	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(dig1).toSources("p3").mapDigest(dig2).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages[0].Payload[1])
	rws3 := util.PrivateRWSet(fetchedMessages[1].Payload[0])
	rws4 := util.PrivateRWSet(fetchedMessages[1].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2, rws3, rws4}
	assert.Contains(t, fetched, p3TransientStore[0])
	assert.Contains(t, fetched, p3TransientStore[1])
	assert.Contains(t, fetched, p2TransientStore[0])
	assert.Contains(t, fetched, p2TransientStore[1])
}
