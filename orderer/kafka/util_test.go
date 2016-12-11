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
	"testing"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
)

func TestProducerConfigMessageMaxBytes(t *testing.T) {
	cp := newChainPartition(provisional.TestChainID, rawPartition)

	broker := sarama.NewMockBroker(t, 1)
	defer func() {
		broker.Close()
	}()
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(cp.Topic(), cp.Partition(), broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	config := newBrokerConfig(testConf.Kafka.Version, rawPartition)
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		producer.Close()
	}()

	testCases := []struct {
		name string
		size int
		err  error
	}{
		{"TypicalDeploy", 8 * 1024 * 1024, nil},
		{"TooBig", 100*1024*1024 + 1, sarama.ErrMessageSizeTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err = producer.SendMessage(&sarama.ProducerMessage{
				Topic: cp.Topic(),
				Value: sarama.ByteEncoder(make([]byte, tc.size)),
			})
			if err != tc.err {
				t.Fatal(err)
			}
		})

	}
}

func TestNewBrokerConfig(t *testing.T) {
	// Use a partition ID that is not the 'default' (rawPartition)
	var differentPartition int32 = 2
	cp = newChainPartition(provisional.TestChainID, differentPartition)

	// Setup a mock broker that reports that it has 3 partitions for the topic
	broker := sarama.NewMockBroker(t, 1)
	defer func() {
		broker.Close()
	}()
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(cp.Topic(), 0, broker.BrokerID()).
			SetLeader(cp.Topic(), 1, broker.BrokerID()).
			SetLeader(cp.Topic(), 2, broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	config := newBrokerConfig(testConf.Kafka.Version, differentPartition)
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal("Failed to create producer:", err)
	}
	defer func() {
		producer.Close()
	}()

	for i := 0; i < 10; i++ {
		assignedPartition, _, err := producer.SendMessage(&sarama.ProducerMessage{Topic: cp.Topic()})
		if err != nil {
			t.Fatal("Failed to send message:", err)
		}
		if assignedPartition != differentPartition {
			t.Fatalf("Message wasn't posted to the right partition - expected: %d, got %v", differentPartition, assignedPartition)
		}
	}
}
