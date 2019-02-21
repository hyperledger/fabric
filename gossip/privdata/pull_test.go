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

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	fcommon "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	policy2Filter = make(map[privdata.CollectionAccessPolicy]privdata.Filter)
}

// protoMatcher is used to test that a slice of protos equals another slice of protos.
// This is needed because in general reflect.Equal(proto1, proto1) may not be true.
func protoMatcher(pvds ...*proto.PvtDataDigest) func([]*proto.PvtDataDigest) bool {
	return func(ipvds []*proto.PvtDataDigest) bool {
		if len(pvds) != len(ipvds) {
			return false
		}

		for i, pvd := range pvds {
			if !pb.Equal(pvd, ipvds[i]) {
				return false
			}
		}

		return true
	}
}

var policyLock sync.Mutex
var policy2Filter map[privdata.CollectionAccessPolicy]privdata.Filter

type mockCollectionStore struct {
	m            map[string]*mockCollectionAccess
	accessFilter privdata.Filter
}

func newCollectionStore() *mockCollectionStore {
	return &mockCollectionStore{
		m:            make(map[string]*mockCollectionAccess),
		accessFilter: nil,
	}
}

func (cs *mockCollectionStore) withPolicy(collection string, btl uint64) *mockCollectionAccess {
	coll := &mockCollectionAccess{cs: cs, btl: btl}
	cs.m[collection] = coll
	return coll
}

func (cs *mockCollectionStore) withAccessFilter(filter privdata.Filter) *mockCollectionStore {
	cs.accessFilter = filter
	return cs
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

func (cs mockCollectionStore) RetrieveCollectionPersistenceConfigs(cc fcommon.CollectionCriteria) (privdata.CollectionPersistenceConfigs, error) {
	return cs.m[cc.Collection], nil
}

func (cs mockCollectionStore) HasReadAccess(cc fcommon.CollectionCriteria, sp *peer.SignedProposal, qe ledger.QueryExecutor) (bool, error) {
	panic("implement me")
}

func (cs mockCollectionStore) AccessFilter(channelName string, collectionPolicyConfig *fcommon.CollectionPolicyConfig) (privdata.Filter, error) {
	if cs.accessFilter != nil {
		return cs.accessFilter, nil
	}
	panic("implement me")
}

type mockCollectionAccess struct {
	cs  *mockCollectionStore
	btl uint64
}

func (mc *mockCollectionAccess) BlockToLive() uint64 {
	return mc.btl
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

func (mc *mockCollectionAccess) IsMemberOnlyRead() bool {
	return false
}

type dataRetrieverMock struct {
	mock.Mock
}

func (dr *dataRetrieverMock) CollectionRWSet(dig []*proto.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, bool, error) {
	args := dr.Called(dig, blockNum)
	return args.Get(0).(Dig2PvtRWSetWithConfig), args.Bool(1), args.Error(2)
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

type peerData struct {
	id           string
	ledgerHeight uint64
}

func membership(knownPeers ...peerData) []discovery.NetworkMember {
	var peers []discovery.NetworkMember
	for _, peer := range knownPeers {
		peers = append(peers, discovery.NetworkMember{
			Endpoint: peer.id,
			PKIid:    common.PKIidType(peer.id),
			Properties: &proto.Properties{
				LedgerHeight: peer.ledgerHeight,
			},
		})
	}
	return peers
}

type gossipNetwork struct {
	peers []*mockGossip
}

func (gn *gossipNetwork) newPullerWithMetrics(metrics *metrics.PrivdataMetrics, id string, ps privdata.CollectionStore,
	factory CollectionAccessFactory, knownMembers ...discovery.NetworkMember) *puller {
	g := newMockGossip(&comm.RemotePeer{PKIID: common.PKIidType(id), Endpoint: id})
	g.network = gn
	g.On("PeersOfChannel", mock.Anything).Return(knownMembers)

	p := NewPuller(metrics, ps, g, &dataRetrieverMock{}, factory, "A", btlPullMarginDefault)
	gn.peers = append(gn.peers, g)
	return p
}

func (gn *gossipNetwork) newPuller(id string, ps privdata.CollectionStore, factory CollectionAccessFactory,
	knownMembers ...discovery.NetworkMember) *puller {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics
	return gn.newPullerWithMetrics(metrics, id, ps, factory, knownMembers...)
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
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock1 := &collectionAccessFactoryMock{}
	policyMock1 := &collectionAccessPolicyMock{}
	policyMock1.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2"))
	}, []string{"org1", "org2"}, false)
	factoryMock1.On("AccessPolicy", mock.Anything, mock.Anything).Return(policyMock1, nil)
	p1 := gn.newPuller("p1", policyStore, factoryMock1, membership(peerData{"p2", uint64(1)}, peerData{"p3", uint64(1)})...)

	p2TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col1",
				},
			},
		},
	}
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p1")
	factoryMock2 := &collectionAccessFactoryMock{}
	policyMock2 := &collectionAccessPolicyMock{}
	policyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock2.On("AccessPolicy", mock.Anything, mock.Anything).Return(policyMock2, nil)

	p2 := gn.newPuller("p2", policyStore, factoryMock2)
	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: p2TransientStore,
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Return(store, true, nil)

	factoryMock3 := &collectionAccessFactoryMock{}
	policyMock3 := &collectionAccessPolicyMock{}
	policyMock3.Setup(1, 2, func(data fcommon.SignedData) bool {
		return false
	}, []string{"org1", "org2"}, false)
	factoryMock3.On("AccessPolicy", mock.Anything, mock.Anything).Return(policyMock3, nil)

	p3 := gn.newPuller("p3", newCollectionStore(), factoryMock3)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	dasf := &digestsAndSourceFactory{}

	fetchedMessages, err := p1.fetch(dasf.mapDigest(toDigKey(dig)).toSources().create())
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, p2TransientStore.RWSet, fetched)
}

