/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sync"
	"testing"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/gossip/discovery"
	gossipMetrics "github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/state/mocks"
	pcomm "github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testMetricProvider struct {
	fakeProvider               *metricsfakes.Provider
	fakeHeightGauge            *metricsfakes.Gauge
	fakeCommitDurationHist     *metricsfakes.Histogram
	fakePayloadBufferSizeGauge *metricsfakes.Gauge
}

func testUtilConstructMetricProvider() *testMetricProvider {
	fakeProvider := &metricsfakes.Provider{}
	fakeHeightGauge := testUtilConstructGuage()
	fakeCommitDurationHist := testUtilConstructHist()
	fakePayloadBufferSizeGauge := testUtilConstructGuage()

	fakeProvider.NewCounterStub = func(opts metrics.CounterOpts) metrics.Counter {
		return nil
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		switch opts.Name {
		case gossipMetrics.CommitDurationOpts.Name:
			return fakeCommitDurationHist
		}
		return nil
	}
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		switch opts.Name {
		case gossipMetrics.PayloadBufferSizeOpts.Name:
			return fakePayloadBufferSizeGauge
		case gossipMetrics.HeightOpts.Name:
			return fakeHeightGauge
		}
		return nil
	}

	return &testMetricProvider{
		fakeProvider,
		fakeHeightGauge,
		fakeCommitDurationHist,
		fakePayloadBufferSizeGauge,
	}
}

func testUtilConstructGuage() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithReturns(fakeGauge)
	return fakeGauge
}

func testUtilConstructHist() *metricsfakes.Histogram {
	fakeHist := &metricsfakes.Histogram{}
	fakeHist.WithReturns(fakeHist)
	return fakeHist
}

func TestMetrics(t *testing.T) {
	t.Parallel()
	mc := &mockCommitter{Mock: &mock.Mock{}}
	mc.On("CommitWithPvtData", mock.Anything).Return(nil)
	mc.On("LedgerHeight", mock.Anything).Return(uint64(100), nil).Twice()
	g := &mocks.GossipMock{}
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	g.On("Accept", mock.Anything, false).Return(make(<-chan *proto.GossipMessage), nil)
	g.On("Accept", mock.Anything, true).Return(nil, make(chan proto.ReceivedMessage))

	heightWG := sync.WaitGroup{}
	heightWG.Add(1)
	committedDurationWG := sync.WaitGroup{}
	committedDurationWG.Add(1)

	testMetricProvider := testUtilConstructMetricProvider()

	testMetricProvider.fakeHeightGauge.SetStub = func(delta float64) {
		heightWG.Done()
	}
	testMetricProvider.fakeCommitDurationHist.ObserveStub = func(value float64) {
		committedDurationWG.Done()
	}

	stateMetrics := gossipMetrics.NewGossipMetrics(testMetricProvider.fakeProvider).StateMetrics

	// create peer with fake metrics provider for gossip state
	p := newPeerNodeWithGossipWithMetrics(newGossipConfig(0, 0),
		mc, noopPeerIdentityAcceptor, g, stateMetrics)
	defer p.shutdown()

	// add a payload to the payload buffer
	err := p.s.AddPayload(&proto.Payload{
		SeqNum: 100,
		Data:   utils.MarshalOrPanic(pcomm.NewBlock(100, []byte{})),
	})
	assert.NoError(t, err)

	// after the push payload buffer size should be 1, and it should've been reported
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.fakePayloadBufferSizeGauge.WithArgsForCall(0),
	)
	assert.EqualValues(t,
		1,
		testMetricProvider.fakePayloadBufferSizeGauge.SetArgsForCall(0),
	)

	// update the ledger height to prepare for the pop operation
	mc.On("LedgerHeight", mock.Anything).Return(uint64(101), nil)

	// ensure the block commit event was sent, and the update event was sent
	heightWG.Wait()
	committedDurationWG.Wait()

	// ensure the right height was reported
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.fakeHeightGauge.WithArgsForCall(0),
	)
	assert.EqualValues(t,
		101,
		testMetricProvider.fakeHeightGauge.SetArgsForCall(0),
	)

	// after the pop payload buffer size should be 0, and it should've been reported
	assert.Equal(t,
		[]string{"channel", "testchainid"},
		testMetricProvider.fakePayloadBufferSizeGauge.WithArgsForCall(1),
	)
	assert.EqualValues(t,
		0,
		testMetricProvider.fakePayloadBufferSizeGauge.SetArgsForCall(1),
	)

}
