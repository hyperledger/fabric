package p2pmessage

import (
	"github.com/hyperledger/fabric/common/metrics"
)

var (
	streamsOpened = metrics.CounterOpts{
		Namespace: "p2p_message",
		Name:      "streams_opened",
		Help:      "The number of GRPC streams that have been opened for the p2p_message service.",
	}
	streamsClosed = metrics.CounterOpts{
		Namespace: "p2p_message",
		Name:      "streams_closed",
		Help:      "The number of GRPC streams that have been closed for the p2p_message service.",
	}

	requestsReceived = metrics.CounterOpts{
		Namespace:    "p2p_message",
		Name:         "requests_received",
		Help:         "The number of p2p_message requests that have been received.",
		LabelNames:   []string{"channel", "filtered", "data_type"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}",
	}
	requestsCompleted = metrics.CounterOpts{
		Namespace:    "p2p_message",
		Name:         "requests_completed",
		Help:         "The number of p2p_message requests that have been completed.",
		LabelNames:   []string{"channel", "filtered", "data_type", "success"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}.%{success}",
	}

	blocksReconciled = metrics.CounterOpts{
		Namespace:    "p2p_message",
		Name:         "blocks_reconciled",
		Help:         "The number of blocks sent by the p2p_message service.",
		LabelNames:   []string{"channel", "filtered", "data_type"},
		StatsdFormat: "%{#fqname}.%{channel}.%{filtered}.%{data_type}",
	}
)

type Metrics struct {
	StreamsOpened     metrics.Counter
	StreamsClosed     metrics.Counter
	RequestsReceived  metrics.Counter
	RequestsCompleted metrics.Counter
	BlocksReconciled  metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		StreamsOpened:     p.NewCounter(streamsOpened),
		StreamsClosed:     p.NewCounter(streamsClosed),
		RequestsReceived:  p.NewCounter(requestsReceived),
		RequestsCompleted: p.NewCounter(requestsCompleted),
		BlocksReconciled:  p.NewCounter(blocksReconciled),
	}
}
