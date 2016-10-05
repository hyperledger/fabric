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

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/config"
)

// Broker allows the caller to get info on the orderer's stream
type Broker interface {
	GetOffset(req *sarama.OffsetRequest) (int64, error)
	Closeable
}

type brokerImpl struct {
	broker *sarama.Broker
	config *config.TopLevel
}

func newBroker(conf *config.TopLevel) Broker {
	broker := sarama.NewBroker(conf.Kafka.Brokers[0])
	if err := broker.Open(nil); err != nil {
		panic(fmt.Errorf("Failed to create Kafka broker: %v", err))
	}
	return &brokerImpl{
		broker: broker,
		config: conf,
	}
}

// GetOffset retrieves the offset number that corresponds to the requested position in the log
func (b *brokerImpl) GetOffset(req *sarama.OffsetRequest) (int64, error) {
	resp, err := b.broker.GetAvailableOffsets(req)
	if err != nil {
		return int64(-1), err
	}
	return resp.GetBlock(b.config.Kafka.Topic, b.config.Kafka.PartitionID).Offsets[0], nil
}

// Close terminates the broker
func (b *brokerImpl) Close() error {
	return b.broker.Close()
}
