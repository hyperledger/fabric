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
	"strconv"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/config"
)

type mockConsumerImpl struct {
	consumedOffset int64
	parent         *mocks.Consumer
	partMgr        *mocks.PartitionConsumer
	partition      sarama.PartitionConsumer
	topic          string
	t              *testing.T
}

func mockNewConsumer(t *testing.T, conf *config.TopLevel, seek int64) (Consumer, error) {
	var err error
	parent := mocks.NewConsumer(t, nil)
	// NOTE The seek flag seems to be useless here.
	// The mock partition will have its highWatermarkOffset
	// initialized to 0 no matter what. I've opened up an issue
	// in the sarama repo: https://github.com/Shopify/sarama/issues/745
	// Until this is resolved, use the testFillWithBlocks() hack below.
	partMgr := parent.ExpectConsumePartition(conf.Kafka.Topic, conf.Kafka.PartitionID, seek)
	partition, err := parent.ConsumePartition(conf.Kafka.Topic, conf.Kafka.PartitionID, seek)
	// mockNewConsumer is basically a helper function when testing.
	// Any errors it generates internally, should result in panic
	// and not get propagated further; checking its errors in the
	// calling functions (i.e. the actual tests) increases boilerplate.
	if err != nil {
		t.Fatal("Cannot create partition consumer:", err)
	}
	mc := &mockConsumerImpl{
		consumedOffset: 0,
		parent:         parent,
		partMgr:        partMgr,
		partition:      partition,
		topic:          conf.Kafka.Topic,
		t:              t,
	}
	// Stop-gap hack until #745 is resolved:
	if seek >= oldestOffset && seek <= (newestOffset-1) {
		mc.testFillWithBlocks(seek - 1) // Prepare the consumer so that the next Recv gives you block "seek"
	} else {
		err = fmt.Errorf("Out of range seek number given to consumer")
	}
	return mc, err
}

func (mc *mockConsumerImpl) Recv() <-chan *sarama.ConsumerMessage {
	if mc.consumedOffset >= newestOffset-1 {
		return nil
	}
	mc.consumedOffset++
	mc.partMgr.YieldMessage(testNewConsumerMessage(mc.consumedOffset, mc.topic))
	return mc.partition.Messages()
}

func (mc *mockConsumerImpl) Close() error {
	if err := mc.partition.Close(); err != nil {
		return err
	}
	return mc.parent.Close()
}

func (mc *mockConsumerImpl) testFillWithBlocks(seek int64) {
	for i := int64(1); i <= seek; i++ {
		<-mc.Recv()
	}
}

func testNewConsumerMessage(offset int64, topic string) *sarama.ConsumerMessage {
	block := &ab.Block{
		Messages: []*ab.BroadcastMessage{
			&ab.BroadcastMessage{
				Data: []byte(strconv.FormatInt(offset, 10)),
			},
		},
		Number: uint64(offset),
	}
	_, data := hashBlock(block)

	return &sarama.ConsumerMessage{
		Value: sarama.ByteEncoder(data),
		Topic: topic,
	}
}
