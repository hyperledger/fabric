/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/transientstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type collectionAccessFactoryMock struct {
	mock.Mock
}

func (mock *collectionAccessFactoryMock) AccessPolicy(config *common.CollectionConfig, chainID string) (privdata.CollectionAccessPolicy, error) {
	res := mock.Called(config, chainID)
	return res.Get(0).(privdata.CollectionAccessPolicy), res.Error(1)
}

type collectionAccessPolicyMock struct {
	mock.Mock
}

func (mock *collectionAccessPolicyMock) AccessFilter() privdata.Filter {
	args := mock.Called()
	return args.Get(0).(privdata.Filter)
}

func (mock *collectionAccessPolicyMock) RequiredPeerCount() int {
	args := mock.Called()
	return args.Int(0)
}

func (mock *collectionAccessPolicyMock) MaximumPeerCount() int {
	args := mock.Called()
	return args.Int(0)
}

func (mock *collectionAccessPolicyMock) MemberOrgs() []string {
	args := mock.Called()
	return args.Get(0).([]string)
}

func (mock *collectionAccessPolicyMock) IsMemberOnlyRead() bool {
	args := mock.Called()
	return args.Get(0).(bool)
}

func (mock *collectionAccessPolicyMock) Setup(requiredPeerCount int, maxPeerCount int,
	accessFilter privdata.Filter, orgs []string, memberOnlyRead bool) {
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

func (g *gossipMock) PeersOfChannel(chainID gcommon.ChainID) []discovery.NetworkMember {
	return g.Called(chainID).Get(0).([]discovery.NetworkMember)
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

	g.On("PeersOfChannel", gcommon.ChainID(channelID)).Return([]discovery.NetworkMember{
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
	accessFactoryMock := &collectionAccessFactoryMock{}
	c1ColConfig := &common.CollectionConfig{
		Payload: &common.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &common.StaticCollectionConfig{
				Name:              "c1",
				RequiredPeerCount: 1,
				MaximumPeerCount:  1,
			},
		},
	}

	c2ColConfig := &common.CollectionConfig{
		Payload: &common.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &common.StaticCollectionConfig{
				Name:              "c2",
				RequiredPeerCount: 1,
				MaximumPeerCount:  1,
			},
		},
	}

	policyMock := &collectionAccessPolicyMock{}
	policyMock.Setup(1, 2, func(_ common.SignedData) bool {
		return true
	}, []string{"org1", "org2"}, false)

	accessFactoryMock.On("AccessPolicy", c1ColConfig, channelID).Return(policyMock, nil)
	accessFactoryMock.On("AccessPolicy", c2ColConfig, channelID).Return(policyMock, nil)

	testMetricProvider := mocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	d := NewDistributor(channelID, g, accessFactoryMock, metrics, 0)
	pdFactory := &pvtDataFactory{}
	pvtData := pdFactory.addRWSet().addNSRWSet("ns1", "c1", "c2").addRWSet().addNSRWSet("ns2", "c1", "c2").create()
	err := d.Distribute("tx1", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[0].WriteSet,
		CollectionConfigs: map[string]*common.CollectionConfigPackage{
			"ns1": {
				Config: []*common.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	assert.NoError(t, err)
	err = d.Distribute("tx2", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[1].WriteSet,
		CollectionConfigs: map[string]*common.CollectionConfigPackage{
			"ns2": {
				Config: []*common.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	assert.NoError(t, err)

	expectedMaxCount := map[string]int{}
	expectedMinAck := map[string]int{}

	i := 0
	assert.Len(t, sendings, 8)
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
	assert.Equal(t, 2, expectedMaxCount["ns1~c1"])
	assert.Equal(t, 2, expectedMaxCount["ns2~c2"])

	// and MinAck is minInternalPeers which is 1
	assert.Equal(t, 1, expectedMinAck["ns1~c1"])
	assert.Equal(t, 1, expectedMinAck["ns2~c2"])

	// Channel is empty after we read 8 times from it
	assert.Len(t, sendings, 0)

	// Bad path: dependencies (gossip and others) don't work properly
	g.err = errors.New("failed obtaining filter")
	err = d.Distribute("tx1", &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: pvtData[0].WriteSet,
		CollectionConfigs: map[string]*common.CollectionConfigPackage{
			"ns1": {
				Config: []*common.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed obtaining filter")

	g.Mock = mock.Mock{}
	g.On("SendByCriteria", mock.Anything, mock.Anything).Return(errors.New("failed sending"))
	g.On("PeersOfChannel", gcommon.ChainID(channelID)).Return([]discovery.NetworkMember{
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
		CollectionConfigs: map[string]*common.CollectionConfigPackage{
			"ns1": {
				Config: []*common.CollectionConfig{c1ColConfig, c2ColConfig},
			},
		},
	}, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed disseminating 2 out of 2 private dissemination plans")

	assert.Equal(t,
		[]string{"channel", channelID},
		testMetricProvider.FakeSendDuration.WithArgsForCall(0),
	)
	assert.True(t, testMetricProvider.FakeSendDuration.ObserveArgsForCall(0) > 0)
}
