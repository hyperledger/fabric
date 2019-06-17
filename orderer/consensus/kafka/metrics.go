/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/metrics"

	gometrics "github.com/rcrowley/go-metrics"
)

/*

 Per the documentation at: https://godoc.org/github.com/Shopify/sarama Sarama exposes the following set of metrics:

 +----------------------------------------------+------------+---------------------------------------------------------------+
 | Name                                         | Type       | Description                                                   |
 +----------------------------------------------+------------+---------------------------------------------------------------+
 | incoming-byte-rate                           | meter      | Bytes/second read off all brokers                             |
 | incoming-byte-rate-for-broker-<broker-id>    | meter      | Bytes/second read off a given broker                          |
 | outgoing-byte-rate                           | meter      | Bytes/second written off all brokers                          |
 | outgoing-byte-rate-for-broker-<broker-id>    | meter      | Bytes/second written off a given broker                       |
 | request-rate                                 | meter      | Requests/second sent to all brokers                           |
 | request-rate-for-broker-<broker-id>          | meter      | Requests/second sent to a given broker                        |
 | request-size                                 | histogram  | Distribution of the request size in bytes for all brokers     |
 | request-size-for-broker-<broker-id>          | histogram  | Distribution of the request size in bytes for a given broker  |
 | request-latency-in-ms                        | histogram  | Distribution of the request latency in ms for all brokers     |
 | request-latency-in-ms-for-broker-<broker-id> | histogram  | Distribution of the request latency in ms for a given broker  |
 | response-rate                                | meter      | Responses/second received from all brokers                    |
 | response-rate-for-broker-<broker-id>         | meter      | Responses/second received from a given broker                 |
 | response-size                                | histogram  | Distribution of the response size in bytes for all brokers    |
 | response-size-for-broker-<broker-id>         | histogram  | Distribution of the response size in bytes for a given broker |
 +----------------------------------------------+------------+---------------------------------------------------------------+

 +-------------------------------------------+------------+--------------------------------------------------------------------------------------+
 | Name                                      | Type       | Description                                                                          |
 +-------------------------------------------+------------+--------------------------------------------------------------------------------------+
 | batch-size                                | histogram  | Distribution of the number of bytes sent per partition per request for all topics    |
 | batch-size-for-topic-<topic>              | histogram  | Distribution of the number of bytes sent per partition per request for a given topic |
 | record-send-rate                          | meter      | Records/second sent to all topics                                                    |
 | record-send-rate-for-topic-<topic>        | meter      | Records/second sent to a given topic                                                 |
 | records-per-request                       | histogram  | Distribution of the number of records sent per request for all topics                |
 | records-per-request-for-topic-<topic>     | histogram  | Distribution of the number of records sent per request for a given topic             |
 | compression-ratio                         | histogram  | Distribution of the compression ratio times 100 of record batches for all topics     |
 | compression-ratio-for-topic-<topic>       | histogram  | Distribution of the compression ratio times 100 of record batches for a given topic  |
 +-------------------------------------------+------------+--------------------------------------------------------------------------------------+
*/

const (
	IncomingByteRateName  = "incoming-byte-rate-for-broker-"
	OutgoingByteRateName  = "outgoing-byte-rate-for-broker-"
	RequestRateName       = "request-rate-for-broker-"
	RequestSizeName       = "request-size-for-broker-"
	RequestLatencyName    = "request-latency-in-ms-for-broker-"
	ResponseRateName      = "response-rate-for-broker-"
	ResponseSizeName      = "response-size-for-broker-"
	BatchSizeName         = "batch-size-for-topic-"
	RecordSendRateName    = "record-send-rate-for-topic-"
	RecordsPerRequestName = "records-per-request-for-topic-"
	CompressionRatioName  = "compression-ratio-for-topic-"
)

var (
	incomingByteRate = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "incoming_byte_rate",
		Help:         "Bytes/second read off brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	outgoingByteRate = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "outgoing_byte_rate",
		Help:         "Bytes/second written to brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	requestRate = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "request_rate",
		Help:         "Requests/second sent to brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	requestSize = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "request_size",
		Help:         "The mean request size in bytes to brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	requestLatency = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "request_latency",
		Help:         "The mean request latency in ms to brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	responseRate = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "response_rate",
		Help:         "Requests/second sent to brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	responseSize = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "response_size",
		Help:         "The mean response size in bytes from brokers.",
		LabelNames:   []string{"broker_id"},
		StatsdFormat: "%{#fqname}.%{broker_id}",
	}

	batchSize = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "batch_size",
		Help:         "The mean batch size in bytes sent to topics.",
		LabelNames:   []string{"topic"},
		StatsdFormat: "%{#fqname}.%{topic}",
	}

	recordSendRate = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "record_send_rate",
		Help:         "The number of records per second sent to topics.",
		LabelNames:   []string{"topic"},
		StatsdFormat: "%{#fqname}.%{topic}",
	}

	recordsPerRequest = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "records_per_request",
		Help:         "The mean number of records sent per request to topics.",
		LabelNames:   []string{"topic"},
		StatsdFormat: "%{#fqname}.%{topic}",
	}

	compressionRatio = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "compression_ratio",
		Help:         "The mean compression ratio (as percentage) for topics.",
		LabelNames:   []string{"topic"},
		StatsdFormat: "%{#fqname}.%{topic}",
	}

	lastOffsetPersisted = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "kafka",
		Name:         "last_offset_persisted",
		Help:         "The offset specified in the block metadata of the most recently committed block.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

