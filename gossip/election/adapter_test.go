/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package election

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
}

func TestNewAdapter(t *testing.T) {
	selfNetworkMember := &discovery.NetworkMember{
		Endpoint: "p0",
		Metadata: []byte{},
		PKIid:    []byte{byte(0)},
	}
	mockGossip := newGossip("peer0", selfNetworkMember)

	peersCluster := newClusterOfPeers("0")
	peersCluster.addPeer("peer0", mockGossip)

	NewAdapter(mockGossip, selfNetworkMember.PKIid, []byte("channel0"),
		metrics.NewGossipMetrics(&disabled.Provider{}).ElectionMetrics)
}

func TestAdapterImpl_CreateMessage(t *testing.T) {
	selfNetworkMember := &discovery.NetworkMember{
		Endpoint: "p0",
		Metadata: []byte{},
		PKIid:    []byte{byte(0)},
	}
	mockGossip := newGossip("peer0", selfNetworkMember)

	adapter := NewAdapter(mockGossip, selfNetworkMember.PKIid, []byte("channel0"),
		metrics.NewGossipMetrics(&disabled.Provider{}).ElectionMetrics)
	msg := adapter.CreateMessage(true)

	if !msg.(*msgImpl).msg.IsLeadershipMsg() {
		t.Error("Newly created message should be LeadershipMsg")
	}

	if !msg.IsDeclaration() {
		t.Error("Newly created msg should be Declaration msg")
	}

	msg = adapter.CreateMessage(false)

	if !msg.(*msgImpl).msg.IsLeadershipMsg() {
		t.Error("Newly created message should be LeadershipMsg")
	}

	if !msg.IsProposal() || msg.IsDeclaration() {
		t.Error("Newly created msg should be Proposal msg")
	}
}

func TestAdapterImpl_Peers(t *testing.T) {
	_, adapters := createCluster(0, 1, 2, 3, 4, 5)

	peersPKIDs := make(map[string]string)
	peersPKIDs[string([]byte{0})] = string([]byte{0})
	peersPKIDs[string([]byte{1})] = string([]byte{1})
	peersPKIDs[string([]byte{2})] = string([]byte{2})
	peersPKIDs[string([]byte{3})] = string([]byte{2})
	peersPKIDs[string([]byte{4})] = string([]byte{4})
	peersPKIDs[string([]byte{5})] = string([]byte{5})

	for _, adapter := range adapters {
		peers := adapter.Peers()
		if len(peers) != 6 {
			t.Errorf("Should return 6 peers, not %d", len(peers))
		}

		for _, peer := range peers {
			if _, exist := peersPKIDs[string(peer.ID())]; !exist {
				t.Errorf("Peer %s PKID not found", peer.(*peerImpl).member.Endpoint)
			}
		}
	}

}

func TestAdapterImpl_Stop(t *testing.T) {
	_, adapters := createCluster(0, 1, 2, 3, 4, 5)

	for _, adapter := range adapters {
		adapter.Accept()
	}

	for _, adapter := range adapters {
		adapter.Stop()
	}
}

func TestAdapterImpl_Gossip(t *testing.T) {
	_, adapters := createCluster(0, 1, 2)

	channels := make(map[string]<-chan Msg)

	for peerID, adapter := range adapters {
		channels[peerID] = adapter.Accept()
	}

	sender := adapters[fmt.Sprintf("Peer%d", 0)]

	sender.Gossip(sender.CreateMessage(true))

	totalMsg := 0

	timer := time.After(time.Duration(1) * time.Second)

	for {
		select {
		case <-timer:
			if totalMsg != 2 {
				t.Error("Not all messages accepted")
				t.FailNow()
			} else {
				return
			}
		case msg := <-channels[fmt.Sprintf("Peer%d", 1)]:
			if !msg.IsDeclaration() {
				t.Error("Msg should be declaration")
			} else if !bytes.Equal(msg.SenderID(), sender.selfPKIid) {
				t.Error("Msg Sender is wrong")
			} else {
				totalMsg++
			}
		case msg := <-channels[fmt.Sprintf("Peer%d", 2)]:
			if !msg.IsDeclaration() {
				t.Error("Msg should be declaration")
			} else if !bytes.Equal(msg.SenderID(), sender.selfPKIid) {
				t.Error("Msg Sender is wrong")
			} else {
				totalMsg++
			}
		}

	}

}

type mockAcceptor struct {
	ch       chan *proto.GossipMessage
	acceptor common.MessageAcceptor
}

type peerMockGossip struct {
	cluster      *clusterOfPeers
	member       *discovery.NetworkMember
	acceptors    []*mockAcceptor
	acceptorLock *sync.RWMutex
	clusterLock  *sync.RWMutex
	id           string
}

