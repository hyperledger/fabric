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
)

func TestStaticPartitioner(t *testing.T) {

	var partition int32 = 3
	var numberOfPartitions int32 = 6

	partitionerConstructor := newStaticPartitioner(partition)
	partitioner := partitionerConstructor(testConf.Kafka.Topic)

	for i := 0; i < 10; i++ {
		assignedPartition, err := partitioner.Partition(new(sarama.ProducerMessage), numberOfPartitions)
		if err != nil {
			t.Fatal(err)
		}
		if assignedPartition != partition {
			t.Fatalf("Expected: %v. Actual: %v", partition, assignedPartition)
		}
	}
}

func TestProducerConfigMessageMaxBytes(t *testing.T) {

	topic := testConf.Kafka.Topic

	broker := sarama.NewMockBroker(t, 1000)
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(topic, 0, broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	config := newBrokerConfig(testConf)
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

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
			_, _, err = producer.SendMessage(&sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(make([]byte, tc.size))})
			if err != tc.err {
				t.Fatal(err)
			}
		})

	}

	producer.Close()
	broker.Close()
}

func TestNewBrokerConfig(t *testing.T) {

	topic := testConf.Kafka.Topic

	// use a partition id that is not the 'default' 0
	var partition int32 = 2
	originalPartitionID := testConf.Kafka.PartitionID
	defer func() {
		testConf.Kafka.PartitionID = originalPartitionID
	}()
	testConf.Kafka.PartitionID = partition

	// setup a mock broker that reports that it has 3 partitions for the topic
	broker := sarama.NewMockBroker(t, 1000)
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(topic, 0, broker.BrokerID()).
			SetLeader(topic, 1, broker.BrokerID()).
			SetLeader(topic, 2, broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	config := newBrokerConfig(testConf)
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		assignedPartition, _, err := producer.SendMessage(&sarama.ProducerMessage{Topic: topic})
		if err != nil {
			t.Fatal(err)
		}
		if assignedPartition != partition {
			t.Fatalf("Expected: %v. Actual: %v", partition, assignedPartition)
		}
	}
	producer.Close()
	broker.Close()
}
