/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"crypto/tls"
	"testing"

	"github.com/Shopify/sarama"
	localconfig "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/mocks/util"
	"github.com/stretchr/testify/require"
)

func TestBrokerConfig(t *testing.T) {
	mockChannel1 := newChannel(channelNameForTest(t), defaultPartition)
	// Use a partition ID that is not the 'default' (defaultPartition)
	var differentPartition int32 = defaultPartition + 1
	mockChannel2 := newChannel(channelNameForTest(t), differentPartition)

	mockBroker := sarama.NewMockBroker(t, 0)
	defer func() { mockBroker.Close() }()

	mockBroker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(mockBroker.Addr(), mockBroker.BrokerID()).
			SetLeader(mockChannel1.topic(), mockChannel1.partition(), mockBroker.BrokerID()).
			SetLeader(mockChannel2.topic(), mockChannel2.partition(), mockBroker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	t.Run("New", func(t *testing.T) {
		producer, err := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
		require.NoError(t, err, "Failed to create producer with given config:", err)
		producer.Close()
	})

	t.Run("Partitioner", func(t *testing.T) {
		mockBrokerConfig2 := newBrokerConfig(
			mockLocalConfig.General.TLS,
			mockLocalConfig.Kafka.SASLPlain,
			mockLocalConfig.Kafka.Retry,
			mockLocalConfig.Kafka.Version,
			differentPartition)
		producer, _ := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig2)
		defer func() { producer.Close() }()

		for i := 0; i < 10; i++ {
			assignedPartition, _, err := producer.SendMessage(&sarama.ProducerMessage{Topic: mockChannel2.topic()})
			require.NoError(t, err, "Failed to send message:", err)
			require.Equal(t, differentPartition, assignedPartition, "Message wasn't posted to the right partition - expected %d, got %v", differentPartition, assignedPartition)
		}
	})

	producer, _ := sarama.NewSyncProducer([]string{mockBroker.Addr()}, mockBrokerConfig)
	defer func() { producer.Close() }()

	testCases := []struct {
		name string
		size int
		err  error
	}{
		{"TypicalDeploy", 4 * 1024, nil},
		{"TooBig", int(sarama.MaxRequestSize + 1), sarama.ErrMessageSizeTooLarge},
	}

	for _, tc := range testCases {
		t.Run("ProducerMessageMaxBytes"+tc.name, func(t *testing.T) {
			_, _, err := producer.SendMessage(&sarama.ProducerMessage{
				Topic: mockChannel1.topic(),
				Value: sarama.ByteEncoder(make([]byte, tc.size)),
			})
			require.Equal(t, tc.err, err)
		})
	}
}

func TestBrokerConfigTLSConfigEnabled(t *testing.T) {
	publicKey, privateKey, _ := util.GenerateMockPublicPrivateKeyPairPEM(false)
	caPublicKey, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)

	t.Run("Enabled", func(t *testing.T) {
		testBrokerConfig := newBrokerConfig(localconfig.TLS{
			Enabled:     true,
			PrivateKey:  privateKey,
			Certificate: publicKey,
			RootCAs:     []string{caPublicKey},
		},
			mockLocalConfig.Kafka.SASLPlain,
			mockLocalConfig.Kafka.Retry,
			mockLocalConfig.Kafka.Version,
			defaultPartition)

		require.True(t, testBrokerConfig.Net.TLS.Enable)
		require.NotNil(t, testBrokerConfig.Net.TLS.Config)
		require.Len(t, testBrokerConfig.Net.TLS.Config.Certificates, 1)
		require.Len(t, testBrokerConfig.Net.TLS.Config.RootCAs.Subjects(), 1)
		require.Equal(t, uint16(0), testBrokerConfig.Net.TLS.Config.MaxVersion)
		require.Equal(t, uint16(tls.VersionTLS12), testBrokerConfig.Net.TLS.Config.MinVersion)
	})

	t.Run("Disabled", func(t *testing.T) {
		testBrokerConfig := newBrokerConfig(localconfig.TLS{
			Enabled:     false,
			PrivateKey:  privateKey,
			Certificate: publicKey,
			RootCAs:     []string{caPublicKey},
		},
			mockLocalConfig.Kafka.SASLPlain,
			mockLocalConfig.Kafka.Retry,
			mockLocalConfig.Kafka.Version,
			defaultPartition)

		require.False(t, testBrokerConfig.Net.TLS.Enable)
		require.Zero(t, testBrokerConfig.Net.TLS.Config)
	})
}

func TestBrokerConfigTLSConfigBadCert(t *testing.T) {
	publicKey, privateKey, _ := util.GenerateMockPublicPrivateKeyPairPEM(false)
	caPublicKey, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)

	t.Run("BadPrivateKey", func(t *testing.T) {
		require.Panics(t, func() {
			newBrokerConfig(localconfig.TLS{
				Enabled:     true,
				PrivateKey:  privateKey,
				Certificate: "TRASH",
				RootCAs:     []string{caPublicKey},
			},
				mockLocalConfig.Kafka.SASLPlain,
				mockLocalConfig.Kafka.Retry,
				mockLocalConfig.Kafka.Version,
				defaultPartition)
		})
	})
	t.Run("BadPublicKey", func(t *testing.T) {
		require.Panics(t, func() {
			newBrokerConfig(localconfig.TLS{
				Enabled:     true,
				PrivateKey:  "TRASH",
				Certificate: publicKey,
				RootCAs:     []string{caPublicKey},
			},
				mockLocalConfig.Kafka.SASLPlain,
				mockLocalConfig.Kafka.Retry,
				mockLocalConfig.Kafka.Version,
				defaultPartition)
		})
	})
	t.Run("BadRootCAs", func(t *testing.T) {
		require.Panics(t, func() {
			newBrokerConfig(localconfig.TLS{
				Enabled:     true,
				PrivateKey:  privateKey,
				Certificate: publicKey,
				RootCAs:     []string{"TRASH"},
			},
				mockLocalConfig.Kafka.SASLPlain,
				mockLocalConfig.Kafka.Retry,
				mockLocalConfig.Kafka.Version,
				defaultPartition)
		})
	})
}
