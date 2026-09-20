/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validation"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestStatsBlockCommit(t *testing.T) {
	conf := testConfig(t)
	testMetricProvider := testutilConstructMetricProvider()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	provider, err := NewProvider(
		&lgr.Initializer{
			DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
			MetricsProvider:               testMetricProvider.fakeProvider,
			Config:                        conf,
			HashProvider:                  cryptoProvider,
		},
	)
	if err != nil {
		t.Fatalf("Failed to create new Provider: %s", err)
	}
	defer provider.Close()

	// create a ledger
	ledgerid := "ledger1"
	_, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	l, err := provider.CreateFromGenesisBlock(gb)
	require.NoError(t, err)
	ledger := l.(*kvLedger)
	defer ledger.Close()

	// calls during committing genesis block
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockProcessingTimeHist.WithArgsForCall(0),
	)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockstorageCommitWithPvtDataTimeHist.WithArgsForCall(0),
	)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeStatedbCommitTimeHist.WithArgsForCall(0),
	)
	require.Equal(t,
		[]string{
			"channel", ledgerid,
			"transaction_type", common.HeaderType_CONFIG.String(),
			"chaincode", "unknown",
			"validation_code", peer.TxValidationCode_VALID.String(),
		},
		testMetricProvider.fakeTransactionsCount.WithArgsForCall(0),
	)

	// invoke updateBlockStats api explicitly and verify the calls with fake metrics
	ledger.updateBlockStats(
		1*time.Second, 2*time.Second, 3*time.Second,
		[]*validation.TxStatInfo{
			{
				ValidationCode: peer.TxValidationCode_VALID,
				TxType:         common.HeaderType_ENDORSER_TRANSACTION,
				ChaincodeID:    &peer.ChaincodeID{Name: "mycc", Version: "1.0"},
				NumCollections: 2,
			},
			{
				ValidationCode: peer.TxValidationCode_INVALID_OTHER_REASON,
				TxType:         -1,
			},
		},
	)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockProcessingTimeHist.WithArgsForCall(1),
	)
	require.Equal(t,
		float64(1),
		testMetricProvider.fakeBlockProcessingTimeHist.ObserveArgsForCall(1),
	)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeBlockstorageCommitWithPvtDataTimeHist.WithArgsForCall(1),
	)
	require.Equal(t,
		float64(2),
		testMetricProvider.fakeBlockstorageCommitWithPvtDataTimeHist.ObserveArgsForCall(1),
	)
	require.Equal(t,
		[]string{"channel", ledgerid},
		testMetricProvider.fakeStatedbCommitTimeHist.WithArgsForCall(1),
	)
	require.Equal(t,
		float64(3),
		testMetricProvider.fakeStatedbCommitTimeHist.ObserveArgsForCall(1),
	)
	require.Equal(t,
		[]string{
			"channel", ledgerid,
			"transaction_type", common.HeaderType_ENDORSER_TRANSACTION.String(),
			"chaincode", "mycc:1.0",
			"validation_code", peer.TxValidationCode_VALID.String(),
		},
		testMetricProvider.fakeTransactionsCount.WithArgsForCall(1),
	)
	require.Equal(t,
		float64(1),
		testMetricProvider.fakeTransactionsCount.AddArgsForCall(1),
	)

	require.Equal(t,
		[]string{
			"channel", ledgerid,
			"transaction_type", "unknown",
			"chaincode", "unknown",
			"validation_code", peer.TxValidationCode_INVALID_OTHER_REASON.String(),
		},
		testMetricProvider.fakeTransactionsCount.WithArgsForCall(2),
	)
	require.Equal(t,
		float64(1),
		testMetricProvider.fakeTransactionsCount.AddArgsForCall(2),
	)
}

type testMetricProvider struct {
	fakeProvider                              *metricsfakes.Provider
	fakeBlockProcessingTimeHist               *metricsfakes.Histogram
	fakeBlockstorageCommitWithPvtDataTimeHist *metricsfakes.Histogram
	fakeStatedbCommitTimeHist                 *metricsfakes.Histogram
	fakeTransactionsCount                     *metricsfakes.Counter
}

func testutilConstructMetricProvider() *testMetricProvider {
	fakeProvider := &metricsfakes.Provider{}
	fakeBlockProcessingTimeHist := testutilConstructHist()
	fakeBlockstorageCommitWithPvtDataTimeHist := testutilConstructHist()
	fakeStatedbCommitTimeHist := testutilConstructHist()
	fakeTransactionsCount := testutilConstructCounter()
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		// return a gauge for metrics in common/ledger
		return testutilConstructGauge()
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		switch opts.Name {
		case blockProcessingTimeOpts.Name:
			return fakeBlockProcessingTimeHist
		case blockAndPvtdataStoreCommitTimeOpts.Name:
			return fakeBlockstorageCommitWithPvtDataTimeHist
		case statedbCommitTimeOpts.Name:
			return fakeStatedbCommitTimeHist
		default:
			// return a histogram for metrics in common/ledger
			return testutilConstructHist()
		}
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
		fakeBlockProcessingTimeHist,
		fakeBlockstorageCommitWithPvtDataTimeHist,
		fakeStatedbCommitTimeHist,
		fakeTransactionsCount,
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

func testutilConstructCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithStub = func(lableValues ...string) metrics.Counter {
		return fakeCounter
	}
	return fakeCounter
}
