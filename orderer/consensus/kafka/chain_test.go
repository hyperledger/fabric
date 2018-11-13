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
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	mockmultichannel "github.com/hyperledger/fabric/orderer/mocks/common/multichannel"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	lastOriginalOffsetProcessed := int64(0)
	lastResubmittedConfigOffset := int64(0)

	message := sarama.StringEncoder("messageFoo")

	newMocks := func(t *testing.T) (mockChannel channel, mockBroker *sarama.MockBroker, mockSupport *mockmultichannel.ConsenterSupport) {
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
		mockSupport = &mockmultichannel.ConsenterSupport{
			ChainIDVal:      mockChannel.topic(),
			HeightVal:       uint64(3),
			SharedConfigVal: &mockconfig.Orderer{KafkaBrokersVal: []string{mockBroker.Addr()}},
		}
		return
	}

	t.Run("New", func(t *testing.T) {
		_, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, err := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

		chain, _ := newChain(mockConsenter, &mockSupportCopy, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

	t.Run("enqueueIfNotStarted", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

		// We don't need to create a legit envelope here as it's not inspected during this test
		assert.False(t, chain.enqueue(newRegularMessage([]byte("fooMessage"))), "Expected enqueue call to return false")
	})

	t.Run("StartWithConsumerForChannelError", func(t *testing.T) {
		// Note that this test is affected by the following parameters:
		// - Net.ReadTimeout
		// - Consumer.Retry.Backoff
		// - Metadata.Retry.Max

		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()

		// Provide an out-of-range offset
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

	t.Run("enqueueProper", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

		// enqueue should have access to the post path, and its ProduceRequest should go by without error.
		// We don't need to create a legit envelope here as it's not inspected during this test
		assert.True(t, chain.enqueue(newRegularMessage([]byte("fooMessage"))), "Expected enqueue call to return true")

		chain.Halt()
	})

	t.Run("enqueueIfHalted", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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

		// haltChan should close access to the post path.
		// We don't need to create a legit envelope here as it's not inspected during this test
		assert.False(t, chain.enqueue(newRegularMessage([]byte("fooMessage"))), "Expected enqueue call to return false")
	})

	t.Run("enqueueError", func(t *testing.T) {
		mockChannel, mockBroker, mockSupport := newMocks(t)
		defer func() { mockBroker.Close() }()
		chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
		defer chain.Halt()

		// Now make it so that the next ProduceRequest is met with an error
		mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(mockChannel.topic(), mockChannel.partition(), sarama.ErrNotLeaderForPartition),
		})

		// We don't need to create a legit envelope here as it's not inspected during this test
		assert.False(t, chain.enqueue(newRegularMessage([]byte("fooMessage"))), "Expected enqueue call to return false")
	})

	t.Run("Order", func(t *testing.T) {
		t.Run("ErrorIfNotStarted", func(t *testing.T) {
			_, mockBroker, mockSupport := newMocks(t)
			defer func() { mockBroker.Close() }()
			chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

			// We don't need to create a legit envelope here as it's not inspected during this test
			assert.Error(t, chain.Order(&cb.Envelope{}, uint64(0)))
		})

		t.Run("Proper", func(t *testing.T) {
			mockChannel, mockBroker, mockSupport := newMocks(t)
			defer func() { mockBroker.Close() }()
			chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
			defer chain.Halt()

			select {
			case <-chain.startChan:
				logger.Debug("startChan is closed as it should be")
			case <-time.After(shortTimeout):
				t.Fatal("startChan should have been closed by now")
			}

			// We don't need to create a legit envelope here as it's not inspected during this test
			assert.NoError(t, chain.Order(&cb.Envelope{}, uint64(0)), "Expect Order successfully")
		})
	})

	t.Run("Configure", func(t *testing.T) {
		t.Run("ErrorIfNotStarted", func(t *testing.T) {
			_, mockBroker, mockSupport := newMocks(t)
			defer func() { mockBroker.Close() }()
			chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

			// We don't need to create a legit envelope here as it's not inspected during this test
			assert.Error(t, chain.Configure(&cb.Envelope{}, uint64(0)))
		})

		t.Run("Proper", func(t *testing.T) {
			mockChannel, mockBroker, mockSupport := newMocks(t)
			defer func() { mockBroker.Close() }()
			chain, _ := newChain(mockConsenter, mockSupport, newestOffset-1, lastOriginalOffsetProcessed, lastResubmittedConfigOffset)

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
			defer chain.Halt()

			select {
			case <-chain.startChan:
				logger.Debug("startChan is closed as it should be")
			case <-time.After(shortTimeout):
				t.Fatal("startChan should have been closed by now")
			}

			// We don't need to create a legit envelope here as it's not inspected during this test
			assert.NoError(t, chain.Configure(&cb.Envelope{}, uint64(0)), "Expect Configure successfully")
		})
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

	mockSupport := &mockmultichannel.ConsenterSupport{
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
			ConsenterSupport: mockSupport,
			producer:         producer,
			parentConsumer:   parentConsumer,
			channelConsumer:  channelConsumer,
		}

		errs := bareMinimumChain.closeKafkaObjects()

		assert.Len(t, errs, 0, "Expected zero errors")

		assert.NotPanics(t, func() {
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
			ConsenterSupport: mockSupport,
			producer:         producer,
			parentConsumer:   mockParentConsumer,
			channelConsumer:  mockChannelConsumer,
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
	mockMetadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{
		LastOffsetPersisted:         int64(5),
		LastOriginalOffsetProcessed: int64(3),
		LastResubmittedConfigOffset: int64(4),
	})}

	testCases := []struct {
		name                string
		md                  []byte
		expectedPersisted   int64
		expectedProcessed   int64
		expectedResubmitted int64
		panics              bool
	}{
		{"Proper", mockMetadata.Value, int64(5), int64(3), int64(4), false},
		{"Empty", nil, sarama.OffsetOldest - 1, int64(0), int64(0), false},
		{"Panics", tamperBytes(mockMetadata.Value), sarama.OffsetOldest - 1, int64(0), int64(0), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.panics {
				persisted, processed, resubmitted := getOffsets(tc.md, mockChannel.String())
				assert.Equal(t, tc.expectedPersisted, persisted)
				assert.Equal(t, tc.expectedProcessed, processed)
				assert.Equal(t, tc.expectedResubmitted, resubmitted)
			} else {
				assert.Panics(t, func() {
					getOffsets(tc.md, mockChannel.String())
				}, "Expected getOffsets call to panic")
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

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), int64(0))
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), int64(0))
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	t.Run("TimeToCut", func(t *testing.T) {
		t.Run("ReceiveTimeToCutProper", func(t *testing.T) {
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
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
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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

			mockSupport := &mockmultichannel.ConsenterSupport{
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
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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

			mockSupport := &mockmultichannel.ConsenterSupport{
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
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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

			mockSupport := &mockmultichannel.ConsenterSupport{
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
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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
	})

	t.Run("Connect", func(t *testing.T) {
		t.Run("ReceiveConnect", func(t *testing.T) {
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			mockSupport := &mockmultichannel.ConsenterSupport{
				ChainIDVal: mockChannel.topic(),
			}

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:          mockChannel,
				ConsenterSupport: mockSupport,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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
	})

	t.Run("Regular", func(t *testing.T) {
		t.Run("Error", func(t *testing.T) {
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			mockSupport := &mockmultichannel.ConsenterSupport{
				ChainIDVal: mockChannel.topic(),
			}

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:          mockChannel,
				ConsenterSupport: mockSupport,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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

		// This ensures regular kafka messages of type UNKNOWN are handled properly
		t.Run("Unknown", func(t *testing.T) {
			t.Run("Enqueue", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
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
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
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

			t.Run("CutBlock", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
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
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{})}

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

			// This test ensures the corner case in FAB-5709 is taken care of
			t.Run("SecondTxOverflows", func(t *testing.T) {
				if testing.Short() {
					t.Skip("Skipping test in short mode")
				}

				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
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
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
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

			t.Run("InvalidConfigEnv", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:              make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal:      mockblockcutter.NewReceiver(),
					ChainIDVal:          mockChannel.topic(),
					HeightVal:           lastCutBlockNumber, // Incremented during the WriteBlock call
					ClassifyMsgVal:      msgprocessor.ConfigMsg,
					ProcessConfigMsgErr: fmt.Errorf("Invalid config message"),
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
					},
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				// This is the config wrappedMessage that the for-loop will process.
				mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockConfigEnvelope()))))

				logger.Debug("Closing haltChan to exit the infinite for-loop")
				// We are guaranteed to hit the haltChan branch after hitting the REGULAR branch at least once
				close(haltChan) // Identical to chain.Halt()
				logger.Debug("haltChan closed")
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message error")
				assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber not to be incremented")
			})

			t.Run("InvalidOrdererTxEnv", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:              make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal:      mockblockcutter.NewReceiver(),
					ChainIDVal:          mockChannel.topic(),
					HeightVal:           lastCutBlockNumber, // Incremented during the WriteBlock call
					ClassifyMsgVal:      msgprocessor.ConfigMsg,
					ProcessConfigMsgErr: fmt.Errorf("Invalid config message"),
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
					},
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				// This is the config wrappedMessage that the for-loop will process.
				mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockOrdererTxEnvelope()))))

				logger.Debug("Closing haltChan to exit the infinite for-loop")
				// We are guaranteed to hit the haltChan branch after hitting the REGULAR branch at least once
				close(haltChan) // Identical to chain.Halt()
				logger.Debug("haltChan closed")
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message error")
				assert.Equal(t, lastCutBlockNumber, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber not to be incremented")
			})

			t.Run("InvalidNormalEnv", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
					},
					ProcessNormalMsgErr: fmt.Errorf("Invalid normal message"),
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				// This is the wrappedMessage that the for-loop will process
				mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")))))

				close(haltChan) // Identical to chain.Halt()
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message processed")
			})

			t.Run("CutConfigEnv", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
					},
					ClassifyMsgVal: msgprocessor.ConfigMsg,
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				configBlkOffset := mpc.HighWaterMarkOffset()
				mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockConfigEnvelope()))))

				var configBlk *cb.Block

				select {
				case configBlk = <-mockSupport.Blocks:
				case <-time.After(shortTimeout):
					logger.Fatalf("Did not receive a config block from the blockcutter as expected")
				}

				close(haltChan) // Identical to chain.Halt()
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
				assert.Equal(t, lastCutBlockNumber+1, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be incremented by 1")
				assert.Equal(t, configBlkOffset, extractEncodedOffset(configBlk.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in second block to be %d", configBlkOffset)
			})

			// We are not expecting this type of message from Kafka
			t.Run("ConfigUpdateEnv", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
					},
					ClassifyMsgVal: msgprocessor.ConfigUpdateMsg,
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				mpc.YieldMessage(newMockConsumerMessage(newRegularMessage(utils.MarshalOrPanic(newMockEnvelope("FooMessage")))))

				close(haltChan) // Identical to chain.Halt()
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message processed")
			})

			t.Run("SendTimeToCut", func(t *testing.T) {
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

				mockSupport := &mockmultichannel.ConsenterSupport{
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
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
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

			t.Run("SendTimeToCutError", func(t *testing.T) {
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

				mockSupport := &mockmultichannel.ConsenterSupport{
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
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
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
		})

		// This ensures regular kafka messages of type NORMAL are handled properly
		t.Run("Normal", func(t *testing.T) {
			lastOriginalOffsetProcessed := int64(3)

			t.Run("ReceiveTwoRegularAndCutTwoBlocks", func(t *testing.T) {
				if testing.Short() {
					t.Skip("Skipping test in short mode")
				}

				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
						CapabilitiesVal: &mockconfig.OrdererCapabilities{
							ResubmissionVal: false,
						},
					},
					SequenceVal: uint64(0),
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:                     mockChannel,
					ConsenterSupport:            mockSupport,
					lastCutBlockNumber:          lastCutBlockNumber,
					lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				var block1, block2 *cb.Block

				// This is the first wrappedMessage that the for-loop will process
				block1LastOffset := mpc.HighWaterMarkOffset()
				mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(0))))
				mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
				logger.Debugf("Mock blockcutter's Ordered call has returned")

				mockSupport.BlockCutterVal.IsolatedTx = true

				// This is the first wrappedMessage that the for-loop will process
				block2LastOffset := mpc.HighWaterMarkOffset()
				mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(0))))
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
				assert.Equal(t, block1LastOffset, extractEncodedOffset(block1.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", block1LastOffset)
				assert.Equal(t, block2LastOffset, extractEncodedOffset(block2.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in second block to be %d", block2LastOffset)
			})

			t.Run("ReceiveRegularAndQueue", func(t *testing.T) {
				if testing.Short() {
					t.Skip("Skipping test in short mode")
				}

				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
						CapabilitiesVal: &mockconfig.OrdererCapabilities{
							ResubmissionVal: false,
						},
					},
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:                     mockChannel,
					ConsenterSupport:            mockSupport,
					lastCutBlockNumber:          lastCutBlockNumber,
					lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				mockSupport.BlockCutterVal.CutNext = true

				mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(0))))
				mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return
				<-mockSupport.Blocks

				close(haltChan)
				logger.Debug("haltChan closed")
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
			})
		})

		// This ensures regular kafka messages of type CONFIG are handled properly
		t.Run("Config", func(t *testing.T) {
			// This test sends a normal tx, followed by a config tx. It should
			// immediately cut them into two blocks.
			t.Run("ReceiveConfigEnvelopeAndCut", func(t *testing.T) {
				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
						CapabilitiesVal: &mockconfig.OrdererCapabilities{
							ResubmissionVal: false,
						},
					},
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				normalBlkOffset := mpc.HighWaterMarkOffset()
				mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(0))))
				mockSupport.BlockCutterVal.Block <- struct{}{} // Let the `mockblockcutter.Ordered` call return

				configBlkOffset := mpc.HighWaterMarkOffset()
				mockSupport.ClassifyMsgVal = msgprocessor.ConfigMsg
				mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
					utils.MarshalOrPanic(newMockConfigEnvelope()),
					uint64(0),
					int64(0))))

				var normalBlk, configBlk *cb.Block
				select {
				case normalBlk = <-mockSupport.Blocks:
				case <-time.After(shortTimeout):
					logger.Fatalf("Did not receive a normal block from the blockcutter as expected")
				}

				select {
				case configBlk = <-mockSupport.Blocks:
				case <-time.After(shortTimeout):
					logger.Fatalf("Did not receive a config block from the blockcutter as expected")
				}

				close(haltChan) // Identical to chain.Halt()
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
				assert.Equal(t, lastCutBlockNumber+2, bareMinimumChain.lastCutBlockNumber, "Expected lastCutBlockNumber to be incremented by 2")
				assert.Equal(t, normalBlkOffset, extractEncodedOffset(normalBlk.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in first block to be %d", normalBlkOffset)
				assert.Equal(t, configBlkOffset, extractEncodedOffset(configBlk.GetMetadata().Metadata[cb.BlockMetadataIndex_ORDERER]), "Expected encoded offset in second block to be %d", configBlkOffset)
			})

			// This ensures config message is re-validated if config seq has advanced
			t.Run("RevalidateConfigEnvInvalid", func(t *testing.T) {
				if testing.Short() {
					t.Skip("Skipping test in short mode")
				}

				errorChan := make(chan struct{})
				close(errorChan)
				haltChan := make(chan struct{})

				lastCutBlockNumber := uint64(3)

				mockSupport := &mockmultichannel.ConsenterSupport{
					Blocks:         make(chan *cb.Block), // WriteBlock will post here
					BlockCutterVal: mockblockcutter.NewReceiver(),
					ChainIDVal:     mockChannel.topic(),
					HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
					ClassifyMsgVal: msgprocessor.ConfigMsg,
					SharedConfigVal: &mockconfig.Orderer{
						BatchTimeoutVal: longTimeout,
						CapabilitiesVal: &mockconfig.OrdererCapabilities{
							ResubmissionVal: false,
						},
					},
					SequenceVal:         uint64(1),
					ProcessConfigMsgErr: fmt.Errorf("Invalid config message"),
				}
				defer close(mockSupport.BlockCutterVal.Block)

				bareMinimumChain := &chainImpl{
					parentConsumer:  mockParentConsumer,
					channelConsumer: mockChannelConsumer,

					channel:            mockChannel,
					ConsenterSupport:   mockSupport,
					lastCutBlockNumber: lastCutBlockNumber,

					errorChan:                      errorChan,
					haltChan:                       haltChan,
					doneProcessingMessagesToBlocks: make(chan struct{}),
				}

				var counts []uint64
				done := make(chan struct{})

				go func() {
					counts, err = bareMinimumChain.processMessagesToBlocks()
					done <- struct{}{}
				}()

				mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
					utils.MarshalOrPanic(newMockConfigEnvelope()),
					uint64(0),
					int64(0))))
				select {
				case <-mockSupport.Blocks:
					t.Fatalf("Expected no block being cut given invalid config message")
				case <-time.After(shortTimeout):
				}

				close(haltChan) // Identical to chain.Halt()
				<-done

				assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
				assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
				assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message error")
			})
		})
	})

	t.Run("KafkaError", func(t *testing.T) {
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

			mockSupport := &mockmultichannel.ConsenterSupport{
				ChainIDVal: mockChannel.topic(),
			}

			bareMinimumChain := &chainImpl{
				consenter:       zeroRetryConsenter, // For sendConnectMessage
				producer:        failedProducer,     // For sendConnectMessage
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:          mockChannel,
				ConsenterSupport: mockSupport,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
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

			mockSupport := &mockmultichannel.ConsenterSupport{
				ChainIDVal: mockChannel.topic(),
			}

			bareMinimumChain := &chainImpl{
				consenter:       zeroRetryConsenter, // For sendConnectMessage
				producer:        failedProducer,     // For sendConnectMessage
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:          mockChannel,
				ConsenterSupport: mockSupport,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			done := make(chan struct{})

			go func() {
				_, err = bareMinimumChain.processMessagesToBlocks()
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
	})
}

// This ensures message is re-validated if config seq has advanced
func TestResubmission(t *testing.T) {
	blockIngressMsg := func(t *testing.T, block bool, fn func() error) {
		wait := make(chan struct{})
		go func() {
			fn()
			wait <- struct{}{}
		}()

		select {
		case <-wait:
			if block {
				t.Fatalf("Expected WaitReady to block")
			}
		case <-time.After(100 * time.Millisecond):
			if !block {
				t.Fatalf("Expected WaitReady not to block")
			}
		}
	}

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockChannel := newChannel(channelNameForTest(t), defaultPartition)
	mockBrokerConfigCopy := *mockBrokerConfig
	mockBrokerConfigCopy.ChannelBufferSize = 0

	mockParentConsumer := mocks.NewConsumer(t, &mockBrokerConfigCopy)
	mpc := mockParentConsumer.ExpectConsumePartition(mockChannel.topic(), mockChannel.partition(), int64(0))
	mockChannelConsumer, err := mockParentConsumer.ConsumePartition(mockChannel.topic(), mockChannel.partition(), int64(0))
	assert.NoError(t, err, "Expected no error when setting up the mock partition consumer")

	t.Run("Normal", func(t *testing.T) {
		// This test lets kafka emit a mock re-submitted message that does not require reprocessing
		// (by setting OriginalOffset <= lastOriginalOffsetProcessed)
		t.Run("AlreadyProcessedDiscard", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)
			lastOriginalOffsetProcessed := int64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:                     mockChannel,
				ConsenterSupport:            mockSupport,
				lastCutBlockNumber:          lastCutBlockNumber,
				lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mockSupport.BlockCutterVal.CutNext = true

			mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(2))))

			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut")
			case <-time.After(shortTimeout):
			}

			close(haltChan)
			logger.Debug("haltChan closed")
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		})

		// This test lets kafka emit a mock re-submitted message that requires reprocessing
		// (by setting OriginalOffset > lastOriginalOffsetProcessed)
		// Two normal messages are enqueued in this test case: reprossed normal message where
		// `originalOffset` is not 0, followed by a normal msg  where `originalOffset` is 0.
		// It tests the case that even no block is cut, `lastOriginalOffsetProcessed` is still
		// updated. We inspect the block to verify correct `LastOriginalOffsetProcessed` in the
		// kafka metadata.
		t.Run("ResubmittedMsgEnqueue", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)
			lastOriginalOffsetProcessed := int64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal: uint64(0),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:                     mockChannel,
				ConsenterSupport:            mockSupport,
				lastCutBlockNumber:          lastCutBlockNumber,
				lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(4))))
			mockSupport.BlockCutterVal.Block <- struct{}{}

			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block to be cut")
			case <-time.After(shortTimeout):
			}

			mockSupport.BlockCutterVal.CutNext = true
			mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(0))))
			mockSupport.BlockCutterVal.Block <- struct{}{}

			select {
			case block := <-mockSupport.Blocks:
				metadata := &cb.Metadata{}
				proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER], metadata)
				kafkaMetadata := &ab.KafkaMetadata{}
				proto.Unmarshal(metadata.Value, kafkaMetadata)
				assert.Equal(t, kafkaMetadata.LastOriginalOffsetProcessed, int64(4))
			case <-time.After(shortTimeout):
				t.Fatalf("Expected one block being cut")
			}

			close(haltChan)
			logger.Debug("haltChan closed")
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 2 REGULAR message processed")
		})

		t.Run("InvalidDiscard", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:         uint64(1),
				ProcessNormalMsgErr: fmt.Errorf("Invalid normal message"),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:            mockChannel,
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(
				utils.MarshalOrPanic(newMockNormalEnvelope(t)),
				uint64(0),
				int64(0))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut given invalid config message")
			case <-time.After(shortTimeout):
			}

			close(haltChan) // Identical to chain.Halt()
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message error")
		})

		// This tests resubmission path with following steps:
		// 1) Kafka emits a message with lagged config seq, consenter is expected to re-process and
		//    re-submit the message. However, `WaitReady` shouldn't be blocked for a normal message
		// 2) Kafka is expected to receive a producer message where config seq is advanced to catch
		//    up with current config seq, and OriginalOffset is not nil to capture the offset that
		//    consenter previously received from Kafka
		// 3) when consenter receives Kafka message submitted in 2), where config seq is in sync,
		//    it cuts a block for it.
		t.Run("ValidResubmit", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			startChan := make(chan struct{})
			close(startChan)
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})
			doneReprocessing := make(chan struct{})
			close(doneReprocessing)

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:  uint64(1),
				ConfigSeqVal: uint64(1),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			expectedKafkaMsg := &ab.KafkaMessage{}
			producer := mocks.NewSyncProducer(t, mockBrokerConfig)
			producer.ExpectSendMessageWithCheckerFunctionAndSucceed(func(val []byte) error {
				if err := proto.Unmarshal(val, expectedKafkaMsg); err != nil {
					return err
				}

				regular := expectedKafkaMsg.GetRegular()
				if regular == nil {
					return fmt.Errorf("Expect message type to be regular")
				}

				if regular.ConfigSeq != mockSupport.Sequence() {
					return fmt.Errorf("Expect new config seq to be %d, got %d", mockSupport.Sequence(), regular.ConfigSeq)
				}

				if regular.OriginalOffset == 0 {
					return fmt.Errorf("Expect Original Offset to be non-zero if resubmission")
				}

				return nil
			})

			bareMinimumChain := &chainImpl{
				producer:        producer,
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:            mockChannel,
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				startChan:                      startChan,
				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
				doneReprocessingMsgInFlight:    doneReprocessing,
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mockSupport.BlockCutterVal.CutNext = true

			mpc.YieldMessage(newMockConsumerMessage(newNormalMessage(
				utils.MarshalOrPanic(newMockNormalEnvelope(t)),
				uint64(0),
				int64(0))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut given invalid config message")
			case <-time.After(shortTimeout):
			}

			// check that WaitReady is not blocked for a in-flight reprocessed messages of type NORMAL
			waitReady := make(chan struct{})
			go func() {
				bareMinimumChain.WaitReady()
				waitReady <- struct{}{}
			}()

			select {
			case <-waitReady:
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Expected WaitReady call to be unblock because all reprocessed messages are consumed")
			}

			// Emits the kafka message produced by consenter
			mpc.YieldMessage(newMockConsumerMessage(expectedKafkaMsg))
			mockSupport.BlockCutterVal.Block <- struct{}{}

			select {
			case <-mockSupport.Blocks:
			case <-time.After(shortTimeout):
				t.Fatalf("Expected one block being cut")
			}

			close(haltChan) // Identical to chain.Halt()
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 2 REGULAR message error")
		})
	})

	t.Run("Config", func(t *testing.T) {
		// This test lets kafka emit a mock re-submitted message that does not require reprocessing
		// (by setting OriginalOffset <= lastOriginalOffsetProcessed)
		t.Run("AlreadyProcessedDiscard", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)
			lastOriginalOffsetProcessed := int64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:                     mockChannel,
				ConsenterSupport:            mockSupport,
				lastCutBlockNumber:          lastCutBlockNumber,
				lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(0), int64(2))))

			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut")
			case <-time.After(shortTimeout):
			}

			close(haltChan)
			logger.Debug("haltChan closed")
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		})

		// This test simulated the non-deterministic case, where somebody resubmitted message at offset X,
		// whereas we didn't. That message was considered invalid by us during revalidation, however somebody
		// else deemed it to be valid, and resubmitted it.
		t.Run("Non-determinism", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			startChan := make(chan struct{})
			close(startChan)
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})
			doneReprocessing := make(chan struct{})
			close(doneReprocessing)

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:         uint64(1),
				ConfigSeqVal:        uint64(1),
				ProcessConfigMsgVal: newMockConfigEnvelope(),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:            mockChannel,
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				lastResubmittedConfigOffset: int64(0),

				startChan:                      startChan,
				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
				doneReprocessingMsgInFlight:    doneReprocessing,
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			// check that WaitReady is not blocked at beginning
			blockIngressMsg(t, false, bareMinimumChain.WaitReady)

			// Message should be revalidated but considered invalid, so we don't resubmit it
			mockSupport.ProcessConfigMsgErr = fmt.Errorf("invalid message found during revalidation")

			// Emits a config message with lagged config sequence
			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
				utils.MarshalOrPanic(newMockConfigEnvelope()),
				uint64(0),
				int64(0))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut")
			case <-time.After(shortTimeout):
			}

			// check that WaitReady is still not blocked because we haven't resubmitted anything
			blockIngressMsg(t, false, bareMinimumChain.WaitReady)

			// Somebody else resubmitted the message which we deemed to be invalid
			// We deliberately keep ProcessConfigMsgErr unchanged, so we could be
			// certain that we are not running into revalidation path.
			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
				utils.MarshalOrPanic(newMockConfigEnvelope()),
				uint64(1),
				int64(5))))

			select {
			case block := <-mockSupport.Blocks:
				metadata, err := utils.GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
				assert.NoError(t, err, "Failed to get metadata from block")
				kafkaMetadata := &ab.KafkaMetadata{}
				err = proto.Unmarshal(metadata.Value, kafkaMetadata)
				assert.NoError(t, err, "Failed to unmarshal metadata")

				assert.Equal(t, kafkaMetadata.LastResubmittedConfigOffset, int64(5), "LastResubmittedConfigOffset didn't catch up")
				assert.Equal(t, kafkaMetadata.LastOriginalOffsetProcessed, int64(5), "LastOriginalOffsetProcessed doesn't match")
			case <-time.After(shortTimeout):
				t.Fatalf("Expected one block being cut")
			}

			close(haltChan) // Identical to chain.Halt()
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 2 REGULAR message error")
		})

		// This test lets kafka emit a mock re-submitted message whose config seq is still behind
		t.Run("ResubmittedMsgStillBehind", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			startChan := make(chan struct{})
			close(startChan)
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)
			lastOriginalOffsetProcessed := int64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:         uint64(2),
				ProcessConfigMsgVal: newMockConfigEnvelope(),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			producer := mocks.NewSyncProducer(t, mockBrokerConfig)
			producer.ExpectSendMessageWithCheckerFunctionAndSucceed(func(val []byte) error {
				return nil
			})

			bareMinimumChain := &chainImpl{
				producer:        producer,
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:                     mockChannel,
				ConsenterSupport:            mockSupport,
				lastCutBlockNumber:          lastCutBlockNumber,
				lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,

				startChan:                      startChan,
				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
				doneReprocessingMsgInFlight:    make(chan struct{}),
			}

			// WaitReady should block at beginning since we are in the middle of reprocessing
			blockIngressMsg(t, true, bareMinimumChain.WaitReady)

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(utils.MarshalOrPanic(newMockEnvelope("fooMessage")), uint64(1), int64(4))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut")
			case <-time.After(shortTimeout):
			}

			// WaitReady should still block as resubmitted config message is still behind current config seq
			blockIngressMsg(t, true, bareMinimumChain.WaitReady)

			close(haltChan)
			logger.Debug("haltChan closed")
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularPass], "Expected 1 REGULAR message processed")
		})

		t.Run("InvalidDiscard", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:         uint64(1),
				ProcessConfigMsgErr: fmt.Errorf("Invalid config message"),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			bareMinimumChain := &chainImpl{
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:            mockChannel,
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
				utils.MarshalOrPanic(newMockNormalEnvelope(t)),
				uint64(0),
				int64(0))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut given invalid config message")
			case <-time.After(shortTimeout):
			}

			close(haltChan) // Identical to chain.Halt()
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(1), counts[indexRecvPass], "Expected 1 message received and unmarshaled")
			assert.Equal(t, uint64(1), counts[indexProcessRegularError], "Expected 1 REGULAR message error")
		})

		// This tests resubmission path with following steps:
		// 1) Kafka emits a message with lagged config seq, consenter is expected to re-process and
		//    re-submit the message, as well as block `WaitReady` API
		// 2) Kafka is expected to receive a producer message where config seq is advanced to catch
		//    up with current config seq, and OriginalOffset is not nil to capture the offset that
		//    consenter previously received from Kafka
		// 3) when consenter receives Kafka message submitted in 2), where config seq is in sync,
		//    it cuts a block for it and lifts block on `WaitReady`.
		t.Run("ValidResubmit", func(t *testing.T) {
			if testing.Short() {
				t.Skip("Skipping test in short mode")
			}

			startChan := make(chan struct{})
			close(startChan)
			errorChan := make(chan struct{})
			close(errorChan)
			haltChan := make(chan struct{})
			doneReprocessing := make(chan struct{})
			close(doneReprocessing)

			lastCutBlockNumber := uint64(3)

			mockSupport := &mockmultichannel.ConsenterSupport{
				Blocks:         make(chan *cb.Block), // WriteBlock will post here
				BlockCutterVal: mockblockcutter.NewReceiver(),
				ChainIDVal:     mockChannel.topic(),
				HeightVal:      lastCutBlockNumber, // Incremented during the WriteBlock call
				SharedConfigVal: &mockconfig.Orderer{
					BatchTimeoutVal: longTimeout,
					CapabilitiesVal: &mockconfig.OrdererCapabilities{
						ResubmissionVal: true,
					},
				},
				SequenceVal:         uint64(1),
				ConfigSeqVal:        uint64(1),
				ProcessConfigMsgVal: newMockConfigEnvelope(),
			}
			defer close(mockSupport.BlockCutterVal.Block)

			expectedKafkaMsg := &ab.KafkaMessage{}
			producer := mocks.NewSyncProducer(t, mockBrokerConfig)
			producer.ExpectSendMessageWithCheckerFunctionAndSucceed(func(val []byte) error {
				if err := proto.Unmarshal(val, expectedKafkaMsg); err != nil {
					return err
				}

				regular := expectedKafkaMsg.GetRegular()
				if regular == nil {
					return fmt.Errorf("Expect message type to be regular")
				}

				if regular.ConfigSeq != mockSupport.Sequence() {
					return fmt.Errorf("Expect new config seq to be %d, got %d", mockSupport.Sequence(), regular.ConfigSeq)
				}

				if regular.OriginalOffset == 0 {
					return fmt.Errorf("Expect Original Offset to be non-zero if resubmission")
				}

				return nil
			})

			bareMinimumChain := &chainImpl{
				producer:        producer,
				parentConsumer:  mockParentConsumer,
				channelConsumer: mockChannelConsumer,

				channel:            mockChannel,
				ConsenterSupport:   mockSupport,
				lastCutBlockNumber: lastCutBlockNumber,

				startChan:                      startChan,
				errorChan:                      errorChan,
				haltChan:                       haltChan,
				doneProcessingMessagesToBlocks: make(chan struct{}),
				doneReprocessingMsgInFlight:    doneReprocessing,
			}

			var counts []uint64
			done := make(chan struct{})

			go func() {
				counts, err = bareMinimumChain.processMessagesToBlocks()
				done <- struct{}{}
			}()

			// check that WaitReady is not blocked at beginning
			blockIngressMsg(t, false, bareMinimumChain.WaitReady)

			// Emits a config message with lagged config sequence
			mpc.YieldMessage(newMockConsumerMessage(newConfigMessage(
				utils.MarshalOrPanic(newMockConfigEnvelope()),
				uint64(0),
				int64(0))))
			select {
			case <-mockSupport.Blocks:
				t.Fatalf("Expected no block being cut given lagged config message")
			case <-time.After(shortTimeout):
			}

			// check that WaitReady is actually blocked because of in-flight reprocessed messages
			blockIngressMsg(t, true, bareMinimumChain.WaitReady)

			// Emits the kafka message produced by consenter
			mpc.YieldMessage(newMockConsumerMessage(expectedKafkaMsg))

			select {
			case <-mockSupport.Blocks:
			case <-time.After(shortTimeout):
				t.Fatalf("Expected one block being cut")
			}

			// `WaitReady` should be unblocked now
			blockIngressMsg(t, false, bareMinimumChain.WaitReady)

			close(haltChan) // Identical to chain.Halt()
			<-done

			assert.NoError(t, err, "Expected the processMessagesToBlocks call to return without errors")
			assert.Equal(t, uint64(2), counts[indexRecvPass], "Expected 2 message received and unmarshaled")
			assert.Equal(t, uint64(2), counts[indexProcessRegularPass], "Expected 2 REGULAR message error")
		})
	})
}

