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

func TestStaticPartitioner(t *testing.T) {
	var partition int32 = 3
	var numberOfPartitions int32 = 6

	partitionerConstructor := newStaticPartitioner(partition)
	partitioner := partitionerConstructor(provisional.TestChainID)

	for i := 0; i < 10; i++ {
		assignedPartition, err := partitioner.Partition(new(sarama.ProducerMessage), numberOfPartitions)
		if err != nil {
			t.Fatal("Partitioner not functioning as expected:", err)
		}
		if assignedPartition != partition {
			t.Fatalf("Partitioner not returning the expected partition - expected %d, got %v", partition, assignedPartition)
		}
	}
}
