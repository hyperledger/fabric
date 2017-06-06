/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"fmt"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mockmultichain "github.com/hyperledger/fabric/orderer/mocks/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

var hitBranch = 50 * time.Millisecond

func TestChain(t *testing.T) {
	oldestOffset := int64(0)
	newestOffset := int64(5)
	message := sarama.StringEncoder("messageFoo")

	mockChannel := newChannel("channelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
			SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t).
			SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
		"FetchRequest": sarama.NewMockFetchResponse(t, 1).
			SetMessage(mockChannel.topic(), mockChannel.partition(), newestOffset, message),
	})

	mockSupport := &mockmultichain.ConsenterSupport{
		ChainIDVal:      mockChannel.topic(),
		HeightVal:       uint64(3),
		SharedConfigVal: &mockconfig.Orderer{KafkaBrokersVal: []string{mockBroker.Addr()}},
	}

	t.Run("New", func(t *testing.T) {
		_, err := newChain(mockConsenter, mockSupport, newestOffset-1)
		assert.NoError(t, err, "Expected newChain to return without errors")
	})

	t.Run("Start", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1) // -1 because we haven't set the CONNECT message yet
		chain.Start()
		assert.Equal(t, true, chain.startCompleted, "Expected chain.startCompleted flag to be set to true")
		assert.Equal(t, false, chain.halted, "Expected chain.halted flag to be set to false")
		close(chain.exitChan)
	})

	t.Run("Halt", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)
		chain.Start()
		chain.Halt()
		_, ok := <-chain.exitChan
		assert.Equal(t, false, ok, "Expected chain.exitChan to be closed")
		time.Sleep(50 * time.Millisecond) // Let closeLoop() do its thing -- TODO Hacky, revise approach
		assert.Equal(t, true, chain.halted, "Expected chain.halted flag to be set true")
	})

	t.Run("DoubleHalt", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)
		chain.Start()
		chain.Halt()

		assert.NotPanics(t, func() { chain.Halt() }, "Calling Halt() more than once shouldn't panic")

		_, ok := <-chain.exitChan
		assert.Equal(t, false, ok, "Expected chain.exitChan to be closed")
	})

	t.Run("StartWithProducerForChannelError", func(t *testing.T) {
		mockSupportCopy := *mockSupport
		mockSupportCopy.SharedConfigVal = &mockconfig.Orderer{KafkaBrokersVal: []string{}}

		chain, _ := newChain(mockConsenter, &mockSupportCopy, newestOffset-1)

		chain.Start()

		assert.Equal(t, false, chain.startCompleted, "Expected chain.startCompleted flag to be set to false")
		assert.Equal(t, true, chain.halted, "Expected chain.halted flag to be set to set to true")
		_, ok := <-chain.exitChan
		assert.Equal(t, false, ok, "Expected chain.exitChan to be closed")
	})

	t.Run("StartWithConnectMessageError", func(t *testing.T) {
		mockBrokerConfigCopy := *mockBrokerConfig
		mockBrokerConfigCopy.Net.ReadTimeout = 5 * time.Millisecond
		mockBrokerConfigCopy.Consumer.Retry.Backoff = 5 * time.Millisecond
		mockBrokerConfigCopy.Metadata.Retry.Max = 1

		mockConsenterCopy := newMockConsenter(&mockBrokerConfigCopy, mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version)

		chain, _ := newChain(mockConsenterCopy, mockSupport, newestOffset-1)

		mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockMetadataResponse(t).
				SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
				SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotLeaderForPartition),
			"OffsetRequest": sarama.NewMockOffsetResponse(t).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
			"FetchRequest": sarama.NewMockFetchResponse(t, 1).
				SetMessage(mockChannel.topic(), mockChannel.partition(), newestOffset, message),
		})

		chain.Start()

		assert.Equal(t, false, chain.startCompleted, "Expected chain.startCompleted flag to be set to false")
		assert.Equal(t, true, chain.halted, "Expected chain.halted flag to be set to true")
		_, ok := <-chain.exitChan
		assert.Equal(t, false, ok, "Expected chain.exitChan to be closed")
	})

	t.Run("StartWithConsumerForChannelError", func(t *testing.T) {
		mockBrokerConfigCopy := *mockBrokerConfig
		mockBrokerConfigCopy.Net.ReadTimeout = 5 * time.Millisecond
		mockBrokerConfigCopy.Consumer.Retry.Backoff = 5 * time.Millisecond
		mockBrokerConfigCopy.Metadata.Retry.Max = 1

		mockConsenterCopy := newMockConsenter(&mockBrokerConfigCopy, mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version)

		chain, _ := newChain(mockConsenterCopy, mockSupport, newestOffset) // Provide an out-of-range offset

		mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockMetadataResponse(t).
				SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
				SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError),
			"OffsetRequest": sarama.NewMockOffsetResponse(t).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
			"FetchRequest": sarama.NewMockFetchResponse(t, 1).
				SetMessage(mockChannel.topic(), mockChannel.partition(), newestOffset, message),
		})

		chain.Start()

		assert.Equal(t, false, chain.startCompleted, "Expected chain.startCompleted flag to be set to false")
		assert.Equal(t, true, chain.halted, "Expected chain.halted flag to be set to set to true")
		_, ok := <-chain.exitChan
		assert.Equal(t, false, ok, "Expected chain.exitChan to be closed")
	})

	t.Run("Enqueue", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)
		chain.Start()

		assert.Equal(t, true, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return true")

		chain.Halt()
	})

	t.Run("EnqueueIfHalted", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)
		chain.Start()

		chain.Halt()
		time.Sleep(50 * time.Millisecond) // Let closeLoop() do its thing -- TODO Hacky, revise approach
		assert.Equal(t, true, chain.halted, "Expected chain.halted flag to be set true")

		assert.Equal(t, false, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return false")
	})

	t.Run("EnqueueError", func(t *testing.T) {
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)
		chain.Start()

		mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockMetadataResponse(t).
				SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
				SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotLeaderForPartition),
			"OffsetRequest": sarama.NewMockOffsetResponse(t).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
				SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
			"FetchRequest": sarama.NewMockFetchResponse(t, 1).
				SetMessage(mockChannel.topic(), mockChannel.partition(), newestOffset, message),
		})

		assert.Equal(t, false, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return false")
	})
}