func TestPullerDataNotAvailable(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and not from p3
	// but the data in p2 doesn't exist
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock := &collectionAccessFactoryMock{}
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(&collectionAccessPolicyMock{}, nil)

	p1 := gn.newPuller("p1", policyStore, factoryMock, membership(peerData{"p2", uint64(1)}, peerData{"p3", uint64(1)})...)

	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p1")
	p2 := gn.newPuller("p2", policyStore, factoryMock)
	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: &util.PrivateRWSetWithConfig{
			RWSet: []util.PrivateRWSet{},
		},
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), mock.Anything).Return(store, true, nil)

	p3 := gn.newPuller("p3", newCollectionStore(), factoryMock)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), mock.Anything).Run(func(_ mock.Arguments) {
		t.Fatal("p3 shouldn't have been selected for pull")
	})

	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(toDigKey(dig)).toSources().create())
	assert.Empty(t, fetchedMessages.AvailableElements)
	assert.NoError(t, err)
}

func TestPullerNoPeersKnown(t *testing.T) {
	t.Parallel()
	// Scenario: p1 doesn't know any peer and therefore fails fetching
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2", "p3")
	factoryMock := &collectionAccessFactoryMock{}
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(&collectionAccessPolicyMock{}, nil)

	p1 := gn.newPuller("p1", policyStore, factoryMock)
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&privdatacommon.DigKey{Collection: "col1", TxId: "txID1", Namespace: "ns1"}).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.Empty(t, fetchedMessages)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Empty membership")
}