type Metrics struct {
	// The first set of metrics are all reported by Sarama
	IncomingByteRate  metrics.Gauge
	OutgoingByteRate  metrics.Gauge
	RequestRate       metrics.Gauge
	RequestSize       metrics.Gauge
	RequestLatency    metrics.Gauge
	ResponseRate      metrics.Gauge
	ResponseSize      metrics.Gauge
	BatchSize         metrics.Gauge
	RecordSendRate    metrics.Gauge
	RecordsPerRequest metrics.Gauge
	CompressionRatio  metrics.Gauge

	GoMetricsRegistry gometrics.Registry

	// LastPersistedOffset is reported by the Fabric/Kafka integration
	LastOffsetPersisted metrics.Gauge
}

func NewMetrics(p metrics.Provider, registry gometrics.Registry) *Metrics {
	return &Metrics{
		IncomingByteRate:  p.NewGauge(incomingByteRate),
		OutgoingByteRate:  p.NewGauge(outgoingByteRate),
		RequestRate:       p.NewGauge(requestRate),
		RequestSize:       p.NewGauge(requestSize),
		RequestLatency:    p.NewGauge(requestLatency),
		ResponseRate:      p.NewGauge(responseRate),
		ResponseSize:      p.NewGauge(responseSize),
		BatchSize:         p.NewGauge(batchSize),
		RecordSendRate:    p.NewGauge(recordSendRate),
		RecordsPerRequest: p.NewGauge(recordsPerRequest),
		CompressionRatio:  p.NewGauge(compressionRatio),

		GoMetricsRegistry: registry,

		LastOffsetPersisted: p.NewGauge(lastOffsetPersisted),
	}
}

// PollGoMetrics takes the current metric values from go-metrics and publishes them to
// the gauges exposed through go-kit's metrics.
func (m *Metrics) PollGoMetrics() {
	m.GoMetricsRegistry.Each(func(name string, value interface{}) {
		recordMeter := func(prefix, label string, gauge metrics.Gauge) bool {
			if !strings.HasPrefix(name, prefix) {
				return false
			}

			meter, ok := value.(gometrics.Meter)
			if !ok {
				logger.Panicf("Expected metric with name %s to be of type Meter but was of type %T", name, value)
			}

			labelValue := name[len(prefix):]
			gauge.With(label, labelValue).Set(meter.Snapshot().Rate1())

			return true
		}

		recordHistogram := func(prefix, label string, gauge metrics.Gauge) bool {
			if !strings.HasPrefix(name, prefix) {
				return false
			}

			histogram, ok := value.(gometrics.Histogram)
			if !ok {
				logger.Panicf("Expected metric with name %s to be of type Histogram but was of type %T", name, value)
			}

			labelValue := name[len(prefix):]
			gauge.With(label, labelValue).Set(histogram.Snapshot().Mean())

			return true
		}

		switch {
		case recordMeter(IncomingByteRateName, "broker_id", m.IncomingByteRate):
		case recordMeter(OutgoingByteRateName, "broker_id", m.OutgoingByteRate):
		case recordMeter(RequestRateName, "broker_id", m.RequestRate):
		case recordHistogram(RequestSizeName, "broker_id", m.RequestSize):
		case recordHistogram(RequestLatencyName, "broker_id", m.RequestLatency):
		case recordMeter(ResponseRateName, "broker_id", m.ResponseRate):
		case recordHistogram(ResponseSizeName, "broker_id", m.ResponseSize):
		case recordHistogram(BatchSizeName, "topic", m.BatchSize):
		case recordMeter(RecordSendRateName, "topic", m.RecordSendRate):
		case recordHistogram(RecordsPerRequestName, "topic", m.RecordsPerRequest):
		case recordHistogram(CompressionRatioName, "topic", m.CompressionRatio):
		default:
			// Ignore unknown metrics
		}
	})
}

// PollGoMetricsUntilStop should generally be invoked on a dedicated go routine.  This go routine
// will then invoke PollGoMetrics at the specified frequency until the stopChannel closes.
func (m *Metrics) PollGoMetricsUntilStop(frequency time.Duration, stopChannel <-chan struct{}) {
	timer := time.NewTimer(frequency)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			m.PollGoMetrics()
			timer.Reset(frequency)
		case <-stopChannel:
			return
		}
	}
}
