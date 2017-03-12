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

	"github.com/Shopify/sarama/mocks"
	"github.com/golang/protobuf/proto"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

type mockProducerImpl struct {
	producer *mocks.SyncProducer
	checker  mocks.ValueChecker

	// This simulates the broker's "disk" where the producer's
	// blobs for a certain chain partition eventually end up.
	disk           chan *ab.KafkaMessage
	producedOffset int64
	isSetup        chan struct{}
	t              *testing.T
}

// Create a new producer whose next "Send" on ChainPartition gives you blob #offset.
func mockNewProducer(t *testing.T, cp ChainPartition, offset int64, disk chan *ab.KafkaMessage) Producer {
	mp := &mockProducerImpl{
		producer:       mocks.NewSyncProducer(t, nil),
		checker:        nil,
		disk:           disk,
		producedOffset: 0,
		isSetup:        make(chan struct{}),
		t:              t,
	}
	mp.init(cp, offset)

	if mp.producedOffset == offset-1 {
		close(mp.isSetup)
	} else {
		mp.t.Fatal("Mock producer failed to initialize itself properly")
	}

	return mp
}

func (mp *mockProducerImpl) Send(cp ChainPartition, payload []byte) error {
	mp.producer.ExpectSendMessageWithCheckerFunctionAndSucceed(mp.checker)
	mp.producedOffset++ // This is the offset that will be assigned to the sent message
	if _, ofs, err := mp.producer.SendMessage(newProducerMessage(cp, payload)); err != nil || ofs != mp.producedOffset {
		// We do NOT check the assigned partition because the mock
		// producer always posts to partition 0 no matter what.
		// This is a deficiency of the Kafka library that we use.
		mp.t.Fatal("Mock producer not functioning as expected")
	}
	msg := new(ab.KafkaMessage)
	if err := proto.Unmarshal(payload, msg); err != nil {
		mp.t.Fatalf("Failed to unmarshal message that reached producer's disk: %s", err)
	}
	mp.disk <- msg // Reaches the cluster's disk for that chain partition
	return nil
}

func (mp *mockProducerImpl) Close() error {
	return mp.producer.Close()
}

// Initializes the mock producer by setting up the offsets.
func (mp *mockProducerImpl) init(cp ChainPartition, offset int64) {
	if offset >= testOldestOffset && offset <= (testNewestOffset-1) {
		// Prepare the producer so that the next Send
		// on that chain partition gives you blob #offset.
		mp.testFillWithBlocks(cp, offset-1)
	} else {
		panic(fmt.Errorf("Out of range offset (seek number) given to producer: %d", offset))
	}
}

func (mp *mockProducerImpl) testFillWithBlocks(cp ChainPartition, offset int64) {
	dieChan := make(chan struct{})
	deadChan := make(chan struct{})

	go func() { // This goroutine is meant to read only the "fill-in" blocks
		for {
			select {
			case <-mp.disk:
			case <-dieChan:
				close(deadChan)
				return
			}
		}
	}()

	for i := int64(1); i <= offset; i++ {
		mp.Send(cp, utils.MarshalOrPanic(newRegularMessage(utils.MarshalOrPanic(newTestEnvelope(fmt.Sprintf("producer fill-in %d", i))))))
	}

	close(dieChan)
	<-deadChan

	return
}
