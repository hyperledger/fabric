/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/util"
	"github.com/stretchr/testify/assert"
)

func TestStatsBlockchainHeight(t *testing.T) {
	testMetricProvider := testutilConstructMetricProvider()
	env := newTestEnvWithMetricsProvider(t, NewConf(testPath(), 0), testMetricProvider.fakeProvider)
	defer env.Cleanup()

	provider := env.provider
	ledgerid := "ledger-stats"
	store, err := provider.OpenBlockStore(ledgerid)
	assert.NoError(t, err)
	defer store.Shutdown()

	// add genesis block
	blockGenerator, genesisBlock := testutil.NewBlockGenerator(t, util.GetTestChainID(), false)
	err = store.AddBlock(genesisBlock)
	assert.NoError(t, err)

	// add one more block
	b1 := blockGenerator.NextBlock([][]byte{})
	err = store.AddBlock(b1)
	assert.NoError(t, err)

	// should have 3 calls for fakeBlockchainHeightGauge: OpenBlockStore, genesis block, and block b1
	fakeBlockchainHeightGauge := testMetricProvider.fakeBlockchainHeightGauge
	expectedCallCount := 3
	assert.Equal(t, expectedCallCount, fakeBlockchainHeightGauge.SetCallCount())

	// verify the call for OpenBlockStore
	assert.Equal(t, float64(0), fakeBlockchainHeightGauge.SetArgsForCall(0))
	assert.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(0))

	// verify the call for adding genesis block
	assert.Equal(t, float64(1), fakeBlockchainHeightGauge.SetArgsForCall(1))
	assert.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(1))

	// verify the call for adding block b1
	assert.Equal(t, float64(2), fakeBlockchainHeightGauge.SetArgsForCall(2))
	assert.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(2))

	// shutdown and reopen the store to verify blockchain height
	store.Shutdown()
	store, err = provider.OpenBlockStore(ledgerid)
	assert.NoError(t, err)

	// verify the call when opening an existing ledger - should set height correctly
	assert.Equal(t, float64(2), fakeBlockchainHeightGauge.SetArgsForCall(3))
	assert.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(3))

	// invoke updateBlockStats api explicitly and verify the call with fake metrics
	store.(*fsBlockStore).updateBlockStats(10, 1*time.Second)
	assert.Equal(t, float64(11), fakeBlockchainHeightGauge.SetArgsForCall(4))
	assert.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(4))
}

func TestStatsBlockCommit(t *testing.T) {
	testMetricProvider := testutilConstructMetricProvider()
	env := newTestEnvWithMetricsProvider(t, NewConf(testPath(), 0), testMetricProvider.fakeProvider)
	defer env.Cleanup()

	provider := env.provider
	ledgerid := "ledger-stats"
	store, err := provider.OpenBlockStore(ledgerid)
	assert.NoError(t, err)
	defer store.Shutdown()

	// add a genesis block
	blockGenerator, genesisBlock := testutil.NewBlockGenerator(t, util.GetTestChainID(), false)
	err = store.AddBlock(genesisBlock)
	assert.NoError(t, err)

	// add 3 more blocks
	for i := 1; i <= 3; i++ {
		b := blockGenerator.NextBlock([][]byte{})
		err = store.AddBlock(b)
		assert.NoError(t, err)
	}

	fakeBlockstorageCommitTimeHist := testMetricProvider.fakeBlockstorageCommitTimeHist

	// should have 4 calls to fakeBlockstorageCommitTimeHist: genesis block, and 3 blocks
	expectedCallCount := 1 + 3
	assert.Equal(t, expectedCallCount, fakeBlockstorageCommitTimeHist.ObserveCallCount())

	// verify the value of channel in each call (0, 1, 2, 3)
	for i := 0; i < expectedCallCount; i++ {
		assert.Equal(t, []string{"channel", ledgerid}, fakeBlockstorageCommitTimeHist.WithArgsForCall(i))
	}

	// invoke updateBlockStats api explicitly and verify with fake metrics (call number is 4)
	store.(*fsBlockStore).updateBlockStats(4, 10*time.Second)
	assert.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockstorageCommitTimeHist.WithArgsForCall(4),
	)
	assert.Equal(t,
		float64(10),
		testMetricProvider.fakeBlockstorageCommitTimeHist.ObserveArgsForCall(4),
	)
}

type testMetricProvider struct {
	fakeProvider                   *metricsfakes.Provider
	fakeBlockchainHeightGauge      *metricsfakes.Gauge
	fakeBlockstorageCommitTimeHist *metricsfakes.Histogram
}

func testutilConstructMetricProvider() *testMetricProvider {
	fakeProvider := &metricsfakes.Provider{}
	fakeBlockchainHeightGauge := testutilConstructGauge()
	fakeBlockstorageCommitTimeHist := testutilConstructHist()
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		switch opts.Name {
		case blockchainHeightOpts.Name:
			return fakeBlockchainHeightGauge
		default:
			return nil
		}
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		switch opts.Name {
		case blockstorageCommitTimeOpts.Name:
			return fakeBlockstorageCommitTimeHist
		default:
			return nil
		}
	}

	return &testMetricProvider{
		fakeProvider,
		fakeBlockchainHeightGauge,
		fakeBlockstorageCommitTimeHist,
	}
}

func testutilConstructGauge() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithStub = func(lableValues ...string) metrics.Gauge {
		return fakeGauge
	}
	return fakeGauge
}

func testutilConstructHist() *metricsfakes.Histogram {
	fakeHist := &metricsfakes.Histogram{}
	fakeHist.WithStub = func(lableValues ...string) metrics.Histogram {
		return fakeHist
	}
	return fakeHist
}
