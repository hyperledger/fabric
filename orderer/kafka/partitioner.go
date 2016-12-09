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

import "github.com/Shopify/sarama"

// newStaticPartitioner returns a PartitionerConstructor that
// returns a Partitioner that always chooses the specified partition.
func newStaticPartitioner(partition int32) sarama.PartitionerConstructor {
	return func(topic string) sarama.Partitioner {
		return &staticPartitioner{partition}
	}
}

type staticPartitioner struct {
	partitionID int32
}

// Partition takes a message and partition count and chooses a partition.
func (p *staticPartitioner) Partition(message *sarama.ProducerMessage, numPartitions int32) (int32, error) {
	return p.partitionID, nil
}

// RequiresConsistency indicates to the user of the partitioner
// whether the mapping of key->partition is consistent or not.
func (p *staticPartitioner) RequiresConsistency() bool {
	return true
}