func TestCloseLoop(t *testing.T) {
	startFrom := int64(3)
	oldestOffset := int64(0)
	newestOffset := int64(5)

	mockChannel := newChannel("channelFoo", defaultPartition)
	message := sarama.StringEncoder("messageFoo")

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()
	mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
			SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
		"FetchRequest": sarama.NewMockFetchResponse(t, 1).
			SetMessage(mockChannel.topic(), mockChannel.partition(), startFrom, message),
	})

	t.Run("Proper", func(t *testing.T) {
		producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
		assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

		parentConsumer, channelConsumer, err := setupConsumerForChannel([]string{mockBroker.Addr()}, mockBrokerConfig, mockChannel, startFrom)
		assert.NoError(t, err, "Expected no error when setting up the consumer")

		haltedFlag := false

		errs := closeLoop(mockChannel.topic(), producer, parentConsumer, channelConsumer, &haltedFlag)

		assert.Len(t, errs, 0, "Expected zero errors")
		assert.Equal(t, true, haltedFlag, "Expected halted flag to be set to true")

		assert.Panics(t, func() {
			channelConsumer.Close()
		})

		assert.NotPanics(t, func() {
			parentConsumer.Close()
		})

		// TODO For some reason this panic cannot be captured by the `assert`
		// test framework. Not a dealbreaker but need to investigate further.
		/* assert.Panics(t, func() {
			producer.Close()
		}) */
	})

	t.Run("ChannelConsumerError", func(t *testing.T) {
		producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
		assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

		// Unlike all other tests in this file, forcing an error on the
		// channelConsumer.Close() call is more easily achieved using the mock
		// Consumer. Thus we bypass the call to `newConsumer` and do
		// type-casting.
		mockParentConsumer := mocks.NewConsumer(t, nil)
		mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), startFrom).YieldError(sarama.ErrOutOfBrokers)
		mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), startFrom)
		assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

		haltedFlag := false

		errs := closeLoop(mockChannel.topic(), producer, mockParentConsumer, mockChannelConsumer, &haltedFlag)

		assert.Len(t, errs, 1, "Expected 1 error returned")
		assert.Equal(t, true, haltedFlag, "Expected halted flag to be set to true")

		assert.NotPanics(t, func() {
			mockChannelConsumer.Close()
		})

		assert.NotPanics(t, func() {
			mockParentConsumer.Close()
		})
	})
}

