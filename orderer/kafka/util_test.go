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
	"testing"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/mocks/util"
	"github.com/stretchr/testify/assert"
)

func TestProducerConfigMessageMaxBytes(t *testing.T) {
	broker := sarama.NewMockBroker(t, 1)
	defer func() {
		broker.Close()
	}()
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(cp.Topic(), cp.Partition(), broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	mockTLS := config.TLS{Enabled: false}
	config := newBrokerConfig(testConf.Kafka.Version, rawPartition, mockTLS)
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		producer.Close()
	}()

	testCases := []struct {
		name string
		size int
		err  error
	}{
		{"TypicalDeploy", 8 * 1024 * 1024, nil},
		{"TooBig", 100*1024*1024 + 1, sarama.ErrMessageSizeTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err = producer.SendMessage(&sarama.ProducerMessage{
				Topic: cp.Topic(),
				Value: sarama.ByteEncoder(make([]byte, tc.size)),
			})
			if err != tc.err {
				t.Fatal(err)
			}
		})

	}
}

func TestNewBrokerConfig(t *testing.T) {
	// Use a partition ID that is not the 'default' (rawPartition)
	var differentPartition int32 = 2
	cp = newChainPartition(provisional.TestChainID, differentPartition)

	// Setup a mock broker that reports that it has 3 partitions for the topic
	broker := sarama.NewMockBroker(t, 1)
	defer func() {
		broker.Close()
	}()
	broker.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker.Addr(), broker.BrokerID()).
			SetLeader(cp.Topic(), 0, broker.BrokerID()).
			SetLeader(cp.Topic(), 1, broker.BrokerID()).
			SetLeader(cp.Topic(), 2, broker.BrokerID()),
		"ProduceRequest": sarama.NewMockProduceResponse(t),
	})

	config := newBrokerConfig(testConf.Kafka.Version, differentPartition, config.TLS{Enabled: false})
	producer, err := sarama.NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal("Failed to create producer:", err)
	}
	defer func() {
		producer.Close()
	}()

	for i := 0; i < 10; i++ {
		assignedPartition, _, err := producer.SendMessage(&sarama.ProducerMessage{Topic: cp.Topic()})
		if err != nil {
			t.Fatal("Failed to send message:", err)
		}
		if assignedPartition != differentPartition {
			t.Fatalf("Message wasn't posted to the right partition - expected: %d, got %v", differentPartition, assignedPartition)
		}
	}
}

func TestTLSConfigEnabled(t *testing.T) {
	publicKey, privateKey, err := util.GenerateMockPublicPrivateKeyPairPEM(false)
	if err != nil {
		t.Fatalf("Enable to generate a public/private key pair: %v", err)
	}
	caPublicKey, _, err := util.GenerateMockPublicPrivateKeyPairPEM(true)
	if err != nil {
		t.Fatalf("Enable to generate a signer certificate: %v", err)
	}

	config := newBrokerConfig(testConf.Kafka.Version, 0, config.TLS{
		Enabled:     true,
		PrivateKey:  privateKey,
		Certificate: publicKey,
		RootCAs:     []string{caPublicKey},
	})

	assert.True(t, config.Net.TLS.Enable)
	assert.NotNil(t, config.Net.TLS.Config)
	assert.Len(t, config.Net.TLS.Config.Certificates, 1)
	assert.Len(t, config.Net.TLS.Config.RootCAs.Subjects(), 1)
	assert.Equal(t, uint16(0), config.Net.TLS.Config.MaxVersion)
	assert.Equal(t, uint16(0), config.Net.TLS.Config.MinVersion)
}

func TestTLSConfigDisabled(t *testing.T) {
	publicKey, privateKey, err := util.GenerateMockPublicPrivateKeyPairPEM(false)
	if err != nil {
		t.Fatalf("Enable to generate a public/private key pair: %v", err)
	}
	caPublicKey, _, err := util.GenerateMockPublicPrivateKeyPairPEM(true)
	if err != nil {
		t.Fatalf("Enable to generate a signer certificate: %v", err)
	}

	config := newBrokerConfig(testConf.Kafka.Version, 0, config.TLS{
		Enabled:     false,
		PrivateKey:  privateKey,
		Certificate: publicKey,
		RootCAs:     []string{caPublicKey},
	})

	assert.False(t, config.Net.TLS.Enable)
	assert.Zero(t, config.Net.TLS.Config)

}

func TestTLSConfigBadCert(t *testing.T) {
	publicKey, privateKey, err := util.GenerateMockPublicPrivateKeyPairPEM(false)
	if err != nil {
		t.Fatalf("Enable to generate a public/private key pair: %v", err)
	}
	caPublicKey, _, err := util.GenerateMockPublicPrivateKeyPairPEM(true)
	if err != nil {
		t.Fatalf("Enable to generate a signer certificate: %v", err)
	}

	t.Run("BadPrivateKey", func(t *testing.T) {
		assert.Panics(t, func() {
			newBrokerConfig(testConf.Kafka.Version, 0, config.TLS{
				Enabled:     true,
				PrivateKey:  privateKey,
				Certificate: "TRASH",
				RootCAs:     []string{caPublicKey},
			})
		})
	})
	t.Run("BadPublicKey", func(t *testing.T) {
		assert.Panics(t, func() {
			newBrokerConfig(testConf.Kafka.Version, 0, config.TLS{
				Enabled:     true,
				PrivateKey:  "TRASH",
				Certificate: publicKey,
				RootCAs:     []string{caPublicKey},
			})
		})
	})
	t.Run("BadRootCAs", func(t *testing.T) {
		assert.Panics(t, func() {
			newBrokerConfig(testConf.Kafka.Version, 0, config.TLS{
				Enabled:     true,
				PrivateKey:  privateKey,
				Certificate: publicKey,
				RootCAs:     []string{"TRASH"},
			})
		})
	})
}
