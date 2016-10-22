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
	"github.com/hyperledger/fabric/orderer/config"
)

// Consumer allows the caller to receive a stream of messages from the orderer
type Consumer interface {
	Recv() <-chan *sarama.ConsumerMessage
	Closeable
}

type consumerImpl struct {
	parent    sarama.Consumer
	partition sarama.PartitionConsumer
}

func newConsumer(conf *config.TopLevel, seek int64) (Consumer, error) {
	parent, err := sarama.NewConsumer(conf.Kafka.Brokers, newBrokerConfig(conf))
	if err != nil {
		return nil, err
	}
	partition, err := parent.ConsumePartition(conf.Kafka.Topic, conf.Kafka.PartitionID, seek)
	if err != nil {
		return nil, err
	}
	c := &consumerImpl{parent: parent, partition: partition}
	logger.Debug("Created new consumer for client beginning from block", seek)
	return c, nil
}

// Recv returns a channel with messages received from the orderer
func (c *consumerImpl) Recv() <-chan *sarama.ConsumerMessage {
	return c.partition.Messages()
}

// Close shuts down the partition consumer
func (c *consumerImpl) Close() error {
	if err := c.partition.Close(); err != nil {
		return err
	}
	return c.parent.Close()
}
