/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"time"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

type stats struct {
	blockchainHeight       metrics.Gauge
	blockProcessingTime    metrics.Histogram
	blockstorageCommitTime metrics.Histogram
	statedbCommitTime      metrics.Histogram
	transactionsCount      metrics.Counter
}

func newStats(metricsProvider metrics.Provider) *stats {
	stats := &stats{}
	stats.blockchainHeight = metricsProvider.NewGauge(blockchainHeightOpts)
	stats.blockProcessingTime = metricsProvider.NewHistogram(blockProcessingTimeOpts)
	stats.blockstorageCommitTime = metricsProvider.NewHistogram(blockstorageCommitTimeOpts)
	stats.statedbCommitTime = metricsProvider.NewHistogram(statedbCommitTimeOpts)
	stats.transactionsCount = metricsProvider.NewCounter(transactionCountOpts)
	return stats
}

type ledgerStats struct {
	stats    *stats
	ledgerid string
}

func (s *stats) ledgerStats(ledgerid string) *ledgerStats {
	return &ledgerStats{
		s, ledgerid,
	}
}

func (s *ledgerStats) updateBlockchainHeight(height uint64) {
	// casting uint64 to float64 guarentees precision for the numbers upto 9,007,199,254,740,992 (1<<53)
	// since, we are not expecting the blockchains of this scale anytime soon, we go ahead with this for now.
	s.stats.blockchainHeight.With("channel_name", s.ledgerid).Set(float64(height))
}

func (s *ledgerStats) updateBlockProcessingTime(timeTaken time.Duration) {
	s.stats.blockProcessingTime.With("channel_name", s.ledgerid).Observe(timeTaken.Seconds())
}

func (s *ledgerStats) updateBlockstorageCommitTime(timeTaken time.Duration) {
	s.stats.blockstorageCommitTime.With("channel_name", s.ledgerid).Observe(timeTaken.Seconds())
}

func (s *ledgerStats) updateStatedbCommitTime(timeTaken time.Duration) {
	s.stats.statedbCommitTime.With("channel_name", s.ledgerid).Observe(timeTaken.Seconds())
}

func (s *ledgerStats) updateTransactionCounts(
	transactionType common.HeaderType,
	chaincodeName string,
	validatioCode peer.TxValidationCode,
) {
	s.stats.transactionsCount.
		With(s.ledgerid,
			transactionType.String(),
			chaincodeName,
			validatioCode.String(),
		).
		Add(1)
}

var (
	blockchainHeightOpts = metrics.GaugeOpts{
		Namespace:    "ledger",
		Subsystem:    "",
		Name:         "blockchain_height",
		Help:         "Height of the chain in blocks.",
		LabelNames:   []string{"channel_name"},
		StatsdFormat: "%{#fqname}.%{channel_name}",
	}

	blockProcessingTimeOpts = metrics.HistogramOpts{
		Namespace:    "ledger",
		Subsystem:    "",
		Name:         "block_processing_time",
		Help:         "Time taken in seconds for ledger block processing.",
		LabelNames:   []string{"channel_name"},
		StatsdFormat: "%{#fqname}.%{channel_name}",
		Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	}

	blockstorageCommitTimeOpts = metrics.HistogramOpts{
		Namespace:    "ledger",
		Subsystem:    "",
		Name:         "blockstorage_commit_time",
		Help:         "Time taken in seconds for committing block and private data to respective storage.",
		LabelNames:   []string{"channel_name"},
		StatsdFormat: "%{#fqname}.%{channel_name}",
		Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	}

	statedbCommitTimeOpts = metrics.HistogramOpts{
		Namespace:    "ledger",
		Subsystem:    "",
		Name:         "statedb_commit_time",
		Help:         "Time taken in seconds for committing block changes to state db.",
		LabelNames:   []string{"channel_name"},
		StatsdFormat: "%{#fqname}.%{channel_name}",
		Buckets:      []float64{0.005, 0.01, 0.015, 0.05, 0.1, 1, 10},
	}

	transactionCountOpts = metrics.CounterOpts{
		Namespace:    "ledger",
		Subsystem:    "",
		Name:         "transaction_counts",
		Help:         "Number of transactions processed.",
		LabelNames:   []string{"channel_name", "transaction_type", "chaincode", "validation_code"},
		StatsdFormat: "%{#fqname}.%{channel_name}.%{transaction_type}.%{chaincode}.%{validation_code}",
	}
)
