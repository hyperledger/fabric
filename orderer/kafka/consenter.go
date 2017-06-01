/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/common/flogging"
	localconfig "github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	logging "github.com/op/go-logging"
)

const pkgLogID = "orderer/kafka"

var logger *logging.Logger

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

// New creates a Kafka-based consenter. Called by orderer's main.go.
func New(tlsConfig localconfig.TLS, retryOptions localconfig.Retry, kafkaVersion sarama.KafkaVersion) multichain.Consenter {
	brokerConfig := newBrokerConfig(tlsConfig, retryOptions, kafkaVersion, defaultPartition)
	return &consenterImpl{
		brokerConfigVal: brokerConfig,
		tlsConfigVal:    tlsConfig,
		retryOptionsVal: retryOptions,
		kafkaVersionVal: kafkaVersion}
}

// consenterImpl holds the implementation of type that satisfies the
// multichain.Consenter interface --as the HandleChain contract requires-- and
// the commonConsenter one.
type consenterImpl struct {
	brokerConfigVal *sarama.Config
	tlsConfigVal    localconfig.TLS
	retryOptionsVal localconfig.Retry
	kafkaVersionVal sarama.KafkaVersion
}

// HandleChain creates/returns a reference to a multichain.Chain object for the
// given set of support resources. Implements the multichain.Consenter
// interface. Called by multichain.newChainSupport(), which is itself called by
// multichain.NewManagerImpl() when ranging over the ledgerFactory's
// existingChains.
func (consenter *consenterImpl) HandleChain(support multichain.ConsenterSupport, metadata *cb.Metadata) (multichain.Chain, error) {
	lastOffsetPersisted := getLastOffsetPersisted(metadata.Value, support.ChainID())
	return newChain(consenter, support, lastOffsetPersisted)
}

// commonConsenter allows us to retrieve the configuration options set on the
// consenter object. These will be common across all chain objects derived by
// this consenter. They are set using using local configuration settings. This
// interface is satisfied by consenterImpl.
type commonConsenter interface {
	brokerConfig() *sarama.Config
	retryOptions() localconfig.Retry
}

func (consenter *consenterImpl) brokerConfig() *sarama.Config {
	return consenter.brokerConfigVal
}

func (consenter *consenterImpl) retryOptions() localconfig.Retry {
	return consenter.retryOptionsVal
}

// closeable allows the shut down of the calling resource.
type closeable interface {
	close() error
}