func TestPullPeerFilterError(t *testing.T) {
	t.Parallel()
	// Scenario: p1 attempts to fetch for the wrong channel
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock := &collectionAccessFactoryMock{}
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(&collectionAccessPolicyMock{}, nil)

	p1 := gn.newPuller("p1", policyStore, factoryMock)
	gn.peers[0].On("PeerFilter", mock.Anything, mock.Anything).Return(nil, errors.New("Failed obtaining filter"))
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&privdatacommon.DigKey{Collection: "col1", TxId: "txID1", Namespace: "ns1"}).toSources().create()
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
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2", "p3")
	factoryMock1 := &collectionAccessFactoryMock{}
	accessPolicyMock1 := &collectionAccessPolicyMock{}
	accessPolicyMock1.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2")) || bytes.Equal(data.Identity, []byte("p3"))
	}, []string{"org1", "org2"}, false)
	factoryMock1.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock1, nil)

	p1 := gn.newPuller("p1", policyStore, factoryMock1, membership(peerData{"p2", uint64(1)}, peerData{"p3", uint64(1)})...)

	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock2 := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2"))
	}, []string{"org1", "org2"}, false)
	factoryMock2.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	p2 := gn.newPuller("p2", policyStore, factoryMock2)

	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: &util.PrivateRWSetWithConfig{
			RWSet: newPRWSet(),
			CollectionConfig: &fcommon.CollectionConfig{
				Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &fcommon.StaticCollectionConfig{
						Name: "col1",
					},
				},
			},
		},
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), mock.Anything).Return(store, true, nil)

	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p3")
	factoryMock3 := &collectionAccessFactoryMock{}
	accessPolicyMock3 := &collectionAccessPolicyMock{}
	accessPolicyMock3.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p3"))
	}, []string{"org1", "org2"}, false)
	factoryMock3.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock1, nil)

	p3 := gn.newPuller("p3", policyStore, factoryMock3)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), mock.Anything).Return(store, true, nil)
	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(&privdatacommon.DigKey{Collection: "col1", TxId: "txID1", Namespace: "ns1"}).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.Empty(t, fetchedMessages.AvailableElements)
	assert.NoError(t, err)
}

func TestPullerDifferentPeersDifferentCollections(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and from p3
	// and each has different collections
	gn := &gossipNetwork{}
	factoryMock1 := &collectionAccessFactoryMock{}
	accessPolicyMock1 := &collectionAccessPolicyMock{}
	accessPolicyMock1.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2")) || bytes.Equal(data.Identity, []byte("p3"))
	}, []string{"org1", "org2"}, false)
	factoryMock1.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock1, nil)

	policyStore := newCollectionStore().withPolicy("col2", uint64(100)).thatMapsTo("p2").withPolicy("col3", uint64(100)).thatMapsTo("p3")
	p1 := gn.newPuller("p1", policyStore, factoryMock1, membership(peerData{"p2", uint64(1)}, peerData{"p3", uint64(1)})...)

	p2TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	policyStore = newCollectionStore().withPolicy("col2", uint64(100)).thatMapsTo("p1")
	factoryMock2 := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock2.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	p2 := gn.newPuller("p2", policyStore, factoryMock2)
	dig1 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col2",
		Namespace:  "ns1",
	}

	store1 := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col2",
			Namespace:  "ns1",
		}: p2TransientStore,
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig1)), mock.Anything).Return(store1, true, nil)

	p3TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col3",
				},
			},
		},
	}

	store2 := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col3",
			Namespace:  "ns1",
		}: p3TransientStore,
	}
	policyStore = newCollectionStore().withPolicy("col3", uint64(100)).thatMapsTo("p1")
	factoryMock3 := &collectionAccessFactoryMock{}
	accessPolicyMock3 := &collectionAccessPolicyMock{}
	accessPolicyMock3.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock3.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock3, nil)

	p3 := gn.newPuller("p3", policyStore, factoryMock3)
	dig2 := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col3",
		Namespace:  "ns1",
	}

	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig2)), mock.Anything).Return(store2, true, nil)

	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(toDigKey(dig1)).toSources().mapDigest(toDigKey(dig2)).toSources().create())
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	rws3 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[0])
	rws4 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2, rws3, rws4}
	assert.Contains(t, fetched, p2TransientStore.RWSet[0])
	assert.Contains(t, fetched, p2TransientStore.RWSet[1])
	assert.Contains(t, fetched, p3TransientStore.RWSet[0])
	assert.Contains(t, fetched, p3TransientStore.RWSet[1])
}

