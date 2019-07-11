/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

//go:generate counterfeiter -o mock/health_checker.go -fake-name HealthChecker . healthChecker

// healthChecker defines the contract for health checker
type healthChecker interface {
	RegisterChecker(component string, checker healthz.HealthChecker) error
}

// New creates a Kafka-based consenter. Called by orderer's main.go.
func New(config localconfig.Kafka, metricsProvider metrics.Provider, healthChecker healthChecker) (consensus.Consenter, *Metrics) {
	if config.Verbose {
		logging.SetLevel(logging.DEBUG, "orderer.consensus.kafka.sarama")
	}

	brokerConfig := newBrokerConfig(
		config.TLS,
		config.SASLPlain,
		config.Retry,
		config.Version,
		defaultPartition)

	metrics := NewMetrics(metricsProvider, brokerConfig.MetricRegistry)

	return &consenterImpl{
		brokerConfigVal: brokerConfig,
		tlsConfigVal:    config.TLS,
		retryOptionsVal: config.Retry,
		kafkaVersionVal: config.Version,
		topicDetailVal: &sarama.TopicDetail{
			NumPartitions:     1,
			ReplicationFactor: config.Topic.ReplicationFactor,
		},
		healthChecker: healthChecker,
		metrics:       metrics,
	}, metrics
}

// consenterImpl holds the implementation of type that satisfies the
// consensus.Consenter interface --as the HandleChain contract requires-- and
// the commonConsenter one.
type consenterImpl struct {
	brokerConfigVal *sarama.Config
	tlsConfigVal    localconfig.TLS
	retryOptionsVal localconfig.Retry
	kafkaVersionVal sarama.KafkaVersion
	topicDetailVal  *sarama.TopicDetail
	metricsProvider metrics.Provider
	healthChecker   healthChecker
	metrics         *Metrics
}

// HandleChain creates/returns a reference to a consensus.Chain object for the
// given set of support resources. Implements the consensus.Consenter
// interface. Called by consensus.newChainSupport(), which is itself called by
// multichannel.NewManagerImpl() when ranging over the ledgerFactory's
// existingChains.
func (consenter *consenterImpl) HandleChain(support consensus.ConsenterSupport, metadata *cb.Metadata) (consensus.Chain, error) {
	lastOffsetPersisted, lastOriginalOffsetProcessed, lastResubmittedConfigOffset := getOffsets(metadata.Value, support.ChainID())
	ch, err := newChain(consenter, support, lastOffsetPersisted, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)
	if err != nil {
		return nil, err
	}
	consenter.healthChecker.RegisterChecker(ch.channel.String(), ch)
	return ch, nil
}

// commonConsenter allows us to retrieve the configuration options set on the
// consenter object. These will be common across all chain objects derived by
// this consenter. They are set using using local configuration settings. This
// interface is satisfied by consenterImpl.
type commonConsenter interface {
	brokerConfig() *sarama.Config
	retryOptions() localconfig.Retry
	topicDetail() *sarama.TopicDetail
	Metrics() *Metrics
}

func (consenter *consenterImpl) Metrics() *Metrics {
	return consenter.metrics
}

func (consenter *consenterImpl) brokerConfig() *sarama.Config {
	return consenter.brokerConfigVal
}

func (consenter *consenterImpl) retryOptions() localconfig.Retry {
	return consenter.retryOptionsVal
}

func (consenter *consenterImpl) topicDetail() *sarama.TopicDetail {
	return consenter.topicDetailVal
}
