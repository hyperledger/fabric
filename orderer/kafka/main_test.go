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
	"sync"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mockmultichain "github.com/hyperledger/fabric/orderer/mocks/multichain"
	mocksharedconfig "github.com/hyperledger/fabric/orderer/mocks/sharedconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var cp = newChainPartition(provisional.TestChainID, rawPartition)

func newMockSharedConfigManager() *mocksharedconfig.Manager {
	return &mocksharedconfig.Manager{KafkaBrokersVal: testConf.Kafka.Brokers}
}

func syncQueueMessage(msg *cb.Envelope, chain multichain.Chain, bc *mockblockcutter.Receiver) {
	chain.Enqueue(msg)
	bc.Block <- struct{}{}
}

type mockConsenterImpl struct {
	consenterImpl
	prodDisk, consDisk chan *ab.KafkaMessage
	consumerSetUp      bool
	t                  *testing.T
}

func mockNewConsenter(t *testing.T, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry) *mockConsenterImpl {
	prodDisk := make(chan *ab.KafkaMessage)
	consDisk := make(chan *ab.KafkaMessage)

	mockBfValue := func(brokers []string, cp ChainPartition) (Broker, error) {
		return mockNewBroker(t, cp)
	}
	mockPfValue := func(brokers []string, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry) Producer {
		return mockNewProducer(t, cp, testOldestOffset, prodDisk)
	}
	mockCfValue := func(brokers []string, kafkaVersion sarama.KafkaVersion, cp ChainPartition, offset int64) (Consumer, error) {
		return mockNewConsumer(t, cp, offset, consDisk)
	}

	return &mockConsenterImpl{
		consenterImpl: consenterImpl{
			kv: kafkaVersion,
			ro: retryOptions,
			bf: mockBfValue,
			pf: mockPfValue,
			cf: mockCfValue,
		},
		prodDisk: prodDisk,
		consDisk: consDisk,
		t:        t,
	}
}

func TestKafkaConsenterEmptyBatch(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: newMockSharedConfigManager(),
	}
	defer close(cs.BlockCutterVal.Block)

	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry)
	ch := newChain(co, cs)
	ch.lastProcessed = testOldestOffset - 1

	go ch.Start()
	defer ch.Halt()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait until the mock producer is done before messing around with its disk
		select {
		case <-ch.producer.(*mockProducerImpl).isSetup:
			// Dispense the CONNECT message that is posted with Start()
			<-co.prodDisk
		case <-time.After(testTimePadding):
			t.Fatal("Mock producer not setup in time")
		}
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Pick up the message that will be posted via the syncQueueMessage/Enqueue call below
		msg := <-co.prodDisk
		// Place it to the right location so that the mockConsumer can read it
		co.consDisk <- msg
	}()

	syncQueueMessage(newTestEnvelope("one"), ch, cs.BlockCutterVal)
	// The message has already been moved to the consumer's disk,
	// otherwise syncQueueMessage wouldn't return, so the Wait()
	// here is unnecessary but let's be paranoid.
	wg.Wait()

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterConfigStyleMultiBatch(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: newMockSharedConfigManager(),
	}
	defer close(cs.BlockCutterVal.Block)

	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry)
	ch := newChain(co, cs)
	ch.lastProcessed = testOldestOffset - 1

	go ch.Start()
	defer ch.Halt()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait until the mock producer is done before messing around with its disk
		select {
		case <-ch.producer.(*mockProducerImpl).isSetup:
			// Dispense the CONNECT message that is posted with Start()
			<-co.prodDisk
		case <-time.After(testTimePadding):
			t.Fatal("Mock producer not setup in time")
		}
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Pick up the message that will be posted via the syncQueueMessage/Enqueue call below
		msg := <-co.prodDisk
		// Place it to the right location so that the mockConsumer can read it
		co.consDisk <- msg
	}()
	syncQueueMessage(newTestEnvelope("one"), ch, cs.BlockCutterVal)
	wg.Wait()

	cs.BlockCutterVal.IsolatedTx = true

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Pick up the message that will be posted via the syncQueueMessage/Enqueue call below
		msg := <-co.prodDisk
		// Place it to the right location so that the mockConsumer can read it
		co.consDisk <- msg
	}()
	syncQueueMessage(newTestEnvelope("two"), ch, cs.BlockCutterVal)
	wg.Wait()

	ch.Halt()

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatal("Expected two blocks to be cut but never got the first")
	}

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatal("Expected the config type tx to create two blocks, but only got the first")
	}

	select {
	case <-time.After(testTimePadding):
		t.Fatal("Should have exited")
	case <-ch.haltedChan:
	}
}
