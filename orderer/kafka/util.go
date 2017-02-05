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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strconv"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/localconfig"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// TODO Set the returned config file to more appropriate
// defaults as we're getting closer to a stable release
func newBrokerConfig(kafkaVersion sarama.KafkaVersion, chosenStaticPartition int32, tlsConfig config.TLS) *sarama.Config {
	brokerConfig := sarama.NewConfig()

	brokerConfig.Version = kafkaVersion
	// Set the level of acknowledgement reliability needed from the broker.
	// WaitForAll means that the partition leader will wait till all ISRs
	// got the message before sending back an ACK to the sender.
	brokerConfig.Producer.RequiredAcks = sarama.WaitForAll
	// A partitioner is actually not needed the way we do things now,
	// but we're adding it now to allow for flexibility in the future.
	brokerConfig.Producer.Partitioner = newStaticPartitioner(chosenStaticPartition)
	// Set equivalent of kafka producer config max.request.bytes to the deafult
	// value of a Kafka broker's socket.request.max.bytes property (100 MiB).
	brokerConfig.Producer.MaxMessageBytes = int(sarama.MaxRequestSize)

	brokerConfig.Net.TLS.Enable = tlsConfig.Enabled

	if brokerConfig.Net.TLS.Enable {
		// create public/private key pair structure
		keyPair, err := tls.X509KeyPair([]byte(tlsConfig.Certificate), []byte(tlsConfig.PrivateKey))
		if err != nil {
			panic(fmt.Errorf("Unable to decode public/private key pair. Error: %v", err))
		}

		// create root CA pool
		rootCAs := x509.NewCertPool()
		for _, certificate := range tlsConfig.RootCAs {
			if !rootCAs.AppendCertsFromPEM([]byte(certificate)) {
				panic(fmt.Errorf("Unable to decode certificate. Error: %v", err))
			}
		}

		brokerConfig.Net.TLS.Config = &tls.Config{
			Certificates: []tls.Certificate{keyPair},
			RootCAs:      rootCAs,
			MinVersion:   0, // TLS 1.0 (no SSL support)
			MaxVersion:   0, // Latest supported TLS version
		}
	}

	return brokerConfig
}

func newConnectMessage() *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_Connect{
			Connect: &ab.KafkaMessageConnect{
				Payload: nil,
			},
		},
	}
}

func newRegularMessage(payload []byte) *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_Regular{
			Regular: &ab.KafkaMessageRegular{
				Payload: payload,
			},
		},
	}
}

func newTimeToCutMessage(blockNumber uint64) *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_TimeToCut{
			TimeToCut: &ab.KafkaMessageTimeToCut{
				BlockNumber: blockNumber,
			},
		},
	}
}

func newProducerMessage(cp ChainPartition, payload []byte) *sarama.ProducerMessage {
	return &sarama.ProducerMessage{
		Topic: cp.Topic(),
		Key:   sarama.StringEncoder(strconv.Itoa(int(cp.Partition()))), // TODO Consider writing an IntEncoder?
		Value: sarama.ByteEncoder(payload),
	}
}

func newOffsetReq(cp ChainPartition, offset int64) *sarama.OffsetRequest {
	req := &sarama.OffsetRequest{}
	// If offset (seek) == -1, ask for the offset assigned to next new message.
	// If offset (seek) == -2, ask for the earliest available offset.
	// The last parameter in the AddBlock call is needed for God-knows-why reasons.
	// From the Kafka folks themselves: "We agree that this API is slightly funky."
	// https://mail-archives.apache.org/mod_mbox/kafka-users/201411.mbox/%3Cc159383825e04129b77253ffd6c448aa@BY2PR02MB505.namprd02.prod.outlook.com%3E
	req.AddBlock(cp.Topic(), cp.Partition(), offset, 1)
	return req
}
