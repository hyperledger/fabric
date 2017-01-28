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
	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/localconfig"
)

// Consumer allows the caller to receive a stream of blobs from the Kafka cluster for a specific partition.
type Consumer interface {
	Recv() <-chan *sarama.ConsumerMessage
	Closeable
}

type consumerImpl struct {
	parent    sarama.Consumer
	partition sarama.PartitionConsumer
}

func newConsumer(brokers []string, kafkaVersion sarama.KafkaVersion, tls config.TLS, cp ChainPartition, offset int64) (Consumer, error) {
	parent, err := sarama.NewConsumer(brokers, newBrokerConfig(kafkaVersion, rawPartition, tls))
	if err != nil {
		return nil, err
	}
	partition, err := parent.ConsumePartition(cp.Topic(), cp.Partition(), offset)
	if err != nil {
		return nil, err
	}
	c := &consumerImpl{
		parent:    parent,
		partition: partition,
	}
	logger.Debugf("Created new consumer for session (partition %s, beginning offset %d)", cp, offset)
	return c, nil
}

// Recv returns a channel with blobs received from the Kafka cluster for a partition.
func (c *consumerImpl) Recv() <-chan *sarama.ConsumerMessage {
	return c.partition.Messages()
}

// Close shuts down the partition consumer.
// Invoked by the session deliverer's Close method, which is itself called
// during the processSeek function, between disabling and enabling the push.
func (c *consumerImpl) Close() error {
	if err := c.partition.Close(); err != nil {
		return err
	}
	return c.parent.Close()
}
