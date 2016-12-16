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
)

// Broker allows the caller to get info on the cluster's partitions
type Broker interface {
	GetOffset(cp ChainPartition, req *sarama.OffsetRequest) (int64, error)
	Closeable
}

type brokerImpl struct {
	broker *sarama.Broker
}

// Connects to the broker that handles all produce and consume
// requests for the given chain (Partition Leader Replica)
func newBroker(brokers []string, cp ChainPartition) (Broker, error) {
	var candidateBroker, connectedBroker, leaderBroker *sarama.Broker

	// Connect to one of the given brokers
	for _, hostPort := range brokers {
		candidateBroker = sarama.NewBroker(hostPort)
		if err := candidateBroker.Open(nil); err != nil {
			logger.Warningf("Failed to connect to broker %s: %s", hostPort, err)
			continue
		}
		if connected, err := candidateBroker.Connected(); !connected {
			logger.Warningf("Failed to connect to broker %s: %s", hostPort, err)
			continue
		}
		connectedBroker = candidateBroker
		break
	}

	if connectedBroker == nil {
		return nil, fmt.Errorf("Failed to connect to any of the given brokers (%v) for metadata request", brokers)
	}
	logger.Debugf("Connected to broker %s", connectedBroker.Addr())

	// Get metadata for the topic that corresponds to this chain
	metadata, err := connectedBroker.GetMetadata(&sarama.MetadataRequest{Topics: []string{cp.Topic()}})
	if err != nil {
		return nil, fmt.Errorf("Failed to get metadata for topic %s: %s", cp, err)
	}

	// Get the leader broker for this chain partition
	if (cp.Partition() >= 0) && (cp.Partition() < int32(len(metadata.Topics[0].Partitions))) {
		leaderBrokerID := metadata.Topics[0].Partitions[cp.Partition()].Leader
		logger.Debugf("Leading broker for chain %s is broker ID %d", cp, leaderBrokerID)
		for _, availableBroker := range metadata.Brokers {
			if availableBroker.ID() == leaderBrokerID {
				leaderBroker = availableBroker
				break
			}
		}
	}

	if leaderBroker == nil {
		return nil, fmt.Errorf("Can't find leader for chain %s", cp)
	}

	// Connect to broker
	if err := leaderBroker.Open(nil); err != nil {
		return nil, fmt.Errorf("Failed to connect ho Kafka broker: %s", err)
	}
	if connected, err := leaderBroker.Connected(); !connected {
		return nil, fmt.Errorf("Failed to connect to Kafka broker: %s", err)
	}

	return &brokerImpl{broker: leaderBroker}, nil
}

// GetOffset retrieves the offset number that corresponds
// to the requested position in the log.
func (b *brokerImpl) GetOffset(cp ChainPartition, req *sarama.OffsetRequest) (int64, error) {
	resp, err := b.broker.GetAvailableOffsets(req)
	if err != nil {
		return int64(-1), err
	}
	return resp.GetBlock(cp.Topic(), cp.Partition()).Offsets[0], nil
}

// Close terminates the broker.
// This is invoked by the session deliverer's getOffset method.
func (b *brokerImpl) Close() error {
	return b.broker.Close()
}