func TestGetLastCutBlockNumber(t *testing.T) {
	testCases := []struct {
		name     string
		input    uint64
		expected uint64
	}{
		{"Proper", uint64(2), uint64(1)},
		{"Zero", uint64(1), uint64(0)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getLastCutBlockNumber(tc.input))
		})
	}
}

func TestGetLastOffsetPersisted(t *testing.T) {
	mockChannel := newChannel("channelFoo", defaultPartition)
	mockMetadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: int64(5)})}

	testCases := []struct {
		name     string
		md       []byte
		expected int64
		panics   bool
	}{
		{"Proper", mockMetadata.Value, int64(5), false},
		{"Empty", nil, sarama.OffsetOldest - 1, false},
		{"Panics", tamperBytes(mockMetadata.Value), sarama.OffsetOldest - 1, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.panics {
				assert.Equal(t, tc.expected, getLastOffsetPersisted(tc.md, mockChannel.String()))
			} else {
				assert.Panics(t, func() {
					getLastOffsetPersisted(tc.md, mockChannel.String())
				}, "Expected getLastOffsetPersisted call to panic")
			}
		})
	}
}

func TestListenForErrors(t *testing.T) {
	mockChannel := newChannel("mockChannelFoo", defaultPartition)
	errChan := make(chan *sarama.ConsumerError, 1)

	exitChan1 := make(chan struct{})
	close(exitChan1)
	assert.Nil(t, listenForErrors(errChan, exitChan1), "Expected listenForErrors call to return nil")

	exitChan2 := make(chan struct{})
	errChan <- &sarama.ConsumerError{
		Topic:     mockChannel.topic(),
		Partition: mockChannel.partition(),
		Err:       fmt.Errorf("foo"),
	}
	assert.NotNil(t, listenForErrors(errChan, exitChan2), "Expected listenForErrors call to return an error")
}

func TestProcessLoopConnect(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(2)
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.ChannelBufferSize = 0

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{}

	var counts []uint64
	done := make(chan struct{})
	go func() {
		counts, err = processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
		done <- struct{}{}
	}()

	// This is the wrappedMessage that the for loop will process
	mpc.YieldMessage(newMockConsumerMessage(newConnectMessage()))

	logger.Debug("Closing exitChan to exit the infinite for loop")
	close(exitChan) // Identical to chain.Halt()
	logger.Debug("exitChan closed")
	<-done

	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessConnectPass], "Expected 1 CONNECT message processed")
}

func TestProcessLoopRegularError(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.ChannelBufferSize = 0

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	var counts []uint64
	done := make(chan struct{})
	go func() {
		counts, err = processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
		done <- struct{}{}
	}()

	// This is the wrappedMessage that the for loop will process
	mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(tamperBytes(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))))

	logger.Debug("Closing exitChan to exit the infinite for loop")
	close(exitChan) // Identical to chain.Halt()
	logger.Debug("exitChan closed")
	<-done

	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexPprocessRegularError], "Expected 1 damaged REGULAR message processed")
}