func TestPullerRetries(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2, p3, p4 and p5.
	// Only p3 considers p1 to be eligible to receive the data.
	// The rest consider p1 as not eligible.
	gn := &gossipNetwork{}
	factoryMock1 := &collectionAccessFactoryMock{}
	accessPolicyMock1 := &collectionAccessPolicyMock{}
	accessPolicyMock1.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2")) || bytes.Equal(data.Identity, []byte("p3")) ||
			bytes.Equal(data.Identity, []byte("p4")) ||
			bytes.Equal(data.Identity, []byte("p5"))
	}, []string{"org1", "org2"}, false)
	factoryMock1.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock1, nil)

	// p1
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2", "p3", "p4", "p5")
	p1 := gn.newPuller("p1", policyStore, factoryMock1, membership(peerData{"p2", uint64(1)},
		peerData{"p3", uint64(1)}, peerData{"p4", uint64(1)}, peerData{"p5", uint64(1)})...)

	// p2, p3, p4, and p5 have the same transient store
	transientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col1",
				},
			},
		},
	}

	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: transientStore,
	}

	// p2
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock2 := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2"))
	}, []string{"org1", "org2"}, false)
	factoryMock2.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	p2 := gn.newPuller("p2", policyStore, factoryMock2)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Return(store, true, nil)

	// p3
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p1")
	factoryMock3 := &collectionAccessFactoryMock{}
	accessPolicyMock3 := &collectionAccessPolicyMock{}
	accessPolicyMock3.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock3.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock3, nil)

	p3 := gn.newPuller("p3", policyStore, factoryMock3)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Return(store, true, nil)

	// p4
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p4")
	factoryMock4 := &collectionAccessFactoryMock{}
	accessPolicyMock4 := &collectionAccessPolicyMock{}
	accessPolicyMock4.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p4"))
	}, []string{"org1", "org2"}, false)
	factoryMock4.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock4, nil)

	p4 := gn.newPuller("p4", policyStore, factoryMock4)
	p4.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Return(store, true, nil)

	// p5
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p5")
	factoryMock5 := &collectionAccessFactoryMock{}
	accessPolicyMock5 := &collectionAccessPolicyMock{}
	accessPolicyMock5.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p5"))
	}, []string{"org1", "org2"}, false)
	factoryMock5.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock5, nil)

	p5 := gn.newPuller("p5", policyStore, factoryMock5)
	p5.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)), uint64(0)).Return(store, true, nil)

	// Fetch from someone
	dasf := &digestsAndSourceFactory{}
	fetchedMessages, err := p1.fetch(dasf.mapDigest(toDigKey(dig)).toSources().create())
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, transientStore.RWSet, fetched)
}

func TestPullerPreferEndorsers(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2, p3, p4, p5
	// and the only endorser for col1 is p3, so it should be selected
	// at the top priority for col1.
	// for col2, only p2 should have the data, but its not an endorser of the data.
	gn := &gossipNetwork{}
	factoryMock := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2")) || bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	policyStore := newCollectionStore().
		withPolicy("col1", uint64(100)).
		thatMapsTo("p1", "p2", "p3", "p4", "p5").
		withPolicy("col2", uint64(100)).
		thatMapsTo("p1", "p2")
	p1 := gn.newPuller("p1", policyStore, factoryMock, membership(peerData{"p2", uint64(1)},
		peerData{"p3", uint64(1)}, peerData{"p4", uint64(1)}, peerData{"p5", uint64(1)})...)

	p3TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	p2TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	p2 := gn.newPuller("p2", policyStore, factoryMock)
	p3 := gn.newPuller("p3", policyStore, factoryMock)
	gn.newPuller("p4", policyStore, factoryMock)
	gn.newPuller("p5", policyStore, factoryMock)

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

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: p3TransientStore,
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col2",
			Namespace:  "ns1",
		}: p2TransientStore,
	}

	// We only define an action for dig2 on p2, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig2
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig2)), uint64(0)).Return(store, true, nil)

	// We only define an action for dig1 on p3, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig1
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig1)), uint64(0)).Return(store, true, nil)

	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(toDigKey(dig1)).toSources("p3").mapDigest(toDigKey(dig2)).toSources().create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	rws3 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[0])
	rws4 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2, rws3, rws4}
	assert.Contains(t, fetched, p3TransientStore.RWSet[0])
	assert.Contains(t, fetched, p3TransientStore.RWSet[1])
	assert.Contains(t, fetched, p2TransientStore.RWSet[0])
	assert.Contains(t, fetched, p2TransientStore.RWSet[1])
}

