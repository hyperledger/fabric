/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kafka

import (
	"fmt"
	"time"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/localconfig"
)

// Producer allows the caller to post blobs to a chain partition on the Kafka cluster.
type Producer interface {
	Send(cp ChainPartition, payload []byte) error
	Closeable
}

type producerImpl struct {
	producer sarama.SyncProducer
}

func newProducer(brokers []string, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry, tls config.TLS) Producer {
	var p sarama.SyncProducer
	var err error
	brokerConfig := newBrokerConfig(kafkaVersion, rawPartition, tls)

	repeatTick := time.NewTicker(retryOptions.Period)
	panicTick := time.NewTicker(retryOptions.Stop)
	defer repeatTick.Stop()
	defer panicTick.Stop()

loop:
	for {
		select {
		case <-panicTick.C:
			panic(fmt.Errorf("Failed to create Kafka producer: %v", err))
		case <-repeatTick.C:
			logger.Debug("Connecting to Kafka cluster:", brokers)
			p, err = sarama.NewSyncProducer(brokers, brokerConfig)
			if err == nil {
				break loop
			}
		}
	}

	logger.Debug("Connected to the Kafka cluster")
	return &producerImpl{producer: p}
}

// Close shuts down the Producer component of the orderer.
func (p *producerImpl) Close() error {
	return p.producer.Close()
}

// Send posts a blob to a chain partition on the Kafka cluster.
func (p *producerImpl) Send(cp ChainPartition, payload []byte) error {
	prt, ofs, err := p.producer.SendMessage(newProducerMessage(cp, payload))
	if prt != cp.Partition() {
		// If this happens, something's up with the partitioner
		logger.Warningf("Blob destined for partition %d, but posted to %d instead", cp.Partition(), prt)
	}
	if err == nil {
		logger.Debugf("Forwarded blob with offset number %d to chain partition %s on the Kafka cluster", ofs, cp)
	} else {
		logger.Infof("Failed to send message to chain partition %s on the Kafka cluster: %s", cp, err)
	}
	return err
}
