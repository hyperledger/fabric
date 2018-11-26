/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/assert"
)

func TestStatsBlockchainHeight(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	testMetricProvider := testutilConstructMetricProvider()
	provider, err := NewProvider()
	assert.NoError(t, err)
	provider.Initialize(&lgr.Initializer{
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		MetricsProvider:               testMetricProvider.fakeProvider,
	})
	defer provider.Close()

	// create a ledger
	ledgerid := "ledger1"
	_, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	l, err := provider.Create(gb)
	assert.NoError(t, err)
	ledger := l.(*kvLedger)
	defer ledger.Close()

	fakeBlockchainHeightGauge := testMetricProvider.fakeBlockchainHeightGauge
	// calls during ledger creation
	assert.Equal(t, float64(0), fakeBlockchainHeightGauge.SetArgsForCall(0))
	assert.Equal(t, []string{"channel_name", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(0))

	// calls during committing genesis block
	assert.Equal(t, float64(1), fakeBlockchainHeightGauge.SetArgsForCall(1))
	assert.Equal(t, []string{"channel_name", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(1))

	ledger.Close()
	provider.Open(ledgerid)
	// calls during opening an existing ledger - opening a ledger should set the current height
	assert.Equal(t, float64(1), fakeBlockchainHeightGauge.SetArgsForCall(2))
	assert.Equal(t, []string{"channel_name", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(2))

	// invoke updateBlockStats api explicitly and verify the calls with fake metrics
	ledger.updateBlockStats(
		10, 1*time.Second, 2*time.Second, 3*time.Second,
	)
	assert.Equal(t, []string{"channel_name", ledgerid}, fakeBlockchainHeightGauge.WithArgsForCall(3))
	assert.Equal(t, float64(11), fakeBlockchainHeightGauge.SetArgsForCall(3))
}

func TestStatsBlockCommitTimings(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	testMetricProvider := testutilConstructMetricProvider()
	provider, err := NewProvider()
	assert.NoError(t, err)
	provider.Initialize(&lgr.Initializer{
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		MetricsProvider:               testMetricProvider.fakeProvider,
	})
	defer provider.Close()

	// create a ledger
	ledgerid := "ledger1"
	_, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	l, err := provider.Create(gb)
	assert.NoError(t, err)
	ledger := l.(*kvLedger)
	defer ledger.Close()

	// calls during committing genesis block
	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeBlockProcessingTimeHist.WithArgsForCall(0))
	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeBlockstorageCommitTimeHist.WithArgsForCall(0))
	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeStatedbCommitTimeHist.WithArgsForCall(0))

	// invoke updateBlockStats api explicitly and verify the calls with fake metrics
	ledger.updateBlockStats(
		10, 1*time.Second, 2*time.Second, 3*time.Second,
	)
	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeBlockProcessingTimeHist.WithArgsForCall(1))
	assert.Equal(t, float64(1),
		testMetricProvider.fakeBlockProcessingTimeHist.ObserveArgsForCall(1))

	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeBlockstorageCommitTimeHist.WithArgsForCall(1))
	assert.Equal(t, float64(2),
		testMetricProvider.fakeBlockstorageCommitTimeHist.ObserveArgsForCall(1))

	assert.Equal(t, []string{"channel_name", ledgerid},
		testMetricProvider.fakeStatedbCommitTimeHist.WithArgsForCall(1))
	assert.Equal(t, float64(3),
		testMetricProvider.fakeStatedbCommitTimeHist.ObserveArgsForCall(1))
}

type testMetricProvider struct {
	fakeProvider                   *metricsfakes.Provider
	fakeBlockchainHeightGauge      *metricsfakes.Gauge
	fakeBlockProcessingTimeHist    *metricsfakes.Histogram
	fakeBlockstorageCommitTimeHist *metricsfakes.Histogram
	fakeStatedbCommitTimeHist      *metricsfakes.Histogram
	fakeTransactionsCount          *metricsfakes.Counter
}

func testutilConstructMetricProvider() *testMetricProvider {
	fakeProvider := &metricsfakes.Provider{}
	fakeBlockchainHeightGauge := testutilConstructGuage()
	fakeBlockProcessingTimeHist := testutilConstructHist()
	fakeBlockstorageCommitTimeHist := testutilConstructHist()
	fakeStatedbCommitTimeHist := testutilConstructHist()
	fakeTransactionsCount := testutilConstructCounter()
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		switch opts.Name {
		case blockchainHeightOpts.Name:
			return fakeBlockchainHeightGauge
		}
		return nil
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		switch opts.Name {
		case blockProcessingTimeOpts.Name:
			return fakeBlockProcessingTimeHist
		case blockstorageCommitTimeOpts.Name:
			return fakeBlockstorageCommitTimeHist
		case statedbCommitTimeOpts.Name:
			return fakeStatedbCommitTimeHist
		}
		return nil
	}

	fakeProvider.NewCounterStub = func(opts metrics.CounterOpts) metrics.Counter {
		switch opts.Name {
		case transactionCountOpts.Name:
			return fakeTransactionsCount
		}
		return nil
	}
	return &testMetricProvider{
		fakeProvider,
		fakeBlockchainHeightGauge,
		fakeBlockProcessingTimeHist,
		fakeBlockstorageCommitTimeHist,
		fakeStatedbCommitTimeHist,
		fakeTransactionsCount,
	}
}

func testutilConstructGuage() *metricsfakes.Gauge {
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

func testutilConstructCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithStub = func(lableValues ...string) metrics.Counter {
		return fakeCounter
	}
	return fakeCounter
}
