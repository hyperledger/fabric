/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import "github.com/hyperledger/fabric/common/metrics"

var (
	shimRequestsReceived = metrics.CounterOpts{
		Namespace:    "chaincode",
		Name:         "shim_requests_received",
		Help:         "The number of chaincode shim requests received.",
		LabelNames:   []string{"type", "channel", "chaincode"},
		StatsdFormat: "%{#fqname}.%{type}.%{channel}.%{chaincode}",
	}
	shimRequestsCompleted = metrics.CounterOpts{
		Namespace:    "chaincode",
		Name:         "shim_requests_completed",
		Help:         "The number of chaincode shim requests completed.",
		LabelNames:   []string{"type", "channel", "chaincode", "success"},
		StatsdFormat: "%{#fqname}.%{type}.%{channel}.%{chaincode}.%{success}",
	}
	shimRequestDuration = metrics.HistogramOpts{
		Namespace:    "chaincode",
		Name:         "shim_request_duration",
		Help:         "The time to complete chaincode shim requests.",
		LabelNames:   []string{"type", "channel", "chaincode", "success"},
		StatsdFormat: "%{#fqname}.%{type}.%{channel}.%{chaincode}.%{success}",
	}
)

type Metrics struct {
	ShimRequestsReceived  metrics.Counter
	ShimRequestsCompleted metrics.Counter
	ShimRequestDuration   metrics.Histogram
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		ShimRequestsReceived:  p.NewCounter(shimRequestsReceived),
		ShimRequestsCompleted: p.NewCounter(shimRequestsCompleted),
		ShimRequestDuration:   p.NewHistogram(shimRequestDuration),
	}
}