func TestPullerFetchReconciledItemsPreferPeersFromOriginalConfig(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2, p3, p4, p5
	// the only peer that was in the collection config while data was created for col1 is p3, so it should be selected
	// at the top priority for col1.
	// for col2, p3 was in the collection config while the data was created but was removed from collection and now only p2 should have the data.
	// so obviously p2 should be selected for col2.
	gn := &gossipNetwork{}
	factoryMock := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2")) || bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	policyStore := newCollectionStore().
		withPolicy("col1", uint64(100)).
		thatMapsTo("p1", "p2", "p3", "p4", "p5").
		withPolicy("col2", uint64(100)).
		thatMapsTo("p1", "p2").
		withAccessFilter(func(data fcommon.SignedData) bool {
			return bytes.Equal(data.Identity, []byte("p3"))
		})

	p1 := gn.newPuller("p1", policyStore, factoryMock, membership(peerData{"p2", uint64(1)},
		peerData{"p3", uint64(1)}, peerData{"p4", uint64(1)}, peerData{"p5", uint64(1)})...)

	p3TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	p2TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	p2 := gn.newPuller("p2", policyStore, factoryMock)
	p3 := gn.newPuller("p3", policyStore, factoryMock)
	gn.newPuller("p4", policyStore, factoryMock)
	gn.newPuller("p5", policyStore, factoryMock)

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

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: p3TransientStore,
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col2",
			Namespace:  "ns1",
		}: p2TransientStore,
	}

	// We only define an action for dig2 on p2, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig2
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig2)), uint64(0)).Return(store, true, nil)

	// We only define an action for dig1 on p3, and the test would fail with panic if any other peer is asked for
	// a private RWSet on dig1
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig1)), uint64(0)).Return(store, true, nil)

	d2cc := privdatacommon.Dig2CollectionConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: &fcommon.StaticCollectionConfig{
			Name: "col1",
		},
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col2",
			Namespace:  "ns1",
		}: &fcommon.StaticCollectionConfig{
			Name: "col2",
		},
	}

	fetchedMessages, err := p1.FetchReconciledItems(d2cc)
	assert.NoError(t, err)
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	rws3 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[0])
	rws4 := util.PrivateRWSet(fetchedMessages.AvailableElements[1].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2, rws3, rws4}
	assert.Contains(t, fetched, p3TransientStore.RWSet[0])
	assert.Contains(t, fetched, p3TransientStore.RWSet[1])
	assert.Contains(t, fetched, p2TransientStore.RWSet[0])
	assert.Contains(t, fetched, p2TransientStore.RWSet[1])
}

