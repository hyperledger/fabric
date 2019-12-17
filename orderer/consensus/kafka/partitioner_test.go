/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
)

func TestStaticPartitioner(t *testing.T) {
	var partition int32 = 3
	var numberOfPartitions int32 = 6

	partitionerConstructor := newStaticPartitioner(partition)
	partitioner := partitionerConstructor(channelNameForTest(t))

	for i := 0; i < 10; i++ {
		assignedPartition, err := partitioner.Partition(new(sarama.ProducerMessage), numberOfPartitions)
		assert.NoError(t, err, "Partitioner not functioning as expected:", err)
		assert.Equal(t, partition, assignedPartition, "Partitioner not returning the expected partition - expected %d, got %v", partition, assignedPartition)
	}
}
