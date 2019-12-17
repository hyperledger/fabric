/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast

import "github.com/hyperledger/fabric/common/metrics"

var (
	validateDuration = metrics.HistogramOpts{
		Namespace:    "broadcast",
		Name:         "validate_duration",
		Help:         "The time to validate a transaction in seconds.",
		LabelNames:   []string{"channel", "type", "status"},
		StatsdFormat: "%{#fqname}.%{channel}.%{type}.%{status}",
	}
	enqueueDuration = metrics.HistogramOpts{
		Namespace:    "broadcast",
		Name:         "enqueue_duration",
		Help:         "The time to enqueue a transaction in seconds.",
		LabelNames:   []string{"channel", "type", "status"},
		StatsdFormat: "%{#fqname}.%{channel}.%{type}.%{status}",
	}
	processedCount = metrics.CounterOpts{
		Namespace:    "broadcast",
		Name:         "processed_count",
		Help:         "The number of transactions processed.",
		LabelNames:   []string{"channel", "type", "status"},
		StatsdFormat: "%{#fqname}.%{channel}.%{type}.%{status}",
	}
)

type Metrics struct {
	ValidateDuration metrics.Histogram
	EnqueueDuration  metrics.Histogram
	ProcessedCount   metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		ValidateDuration: p.NewHistogram(validateDuration),
		EnqueueDuration:  p.NewHistogram(enqueueDuration),
		ProcessedCount:   p.NewCounter(processedCount),
	}
}
