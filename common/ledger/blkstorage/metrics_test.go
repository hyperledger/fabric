/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/stretchr/testify/require"
)

func TestStatsBlockchainHeight(t *testing.T) {
	testMetricProvider := testutilConstructMetricProvider()
	env := newTestEnvWithMetricsProvider(t, NewConf(t.TempDir(), 0), testMetricProvider.fakeProvider)
	defer env.Cleanup()

	provider := env.provider
	ledgerid := "ledger-stats"
	store, err := provider.Open(ledgerid)
	require.NoError(t, err)
	defer store.Shutdown()

	// add genesis block
	blockGenerator, genesisBlock := testutil.NewBlockGenerator(t, "testchannelid", false)
	err = store.AddBlock(genesisBlock)
	require.NoError(t, err)

	// add one more block
	b1 := blockGenerator.NextBlock([][]byte{})
	err = store.AddBlock(b1)
	require.NoError(t, err)

	// should have 3 calls for fakeBlockchainHeightGauge: OpenBlockStore, genesis block, and block b1
	fakeBlockchainHeightGauge := testMetricProvider.fakeBlockchainHeightGauge
	expectedCallCount := 3
	require.Equal(t, expectedCallCount, fakeBlockchainHeightGauge.SetCallCount())

	// verify the call for OpenBlockStore
	require.Equal(t, float64(0), fakeBlockchainHeightGauge.SetArgsForCall(0))
	require.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(0))

	// verify the call for adding genesis block
	require.Equal(t, float64(1), fakeBlockchainHeightGauge.SetArgsForCall(1))
	require.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(1))

	// verify the call for adding block b1
	require.Equal(t, float64(2), fakeBlockchainHeightGauge.SetArgsForCall(2))
	require.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(2))

	// shutdown and reopen the store to verify blockchain height
	store.Shutdown()
	store, err = provider.Open(ledgerid)
	require.NoError(t, err)

	// verify the call when opening an existing ledger - should set height correctly
	require.Equal(t, float64(2), fakeBlockchainHeightGauge.SetArgsForCall(3))
	require.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(3))

	// invoke updateBlockStats api explicitly and verify the call with fake metrics
	store.updateBlockStats(10, 1*time.Second)
	require.Equal(t, float64(11), fakeBlockchainHeightGauge.SetArgsForCall(4))
	require.Equal(t, []string{"channel", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(4))
}

func TestStatsBlockCommit(t *testing.T) {
	testMetricProvider := testutilConstructMetricProvider()
	env := newTestEnvWithMetricsProvider(t, NewConf(t.TempDir(), 0), testMetricProvider.fakeProvider)
	defer env.Cleanup()

	provider := env.provider
	ledgerid := "ledger-stats"
	store, err := provider.Open(ledgerid)
	require.NoError(t, err)
	defer store.Shutdown()

	// add a genesis block
	blockGenerator, genesisBlock := testutil.NewBlockGenerator(t, "testchannelid", false)
	err = store.AddBlock(genesisBlock)
	require.NoError(t, err)

	// add 3 more blocks
	for i := 1; i <= 3; i++ {
		b := blockGenerator.NextBlock([][]byte{})
		err = store.AddBlock(b)
		require.NoError(t, err)
	}

	fakeBlockstorageCommitTimeHist := testMetricProvider.fakeBlockstorageCommitTimeHist

	// should have 4 calls to fakeBlockstorageCommitTimeHist: genesis block, and 3 blocks
	expectedCallCount := 1 + 3
	require.Equal(t, expectedCallCount, fakeBlockstorageCommitTimeHist.ObserveCallCount())

	// verify the value of channel in each call (0, 1, 2, 3)
	for i := 0; i < expectedCallCount; i++ {
		require.Equal(t, []string{"channel", ledgerid}, fakeBlockstorageCommitTimeHist.WithArgsForCall(i))
	}

	// invoke updateBlockStats api explicitly and verify with fake metrics (call number is 4)
	store.updateBlockStats(4, 10*time.Second)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockstorageCommitTimeHist.WithArgsForCall(4),
	)
	require.Equal(t,
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