// Test helper functions here.

func newRegularMessage(payload []byte) *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_Regular{
			Regular: &ab.KafkaMessageRegular{
				Payload: payload,
			},
		},
	}
}

func newMockNormalEnvelope(t *testing.T) *cb.Envelope {
	return &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{
		Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(
			&cb.ChannelHeader{Type: int32(cb.HeaderType_MESSAGE), ChannelId: channelNameForTest(t)})},
		Data: []byte("Foo"),
	})}
}

func newMockConfigEnvelope() *cb.Envelope {
	return &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{
		Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(
			&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG), ChannelId: "foo"})},
		Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{}),
	})}
}

func newMockOrdererTxEnvelope() *cb.Envelope {
	return &cb.Envelope{Payload: utils.MarshalOrPanic(&cb.Payload{
		Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(
			&cb.ChannelHeader{Type: int32(cb.HeaderType_ORDERER_TRANSACTION), ChannelId: "foo"})},
		Data: utils.MarshalOrPanic(newMockConfigEnvelope()),
	})}
}

func TestDeliverSession(t *testing.T) {

	type testEnvironment struct {
		channelID  string
		topic      string
		partition  int32
		height     int64
		nextOffset int64
		support    *mockConsenterSupport
		broker0    *sarama.MockBroker
		broker1    *sarama.MockBroker
		broker2    *sarama.MockBroker
		testMsg    sarama.Encoder
	}

	// initializes test environment
	newTestEnvironment := func(t *testing.T) *testEnvironment {

		channelID := channelNameForTest(t)
		topic := channelID
		partition := int32(defaultPartition)
		height := int64(100)
		nextOffset := height + 1
		broker0 := sarama.NewMockBroker(t, 0)
		broker1 := sarama.NewMockBroker(t, 1)
		broker2 := sarama.NewMockBroker(t, 2)

		// broker0 will seed the info about the other brokers and the partition leader
		broker0.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockMetadataResponse(t).
				SetBroker(broker1.Addr(), broker1.BrokerID()).
				SetBroker(broker2.Addr(), broker2.BrokerID()).
				SetLeader(topic, partition, broker1.BrokerID()),
		})

		// configure broker1 with responses needed for startup
		broker1.SetHandlerByMap(map[string]sarama.MockResponse{
			// CONNECT ProduceRequest
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(topic, partition, sarama.ErrNoError),
			// respond to request for offset of topic
			"OffsetRequest": sarama.NewMockOffsetResponse(t).
				SetOffset(topic, partition, sarama.OffsetOldest, 0).
				SetOffset(topic, partition, sarama.OffsetNewest, nextOffset),
			// respond to fetch requests with empty response while starting up
			"FetchRequest": sarama.NewMockFetchResponse(t, 1),
		})

		// configure broker2 with a default fetch request response
		broker2.SetHandlerByMap(map[string]sarama.MockResponse{
			// respond to fetch requests with empty response while starting up
			"FetchRequest": sarama.NewMockFetchResponse(t, 1),
		})

		// setup mock blockcutter
		blockcutter := &mockReceiver{}
		blockcutter.On("Ordered", mock.Anything).Return([][]*cb.Envelope{{&cb.Envelope{}}}, false)

		// setup mock chain support and mock method calls
		support := &mockConsenterSupport{}
		support.On("Height").Return(uint64(height))
		support.On("ChainID").Return(topic)
		support.On("Sequence").Return(uint64(0))
		support.On("SharedConfig").Return(&mockconfig.Orderer{KafkaBrokersVal: []string{broker0.Addr()}})
		support.On("ClassifyMsg", mock.Anything).Return(msgprocessor.NormalMsg, nil)
		support.On("ProcessNormalMsg", mock.Anything).Return(uint64(0), nil)
		support.On("BlockCutter").Return(blockcutter)
		support.On("CreateNextBlock", mock.Anything).Return(&cb.Block{})

		// test message that will be returned by mock brokers
		testMsg := sarama.ByteEncoder(utils.MarshalOrPanic(
			newRegularMessage(utils.MarshalOrPanic(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: topic,
						}),
					},
					Data: []byte("TEST_DATA"),
				})})),
		))

		return &testEnvironment{
			channelID:  channelID,
			topic:      topic,
			partition:  partition,
			height:     height,
			nextOffset: nextOffset,
			support:    support,
			broker0:    broker0,
			broker1:    broker1,
			broker2:    broker2,
			testMsg:    testMsg,
		}
	}

	// BrokerDeath simulates the partition leader dying and a
	// second broker becoming the leader before the deliver session times out.
	t.Run("BrokerDeath", func(t *testing.T) {

		// initialize test environment
		env := newTestEnvironment(t)

		// broker1 will be closed within the test
		defer env.broker0.Close()
		defer env.broker2.Close()

		// initialize consenter
		consenter := New(mockLocalConfig.Kafka)

		// initialize chain
		metadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: env.height})}
		chain, err := consenter.HandleChain(env.support, metadata)
		if err != nil {
			t.Fatal(err)
		}

		// start the chain, and wait for it to settle down
		chain.Start()
		select {
		case <-chain.(*chainImpl).startChan:
			logger.Debug("chain started")
		case <-time.After(shortTimeout):
			t.Fatal("chain should have started by now")
		}

		// direct blocks to this channel
		blocks := make(chan *cb.Block, 1)
		env.support.On("WriteBlock", mock.Anything, mock.Anything).Return().Run(func(arg1 mock.Arguments) {
			blocks <- arg1.Get(0).(*cb.Block)
		})

		// send a few messages from broker1
		fetchResponse1 := sarama.NewMockFetchResponse(t, 1)
		for i := 0; i < 5; i++ {
			fetchResponse1.SetMessage(env.topic, env.partition, env.nextOffset, env.testMsg)
			env.nextOffset++
		}
		env.broker1.SetHandlerByMap(map[string]sarama.MockResponse{
			"FetchRequest": fetchResponse1,
		})

		logger.Debug("Waiting for messages from broker1")
		for i := 0; i < 5; i++ {
			select {
			case <-blocks:
			case <-time.After(shortTimeout):
				t.Fatalf("timed out waiting for messages (receieved %d messages)", i)
			}
		}

		// prepare broker2 to send a few messages
		fetchResponse2 := sarama.NewMockFetchResponse(t, 1)
		for i := 0; i < 5; i++ {
			fetchResponse2.SetMessage(env.topic, env.partition, env.nextOffset, env.testMsg)
			env.nextOffset++
		}

		env.broker2.SetHandlerByMap(map[string]sarama.MockResponse{
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(env.topic, env.partition, sarama.ErrNoError),
			"FetchRequest": fetchResponse2,
		})

		// shutdown broker1
		env.broker1.Close()

		// prepare broker0 to respond that broker2 is now the leader
		env.broker0.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockMetadataResponse(t).
				SetLeader(env.topic, env.partition, env.broker2.BrokerID()),
		})

		logger.Debug("Waiting for messages from broker2")
		for i := 0; i < 5; i++ {
			select {
			case <-blocks:
			case <-time.After(shortTimeout):
				t.Fatalf("timed out waiting for messages (receieved %d messages)", i)
			}
		}

		chain.Halt()
	})

	// An ErrOffsetOutOfRange is non-recoverable
	t.Run("ErrOffsetOutOfRange", func(t *testing.T) {

		// initialize test environment
		env := newTestEnvironment(t)

		// broker cleanup
		defer env.broker2.Close()
		defer env.broker1.Close()
		defer env.broker0.Close()

		// initialize consenter
		consenter := New(mockLocalConfig.Kafka)

		// initialize chain
		metadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: env.height})}
		chain, err := consenter.HandleChain(env.support, metadata)
		if err != nil {
			t.Fatal(err)
		}

		// start the chain, and wait for it to settle down
		chain.Start()
		select {
		case <-chain.(*chainImpl).startChan:
			logger.Debug("chain started")
		case <-time.After(shortTimeout):
			t.Fatal("chain should have started by now")
		}

		// direct blocks to this channel
		blocks := make(chan *cb.Block, 1)
		env.support.On("WriteBlock", mock.Anything, mock.Anything).Return().Run(func(arg1 mock.Arguments) {
			blocks <- arg1.Get(0).(*cb.Block)
		})

		// set broker1 to respond to two fetch requests:
		// - The first fetch request will get an ErrOffsetOutOfRange error response.
		// - The second fetch request will get a valid (i.e. non-error) response.
		fetchResponse := &sarama.FetchResponse{}
		fetchResponse.AddError(env.topic, env.partition, sarama.ErrOffsetOutOfRange)
		fetchResponse.AddMessage(env.topic, env.partition, nil, env.testMsg, env.nextOffset)
		env.nextOffset++
		env.broker1.SetHandlerByMap(map[string]sarama.MockResponse{
			"FetchRequest": sarama.NewMockWrapper(fetchResponse),
			// answers for CONNECT message
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(env.topic, env.partition, sarama.ErrNoError),
		})

		select {
		case <-blocks:
			// the valid fetch response should not of been fetched
			t.Fatal("Did not expect new blocks")
		case <-time.After(shortTimeout):
			t.Fatal("Errored() should have closed by now")
		case <-chain.Errored():
		}

		chain.Halt()
	})

	// test chain timeout
	t.Run("DeliverSessionTimedOut", func(t *testing.T) {

		// initialize test environment
		env := newTestEnvironment(t)

		// broker cleanup
		defer env.broker2.Close()
		defer env.broker1.Close()
		defer env.broker0.Close()

		// initialize consenter
		consenter := New(mockLocalConfig.Kafka)

		// initialize chain
		metadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: env.height})}
		chain, err := consenter.HandleChain(env.support, metadata)
		if err != nil {
			t.Fatal(err)
		}

		// start the chain, and wait for it to settle down
		chain.Start()
		select {
		case <-chain.(*chainImpl).startChan:
			logger.Debug("chain started")
		case <-time.After(shortTimeout):
			t.Fatal("chain should have started by now")
		}

		// direct blocks to this channel
		blocks := make(chan *cb.Block, 1)
		env.support.On("WriteBlock", mock.Anything, mock.Anything).Return().Run(func(arg1 mock.Arguments) {
			blocks <- arg1.Get(0).(*cb.Block)
		})

		metadataResponse := new(sarama.MetadataResponse)
		metadataResponse.AddTopicPartition(env.topic, env.partition, -1, []int32{}, []int32{}, sarama.ErrBrokerNotAvailable)

		// configure seed broker to return error on metadata request, otherwise the
		// consumer client will keep 'subscribing' successfully to the topic/partition
		env.broker0.SetHandlerByMap(map[string]sarama.MockResponse{
			"MetadataRequest": sarama.NewMockWrapper(metadataResponse),
		})

		// set broker1 to return an error.
		// Note that the following are not considered errors from the sarama client
		// consumer's point of view:
		// - ErrUnknownTopicOrPartition
		// - ErrNotLeaderForPartition
		// - ErrLeaderNotAvailable
		// - ErrReplicaNotAvailable:
		fetchResponse := &sarama.FetchResponse{}
		fetchResponse.AddError(env.topic, env.partition, sarama.ErrUnknown)
		env.broker1.SetHandlerByMap(map[string]sarama.MockResponse{
			"FetchRequest": sarama.NewMockWrapper(fetchResponse),
			// answers for CONNECT message
			"ProduceRequest": sarama.NewMockProduceResponse(t).
				SetError(env.topic, env.partition, sarama.ErrNoError),
		})

		select {
		case <-blocks:
			t.Fatal("Did not expect new blocks")
		case <-time.After(mockRetryOptions.NetworkTimeouts.ReadTimeout + shortTimeout):
			t.Fatal("Errored() should have closed by now")
		case <-chain.Errored():
			t.Log("Errored() closed")
		}

		chain.Halt()
	})

}