func TestPullerAvoidPullingPurgedData(t *testing.T) {
	// Scenario: p1 missing private data for col1
	// p2 and p3 is suppose to have it, while p3 has more advanced
	// ledger and based on BTL already purged data for, so p1
	// suppose to fetch data only from p2

	t.Parallel()
	gn := &gossipNetwork{}
	factoryMock := &collectionAccessFactoryMock{}
	accessPolicyMock2 := &collectionAccessPolicyMock{}
	accessPolicyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(accessPolicyMock2, nil)

	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p1", "p2", "p3").
		withPolicy("col2", uint64(1000)).thatMapsTo("p1", "p2", "p3")

	// p2 is at ledger height 1, while p2 is at 111 which is beyond BTL defined for col1 (100)
	p1 := gn.newPuller("p1", policyStore, factoryMock, membership(peerData{"p2", uint64(1)},
		peerData{"p3", uint64(111)})...)

	privateData1 := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col1",
				},
			},
		},
	}
	privateData2 := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col2",
				},
			},
		},
	}

	p2 := gn.newPuller("p2", policyStore, factoryMock)
	p3 := gn.newPuller("p3", policyStore, factoryMock)

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

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: privateData1,
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col2",
			Namespace:  "ns1",
		}: privateData2,
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig1)), 0).Return(store, true, nil)
	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig1)), 0).Return(store, true, nil).
		Run(
			func(arg mock.Arguments) {
				assert.Fail(t, "we should not fetch private data from peers where it was purged")
			},
		)

	p3.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig2)), uint64(0)).Return(store, true, nil)
	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig2)), uint64(0)).Return(store, true, nil).
		Run(
			func(mock.Arguments) {
				assert.Fail(t, "we should not fetch private data of collection2 from peer 2")

			},
		)

	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(toDigKey(dig1)).toSources("p3", "p2").mapDigest(toDigKey(dig2)).toSources("p3").create()
	// trying to fetch missing pvt data for block seq 1
	fetchedMessages, err := p1.fetch(d2s)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(fetchedMessages.PurgedElements))
	assert.Equal(t, dig1, fetchedMessages.PurgedElements[0])
	p3.PrivateDataRetriever.(*dataRetrieverMock).AssertNumberOfCalls(t, "CollectionRWSet", 1)

}

type counterDataRetreiver struct {
	numberOfCalls int
	PrivateDataRetriever
}

func (c *counterDataRetreiver) CollectionRWSet(dig []*proto.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, bool, error) {
	c.numberOfCalls += 1
	return c.PrivateDataRetriever.CollectionRWSet(dig, blockNum)
}

func (c *counterDataRetreiver) getNumberOfCalls() int {
	return c.numberOfCalls
}

func TestPullerIntegratedWithDataRetreiver(t *testing.T) {
	t.Parallel()
	gn := &gossipNetwork{}

	ns1, ns2 := "testChaincodeName1", "testChaincodeName2"
	col1, col2 := "testCollectionName1", "testCollectionName2"

	ap := &collectionAccessPolicyMock{}
	ap.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)

	factoryMock := &collectionAccessFactoryMock{}
	factoryMock.On("AccessPolicy", mock.Anything, mock.Anything).Return(ap, nil)

	policyStore := newCollectionStore().withPolicy(col1, uint64(1000)).thatMapsTo("p1", "p2").
		withPolicy(col2, uint64(1000)).thatMapsTo("p1", "p2")

	p1 := gn.newPuller("p1", policyStore, factoryMock, membership(peerData{"p2", uint64(10)})...)
	p2 := gn.newPuller("p2", policyStore, factoryMock, membership(peerData{"p1", uint64(1)})...)

	dataStore := &mocks.DataStore{}
	result := []*ledger.TxPvtData{
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns1, col1, []byte{1}),
					pvtReadWriteSet(ns1, col1, []byte{2}),
				},
			},
			SeqInBlock: 1,
		},
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns2, col2, []byte{3}),
					pvtReadWriteSet(ns2, col2, []byte{4}),
				},
			},
			SeqInBlock: 2,
		},
	}

	dataStore.On("LedgerHeight").Return(uint64(10), nil)
	dataStore.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns1).Return(newCollectionConfig(col1), nil)
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns2).Return(newCollectionConfig(col2), nil)
	dataStore.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	dataRetreiver := &counterDataRetreiver{PrivateDataRetriever: NewDataRetriever(dataStore), numberOfCalls: 0}
	p2.PrivateDataRetriever = dataRetreiver

	dig1 := &privdatacommon.DigKey{
		TxId:       "txID1",
		Collection: col1,
		Namespace:  ns1,
		BlockSeq:   5,
		SeqInBlock: 1,
	}

	dig2 := &privdatacommon.DigKey{
		TxId:       "txID1",
		Collection: col2,
		Namespace:  ns2,
		BlockSeq:   5,
		SeqInBlock: 2,
	}

	dasf := &digestsAndSourceFactory{}
	d2s := dasf.mapDigest(dig1).toSources("p2").mapDigest(dig2).toSources("p2").create()
	fetchedMessages, err := p1.fetch(d2s)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(fetchedMessages.AvailableElements))
	assert.Equal(t, 1, dataRetreiver.getNumberOfCalls())
	assert.Equal(t, 2, len(fetchedMessages.AvailableElements[0].Payload))
	assert.Equal(t, 2, len(fetchedMessages.AvailableElements[1].Payload))
}

