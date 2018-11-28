/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import "github.com/hyperledger/fabric/common/metrics"

var (
	launchDuration = metrics.HistogramOpts{
		Namespace:    "chaincode",
		Name:         "launch_duration",
		Help:         "The time to launch a chaincode.",
		LabelNames:   []string{"chaincode", "success"},
		StatsdFormat: "%{#fqname}.%{chaincode}.%{success}",
	}
	launchFailures = metrics.CounterOpts{
		Namespace:    "chaincode",
		Name:         "launch_failures",
		Help:         "The number of chaincode launches that have failed.",
		LabelNames:   []string{"chaincode"},
		StatsdFormat: "%{#fqname}.%{chaincode}",
	}
	launchTimeouts = metrics.CounterOpts{
		Namespace:    "chaincode",
		Name:         "launch_timeouts",
		Help:         "The number of chaincode launches that have timed out.",
		LabelNames:   []string{"chaincode"},
		StatsdFormat: "%{#fqname}.%{chaincode}",
	}

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
	executeTimeouts = metrics.CounterOpts{
		Namespace:    "chaincode",
		Name:         "execute_timeouts",
		Help:         "The number of chaincode executions (Init or Invoke) that have timed out.",
		LabelNames:   []string{"chaincode"},
		StatsdFormat: "%{#fqname}.%{chaincode}",
	}
)

type HandlerMetrics struct {
	ShimRequestsReceived  metrics.Counter
	ShimRequestsCompleted metrics.Counter
	ShimRequestDuration   metrics.Histogram
	ExecuteTimeouts       metrics.Counter
}

func NewHandlerMetrics(p metrics.Provider) *HandlerMetrics {
	return &HandlerMetrics{
		ShimRequestsReceived:  p.NewCounter(shimRequestsReceived),
		ShimRequestsCompleted: p.NewCounter(shimRequestsCompleted),
		ShimRequestDuration:   p.NewHistogram(shimRequestDuration),
		ExecuteTimeouts:       p.NewCounter(executeTimeouts),
	}
}

type LaunchMetrics struct {
	LaunchDuration metrics.Histogram
	LaunchFailures metrics.Counter
	LaunchTimeouts metrics.Counter
}

func NewLaunchMetrics(p metrics.Provider) *LaunchMetrics {
	return &LaunchMetrics{
		LaunchDuration: p.NewHistogram(launchDuration),
		LaunchFailures: p.NewCounter(launchFailures),
		LaunchTimeouts: p.NewCounter(launchTimeouts),
	}
}