func TestProcessLoopRegularQueueEnvelope(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1s")
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			BatchTimeoutVal: batchTimeout,
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	go func() { // Note: Unlike the CONNECT test case, the following does NOT introduce a race condition, so we're good
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		logger.Debug("Closing exitChan to exit the infinite for loop") // We are guaranteed to hit the exitChan branch after hitting the REGULAR branch at least once
		close(exitChan)                                                // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
}

func TestProcessLoopRegularCutBlock(t *testing.T) {
	batchTimeout, _ := time.ParseDuration("1s")
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber + 1
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			BatchTimeoutVal: batchTimeout,
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	mockSupport.BlockCutterVal.CutNext = true

	go func() { // Note: Unlike the CONNECT test case, the following does NOT introduce a race condition, so we're good
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		<-mockSupport.Batches                                          // Let the `mockConsenterSupport.WriteBlock` proceed
		logger.Debug("Closing exitChan to exit the infinite for loop") // We are guaranteed to hit the exitChan branch after hitting the REGULAR branch at least once
		close(exitChan)                                                // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by one")
}

func TestProcessLoopRegularCutTwoBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	batchTimeout, _ := time.ParseDuration("100s") // Something big
	newestOffset := int64(0)
	lastCutBlockNumber := uint64(0)
	lastCutBlockNumberEnd := lastCutBlockNumber + 2
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			BatchTimeoutVal: batchTimeout,
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	var block1, block2 *cb.Block

	go func() { // Note: Unlike the CONNECT test case, the following does NOT introduce a race condition, so we're good
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))) // This is the wrappedMessage that the for loop will process)
		mockSupport.BlockCutterVal.Block <- struct{}{}
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		mockSupport.BlockCutterVal.IsolatedTx = true

		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return once again
		logger.Debugf("Mock blockcutter's Ordered call has returned for the second time")

		select {
		case <-mockSupport.Batches: // Let the `mockConsenterSupport.WriteBlock` proceed
			block1 = mockSupport.WriteBlockVal
		case <-time.After(hitBranch):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		select {
		case <-mockSupport.Batches:
			block2 = mockSupport.WriteBlockVal
		case <-time.After(hitBranch):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		logger.Debug("Closing exitChan to exit the infinite for loop") // We are guaranteed to hit the exitChan branch after hitting the REGULAR branch at least once
		close(exitChan)                                                // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 messages received and unmarshaled")
	assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 2 REGULAR messages processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by two")
	assert.Equal(t, newestOffset+1, extractEncodedOffset(block1.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", newestOffset+1)
	assert.Equal(t, newestOffset+2, extractEncodedOffset(block2.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", newestOffset+2)
}

func TestProcessLoopRegularAndSendTimeToCutRegular(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	batchTimeout, _ := time.ParseDuration("1ms")
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	successResponse := new(sarama.ProduceResponse)
	successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
	mockBroker.Returns(successResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			BatchTimeoutVal: batchTimeout,
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	go func() { // TODO Hacky, see comments below, revise approach
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return (in `processRegular`)
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		time.Sleep(hitBranch) // This introduces a race: we're basically sleeping so as to let select hit the TIMER branch first before the EXITCHAN one
		logger.Debug("Closing exitChan to exit the infinite for loop")
		close(exitChan) // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
	assert.Equal(t, uint64(1), counts[indexSendTimeToCutPass], "Expected 1 TIMER event processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
}

func TestProcessLoopRegularAndSendTimeToCutError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	batchTimeout, _ := time.ParseDuration("1ms")
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.Net.ReadTimeout = 5 * time.Millisecond
	mockBrokerConfigCopy.Consumer.Retry.Backoff = 5 * time.Millisecond
	mockBrokerConfigCopy.Metadata.Retry.Max = 1

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, &mockBrokerConfigCopy)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	failureResponse := new(sarama.ProduceResponse)
	failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
	mockBroker.Returns(failureResponse)

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			BatchTimeoutVal: batchTimeout,
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	go func() { // TODO Hacky, see comments below, revise approach
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return (in `processRegular`)
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		time.Sleep(hitBranch) // This introduces a race: we're basically sleeping so as to let select hit the TIMER branch first before the EXITCHAN one
		logger.Debug("Closing exitChan to exit the infinite for loop")
		close(exitChan) // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
	assert.Equal(t, uint64(1), counts[indexSendTimeToCutError], "Expected 1 faulty TIMER event processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
}

func TestProcessLoopTimeToCutFromReceivedMessageRegular(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber + 1
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 1))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	// We need the mock blockcutter to deliver a non-empty batch
	go func() {
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call below return
	}()
	mockSupport.BlockCutterVal.Ordered(newMockEnvelope("fooMessage"))

	go func() { // Note: Unlike the CONNECT test case, the following does NOT introduce a race condition, so we're good
		<-mockSupport.Batches                                          // Let the `mockConsenterSupport.WriteBlock` proceed
		logger.Debug("Closing exitChan to exit the infinite for loop") // We are guaranteed to hit the exitChan branch after hitting the REGULAR branch at least once
		close(exitChan)                                                // Identical to chain.Halt()
		logger.Debug("exitChan closed")
	}()

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessTimeToCutPass], "Expected 1 TIMETOCUT message processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by one")
}

func TestProcessLoopTimeToCutFromReceivedMessageZeroBatch(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 1))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.Error(t, err, "Expected the processMessagesToBlock call to return an error")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessTimeToCutError], "Expected 1 faulty TIMETOCUT message processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
}

func TestProcessLoopTimeToCutFromReceivedMessageLargerThanExpected(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockParentConsumer := mocks.NewConsumer(t, nil)
	mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset).
		YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 2))) // This is the wrappedMessage that the for loop will process
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	counts, err := processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
	assert.Error(t, err, "Expected the processMessagesToBlock call to return an error")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessTimeToCutError], "Expected 1 faulty TIMETOCUT message processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
}

