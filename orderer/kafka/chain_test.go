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

var (
	extraShortTimeout = 1 * time.Millisecond
	shortTimeout      = 1 * time.Second
	longTimeout       = 1 * time.Hour

	hitBranch = 50 * time.Millisecond
)

func TestChain(t *testing.T) {

	oldestOffset := int64(0)
	newestOffset := int64(5)

	message := sarama.StringEncoder("messageFoo")

	newMocks := func(t *testing.T) (mockChannel channel, mockBroker *sarama.MockBroker, mockSupport *mockmultichain.ConsenterSupport) {
		mockChannel = newChannel(channelNameForTest(t), defaultPartition)
		mockBroker = sarama.NewMockBroker(t, 0)
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
		mockSupport = &mockmultichain.ConsenterSupport{
			ChainIDVal:      mockChannel.topic(),
			HeightVal:       uint64(3),
			SharedConfigVal: &mockconfig.Orderer{KafkaBrokersVal: []string{mockBroker.Addr()}},
		}
		return
	}

	t.Run("New", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, err := newChain(mockConsenter, mockSupport, newestOffset-1)

		assert.NoError(t, err, "Expected newChain to return without errors")
		select {
		case <-chain.errorChan:
			logger.Debug("errorChan is closed as it should be")
		default:
			t.Fatal("errorChan should have been closed")
		}

		select {
		case <-chain.haltChan:
			t.Fatal("haltChan should have been open")
		default:
			logger.Debug("haltChan is open as it should be")
		}

		select {
		case <-chain.startChan:
			t.Fatal("startChan should have been open")
		default:
			logger.Debug("startChan is open as it should be")
		}
	})

	t.Run("Start", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		// Set to -1 because we haven't sent the CONNECT message yet
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		chain.Start()
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}

		// Trigger the haltChan clause in the processMessagesToBlocks goroutine
		close(chain.haltChan)
	})

	t.Run("Halt", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		chain.Start()
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}

		// Wait till the start phase has completed, then:
		chain.Halt()

		select {
		case <-chain.haltChan:
			logger.Debug("haltChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("haltChan should have been closed")
		}

		select {
		case <-chain.errorChan:
			logger.Debug("errorChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("errorChan should have been closed")
		}
	})

	t.Run("DoubleHalt", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		chain.Start()
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}

		chain.Halt()

		assert.NotPanics(t, func() { chain.Halt() }, "Calling Halt() more than once shouldn't panic")
	})

	t.Run("StartWithProducerForChannelError", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		// Point to an empty brokers list
		mockSupportCopy := *mockSupport
		mockSupportCopy.SharedConfigVal = &mockconfig.Orderer{KafkaBrokersVal: []string{}}

		chain, _ := newChain(mockConsenter, &mockSupportCopy, newestOffset-1)

		// The production path will actually call chain.Start(). This is
		// functionally equivalent and allows us to run assertions on it.
		assert.Panics(t, func() { startThread(chain) }, "Expected the Start() call to panic")
	})

	t.Run("StartWithConnectMessageError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		// Have the broker return an ErrNotLeaderForPartition error
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

		assert.Panics(t, func() { startThread(chain) }, "Expected the Start() call to panic")
	})

	t.Run("EnqueueIfNotStarted", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		// As in StartWithConnectMessageError, have the broker return an
		// ErrNotLeaderForPartition error, i.e. cause an error in the
		// 'post connect message' step.
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

		assert.False(t, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return false")
	})

	t.Run("StartWithConsumerForChannelError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max

		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()

		// Provide an out-of-range offset
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset)

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

		assert.Panics(t, func() { startThread(chain) }, "Expected the Start() call to panic")
	})

	t.Run("EnqueueProper", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

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
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}

		// Enqueue should have access to the post path, and its ProduceRequest
		// should go by without error
		assert.True(t, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return true")

		chain.Halt()
	})

	t.Run("EnqueueIfHalted", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

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
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}
		chain.Halt()

		// haltChan should close access to the post path
		assert.False(t, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return false")
	})

	t.Run("EnqueueError", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1)

		// Use the "good" handler map that allows the Stage to complete without
		// issues
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
		select {
		case <-chain.startChan:
			logger.Debug("startChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("startChan should have been closed by now")
		}

		// Now make it so that the next ProduceRequest is met with an error
		mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotLeaderForPartition),
		})

		assert.False(t, chain.Enqueue(newMockEnvelope("fooMessage")), "Expected Enqueue call to return false")
	})
}

func TestSetupProducerForChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	mockBroker := sarama.NewMockBroker(t, 0)
	defer mockBroker.Close()

	mockChannel := newChannel(channelNameForTest(t), defaultPartition)

	haltChan := make(chan struct{})

	t.Run("Proper", func(t *testing.T) {
		metadataResponse := new(sarama.MetadataResponse)
		metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
		metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
		mockBroker.Returns(metadataResponse)

		producer, err := setupProducerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		assert.NoError(t, err, "Expected the setupProducerForChannel call to return without errors")
		assert.NoError(t, producer.Close(), "Expected to close the producer without errors")
	})

	t.Run("WithError", func(t *testing.T) {
		_, err := setupProducerForChannel(mockConsenter.retryOptions(), haltChan, []string{}, mockBrokerConfig, mockChannel)
		assert.Error(t, err, "Expected the setupProducerForChannel call to return an error")
	})
}

func TestSetupConsumerForChannel(t *testing.T) {
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockChannel := newChannel(channelNameForTest(t), defaultPartition)

	oldestOffset := int64(0)
	newestOffset := int64(5)

	startFrom := int64(3)
	message := sarama.StringEncoder("messageFoo")

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

	haltChan := make(chan struct{})

	t.Run("ProperParent", func(t *testing.T) {
		parentConsumer, err := setupParentConsumerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		assert.NoError(t, err, "Expected the setupParentConsumerForChannel call to return without errors")
		assert.NoError(t, parentConsumer.Close(), "Expected to close the parentConsumer without errors")
	})

	t.Run("ProperChannel", func(t *testing.T) {
		parentConsumer, _ := setupParentConsumerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		defer func() { parentConsumer.Close() }()
		channelConsumer, err := setupChannelConsumerForChannel(mockConsenter.retryOptions(), haltChan, parentConsumer, mockChannel, newestOffset)
		assert.NoError(t, err, "Expected the setupChannelConsumerForChannel call to return without errors")
		assert.NoError(t, channelConsumer.Close(), "Expected to close the channelConsumer without errors")
	})

	t.Run("WithParentConsumerError", func(t *testing.T) {
		// Provide an empty brokers list
		_, err := setupParentConsumerForChannel(mockConsenter.retryOptions(), haltChan, []string{}, mockBrokerConfig, mockChannel)
		assert.Error(t, err, "Expected the setupParentConsumerForChannel call to return an error")
	})

	t.Run("WithChannelConsumerError", func(t *testing.T) {
		// Provide an out-of-range offset
		parentConsumer, _ := setupParentConsumerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		_, err := setupChannelConsumerForChannel(mockConsenter.retryOptions(), haltChan, parentConsumer, mockChannel, newestOffset+1)
		defer func() { parentConsumer.Close() }()
		assert.Error(t, err, "Expected the setupChannelConsumerForChannel call to return an error")
	})
}

