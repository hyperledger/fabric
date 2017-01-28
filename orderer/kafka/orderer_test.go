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
	"github.com/hyperledger/fabric/protos/utils"
)

var cp = newChainPartition(provisional.TestChainID, rawPartition)

func newMockSharedConfigManager() *mocksharedconfig.Manager {
	return &mocksharedconfig.Manager{KafkaBrokersVal: testConf.Kafka.Brokers}
}

type mockConsenterImpl struct {
	consenterImpl
	prodDisk, consDisk chan *ab.KafkaMessage
	consumerSetUp      bool
	t                  *testing.T
}

func mockNewConsenter(t *testing.T, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry, nextProducedOffset int64) *mockConsenterImpl {
	prodDisk := make(chan *ab.KafkaMessage)
	consDisk := make(chan *ab.KafkaMessage)

	mockTLS := config.TLS{Enabled: false}

	mockBfValue := func(brokers []string, cp ChainPartition) (Broker, error) {
		return mockNewBroker(t, cp)
	}
	mockPfValue := func(brokers []string, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry, tls config.TLS) Producer {
		// The first Send on this producer will return a blob with offset #nextProducedOffset
		return mockNewProducer(t, cp, nextProducedOffset, prodDisk)
	}
	mockCfValue := func(brokers []string, kafkaVersion sarama.KafkaVersion, tls config.TLS, cp ChainPartition, lastPersistedOffset int64) (Consumer, error) {
		if lastPersistedOffset != nextProducedOffset {
			panic(fmt.Errorf("Mock objects about to be set up incorrectly (consumer to seek to %d, producer to post %d)", lastPersistedOffset, nextProducedOffset))
		}
		return mockNewConsumer(t, cp, lastPersistedOffset, consDisk)
	}

	return &mockConsenterImpl{
		consenterImpl: consenterImpl{
			kv:  kafkaVersion,
			ro:  retryOptions,
			tls: mockTLS,
			bf:  mockBfValue,
			pf:  mockPfValue,
			cf:  mockCfValue,
		},
		prodDisk: prodDisk,
		consDisk: consDisk,
		t:        t,
	}
}

func prepareMockObjectDisks(t *testing.T, co *mockConsenterImpl, ch *chainImpl) {
	// Wait until the mock producer is done before messing around with its disk
	select {
	case <-ch.producer.(*mockProducerImpl).isSetup:
		// Dispense with the CONNECT message that is posted with Start()
		<-co.prodDisk
	case <-time.After(testTimePadding):
		t.Fatal("Mock producer not setup in time")
	}
	// Same for the mock consumer
	select {
	case <-ch.setupChan:
	case <-time.After(testTimePadding):
		t.Fatal("Mock consumer not setup in time")
	}
}

func syncQueueMessage(msg *cb.Envelope, chain multichain.Chain, bc *mockblockcutter.Receiver) {
	chain.Enqueue(msg)
	bc.Block <- struct{}{}
}

func waitableSyncQueueMessage(env *cb.Envelope, messagesToPickUp int, wg *sync.WaitGroup,
	co *mockConsenterImpl, cs *mockmultichain.ConsenterSupport, ch *chainImpl) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < messagesToPickUp; i++ {
			// On the first iteration of this loop, the message that will be picked up
			// is the one posted via the syncQueueMessage/Enqueue call below
			msg := <-co.prodDisk
			// Place it to the right location so that the mockConsumer can read it
			co.consDisk <- msg
		}
	}()

	syncQueueMessage(env, ch, cs.BlockCutterVal)
	// The message has already been moved to the consumer's disk,
	// otherwise syncQueueMessage wouldn't return, so the Wait()
	// here is unnecessary but let's be paranoid.
	wg.Wait()
}