func (g *peerMockGossip) Peers() []discovery.NetworkMember {

	g.clusterLock.RLock()
	if g.cluster == nil {
		return []discovery.NetworkMember{*g.member}
	}
	peerLock := g.cluster.peersLock
	g.clusterLock.RUnlock()

	peerLock.RLock()
	res := make([]discovery.NetworkMember, 0)
	g.clusterLock.RLock()
	for _, val := range g.cluster.peersGossip {
		res = append(res, *val.member)

	}
	g.clusterLock.RUnlock()
	peerLock.RUnlock()
	return res
}

func (g *peerMockGossip) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage) {
	ch := make(chan *proto.GossipMessage, 100)
	g.acceptorLock.Lock()
	g.acceptors = append(g.acceptors, &mockAcceptor{
		ch:       ch,
		acceptor: acceptor,
	})
	g.acceptorLock.Unlock()
	return ch, nil
}

func (g *peerMockGossip) Gossip(msg *proto.GossipMessage) {
	g.clusterLock.RLock()
	if g.cluster == nil {
		return
	}
	peersLock := g.cluster.peersLock
	g.clusterLock.RUnlock()

	peersLock.RLock()
	g.clusterLock.RLock()
	for _, val := range g.cluster.peersGossip {
		if strings.Compare(val.id, g.id) != 0 {
			val.putToAcceptors(msg)
		}
	}
	g.clusterLock.RUnlock()
	peersLock.RUnlock()

}

func (g *peerMockGossip) putToAcceptors(msg *proto.GossipMessage) {
	g.acceptorLock.RLock()
	for _, acceptor := range g.acceptors {
		if acceptor.acceptor(msg) {
			if len(acceptor.ch) < 10 {
				acceptor.ch <- msg
			}
		}
	}
	g.acceptorLock.RUnlock()

}

func newGossip(peerID string, member *discovery.NetworkMember) *peerMockGossip {
	return &peerMockGossip{
		id:           peerID,
		member:       member,
		acceptorLock: &sync.RWMutex{},
		clusterLock:  &sync.RWMutex{},
		acceptors:    make([]*mockAcceptor, 0),
	}
}

type clusterOfPeers struct {
	peersGossip map[string]*peerMockGossip
	peersLock   *sync.RWMutex
	id          string
}

func (cop *clusterOfPeers) addPeer(peerID string, gossip *peerMockGossip) {
	cop.peersLock.Lock()
	cop.peersGossip[peerID] = gossip
	gossip.clusterLock.Lock()
	gossip.cluster = cop
	gossip.clusterLock.Unlock()
	cop.peersLock.Unlock()

}

func newClusterOfPeers(id string) *clusterOfPeers {
	return &clusterOfPeers{
		id:          id,
		peersGossip: make(map[string]*peerMockGossip),
		peersLock:   &sync.RWMutex{},
	}

}

func createCluster(peers ...int) (*clusterOfPeers, map[string]*adapterImpl) {
	adapters := make(map[string]*adapterImpl)
	cluster := newClusterOfPeers("0")
	for _, peer := range peers {
		peerEndpoint := fmt.Sprintf("Peer%d", peer)
		peerPKID := []byte{byte(peer)}
		peerMember := &discovery.NetworkMember{
			Metadata: []byte{},
			Endpoint: peerEndpoint,
			PKIid:    peerPKID,
		}

		mockGossip := newGossip(peerEndpoint, peerMember)
		adapter := NewAdapter(mockGossip, peerMember.PKIid, []byte("channel0"),
			metrics.NewGossipMetrics(&disabled.Provider{}).ElectionMetrics)
		adapters[peerEndpoint] = adapter.(*adapterImpl)
		cluster.addPeer(peerEndpoint, mockGossip)
	}

	return cluster, adapters
}

func TestReportMetrics(t *testing.T) {

	testMetricProvider := mocks.TestUtilConstructMetricProvider()
	electionMetrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).ElectionMetrics

	mockGossip := newGossip("", &discovery.NetworkMember{})
	adapter := NewAdapter(mockGossip, nil, []byte("channel0"), electionMetrics)

	adapter.ReportMetrics(true)

	assert.Equal(t,
		[]string{"channel", "channel0"},
		testMetricProvider.FakeDeclarationGauge.WithArgsForCall(0),
	)
	assert.EqualValues(t,
		1,
		testMetricProvider.FakeDeclarationGauge.SetArgsForCall(0),
	)

	adapter.ReportMetrics(false)

	assert.Equal(t,
		[]string{"channel", "channel0"},
		testMetricProvider.FakeDeclarationGauge.WithArgsForCall(1),
	)
	assert.EqualValues(t,
		0,
		testMetricProvider.FakeDeclarationGauge.SetArgsForCall(1),
	)

}