func TestCloseKafkaObjects(t *testing.T) {
	mockChannel := newChannel(channelNameForTest(t), defaultPartition)

	mockSupport := &mockmultichain.ConsenterSupport{
		ChainIDVal: mockChannel.topic(),
	}

	oldestOffset := int64(0)
	newestOffset := int64(5)

	startFrom := int64(3)
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

	haltChan := make(chan struct{})

	t.Run("Proper", func(t *testing.T) {
		producer, _ := setupProducerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		parentConsumer, _ := setupParentConsumerForChannel(mockConsenter.retryOptions(), haltChan, []string{mockBroker.Addr()}, mockBrokerConfig, mockChannel)
		channelConsumer, _ := setupChannelConsumerForChannel(mockConsenter.retryOptions(), haltChan, parentConsumer, mockChannel, startFrom)

		// Set up a chain with just the minimum necessary fields instantiated so
		// as to test the function
		bareMinimumChain := &chainImpl{
			support:         mockSupport,
			producer:        producer,
			parentConsumer:  parentConsumer,
			channelConsumer: channelConsumer,
		}

		errs := bareMinimumChain.closeKafkaObjects()

		assert.Len(t, errs, 0, "Expected zero errors")

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
		producer, _ := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)

		// Unlike all other tests in this file, forcing an error on the
		// channelConsumer.Close() call is more easily achieved using the mock
		// Consumer. Thus we bypass the call to `setup*Consumer`.

		// Have the consumer receive an ErrOutOfBrokers error.
		mockParentConsumer := mocks.NewConsumer(t, nil)
		mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), startFrom).YieldError(sarama.ErrOutOfBrokers)
		mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), startFrom)
		assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

		bareMinimumChain := &chainImpl{
			support:         mockSupport,
			producer:        producer,
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,
		}

		errs := bareMinimumChain.closeKafkaObjects()

		assert.Len(t, errs, 1, "Expected 1 error returned")

		assert.NotPanics(t, func() {
			mockChannelConsumer.Close()
		})

		assert.NotPanics(t, func() {
			mockParentConsumer.Close()
		})
	})
}

// Test helper functions here.

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
	mockChannel := newChannel(channelNameForTest(t), defaultPartition)
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

func TestSendConnectMessage(t *testing.T) {
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, _ := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	defer func() { producer.Close() }()

	haltChan := make(chan struct{})

	t.Run("Proper", func(t *testing.T) {
		successResponse := new(sarama.ProduceResponse)
		successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
		mockBroker.Returns(successResponse)

		assert.NoError(t, sendConnectMessage(mockConsenter.retryOptions(), haltChan, producer, mockChannel), "Expected the sendConnectMessage call to return without errors")
	})

	t.Run("WithError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max

		// Have the broker return an ErrNotEnoughReplicas error
		failureResponse := new(sarama.ProduceResponse)
		failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
		mockBroker.Returns(failureResponse)

		assert.Error(t, sendConnectMessage(mockConsenter.retryOptions(), haltChan, producer, mockChannel), "Expected the sendConnectMessage call to return an error")
	})
}

func TestSendTimeToCut(t *testing.T) {
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	assert.NoError(t, err, "Expected no error when setting up the sarama SyncProducer")
	defer func() { producer.Close() }()

	timeToCutBlockNumber := uint64(3)
	var timer <-chan time.Time

	t.Run("Proper", func(t *testing.T) {
		successResponse := new(sarama.ProduceResponse)
		successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
		mockBroker.Returns(successResponse)

		timer = time.After(longTimeout)

		assert.NoError(t, sendTimeToCut(producer, mockChannel, timeToCutBlockNumber, &timer), "Expected the sendTimeToCut call to return without errors")
		assert.Nil(t, timer, "Expected the sendTimeToCut call to nil the timer")
	})

	t.Run("WithError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max
		failureResponse := new(sarama.ProduceResponse)
		failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
		mockBroker.Returns(failureResponse)

		timer = time.After(longTimeout)

		assert.Error(t, sendTimeToCut(producer, mockChannel, timeToCutBlockNumber, &timer), "Expected the sendTimeToCut call to return an error")
		assert.Nil(t, timer, "Expected the sendTimeToCut call to nil the timer")
	})
}

