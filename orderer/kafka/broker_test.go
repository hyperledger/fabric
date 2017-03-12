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
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
)

func TestBrokerGetOffset(t *testing.T) {
	t.Run("oldest", testBrokerGetOffsetFunc(sarama.OffsetOldest, testOldestOffset))
	t.Run("newest", testBrokerGetOffsetFunc(sarama.OffsetNewest, testNewestOffset))
}

func testBrokerGetOffsetFunc(given, expected int64) func(t *testing.T) {
	cp := newChainPartition(provisional.TestChainID, rawPartition)
	return func(t *testing.T) {
		mb, _ := mockNewBroker(t, cp)
		defer testClose(t, mb)

		ofs, _ := mb.GetOffset(cp, newOffsetReq(cp, given))
		if ofs != expected {
			t.Fatalf("Expected offset %d, got %d instead", expected, ofs)
		}
	}
}

func TestNewBrokerReturnsPartitionLeader(t *testing.T) {
	cp := newChainPartition(provisional.TestChainID, rawPartition)
	broker1 := sarama.NewMockBroker(t, 1)
	broker2 := sarama.NewMockBroker(t, 2)
	broker3 := sarama.NewMockBroker(t, 3)
	defer func() {
		broker2.Close()
		broker3.Close()
	}()

	// Use broker1 and broker2 as bootstrap brokers, but shutdown broker1 right away
	broker1.Close()

	// Add expectation that broker2 will return a metadata response
	// that identifies broker3 as the topic partition leader
	broker2.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker1.Addr(), broker1.BrokerID()).
			SetBroker(broker2.Addr(), broker2.BrokerID()).
			SetBroker(broker3.Addr(), broker3.BrokerID()).
			SetLeader(cp.Topic(), cp.Partition(), broker3.BrokerID()),
	})

	// Add expectation that broker3 responds to an offset request
	broker3.SetHandlerByMap(map[string]sarama.MockResponse{
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(cp.Topic(), cp.Partition(), sarama.OffsetOldest, testOldestOffset).
			SetOffset(cp.Topic(), cp.Partition(), sarama.OffsetNewest, testNewestOffset),
	})

	// Get leader for the test chain partition
	leaderBroker, _ := newBroker([]string{broker1.Addr(), broker2.Addr()}, cp)

	// Only broker3 will respond successfully to an offset request
	offsetRequest := new(sarama.OffsetRequest)
	offsetRequest.AddBlock(cp.Topic(), cp.Partition(), -1, 1)
	if _, err := leaderBroker.GetOffset(cp, offsetRequest); err != nil {
		t.Fatal("Expected leader broker to respond to request:", err)
	}
}
