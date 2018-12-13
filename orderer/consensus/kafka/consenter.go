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
	"github.com/hyperledger/fabric/orderer/consensus/migration"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

//go:generate counterfeiter -o mock/health_checker.go -fake-name HealthChecker . healthChecker

// healthChecker defines the contract for health checker
type healthChecker interface {
	RegisterChecker(component string, checker healthz.HealthChecker) error
}

// New creates a Kafka-based consenter. Called by orderer's main.go.
func New(config *localconfig.TopLevel, metricsProvider metrics.Provider, healthChecker healthChecker, migCtrl migration.Controller) (consensus.Consenter, *Metrics) {
	if config.Kafka.Verbose {
		logging.SetLevel(logging.DEBUG, "orderer.consensus.kafka.sarama")
	}

	brokerConfig := newBrokerConfig(
		config.Kafka.TLS,
		config.Kafka.SASLPlain,
		config.Kafka.Retry,
		config.Kafka.Version,
		defaultPartition)

	bootFile := ""
	if config.General.GenesisMethod == "file" {
		bootFile = config.General.GenesisFile
	}

	return &consenterImpl{
		brokerConfigVal: brokerConfig,
		tlsConfigVal:    config.Kafka.TLS,
		retryOptionsVal: config.Kafka.Retry,
		kafkaVersionVal: config.Kafka.Version,
		topicDetailVal: &sarama.TopicDetail{
			NumPartitions:     1,
			ReplicationFactor: config.Kafka.Topic.ReplicationFactor,
		},
		healthChecker:     healthChecker,
		migController:     migCtrl,
		bootstrapFileName: bootFile,
	}, NewMetrics(metricsProvider, brokerConfig.MetricRegistry)
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
	// The migController is needed in order to coordinate consensus-type migration.
	migController migration.Controller
	// The bootstrap filename is needed in order to replace the bootstrap block in case of consensus-type migration.
	bootstrapFileName string
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
	bootstrapFile() string
	migrationController() migration.Controller
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

// bootstrapFile returns the  bootstrap (genesis) filename, if defined, or an empty string.
// Used during consensus-type migration commit.
func (consenter *consenterImpl) bootstrapFile() string {
	return consenter.bootstrapFileName
}

// migrationController returns the passed-in migration.Controller implementation, which coordinates
// consensus-type migration. This is implemented the multichannel.Registrar.
func (consenter *consenterImpl) migrationController() migration.Controller {
	return consenter.migController
}
