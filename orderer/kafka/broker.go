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
	"github.com/hyperledger/fabric/orderer/localconfig"
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

	// connect to one of the bootstrap servers
	var bootstrapServer *sarama.Broker
	for _, hostPort := range conf.Kafka.Brokers {
		broker := sarama.NewBroker(hostPort)
		if err := broker.Open(nil); err != nil {
			logger.Warningf("Failed to connect to bootstrap server at %s: %v.", hostPort, err)
			continue
		}
		if connected, err := broker.Connected(); !connected {
			logger.Warningf("Failed to connect to bootstrap server at %s: %v.", hostPort, err)
			continue
		}
		bootstrapServer = broker
		break
	}
	if bootstrapServer == nil {
		panic(fmt.Errorf("Failed to connect to any of the bootstrap servers (%v) for metadata request.", conf.Kafka.Brokers))
	}
	logger.Debugf("Connected to bootstrap server at %s.", bootstrapServer.Addr())

	// get metadata for topic
	topic := conf.Kafka.Topic
	metadata, err := bootstrapServer.GetMetadata(&sarama.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		panic(fmt.Errorf("GetMetadata failed for topic %s: %v", topic, err))
	}

	// get leader broker for given topic/partition
	var broker *sarama.Broker
	partitionID := conf.Kafka.PartitionID
	if (partitionID >= 0) && (partitionID < int32(len(metadata.Topics[0].Partitions))) {
		leader := metadata.Topics[0].Partitions[partitionID].Leader
		logger.Debugf("Leading broker for topic %s/partition %d is broker ID %d", topic, partitionID, leader)
		for _, b := range metadata.Brokers {
			if b.ID() == leader {
				broker = b
				break
			}
		}
	}
	if broker == nil {
		panic(fmt.Errorf("Can't find leader for topic %s/partition %d", topic, partitionID))
	}

	// connect to broker
	if err := broker.Open(nil); err != nil {
		panic(fmt.Errorf("Failed to open Kafka broker: %v", err))
	}
	if connected, err := broker.Connected(); !connected {
		panic(fmt.Errorf("Failed to open Kafka broker: %v", err))
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
