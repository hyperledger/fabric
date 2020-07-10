/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"fmt"
	"testing"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	mocks2 "github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Setup(mock *mocks2.CollectionAccessPolicy, requiredPeerCount int, maxPeerCount int,
	accessFilter privdata.Filter, orgs map[string]struct{}, memberOnlyRead bool) {
	mock.On("AccessFilter").Return(accessFilter)
	mock.On("RequiredPeerCount").Return(requiredPeerCount)
	mock.On("MaximumPeerCount").Return(maxPeerCount)
	mock.On("MemberOrgs").Return(orgs)
	mock.On("IsMemberOnlyRead").Return(memberOnlyRead)
}

type gossipMock struct {
	err error
	mock.Mock
	api.PeerSignature
}

func (g *gossipMock) IdentityInfo() api.PeerIdentitySet {
	return g.Called().Get(0).(api.PeerIdentitySet)
}

func (g *gossipMock) PeersOfChannel(channelID gcommon.ChannelID) []discovery.NetworkMember {
	return g.Called(channelID).Get(0).([]discovery.NetworkMember)
}

func (g *gossipMock) SendByCriteria(message *protoext.SignedGossipMessage, criteria gossip2.SendCriteria) error {
	args := g.Called(message, criteria)
	if args.Get(0) != nil {
		return args.Get(0).(error)
	}
	return nil
}

func (g *gossipMock) PeerFilter(channel gcommon.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	if g.err != nil {
		return nil, g.err
	}
	return func(member discovery.NetworkMember) bool {
		return messagePredicate(g.PeerSignature)
	}, nil
}

func TestDistributor(t *testing.T) {
	channelID := "test"

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

	g.On("PeersOfChannel", gcommon.ChannelID(channelID)).Return([]discovery.NetworkMember{
		{PKIid: gcommon.PKIidType{1}},
		{PKIid: gcommon.PKIidType{2}},
	})

	g.On("IdentityInfo").Return(api.PeerIdentitySet{
		{
			PKIId:        gcommon.PKIidType{1},
			Organization: api.OrgIdentityType("org1"),
		},
		{
			PKIId:        gcommon.PKIidType{2},
			Organization: api.OrgIdentityType("org2"),
		},
	})

	g.On("SendByCriteria", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		msg := args.Get(0).(*protoext.SignedGossipMessage)
		sendCriteria := args.Get(1).(gossip2.SendCriteria)
		sendings <- struct {
			*proto.PrivatePayload
			gossip2.SendCriteria
		}{
			PrivatePayload: msg.GetPrivateData().Payload,
			SendCriteria:   sendCriteria,
		}
	}).Return(nil)
	accessFactoryMock := &mocks2.CollectionAccessFactory{}
	c1ColConfig := &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:              "c1",
				RequiredPeerCount: 1,
				MaximumPeerCount:  1,
			},
		},
	}

	c2ColConfig := &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:              "c2",
				RequiredPeerCount: 1,
				MaximumPeerCount:  1,
			},
		},
	}

	policyMock := &mocks2.CollectionAccessPolicy{}
	Setup(policyMock, 1, 2, func(_ protoutil.SignedData) bool {
		return true
	}, map[string]struct{}{
		"org1": {},
		"org2": {},
	}, false)

	accessFactoryMock.On("AccessPolicy", c1ColConfig, channelID).Return(policyMock, nil)
	accessFactoryMock.On("AccessPolicy", c2ColConfig, channelID).Return(policyMock, nil)

	testMetricProvider := mocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	d := NewDistributor(channelID, g, accessFactoryMock, metrics, 0)
	pdFactory := &pvtDataFactory{}
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1", "c2").create()
	err := d.Distribute("tx1", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[0].WriteSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			"ns1": {
				Config: []*peer.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	require.NoError(t, err)
	err = d.Distribute("tx2", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[1].WriteSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			"ns2": {
				Config: []*peer.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	require.NoError(t, err)

	expectedMaxCount := map[string]int{}
	expectedMinAck := map[string]int{}

	i := 0
	require.Len(t, sendings, 8)
	for dis := range sendings {
		key := fmt.Sprintf("%s~%s", dis.PrivatePayload.Namespace, dis.PrivatePayload.CollectionName)
		expectedMaxCount[key] += dis.SendCriteria.MaxPeers
		expectedMinAck[key] += dis.SendCriteria.MinAck
		i++
		if i == 8 {
			break
		}
	}

	// Ensure MaxPeers is maxInternalPeers which is 2
	require.Equal(t, 2, expectedMaxCount["ns1~c1"])
	require.Equal(t, 2, expectedMaxCount["ns2~c2"])

	// and MinAck is minInternalPeers which is 1
	require.Equal(t, 1, expectedMinAck["ns1~c1"])
	require.Equal(t, 1, expectedMinAck["ns2~c2"])

	// Channel is empty after we read 8 times from it
	require.Len(t, sendings, 0)

	// Bad path: dependencies (gossip and others) don't work properly
	g.err = errors.New("failed obtaining filter")
	err = d.Distribute("tx1", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[0].WriteSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			"ns1": {
				Config: []*peer.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed obtaining filter")

	g.Mock = mock.Mock{}
	g.On("SendByCriteria", mock.Anything, mock.Anything).Return(errors.New("failed sending"))
	g.On("PeersOfChannel", gcommon.ChannelID(channelID)).Return([]discovery.NetworkMember{
		{PKIid: gcommon.PKIidType{1}},
	})

	g.On("IdentityInfo").Return(api.PeerIdentitySet{
		{
			PKIId:        gcommon.PKIidType{1},
			Organization: api.OrgIdentityType("org1"),
		},
	})

	g.err = nil
	err = d.Distribute("tx1", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[0].WriteSet,
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			"ns1": {
				Config: []*peer.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed disseminating 2 out of 2 private dissemination plans")

	require.Equal(t,
		[]string{"channel", channelID},
		testMetricProvider.FakeSendDuration.WithArgsForCall(0),
	)
	require.True(t, testMetricProvider.FakeSendDuration.ObserveArgsForCall(0) > 0)
}