type mockReceiver struct {
	mock.Mock
}

func (r *mockReceiver) Ordered(msg *cb.Envelope) (messageBatches [][]*cb.Envelope, pending bool) {
	args := r.Called(msg)
	return args.Get(0).([][]*cb.Envelope), args.Bool(1)
}

func (r *mockReceiver) Cut() []*cb.Envelope {
	args := r.Called()
	return args.Get(0).([]*cb.Envelope)
}

type mockConsenterSupport struct {
	mock.Mock
}

func (c *mockConsenterSupport) NewSignatureHeader() (*cb.SignatureHeader, error) {
	args := c.Called()
	return args.Get(0).(*cb.SignatureHeader), args.Error(1)
}

func (c *mockConsenterSupport) Sign(message []byte) ([]byte, error) {
	args := c.Called(message)
	return args.Get(0).([]byte), args.Error(1)
}

func (c *mockConsenterSupport) ClassifyMsg(chdr *cb.ChannelHeader) msgprocessor.Classification {
	args := c.Called(chdr)
	return args.Get(0).(msgprocessor.Classification)
}

func (c *mockConsenterSupport) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	args := c.Called(env)
	return args.Get(0).(uint64), args.Error(1)
}

func (c *mockConsenterSupport) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	args := c.Called(env)
	return args.Get(0).(*cb.Envelope), args.Get(1).(uint64), args.Error(2)
}