func TestKafkaConsenterEmptyBatch(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: testTimePadding},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterBatchTimer(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1ms")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	// The second message that will be picked up is the time-to-cut message
	// that will be posted when the short timer expires
	waitableSyncQueueMessage(newTestEnvelope("one"), 2, &wg, co, cs, ch)

	select {
	case <-cs.Batches: // This is the success path
	case <-time.After(testTimePadding):
		t.Fatal("Expected block to be cut because batch timer expired")
	}

	// As above
	waitableSyncQueueMessage(newTestEnvelope("two"), 2, &wg, co, cs, ch)

	select {
	case <-cs.Batches: // This is the success path
	case <-time.After(testTimePadding):
		t.Fatal("Expected second block to be cut, batch timer not reset")
	}

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterTimerHaltOnFilledBatch(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1h")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true

	waitableSyncQueueMessage(newTestEnvelope("two"), 1, &wg, co, cs, ch)

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatal("Expected block to be cut because batch timer expired")
	}

	// Change the batch timeout to be near instant.
	// If the timer was not reset, it will still be waiting an hour.
	ch.batchTimeout = time.Millisecond

	cs.BlockCutterVal.CutNext = false

	// The second message that will be picked up is the time-to-cut message
	// that will be posted when the short timer expires
	waitableSyncQueueMessage(newTestEnvelope("three"), 2, &wg, co, cs, ch)

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatalf("Did not cut the second block, indicating that the old timer was still running")
	}

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
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: testTimePadding},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.IsolatedTx = true

	waitableSyncQueueMessage(newTestEnvelope("two"), 1, &wg, co, cs, ch)

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

func TestKafkaConsenterTimeToCutForced(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1h")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true

	// This is like the waitableSyncQueueMessage routine with the difference
	// that we post a time-to-cut message instead of a test envelope.
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := <-co.prodDisk
		co.consDisk <- msg
	}()

	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock+1))); err != nil {
		t.Fatalf("Couldn't post to %s: %s", ch.partition, err)
	}
	wg.Wait()

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatal("Expected block to be cut because proper time-to-cut was sent")
	}

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterTimeToCutDuplicate(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1h")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true

	// This is like the waitableSyncQueueMessage routine with the difference
	// that we post a time-to-cut message instead of a test envelope.
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := <-co.prodDisk
		co.consDisk <- msg
	}()

	// Send a proper time-to-cut message
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock+1))); err != nil {
		t.Fatalf("Couldn't post to %s: %s", ch.partition, err)
	}
	wg.Wait()

	select {
	case <-cs.Batches:
	case <-time.After(testTimePadding):
		t.Fatal("Expected block to be cut because proper time-to-cut was sent")
	}

	cs.BlockCutterVal.CutNext = false

	waitableSyncQueueMessage(newTestEnvelope("two"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true
	// ATTN: We set `cs.BlockCutterVal.CutNext` to true on purpose
	// If the logic works right, the orderer should discard the
	// duplicate TTC message below and a call to the block cutter
	// will only happen when the long, hour-long timer expires

	// As above
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := <-co.prodDisk
		co.consDisk <- msg
	}()

	// Send a duplicate time-to-cut message
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock))); err != nil {
		t.Fatalf("Couldn't post to %s: %s", ch.partition, err)
	}
	wg.Wait()

	select {
	case <-cs.Batches:
		t.Fatal("Should have discarded duplicate time-to-cut")
	case <-time.After(testTimePadding):
		// This is the success path
	}

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterTimeToCutStale(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1h")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true

	// This is like the waitableSyncQueueMessage routine with the difference
	// that we post a time-to-cut message instead of a test envelope.
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := <-co.prodDisk
		co.consDisk <- msg
	}()

	// Send a stale time-to-cut message
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock))); err != nil {
		t.Fatalf("Couldn't post to %s: %s", ch.partition, err)
	}
	wg.Wait()

	select {
	case <-cs.Batches:
		t.Fatal("Should have ignored stale time-to-cut")
	case <-time.After(testTimePadding):
		// This is the success path
	}

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestKafkaConsenterTimeToCutLarger(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1h")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	waitableSyncQueueMessage(newTestEnvelope("one"), 1, &wg, co, cs, ch)

	cs.BlockCutterVal.CutNext = true

	// This is like the waitableSyncQueueMessage routine with the difference
	// that we post a time-to-cut message instead of a test envelope.
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := <-co.prodDisk
		co.consDisk <- msg
	}()

	// Send a stale time-to-cut message
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock+2))); err != nil {
		t.Fatalf("Couldn't post to %s: %s", ch.partition, err)
	}
	wg.Wait()

	select {
	case <-cs.Batches:
		t.Fatal("Should have ignored larger time-to-cut than expected")
	case <-time.After(testTimePadding):
		// This is the success path
	}

	// Loop is already stopped, but this is a good test to see
	// if a second invokation of Halt() panicks. (It shouldn't.)
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("Expected duplicate call to Halt to succeed")
		}
	}()

	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}
}

