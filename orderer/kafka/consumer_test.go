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

import "testing"

func TestConsumerInitWrong(t *testing.T) {
	cases := []int64{oldestOffset - 1, newestOffset}

	for _, seek := range cases {
		mc, err := mockNewConsumer(t, testConf, seek)
		testClose(t, mc)
		if err == nil {
			t.Fatal("Consumer should have failed with out-of-range error")
		}
	}
}

/* Disabling this until the upgrade to Go 1.7 kicks in
func TestConsumerRecv(t *testing.T) {
	t.Run("oldest", testConsumerRecvFunc(oldestOffset, oldestOffset))
	t.Run("in-between", testConsumerRecvFunc(middleOffset, middleOffset))
	t.Run("newest", testConsumerRecvFunc(newestOffset-1, newestOffset-1))
} */

func testConsumerRecvFunc(given, expected int64) func(t *testing.T) {
	return func(t *testing.T) {
		mc, err := mockNewConsumer(t, testConf, given)
		if err != nil {
			testClose(t, mc)
			t.Fatalf("Consumer should have proceeded normally: %s", err)
		}
		msg := <-mc.Recv()
		if (msg.Topic != testConf.Kafka.Topic) ||
			msg.Partition != testConf.Kafka.PartitionID ||
			msg.Offset != mc.(*mockConsumerImpl).consumedOffset ||
			msg.Offset != expected {
			t.Fatalf("Expected block %d, got %d", expected, msg.Offset)
		}
		testClose(t, mc)
	}
}