func toDigKey(dig *proto.PvtDataDigest) *privdatacommon.DigKey {
	return &privdatacommon.DigKey{
		TxId:       dig.TxId,
		BlockSeq:   dig.BlockSeq,
		SeqInBlock: dig.SeqInBlock,
		Namespace:  dig.Namespace,
		Collection: dig.Collection,
	}
}

func TestPullerMetrics(t *testing.T) {
	t.Parallel()
	// Scenario: p1 pulls from p2 and sends metric reports
	gn := &gossipNetwork{}
	policyStore := newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p2")
	factoryMock1 := &collectionAccessFactoryMock{}
	policyMock1 := &collectionAccessPolicyMock{}
	policyMock1.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p2"))
	}, []string{"org1", "org2"}, false)
	factoryMock1.On("AccessPolicy", mock.Anything, mock.Anything).Return(policyMock1, nil)

	testMetricProvider := gmetricsmocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	p1 := gn.newPullerWithMetrics(metrics, "p1", policyStore, factoryMock1, membership(peerData{"p2", uint64(1)})...)

	p2TransientStore := &util.PrivateRWSetWithConfig{
		RWSet: newPRWSet(),
		CollectionConfig: &fcommon.CollectionConfig{
			Payload: &fcommon.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &fcommon.StaticCollectionConfig{
					Name: "col1",
				},
			},
		},
	}
	policyStore = newCollectionStore().withPolicy("col1", uint64(100)).thatMapsTo("p1")
	factoryMock2 := &collectionAccessFactoryMock{}
	policyMock2 := &collectionAccessPolicyMock{}
	policyMock2.Setup(1, 2, func(data fcommon.SignedData) bool {
		return bytes.Equal(data.Identity, []byte("p1"))
	}, []string{"org1", "org2"}, false)
	factoryMock2.On("AccessPolicy", mock.Anything, mock.Anything).Return(policyMock2, nil)

	p2 := gn.newPullerWithMetrics(metrics, "p2", policyStore, factoryMock2)

	dig := &proto.PvtDataDigest{
		TxId:       "txID1",
		Collection: "col1",
		Namespace:  "ns1",
	}

	store := Dig2PvtRWSetWithConfig{
		privdatacommon.DigKey{
			TxId:       "txID1",
			Collection: "col1",
			Namespace:  "ns1",
		}: p2TransientStore,
	}

	p2.PrivateDataRetriever.(*dataRetrieverMock).On("CollectionRWSet", mock.MatchedBy(protoMatcher(dig)),
		uint64(0)).Return(store, true, nil)

	dasf := &digestsAndSourceFactory{}

	fetchedMessages, err := p1.fetch(dasf.mapDigest(toDigKey(dig)).toSources().create())
	rws1 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[0])
	rws2 := util.PrivateRWSet(fetchedMessages.AvailableElements[0].Payload[1])
	fetched := []util.PrivateRWSet{rws1, rws2}
	assert.NoError(t, err)
	assert.Equal(t, p2TransientStore.RWSet, fetched)

	assert.Equal(t,
		[]string{"channel", "A"},
		testMetricProvider.FakePullDuration.WithArgsForCall(0),
	)
	assert.True(t, testMetricProvider.FakePullDuration.ObserveArgsForCall(0) > 0)
	assert.Equal(t,
		[]string{"channel", "A"},
		testMetricProvider.FakeRetrieveDuration.WithArgsForCall(0),
	)
	assert.True(t, testMetricProvider.FakeRetrieveDuration.ObserveArgsForCall(0) > 0)
}
