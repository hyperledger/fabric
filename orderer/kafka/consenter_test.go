/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	localconfig "github.com/hyperledger/fabric/orderer/localconfig"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mockmultichain "github.com/hyperledger/fabric/orderer/mocks/multichain"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

var mockRetryOptions = localconfig.Retry{
	ShortInterval: 50 * time.Millisecond,
	ShortTotal:    100 * time.Millisecond,
	LongInterval:  60 * time.Millisecond,
	LongTotal:     120 * time.Millisecond,
	NetworkTimeouts: localconfig.NetworkTimeouts{
		DialTimeout:  40 * time.Millisecond,
		ReadTimeout:  40 * time.Millisecond,
		WriteTimeout: 40 * time.Millisecond,
	},
	Metadata: localconfig.Metadata{
		RetryMax:     2,
		RetryBackoff: 40 * time.Millisecond,
	},
	Producer: localconfig.Producer{
		RetryMax:     2,
		RetryBackoff: 40 * time.Millisecond,
	},
	Consumer: localconfig.Consumer{
		RetryBackoff: 40 * time.Millisecond,
	},
}

func init() {
	mockLocalConfig = newMockLocalConfig(false, mockRetryOptions, false)
	mockBrokerConfig = newMockBrokerConfig(mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version, defaultPartition)
	mockConsenter = newMockConsenter(mockBrokerConfig, mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version)
	setupTestLogging("ERROR", mockLocalConfig.Kafka.Verbose)
}

func TestNew(t *testing.T) {
	_ = multichain.Consenter(New(mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version))
}

func TestHandleChain(t *testing.T) {
	consenter := multichain.Consenter(New(mockLocalConfig.General.TLS, mockLocalConfig.Kafka.Retry, mockLocalConfig.Kafka.Version))

	oldestOffset := int64(0)
	newestOffset := int64(5)
	message := sarama.StringEncoder("messageFoo")

	mockChannel := newChannel(channelNameForTest(t), defaultPartition)

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
		ChainIDVal: mockChannel.topic(),
		SharedConfigVal: &mockconfig.Orderer{
			KafkaBrokersVal: []string{mockBroker.Addr()},
		},
	}

	mockMetadata := &cb.Metadata{Value: utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: newestOffset - 1})}

	_, err := consenter.HandleChain(mockSupport, mockMetadata)
	assert.NoError(t, err, "Expected the HandleChain call to return without errors")
}

// Test helper functions and mock objects defined here

var mockConsenter commonConsenter
var mockLocalConfig *localconfig.TopLevel
var mockBrokerConfig *sarama.Config

func extractEncodedOffset(marshalledOrdererMetadata []byte) int64 {
	omd := &cb.Metadata{}
	_ = proto.Unmarshal(marshalledOrdererMetadata, omd)
	kmd := &ab.KafkaMetadata{}
	_ = proto.Unmarshal(omd.GetValue(), kmd)
	return kmd.LastOffsetPersisted
}

func newMockBrokerConfig(tlsConfig localconfig.TLS, retryOptions localconfig.Retry, kafkaVersion sarama.KafkaVersion, chosenStaticPartition int32) *sarama.Config {
	brokerConfig := newBrokerConfig(tlsConfig, retryOptions, kafkaVersion, chosenStaticPartition)
	brokerConfig.ClientID = "test"
	return brokerConfig
}

func newMockConsenter(brokerConfig *sarama.Config, tlsConfig localconfig.TLS, retryOptions localconfig.Retry, kafkaVersion sarama.KafkaVersion) *consenterImpl {
	return &consenterImpl{
		brokerConfigVal: brokerConfig,
		tlsConfigVal:    tlsConfig,
		retryOptionsVal: retryOptions,
		kafkaVersionVal: kafkaVersion,
	}
}

func newMockConsumerMessage(wrappedMessage *ab.KafkaMessage) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Value: sarama.ByteEncoder(utils.MarshalOrPanic(wrappedMessage)),
	}
}

func newMockEnvelope(content string) *cb.Envelope {
	return &cb.Envelope{Payload: []byte(content)}
}

func newMockLocalConfig(enableTLS bool, retryOptions localconfig.Retry, verboseLog bool) *localconfig.TopLevel {
	return &localconfig.TopLevel{
		General: localconfig.General{
			TLS: localconfig.TLS{
				Enabled: enableTLS,
			},
		},
		Kafka: localconfig.Kafka{
			Retry:   retryOptions,
			Verbose: verboseLog,
			Version: sarama.V0_9_0_1,
		},
	}
}

func setupTestLogging(logLevel string, verbose bool) {
	// This call allows us to (a) get the logging backend initialization that
	// takes place in the `flogging` package, and (b) adjust the verbosity of
	// the logs when running tests on this package.
	flogging.SetModuleLevel(pkgLogID, logLevel)

	if verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
	}
}

// Taken from orderer/solo/consensus_test.go
func syncQueueMessage(message *cb.Envelope, chain *chainImpl, mockBlockcutter *mockblockcutter.Receiver) {
	chain.Enqueue(message)
	mockBlockcutter.Block <- struct{}{} // We'll move past this line (and the function will return) only when the mock blockcutter is about to return
}

func tamperBytes(original []byte) []byte {
	byteCount := len(original)
	return original[:byteCount-1]
}

func channelNameForTest(t *testing.T) string {
	name := strings.Split(fmt.Sprint(t), " ")[18] // w/golang 1.8, use t.Name()
	return fmt.Sprintf("%s.channel", strings.Replace(strings.ToLower(name), "/", ".", -1))
}
