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

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

func TestConsumerInitWrong(t *testing.T) {
	cases := []int64{testOldestOffset - 1, testNewestOffset}

	for _, offset := range cases {
		mc, err := mockNewConsumer(t, newChainPartition(provisional.TestChainID, rawPartition), offset, make(chan *ab.KafkaMessage))
		testClose(t, mc)
		if err == nil {
			t.Fatal("Consumer should have failed with out-of-range error")
		}
	}
}

func TestConsumerRecv(t *testing.T) {
	t.Run("oldest", testConsumerRecvFunc(testOldestOffset, testOldestOffset))
	t.Run("in-between", testConsumerRecvFunc(testMiddleOffset, testMiddleOffset))
	t.Run("newest", testConsumerRecvFunc(testNewestOffset-1, testNewestOffset-1))
}

func testConsumerRecvFunc(given, expected int64) func(t *testing.T) {
	disk := make(chan *ab.KafkaMessage)
	return func(t *testing.T) {
		cp := newChainPartition(provisional.TestChainID, rawPartition)
		mc, err := mockNewConsumer(t, cp, given, disk)
		if err != nil {
			testClose(t, mc)
			t.Fatal("Consumer should have proceeded normally:", err)
		}
		<-mc.(*mockConsumerImpl).isSetup
		go func() {
			disk <- newRegularMessage([]byte("foo"))
		}()
		msg := <-mc.Recv()
		if (msg.Topic != cp.Topic()) ||
			msg.Partition != cp.Partition() ||
			msg.Offset != mc.(*mockConsumerImpl).consumedOffset ||
			msg.Offset != expected {
			t.Fatalf("Expected message with offset %d, got %d", expected, msg.Offset)
		}
		testClose(t, mc)
	}
}