func TestProcessMessagesToBlocks(t *testing.T) {
	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockChannel := newChannel("mockChannelFoo", defaultPartition)

	metadataResponse := new(sarama.MetadataResponse)
	metadataResponse.AddBroker(mockBroker.Addr(), mockBroker.BrokerID())
	metadataResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), mockBroker.BrokerID(), nil, nil, sarama.ErrNoError)
	mockBroker.Returns(metadataResponse)

	producer, _ := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)

	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.ChannelBufferSize = 0

	newestOffset := int64(0)

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), newestOffset)
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	t.Run("ReceiveConnect", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		mockSupport := &mockmultichain.ConsenterSupport{
			ChainIDVal: mockChannel.topic(),
		}

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel: mockChannel,
			support: mockSupport,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newConnectMessage()))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessConnectPass], "Expected 1 CONNECT message processed")
	})

	t.Run("ReceiveRegularWithError", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		mockSupport := &mockmultichain.ConsenterSupport{
			ChainIDVal: mockChannel.topic(),
		}

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel: mockChannel,
			support: mockSupport,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(tamperBytes(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 damaged REGULAR message processed")
	})

	t.Run("ReceiveRegularAndQueue", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: longTimeout,
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))

		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		// We are guaranteed to hit the haltChan branch after hitting the REGULAR branch at least once
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
	})

	t.Run("ReceiveRegularAndCutBlock", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: longTimeout,
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		mockSupport.BlockCutterVal.CutNext = true

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))

		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")
		<-mockSupport.Blocks // Let the `mockConsenterSupport.WriteBlock` proceed

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		assert.Equal(t, lastCutBlockNumber+1, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by one")
	})

	t.Run("ReceiveTwoRegularAndCutTwoBlocks", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping test in short mode")
		}

		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: longTimeout,
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		var block1, block2 *cb.Block

		// This is the first wrappedMessage that the for-loop will process
		block1Offset := mpc.HighWaterMarkOffset()
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")

		mockSupport.BlockCutterVal.IsolatedTx = true

		// This is the first wrappedMessage that the for-loop will process
		block2Offset := mpc.HighWaterMarkOffset()
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{}
		logger.Debugf("Mock blockcutter's Ordered call has returned for the second time")

		select {
		case block1 = <-mockSupport.Blocks: // Let the `mockConsenterSupport.WriteBlock` proceed
		case <-time.After(shortTimeout):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		select {
		case block2 = <-mockSupport.Blocks:
		case <-time.After(shortTimeout):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 messages received and unmarshaled")
		assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 2 REGULAR messages processed")
		assert.Equal(t, lastCutBlockNumber+2, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by two")
		assert.Equal(t, block1Offset, extractEncodedOffset(block1.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", block1Offset)
		assert.Equal(t, block2Offset, extractEncodedOffset(block2.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", block2Offset)
	})

	t.Run("SecondTxOverflows", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping test in short mode")
		}

		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: longTimeout,
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		var block1, block2 *cb.Block

		block1LastOffset := mpc.HighWaterMarkOffset()
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return

		// Set CutAncestors to true so that second message overflows receiver batch
		mockSupport.BlockCutterVal.CutAncestors = true
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{}

		select {
		case block1 = <-mockSupport.Blocks: // Let the `mockConsenterSupport.WriteBlock` proceed
		case <-time.After(shortTimeout):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		// Set CutNext to true to flush all pending messages
		mockSupport.BlockCutterVal.CutAncestors = false
		mockSupport.BlockCutterVal.CutNext = true
		block2LastOffset := mpc.HighWaterMarkOffset()
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))
		mockSupport.BlockCutterVal.Block <- struct{}{}

		select {
		case block2 = <-mockSupport.Blocks:
		case <-time.After(shortTimeout):
			logger.Fatalf("Did not receive a block from the blockcutter as expected")
		}

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(3), counts[indexRecvPass], "Expected 2 messages received and unmarshaled")
		assert.Equal(t, uint64(3), counts[indexProcessRegularPass], "Expected 2 REGULAR messages processed")
		assert.Equal(t, lastCutBlockNumber+2, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by two")
		assert.Equal(t, block1LastOffset, extractEncodedOffset(block1.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", block1LastOffset)
		assert.Equal(t, block2LastOffset, extractEncodedOffset(block2.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in second block to be %d", block2LastOffset)
	})

	t.Run("ReceiveRegularAndSendTimeToCut", func(t *testing.T) {
		t.Skip("Skipping test as it introduces a race condition")

		// NB We haven't set a handlermap for the mock broker so we need to set
		// the ProduceResponse
		successResponse := new(sarama.ProduceResponse)
		successResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNoError)
		mockBroker.Returns(successResponse)

		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: extraShortTimeout, // ATTN
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			producer:        producer,
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))

		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")

		// Sleep so that the timer branch is activated before the exitChan one.
		// TODO This is a race condition, will fix in follow-up changeset
		time.Sleep(hitBranch)

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		assert.Equal(t, uint64(1), counts[indexSendTimeToCutPass], "Expected 1 TIMER event processed")
		assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
	})

	t.Run("ReceiveRegularAndSendTimeToCutError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max

		t.Skip("Skipping test as it introduces a race condition")

		// Exact same test as ReceiveRegularAndSendTimeToCut.
		// Only difference is that the producer's attempt to send a TTC will
		// fail with an ErrNotEnoughReplicas error.
		failureResponse := new(sarama.ProduceResponse)
		failureResponse.AddTopicPartition(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotEnoughReplicas)
		mockBroker.Returns(failureResponse)

		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
			SharedConfigVal: &mockconfig.Orderer{
				BatchTimeoutVal: extraShortTimeout, // ATTN
			},
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			producer:        producer,
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))

		mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
		logger.Debugf("Mock blockcutter's Ordered call has returned")

		// Sleep so that the timer branch is activated before the exitChan one.
		// TODO This is a race condition, will fix in follow-up changeset
		time.Sleep(hitBranch)

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		assert.Equal(t, uint64(1), counts[indexSendTimeToCutError], "Expected 1 faulty TIMER event processed")
		assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
	})

	t.Run("ReceiveTimeToCutProper", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		// We need the mock blockcutter to deliver a non-empty batch
		go func() {
			mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call below return
			logger.Debugf("Mock blockcutter's Ordered call has returned")
		}()
		// We are "planting" a message directly to the mock blockcutter
		mockSupport.BlockCutterVal.Ordered(newMockEnvelope("fooMessage"))

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 1)))

		<-mockSupport.Blocks // Let the `mockConsenterSupport.WriteBlock` proceed

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessTimeToCutPass], "Expected 1 TIMETOCUT message processed")
		assert.Equal(t, lastCutBlockNumber+1, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be bumped up by one")
	})

	t.Run("ReceiveTimeToCutZeroBatch", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 1)))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.Error(t, err, "Expected the processMessagesToBlocks call to return an error")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessTimeToCutError], "Expected 1 faulty TIMETOCUT message processed")
		assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
	})

	t.Run("ReceiveTimeToCutLargerThanExpected", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber + 2)))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.Error(t, err, "Expected the processMessagesToBlocks call to return an error")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessTimeToCutError], "Expected 1 faulty TIMETOCUT message processed")
		assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
	})

	t.Run("ReceiveTimeToCutStale", func(t *testing.T) {
		errorChan := make(chan struct{})
		close(errorChan)
		haltChan := make(chan struct{})

		lastCutBlockNumber := uint64(3)

		mockSupport := &mockmultichain.ConsenterSupport{
			Blocks:         make(chan *cb.Block), // WriteBlock will post here
			BlockCutterVal: mockblockcutter.NewReceiver(),
			ChainIDVal:     mockChannel.topic(),
			HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
		}
		defer close(mockSupport.BlockCutterVal.Block)

		bareMinimumChain := &chainImpl{
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel:            mockChannel,
			support:            mockSupport,
			lastCutBlockNumber: lastCutBlockNumber,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is the wrappedMessage that the for-loop will process
		mpc.YieldMessage(newMockConsumerMessage(newTimeToCutMessage(lastCutBlockNumber)))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
		assert.Equal(t, uint64(1), counts[indexProcessTimeToCutPass], "Expected 1 TIMETOCUT message processed")
		assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to stay the same")
	})

	t.Run("ReceiveKafkaErrorAndCloseErrorChan", func(t *testing.T) {
		// If we set up the mock broker so that it returns a response, if the
		// test finishes before the sendConnectMessage goroutine has received
		// this response, we will get a failure ("not all expectations were
		// satisfied") from the mock broker. So we sabotage the producer.
		failedProducer, _ := sarama.NewSyncProducer([]string{}, mockBrokerConfig)

		// We need to have the sendConnectMessage goroutine die instantaneously,
		// otherwise we'll get a nil pointer dereference panic. We are
		// exploiting the admittedly hacky shortcut where a retriable process
		// returns immediately when given the nil time.Duration value for its
		// ticker.
		zeroRetryConsenter := &consenterImpl{}

		// Let's assume an open errorChan, i.e. a healthy link between the
		// consumer and the Kafka partition corresponding to the channel
		errorChan := make(chan struct{})

		haltChan := make(chan struct{})

		mockSupport := &mockmultichain.ConsenterSupport{
			ChainIDVal: mockChannel.topic(),
		}

		bareMinimumChain := &chainImpl{
			consenter:       zeroRetryConsenter, // For sendConnectMessage
			producer:        failedProducer,     // For sendConnectMessage
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel: mockChannel,
			support: mockSupport,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is what the for-loop will process
		mpc.YieldError(fmt.Errorf("fooError"))

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done

		assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
		assert.Equal(t, uint64(1), counts[indexRecvError], "Expected 1 Kafka error received")

		select {
		case <-bareMinimumChain.errorChan:
			logger.Debug("errorChan is closed as it should be")
		default:
			t.Fatal("errorChan should have been closed")
		}
	})

	t.Run("ReceiveKafkaErrorAndThenReceiveRegularMessage", func(t *testing.T) {
		t.Skip("Skipping test as it introduces a race condition")

		// If we set up the mock broker so that it returns a response, if the
		// test finishes before the sendConnectMessage goroutine has received
		// this response, we will get a failure ("not all expectations were
		// satisfied") from the mock broker. So we sabotage the producer.
		failedProducer, _ := sarama.NewSyncProducer([]string{}, mockBrokerConfig)

		// We need to have the sendConnectMessage goroutine die instantaneously,
		// otherwise we'll get a nil pointer dereference panic. We are
		// exploiting the admittedly hacky shortcut where a retriable process
		// returns immediately when given the nil time.Duration value for its
		// ticker.
		zeroRetryConsenter := &consenterImpl{}

		// If the errorChan is closed already, the kafkaErr branch shouldn't
		// touch it
		errorChan := make(chan struct{})
		close(errorChan)

		haltChan := make(chan struct{})

		mockSupport := &mockmultichain.ConsenterSupport{
			ChainIDVal: mockChannel.topic(),
		}

		bareMinimumChain := &chainImpl{
			consenter:       zeroRetryConsenter, // For sendConnectMessage
			producer:        failedProducer,     // For sendConnectMessage
			parentConsumer:  mockParentConsumer,
			channelConsumer: mockChannelConsumer,

			channel: mockChannel,
			support: mockSupport,

			errorChan: errorChan,
			haltChan:  haltChan,
		}

		var counts []uint64
		done := make(chan struct{})

		go func() {
			counts, err = bareMinimumChain.processMessagesToBlocks()
			done <- struct{}{}
		}()

		// This is what the for-loop will process
		mpc.YieldError(fmt.Errorf("foo"))

		// We tested this in ReceiveKafkaErrorAndCloseErrorChan, so this check
		// is redundant in that regard. We use it however to ensure the
		// kafkaErrBranch has been activated before proceeding with pushing the
		// regular message.
		select {
		case <-bareMinimumChain.errorChan:
			logger.Debug("errorChan is closed as it should be")
		case <-time.After(shortTimeout):
			t.Fatal("errorChan should have been closed by now")
		}

		// This is the wrappedMessage that the for-loop will process. We use
		// a broken regular message here on purpose since this is the shortest
		// path and it allows us to test what we want.
		mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(tamperBytes(utils.MarshalOrPanic(newMockEnvelope("fooMessage"))))))

		// Sleep so that the Messages/errorChan branch is activated.
		// TODO Hacky approach, will need to revise eventually
		time.Sleep(hitBranch)

		// Check that the errorChan was recreated
		select {
		case <-bareMinimumChain.errorChan:
			t.Fatal("errorChan should have been open")
		default:
			logger.Debug("errorChan is open as it should be")
		}

		logger.Debug("Closing haltChan to exit the infinite for-loop")
		close(haltChan) // Identical to chain.Halt()
		logger.Debug("haltChan closed")
		<-done
	})
}
