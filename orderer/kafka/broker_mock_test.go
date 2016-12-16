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
	"testing"

	"github.com/Shopify/sarama"
)

type mockBrockerImpl struct {
	brokerImpl

	mockBroker *sarama.MockBroker
	handlerMap map[string]sarama.MockResponse
}

func mockNewBroker(t *testing.T, cp ChainPartition) (Broker, error) {
	mockBroker := sarama.NewMockBroker(t, testBrokerID)
	handlerMap := make(map[string]sarama.MockResponse)
	// The sarama mock package doesn't allow us to return an error
	// for invalid offset requests, so we return an offset of -1.
	// Note that the mock offset responses below imply a broker with
	// newestOffset-1 blocks available. Therefore, if you are using this
	// broker as part of a bigger test where you intend to consume blocks,
	// make sure that the mockConsumer has been initialized accordingly
	// (Set the 'offset' parameter to newestOffset-1.)
	handlerMap["OffsetRequest"] = sarama.NewMockOffsetResponse(t).
		SetOffset(cp.Topic(), cp.Partition(), sarama.OffsetOldest, testOldestOffset).
		SetOffset(cp.Topic(), cp.Partition(), sarama.OffsetNewest, testNewestOffset)
	mockBroker.SetHandlerByMap(handlerMap)

	broker := sarama.NewBroker(mockBroker.Addr())
	if err := broker.Open(nil); err != nil {
		return nil, fmt.Errorf("Cannot connect to mock broker: %s", err)
	}

	return &mockBrockerImpl{
		brokerImpl: brokerImpl{
			broker: broker,
		},
		mockBroker: mockBroker,
		handlerMap: handlerMap,
	}, nil
}

func (mb *mockBrockerImpl) Close() error {
	mb.mockBroker.Close()
	return nil
}
