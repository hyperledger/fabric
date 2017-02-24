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
	"time"

	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
)

type mockConsumerImpl struct {
	consumedOffset int64
	chainPartition ChainPartition

	parentConsumer         *mocks.Consumer
	chainPartitionManager  *mocks.PartitionConsumer
	chainPartitionConsumer sarama.PartitionConsumer

	disk         chan *ab.KafkaMessage
	isSetup      chan struct{}
	targetOffset int64
	t            *testing.T
}

func mockNewConsumer(t *testing.T, cp ChainPartition, offset int64, disk chan *ab.KafkaMessage) (Consumer, error) {
	var err error
	parentConsumer := mocks.NewConsumer(t, nil)
	// NOTE The seek flag seems to be useless here.
	// The mock partition will have its highWatermarkOffset
	// initialized to 0 no matter what. I've opened up an issue
	// in the sarama repo: https://github.com/Shopify/sarama/issues/745
	// Until this is resolved, use the testFillWithBlocks() hack below.
	cpManager := parentConsumer.ExpectConsumePartition(cp.Topic(), cp.Partition(), offset)
	cpConsumer, err := parentConsumer.ConsumePartition(cp.Topic(), cp.Partition(), offset)
	// mockNewConsumer is basically a helper function when testing.
	// Any errors it generates internally, should result in panic
	// and not get propagated further; checking its errors in the
	// calling functions (i.e. the actual tests) increases boilerplate.
	if err != nil {
		t.Fatal("Cannot create mock partition consumer:", err)
	}
	mc := &mockConsumerImpl{
		consumedOffset: 0,
		targetOffset:   offset,
		chainPartition: cp,

		parentConsumer:         parentConsumer,
		chainPartitionManager:  cpManager,
		chainPartitionConsumer: cpConsumer,
		disk:    disk,
		isSetup: make(chan struct{}),
		t:       t,
	}
	// Stop-gap hack until sarama issue #745 is resolved:
	if mc.targetOffset >= testOldestOffset && mc.targetOffset <= (testNewestOffset-1) {
		mc.testFillWithBlocks(mc.targetOffset - 1) // Prepare the consumer so that the next Recv gives you blob #targetOffset
	} else {
		err = fmt.Errorf("Out of range offset (seek number) given to consumer: %d", offset)
		return mc, err
	}

	return mc, err
}

func (mc *mockConsumerImpl) Recv() <-chan *sarama.ConsumerMessage {
	if mc.consumedOffset >= testNewestOffset-1 {
		return nil
	}

	// This is useful in cases where we want to <-Recv() in a for/select loop in
	// a non-blocking manner. Without the timeout, the Go runtime will always
	// execute the body of the Recv() method. If there in no outgoing message
	// available, it will block while waiting on mc.disk. All the other cases in
	// the original for/select loop then won't be evaluated until we unblock on
	// <-mc.disk (which may never happen).
	select {
	case <-time.After(testTimePadding / 2):
	case outgoingMsg := <-mc.disk:
		mc.consumedOffset++
		mc.chainPartitionManager.YieldMessage(testNewConsumerMessage(mc.chainPartition, mc.consumedOffset, outgoingMsg))
		if mc.consumedOffset == mc.targetOffset-1 {
			close(mc.isSetup) // Hook for callers
		}
		return mc.chainPartitionConsumer.Messages()
	}

	return nil
}

func (mc *mockConsumerImpl) Errors() <-chan *sarama.ConsumerError {
	return nil
}

func (mc *mockConsumerImpl) Close() error {
	if err := mc.chainPartitionManager.Close(); err != nil {
		return err
	}
	return mc.parentConsumer.Close()
}

func (mc *mockConsumerImpl) testFillWithBlocks(offset int64) {
	for i := int64(1); i <= offset; i++ {
		go func() {
			mc.disk <- newRegularMessage(utils.MarshalOrPanic(newTestEnvelope(fmt.Sprintf("consumer fill-in %d", i))))
		}()
		<-mc.Recv()
	}
	return
}

func testNewConsumerMessage(cp ChainPartition, offset int64, kafkaMessage *ab.KafkaMessage) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Value:     sarama.ByteEncoder(utils.MarshalOrPanic(kafkaMessage)),
		Topic:     cp.Topic(),
		Partition: cp.Partition(),
		Offset:    offset,
	}
}
