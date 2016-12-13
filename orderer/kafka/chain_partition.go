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

import "fmt"

const rawPartition = 0

// ChainPartition identifies the Kafka partition the orderer interacts with.
type ChainPartition interface {
	Topic() string
	Partition() int32
	fmt.Stringer
}

type chainPartitionImpl struct {
	tpc string
	prt int32
}

// Returns a new chain partition for a given chain ID and partition.
func newChainPartition(chainID string, partition int32) ChainPartition {
	return &chainPartitionImpl{
		// TODO https://github.com/apache/kafka/blob/trunk/core/src/main/scala/kafka/common/Topic.scala#L29
		tpc: fmt.Sprintf("%x", chainID),
		prt: partition,
	}
}

// Topic returns the Kafka topic of this chain partition.
func (cp *chainPartitionImpl) Topic() string {
	return cp.tpc
}

// Partition returns the Kafka partition of this chain partition.
func (cp *chainPartitionImpl) Partition() int32 {
	return cp.prt
}

// String returns a string identifying the chain partition.
func (cp *chainPartitionImpl) String() string {
	return fmt.Sprintf("%s/%d", cp.tpc, cp.prt)
}
