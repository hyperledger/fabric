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

func TestBrokerGetOffset(t *testing.T) {
	t.Run("oldest", testBrokerGetOffsetFunc(sarama.OffsetOldest, oldestOffset))
	t.Run("newest", testBrokerGetOffsetFunc(sarama.OffsetNewest, newestOffset))
}

func testBrokerGetOffsetFunc(given, expected int64) func(t *testing.T) {
	return func(t *testing.T) {
		mb := mockNewBroker(t, testConf)
		defer testClose(t, mb)

		offset, _ := mb.GetOffset(newOffsetReq(mb.(*mockBrockerImpl).config, given))
		if offset != expected {
			t.Fatalf("Expected offset %d, got %d instead", expected, offset)
		}
	}
}

func TestNewBrokerReturnsPartitionLeader(t *testing.T) {

	// sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Lshortfile)
	// SetLogLevel("debug")

	broker1 := sarama.NewMockBroker(t, 1001)
	broker2 := sarama.NewMockBroker(t, 1002)
	broker3 := sarama.NewMockBroker(t, 1003)

	// shutdown broker1
	broker1.Close()

	// update list of bootstrap brokers in config
	originalKafkaBrokers := testConf.Kafka.Brokers
	defer func() {
		testConf.Kafka.Brokers = originalKafkaBrokers
	}()
	// add broker1, and broker2 to list of bootstrap brokers
	// broker1 is 'down'
	// broker3 will be discovered via a metadata request
	testConf.Kafka.Brokers = []string{broker1.Addr(), broker2.Addr()}

	// handy references
	topic := testConf.Kafka.Topic
	partition := testConf.Kafka.PartitionID

	// add expectation that broker2 will return a metadata response that
	// identifies broker3 as the topic partition leader
	broker2.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker1.Addr(), broker1.BrokerID()).
			SetBroker(broker2.Addr(), broker2.BrokerID()).
			SetBroker(broker3.Addr(), broker3.BrokerID()).
			SetLeader(topic, partition, broker3.BrokerID()),
	})

	// add expectation that broker3 respond to an offset request
	broker3.SetHandlerByMap(map[string]sarama.MockResponse{
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(topic, partition, sarama.OffsetOldest, 0).
			SetOffset(topic, partition, sarama.OffsetNewest, 42),
	})

	// get leader for topic partition
	broker := newBroker(testConf)

	// only broker3 will respond successfully to an offset request
	offsetRequest := new(sarama.OffsetRequest)
	offsetRequest.AddBlock(topic, partition, -1, 1)
	if _, err := broker.GetOffset(offsetRequest); err != nil {
		t.Fatal(err)
	}

	broker2.Close()
	broker3.Close()

}