func TestProcessLoopTimeToCutFromReceivedMessageStale(t *testing.T) {
	newestOffset := int64(5)
	lastCutBlockNumber := uint64(3)
	lastCutBlockNumberEnd := lastCutBlockNumber
	haltedFlag := false
	exitChan := make(chan struct{})

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.ChannelBufferSize = 0

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	mockSupport := &mockmultichain.ConsenterSupport{
		Batches:        make(chan []*cb.Envelope), // WriteBlock will post here
		BlockCutterVal: mockblockcutter.NewReceiver(),
		ChainIDVal:     mockChannel.topic(),
		HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}
	defer close(mockSupport.BlockCutterVal.Block)

	var counts []uint64
	done := make(chan struct{})
	go func() {
		counts, err = processMessagesToBlock(mockSupport, producer, mockParentConsumer, mockChannelConsumer, mockChannel, &lastCutBlockNumber, &haltedFlag, &exitChan)
		done <- struct{}{}
	}()

	// This is the wrappedMessage that the for loop will process
	mpc.YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber)))

	logger.Debug("Closing exitChan to exit the infinite for loop")
	close(exitChan) // Identical to chain.Halt()
	logger.Debug("exitChan closed")
	<-done

	assert.NoError(t, err, "Expected the processMessagesToBlock call to return without errors")
	assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
	assert.Equal(t, uint64(1), counts[indexProcessTimeToCutPass], "Expected 1 TIMETOCUT message processed")
	assert.Equal(t, lastCutBlockNumberEnd, lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
}

func TestSendConnectMessage(t *testing.T) {
	mockChannel := newChannel("mockChannelFoo", defaultPartition)
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.Net.ReadTimeout = 5 * time.Millisecond
	mockBrokerConfigCopy.Consumer.Retry.Backoff = 5 * time.Millisecond
	mockBrokerConfigCopy.Metadata.Retry.Max = 1

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, &mockBrokerConfigCopy)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	t.Run("Proper", func(t *testing.T) {
		successResponse := new(sarama.ProduceResponse)
		successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
		mockBroker.Returns(successResponse)

		assert.NoError(t, sendConnectMessage(producer, mockChannel), "Expected the sendConnectMessage call to return without errors")
	})

	t.Run("WithError", func(t *testing.T) {
		failureResponse := new(sarama.ProduceResponse)
		failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
		mockBroker.Returns(failureResponse)
		err := sendConnectMessage(producer, mockChannel)
		assert.Error(t, err, "Expected the sendConnectMessage call to return an error")
	})
}

