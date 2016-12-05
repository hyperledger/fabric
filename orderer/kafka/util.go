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
	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/localconfig"
)

const (
	ackOutOfRangeError    = "ACK out of range"
	seekOutOfRangeError   = "Seek out of range"
	windowOutOfRangeError = "Window out of range"
)

func newBrokerConfig(conf *config.TopLevel) *sarama.Config {
	brokerConfig := sarama.NewConfig()
	brokerConfig.Version = conf.Kafka.Version
	brokerConfig.Producer.Partitioner = newStaticPartitioner(conf.Kafka.PartitionID)
	// set equivalent of kafka producer config max.request.bytes to the deafult
	// value of a kafka server's socket.request.max.bytes property (100MiB).
	brokerConfig.Producer.MaxMessageBytes = int(sarama.MaxRequestSize)
	return brokerConfig
}

func newMsg(payload []byte, topic string) *sarama.ProducerMessage {
	return &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(payload),
	}
}

func newOffsetReq(conf *config.TopLevel, seek int64) *sarama.OffsetRequest {
	req := &sarama.OffsetRequest{}
	// If seek == -1, ask for the for the offset assigned to next new message
	// If seek == -2, ask for the earliest available offset
	// The last parameter in the AddBlock call is needed for God-knows-why reasons.
	// From the Kafka folks themselves: "We agree that this API is slightly funky."
	// https://mail-archives.apache.org/mod_mbox/kafka-users/201411.mbox/%3Cc159383825e04129b77253ffd6c448aa@BY2PR02MB505.namprd02.prod.outlook.com%3E
	req.AddBlock(conf.Kafka.Topic, conf.Kafka.PartitionID, seek, 1)
	return req
}

// newStaticPartitioner returns a PartitionerConstructor that returns a Partitioner
// that always chooses the specified partition.
func newStaticPartitioner(partition int32) sarama.PartitionerConstructor {
	return func(topic string) sarama.Partitioner {
		return &staticPartitioner{partition}
	}
}

type staticPartitioner struct {
	partitionID int32
}

func (p *staticPartitioner) Partition(message *sarama.ProducerMessage, numPartitions int32) (int32, error) {
	return p.partitionID, nil
}

func (p *staticPartitioner) RequiresConsistency() bool {
	return true
}