func TestGetLastOffsetPersistedEmpty(t *testing.T) {
	expected := sarama.OffsetOldest - 1
	actual := getLastOffsetPersisted(&cb.Metadata{})
	if actual != expected {
		t.Fatalf("Expected last offset %d, got %d", expected, actual)
	}
}

func TestGetLastOffsetPersistedRight(t *testing.T) {
	expected := int64(100)
	actual := getLastOffsetPersisted(&cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: expected})})
	if actual != expected {
		t.Fatalf("Expected last offset %d, got %d", expected, actual)
	}
}

func TestKafkaConsenterRestart(t *testing.T) {
	var wg sync.WaitGroup
	defer wg.Wait()

	batchTimeout, _ := time.ParseDuration("1ms")
	cs := &mockmultichain.ConsenterSupport{
		Batches:         make(chan []*cb.Envelope),
		BlockCutterVal:  mockblockcutter.NewReceiver(),
		ChainIDVal:      provisional.TestChainID,
		SharedConfigVal: &mocksharedconfig.Manager{BatchTimeoutVal: batchTimeout},
	}
	defer close(cs.BlockCutterVal.Block)

	lastPersistedOffset := testOldestOffset - 1
	nextProducedOffset := lastPersistedOffset + 1
	co := mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch := newChain(co, cs, lastPersistedOffset)

	go ch.Start()
	defer ch.Halt()

	prepareMockObjectDisks(t, co, ch)

	// The second message that will be picked up is the time-to-cut message
	// that will be posted when the short timer expires
	waitableSyncQueueMessage(newTestEnvelope("one"), 2, &wg, co, cs, ch)

	select {
	case <-cs.Batches: // This is the success path
	case <-time.After(testTimePadding):
		t.Fatal("Expected block to be cut because batch timer expired")
	}

	// Stop the loop
	ch.Halt()

	select {
	case <-cs.Batches:
		t.Fatal("Expected no invocations of Append")
	case <-ch.haltedChan: // If we're here, we definitely had a chance to invoke Append but didn't (which is great)
	}

	lastBlock := cs.WriteBlockVal
	metadata, err := utils.GetMetadataFromBlock(lastBlock, cb.BlockMetadataIndex_ORDERER)
	if err != nil {
		logger.Fatalf("Error extracting orderer metadata for chain %x: %s", cs.ChainIDVal, err)
	}

	lastPersistedOffset = getLastOffsetPersisted(metadata)
	nextProducedOffset = lastPersistedOffset + 1

	co = mockNewConsenter(t, testConf.Kafka.Version, testConf.Kafka.Retry, nextProducedOffset)
	ch = newChain(co, cs, lastPersistedOffset)
	go ch.Start()
	prepareMockObjectDisks(t, co, ch)

	actual := ch.producer.(*mockProducerImpl).producedOffset
	if actual != nextProducedOffset {
		t.Fatalf("Restarted orderer post-connect should have been at offset %d, got %d instead", nextProducedOffset, actual)
	}
}