func TestSendTimeToCut(t *testing.T) {
	mockChannel := newChannel("mockChannelFoo", defaultPartition)
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.Net.ReadTimeout = 5 * time.Millisecond
	mockBrokerConfigCopy.Consumer.Retry.Backoff = 5 * time.Millisecond
	mockBrokerConfigCopy.Metadata.Retry.Max = 1

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, &mockBrokerConfigCopy)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")

	timeToCutBlockNumber := uint64(3)
	var timer <-chan time.Time

	t.Run("Proper", func(t *testing.T) {
		successResponse := new(sarama.ProduceResponse)
		successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
		mockBroker.Returns(successResponse)

		timer = time.After(1 * time.Hour) // Just a very long amount of time

		assert.NoError(t, sendTimeToCut(producer, mockChannel, timeToCutBlockNumber, &timer), "Expected the sendTimeToCut call to return without errors")
		assert.Nil(t, timer, "Expected the sendTimeToCut call to nil the timer")
	})

	t.Run("WithError", func(t *testing.T) {
		failureResponse := new(sarama.ProduceResponse)
		failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
		mockBroker.Returns(failureResponse)

		timer = time.After(1 * time.Hour) // Just a very long amount of time

		assert.Error(t, sendTimeToCut(producer, mockChannel, timeToCutBlockNumber, &timer), "Expected the sendTimeToCut call to return an error")
		assert.Nil(t, timer, "Expected the sendTimeToCut call to nil the timer")
	})
}

func TestSetupConsumerForChannel(t *testing.T) {
	startFrom := int64(3)
	oldestOffset := int64(0)
	newestOffset := int64(5)

	mockChannel := newChannel("channelFoo", defaultPartition)
	message := sarama.StringEncoder("messageFoo")

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()
	mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
			SetLeader(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetOldest, oldestOffset).
			SetOffset(mockChannel.topic(), mockChannel.partition(), sarama.OffsetNewest, newestOffset),
		"FetchRequest": sarama.NewMockFetchResponse(t, 1).
			SetMessage(mockChannel.topic(), mockChannel.partition(), startFrom, message),
	})

	t.Run("Proper", func(t *testing.T) {
		parentConsumer, channelConsumer, err := setupConsumerForChannel([]string{mockBroker.Addr()}, mockBrokerConfig, mockChannel, startFrom)
		assert.NoError(t, err, "Expected the setupConsumerForChannel call to return without errors")
		assert.NoError(t, channelConsumer.Close(), "Expected to close the channelConsumer without errors")
		assert.NoError(t, parentConsumer.Close(), "Expected to close the parentConsumer without errors")
	})

	t.Run("WithParentConsumerError", func(t *testing.T) {
		// Provide an empty brokers list
		parentConsumer, channelConsumer, err := setupConsumerForChannel([]string{}, mockBrokerConfig, mockChannel, startFrom)
		defer func() {
			if err == nil {
				channelConsumer.Close()
				parentConsumer.Close()
			}
		}()
		assert.Error(t, err, "Expected the setupConsumerForChannel call to return an error")
	})

	t.Run("WithChannelConsumerError", func(t *testing.T) {
		// Provide an out-of-range offset
		parentConsumer, channelConsumer, err := setupConsumerForChannel([]string{mockBroker.Addr()}, mockBrokerConfig, mockChannel, newestOffset+1)
		defer func() {
			if err == nil {
				channelConsumer.Close()
				parentConsumer.Close()
			}
		}()
		assert.Error(t, err, "Expected the setupConsumerForChannel call to return an error")
	})
}

func TestSetupProducerForChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	mockChannel := newChannel("channelFoo", defaultPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer mockBroker.Close()

	t.Run("Proper", func(t *testing.T) {
		metadataResponse := new(sarama.MetadataResponse)
		metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
		metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
		mockBroker.Returns(metadataResponse)

		producer, err := setupProducerForChannel([]string{mockBroker.Addr()}, mockBrokerConfig, mockChannel, mockConsenter.retryOptions())
		assert.NoError(t, err, "Expected the setupProducerForChannel call to return without errors")
		assert.NoError(t, producer.Close(), "Expected to close the producer without errors")
	})

	t.Run("WithError", func(t *testing.T) {
		_, err := setupProducerForChannel([]string{}, mockBrokerConfig, mockChannel, mockConsenter.retryOptions())
		assert.Error(t, err, "Expected the setupProducerForChannel call to return an error")
	})
}