func (c *mockConsenterSupport) ProcessConfigMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	args := c.Called(env)
	return args.Get(0).(*cb.Envelope), args.Get(1).(uint64), args.Error(2)
}

func (c *mockConsenterSupport) BlockCutter() blockcutter.Receiver {
	args := c.Called()
	return args.Get(0).(blockcutter.Receiver)
}

func (c *mockConsenterSupport) SharedConfig() channelconfig.Orderer {
	args := c.Called()
	return args.Get(0).(channelconfig.Orderer)
}

func (c *mockConsenterSupport) CreateNextBlock(messages []*cb.Envelope) *cb.Block {
	args := c.Called(messages)
	return args.Get(0).(*cb.Block)
}

func (c *mockConsenterSupport) WriteBlock(block *cb.Block, encodedMetadataValue []byte) {
	c.Called(block, encodedMetadataValue)
	return
}

func (c *mockConsenterSupport) WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) {
	c.Called(block, encodedMetadataValue)
	return
}

func (c *mockConsenterSupport) Sequence() uint64 {
	args := c.Called()
	return args.Get(0).(uint64)
}

func (c *mockConsenterSupport) ChainID() string {
	args := c.Called()
	return args.String(0)
}

func (c *mockConsenterSupport) Height() uint64 {
	args := c.Called()
	return args.Get(0).(uint64)
}
