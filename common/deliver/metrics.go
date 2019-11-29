/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"github.com/hyperledger/fabric/common/metrics"
)

var (
	streamsOpened = metrics.CounterOpts{
		Namespace: "deliver",
		Name:      "streams_opened",
		Help:      "The number of GRPC streams that have been opened for the deliver service.",
	}
	streamsClosed = metrics.CounterOpts{
		Namespace: "deliver",
		Name:      "streams_closed",
		Help:      "The number of GRPC streams that have been closed for the deliver service.",
	}

	requestsReceived = metrics.CounterOpts{
		Namespace:    "deliver",
		Name:         "requests_received",
		Help:         "The number of deliver requests that have been received.",
		LabelNames:   []string{"channel", "filtered", "data_type"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}",
	}
	requestsCompleted = metrics.CounterOpts{
		Namespace:    "deliver",
		Name:         "requests_completed",
		Help:         "The number of deliver requests that have been completed.",
		LabelNames:   []string{"channel", "filtered", "data_type", "success"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}.%{success}",
	}

	blocksSent = metrics.CounterOpts{
		Namespace:    "deliver",
		Name:         "blocks_sent",
		Help:         "The number of blocks sent by the deliver service.",
		LabelNames:   []string{"channel", "filtered", "data_type"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}",
	}
)

type Metrics struct {
	StreamsOpened     metrics.Counter
	StreamsClosed     metrics.Counter
	RequestsReceived  metrics.Counter
	RequestsCompleted metrics.Counter
	BlocksSent        metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		StreamsOpened:     p.NewCounter(streamsOpened),
		StreamsClosed:     p.NewCounter(streamsClosed),
		RequestsReceived:  p.NewCounter(requestsReceived),
		RequestsCompleted: p.NewCounter(requestsCompleted),
		BlocksSent:        p.NewCounter(blocksSent),
	}
}
