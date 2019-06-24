/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sync"
	"testing"

	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	pcomm "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMetrics(t *testing.T) {
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("CommitWithPvtData", mock.Anything).Return(nil)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(100), nil).Twice()
	mc.On("DoesPvtDataInfoExistInLedger", mock.Anything).Return(false, nil)
	g := &mocks.GossipMock{}
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))

	heightWG := sync.WaitGroup{}
	heightWG.Add(1)
	committedDurationWG := sync.WaitGroup{}
	committedDurationWG.Add(1)

	testMetricProvider := gmetricsmocks.TestUtilConstructMetricProvider()

	testMetricProvider.FakeHeightGauge.SetStub = func(delta float64) {
		heightWG.Done()
	}
	testMetricProvider.FakeCommitDurationHist.ObserveStub = func(value float64) {
		committedDurationWG.Done()
	}

	gossipMetrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider)

	// create peer with fake metrics provider for gossip state
	p := newPeerNodeWithGossipWithMetrics(0, mc, noopPeerIdentityAcceptor, g, gossipMetrics)
	defer p.shutdown()

	// add a payload to the payload buffer
	err := p.s.AddPayload(&proto.Payload{
		SeqNum: 100,
		Data:   utils.MarshalOrPanic(pcomm.NewBlock(100, []byte{})),
	})
	assert.NoError(t, err)

	// update the ledger height to prepare for the pop operation
	mc.On("LedgerHeight", mock.Anything).Return(uint64(101), nil)

	// ensure the block commit event was sent, and the update event was sent
	heightWG.Wait()
	committedDurationWG.Wait()

	// ensure the right height was reported
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.FakeHeightGauge.WithArgsForCall(0),
	)
	assert.EqualValues(t,
		101,
		testMetricProvider.FakeHeightGauge.SetArgsForCall(0),
	)

	// after push or pop payload buffer size should be reported
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.FakePayloadBufferSizeGauge.WithArgsForCall(0),
	)
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.FakePayloadBufferSizeGauge.WithArgsForCall(1),
	)
	// both 0 and 1 as size can be reported, depends on timing
	size := testMetricProvider.FakePayloadBufferSizeGauge.SetArgsForCall(0)
	assert.True(t, size == 1 || size == 0)
	size = testMetricProvider.FakePayloadBufferSizeGauge.SetArgsForCall(1)
	assert.True(t, size == 1 || size == 0)

}
