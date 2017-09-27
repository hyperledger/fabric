/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type gossipMock struct {
	err error
	mock.Mock
	api.PeerSignature
}

func (g *gossipMock) SendByCriteria(message *proto.SignedGossipMessage, criteria gossip2.SendCriteria) error {
	args := g.Called(message, criteria)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return nil
}

func (g *gossipMock) PeerFilter(channel gcommon.ChainID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	if g.err != nil {
		return nil, g.err
	}
	return func(member discovery.NetworkMember) bool {
		var fromOurOrg bool
		if string(member.PKIid) == "ORG1" {
			fromOurOrg = true
		}
		return messagePredicate(g.PeerSignature, fromOurOrg)
	}, nil
}

func TestDistributor(t *testing.T) {
	viper.Set("peer.gossip.pvtData.minInternalPeers", 1)
	viper.Set("peer.gossip.pvtData.maxInternalPeers", 2)
	viper.Set("peer.gossip.pvtData.minExternalPeers", 3)
	viper.Set("peer.gossip.pvtData.maxExternalPeers", 4)
	g := &gossipMock{
		Mock: mock.Mock{},
		PeerSignature: api.PeerSignature{
			Signature:    []byte{3, 4, 5},
			Message:      []byte{6, 7, 8},
			PeerIdentity: []byte{0, 1, 2},
		},
	}
	sendings := make(chan struct {
		*proto.PrivatePayload
		gossip2.SendCriteria
	}, 8)
	g.On("SendByCriteria", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		msg := args.Get(0).(*proto.SignedGossipMessage)
		sendCriteria := args.Get(1).(gossip2.SendCriteria)
		sendings <- struct {
			*proto.PrivatePayload
			gossip2.SendCriteria
		}{
			PrivatePayload: msg.GetPrivateData().Payload,
			SendCriteria:   sendCriteria,
		}
	}).Return(nil)
	d := NewDistributor("test", g)
	pdFactory := &pvtDataFactory{}
	peerSelfSignedData := common.SignedData{
		Identity:  []byte{0, 1, 2},
		Signature: []byte{3, 4, 5},
		Data:      []byte{6, 7, 8},
	}
	cs := createcollectionStore(peerSelfSignedData).thatAccepts(common.CollectionCriteria{
		TxId:       "tx1",
		Namespace:  "ns1",
		Channel:    "test",
		Collection: "c1",
	}).thatAccepts(common.CollectionCriteria{
		TxId:       "tx2",
		Namespace:  "ns2",
		Channel:    "test",
		Collection: "c2",
	})
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1", "c2").create()
	// Each private RWSet is sent to inside our org, and outside of the org, totalling in 8 sends.
	err := d.Distribute("tx1", pvtData[0].WriteSet, cs)
	assert.NoError(t, err)
	err = d.Distribute("tx2", pvtData[1].WriteSet, cs)
	assert.NoError(t, err)

	assertACL := func(pp *proto.PrivatePayload, sc gossip2.SendCriteria) {
		eligible := pp.Namespace == "ns1" && pp.CollectionName == "c1"
		eligible = eligible || (pp.Namespace == "ns2" && pp.CollectionName == "c2")
		// The mock collection store returns policies that for ns1 and c1, or ns2 and c2 return true regardless
		// of the network member, and for any other collection and namespace combination - return false

		// If this is a dissemination to a foreign org, ensure MaxPeers is maxExternalPeers which is 4
		// and MinAck is minExternalPeers which is is 3
		foreignOrg := sc.IsEligible(discovery.NetworkMember{PKIid: gcommon.PKIidType("ORG2")})
		if foreignOrg {
			assert.Equal(t, 4, sc.MaxPeers)
			assert.Equal(t, 3, sc.MinAck)
		}

		// If this is a dissemination within the org
		intraOrg := sc.IsEligible(discovery.NetworkMember{PKIid: gcommon.PKIidType("ORG1")})
		if intraOrg {
			// Ensure MaxPeers is maxInternalPeers which is 2
			//and MinAck is minInternalPeers which is 1
			assert.Equal(t, 2, sc.MaxPeers)
			assert.Equal(t, 1, sc.MinAck)
		}

		// If this private payload isn't allowed to be disseminated to any org,
		// ensure this is because the private data is either ns1 and c2 or ns2 and c1.
		// Otherwise, this private data is allowed to be disseminated to someone,
		// so it has to be ns1 and c1 or ns2 and c2
		if !foreignOrg && !intraOrg {
			assert.True(t, (pp.Namespace == "ns1" && pp.CollectionName == "c2") || (pp.Namespace == "ns2" && pp.CollectionName == "c1"))
		} else {
			assert.True(t, (pp.Namespace == "ns1" && pp.CollectionName == "c1") || (pp.Namespace == "ns2" && pp.CollectionName == "c2"))
		}
	}
	i := 0
	for dis := range sendings {
		assertACL(dis.PrivatePayload, dis.SendCriteria)
		i++
		if i == 8 {
			break
		}
	}
	// Channel is empty after we read 8 times from it
	assert.Len(t, sendings, 0)

	// Bad path: dependencies (gossip and others) don't work properly
	g.err = errors.New("failed obtaining filter")
	err = d.Distribute("tx1", pvtData[0].WriteSet, cs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed obtaining filter")

	g.Mock = mock.Mock{}
	g.On("SendByCriteria", mock.Anything, mock.Anything).Return(errors.New("failed sending"))
	g.err = nil
	err = d.Distribute("tx1", pvtData[0].WriteSet, cs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed disseminating 4 out of 4 private RWSets")
}
