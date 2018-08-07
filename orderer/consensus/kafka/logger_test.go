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
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/stretchr/testify/assert"
)

func TestLoggerInit(t *testing.T) {
	assert.IsType(t, &saramaLoggerImpl{}, sarama.Logger, "Sarama logger (sarama.Logger) is not properly initialized.")
	assert.NotNil(t, saramaLogger, "Event logger (saramaLogger) is not properly initialized, it's Nil.")
	assert.Equal(t, sarama.Logger, saramaLogger, "Sarama logger (sarama.Logger) and Event logger (saramaLogger) should be the same.")
}

func TestEventLogger(t *testing.T) {
	eventMessage := "this message contains an interesting string within"
	substr := "interesting string"

	// register listener
	eventChan := saramaLogger.NewListener(substr)

	// register a second listener (same subscription)
	eventChan2 := saramaLogger.NewListener(substr)
	defer saramaLogger.RemoveListener(substr, eventChan2)

	// invoke logger
	saramaLogger.Print("this message is not interesting")
	saramaLogger.Print(eventMessage)

	select {
	// expect event from first listener
	case receivedEvent := <-eventChan:
		assert.Equal(t, eventMessage, receivedEvent, "")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event on eventChan")
	}

	// expect event from sesond listener
	select {
	case receivedEvent := <-eventChan2:
		assert.Equal(t, eventMessage, receivedEvent, "")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event on eventChan2")
	}

	// unregister the first listener
	saramaLogger.RemoveListener(substr, eventChan)

	// invoke logger
	saramaLogger.Print(eventMessage)

	// expect no events from first listener
	select {
	case <-eventChan:
		t.Fatal("did not expect an event")
	case <-time.After(10 * time.Millisecond):
	}

	// expect event from sesond listener
	select {
	case receivedEvent := <-eventChan2:
		assert.Equal(t, eventMessage, receivedEvent, "")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event on eventChan2")
	}

}

func TestEventListener(t *testing.T) {
	topic := channelNameForTest(t)
	partition := int32(0)

	subscription := fmt.Sprintf("added subscription to %s/%d", topic, partition)
	listenerChan := saramaLogger.NewListener(subscription)
	defer saramaLogger.RemoveListener(subscription, listenerChan)

	go func() {
		event := <-listenerChan
		t.Logf("GOT: %s", event)
	}()

	broker := sarama.NewMockBroker(t, 500)
	defer broker.Close()

	config := sarama.NewConfig()
	config.ClientID = t.Name()
	config.Metadata.Retry.Max = 0
	config.Metadata.Retry.Backoff = 250 * time.Millisecond
	config.Net.ReadTimeout = 100 * time.Millisecond

	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(topic, partition, broker.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(topic, partition, sarama.OffsetNewest, 1000).
			SetOffset(topic, partition, sarama.OffsetOldest, 0),
		"FetchRequest": sarama.NewMockFetchResponse(t, 1).
			SetMessage(topic, partition, 0, sarama.StringEncoder("MSG 00")).
			SetMessage(topic, partition, 1, sarama.StringEncoder("MSG 01")).
			SetMessage(topic, partition, 2, sarama.StringEncoder("MSG 02")).
			SetMessage(topic, partition, 3, sarama.StringEncoder("MSG 03")),
	})

	consumer, err := sarama.NewConsumer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()

	partitionConsumer, err := consumer.ConsumePartition(topic, partition, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer partitionConsumer.Close()

	for i := 0; i < 3; i++ {
		select {
		case <-partitionConsumer.Messages():
		case <-time.After(shortTimeout):
			t.Fatalf("timed out waiting for messages (receieved %d messages)", i)
		}
	}
}

func TestLogPossibleKafkaVersionMismatch(t *testing.T) {
	topic := channelNameForTest(t)
	partition := int32(0)

	oldLogger := logger
	defer func() { logger = oldLogger }()

	l, recorder := floggingtest.NewTestLogger(t)
	logger = l

	broker := sarama.NewMockBroker(t, 500)
	defer broker.Close()

	config := sarama.NewConfig()
	config.ClientID = t.Name()
	config.Metadata.Retry.Max = 0
	config.Metadata.Retry.Backoff = 250 * time.Millisecond
	config.Net.ReadTimeout = 100 * time.Millisecond
	config.Version = sarama.V0_10_0_0

	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(topic, partition, broker.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(topic, partition, sarama.OffsetNewest, 1000).
			SetOffset(topic, partition, sarama.OffsetOldest, 0),
		"FetchRequest": sarama.NewMockFetchResponse(t, 1).
			SetMessage(topic, partition, 0, sarama.StringEncoder("MSG 00")),
	})

	consumer, err := sarama.NewConsumer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()

	partitionConsumer, err := consumer.ConsumePartition(topic, partition, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer partitionConsumer.Close()

	select {
	case <-partitionConsumer.Messages():
		t.Fatalf("did not expect to receive message")
	case <-time.After(shortTimeout):
		entries := recorder.MessagesContaining("Kafka.Version specified in the orderer configuration is incorrectly set")
		assert.NotEmpty(t, entries)
	}
}
