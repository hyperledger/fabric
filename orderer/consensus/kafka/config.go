/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"crypto/tls"
	"crypto/x509"

	localconfig "github.com/hyperledger/fabric/orderer/common/localconfig"

	"github.com/Shopify/sarama"
)

func newBrokerConfig(
	tlsConfig localconfig.TLS,
	saslPlain localconfig.SASLPlain,
	retryOptions localconfig.Retry,
	kafkaVersion sarama.KafkaVersion,
	chosenStaticPartition int32) *sarama.Config {

	// Max. size for request headers, etc. Set in bytes. Too big on purpose.
	paddingDelta := 1 * 1024 * 1024

	brokerConfig := sarama.NewConfig()

	brokerConfig.Consumer.Retry.Backoff = retryOptions.Consumer.RetryBackoff

	// Allows us to retrieve errors that occur when consuming a channel
	brokerConfig.Consumer.Return.Errors = true

	brokerConfig.Metadata.Retry.Backoff = retryOptions.Metadata.RetryBackoff
	brokerConfig.Metadata.Retry.Max = retryOptions.Metadata.RetryMax

	brokerConfig.Net.DialTimeout = retryOptions.NetworkTimeouts.DialTimeout
	brokerConfig.Net.ReadTimeout = retryOptions.NetworkTimeouts.ReadTimeout
	brokerConfig.Net.WriteTimeout = retryOptions.NetworkTimeouts.WriteTimeout

	brokerConfig.Net.TLS.Enable = tlsConfig.Enabled
	if brokerConfig.Net.TLS.Enable {
		// create public/private key pair structure
		keyPair, err := tls.X509KeyPair([]byte(tlsConfig.Certificate), []byte(tlsConfig.PrivateKey))
		if err != nil {
			logger.Panic("Unable to decode public/private key pair:", err)
		}
		// create root CA pool
		rootCAs := x509.NewCertPool()
		for _, certificate := range tlsConfig.RootCAs {
			if !rootCAs.AppendCertsFromPEM([]byte(certificate)) {
				logger.Panic("Unable to parse the root certificate authority certificates (Kafka.Tls.RootCAs)")
			}
		}
		brokerConfig.Net.TLS.Config = &tls.Config{
			Certificates: []tls.Certificate{keyPair},
			RootCAs:      rootCAs,
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   0, // Latest supported TLS version
		}
	}
	brokerConfig.Net.SASL.Enable = saslPlain.Enabled
	if brokerConfig.Net.SASL.Enable {
		brokerConfig.Net.SASL.User = saslPlain.User
		brokerConfig.Net.SASL.Password = saslPlain.Password
	}

	// Set equivalent of Kafka producer config max.request.bytes to the default
	// value of a Kafka broker's socket.request.max.bytes property (100 MiB).
	brokerConfig.Producer.MaxMessageBytes = int(sarama.MaxRequestSize) - paddingDelta

	brokerConfig.Producer.Retry.Backoff = retryOptions.Producer.RetryBackoff
	brokerConfig.Producer.Retry.Max = retryOptions.Producer.RetryMax

	// A partitioner is actually not needed the way we do things now,
	// but we're adding it now to allow for flexibility in the future.
	brokerConfig.Producer.Partitioner = newStaticPartitioner(chosenStaticPartition)
	// Set the level of acknowledgement reliability needed from the broker.
	// WaitForAll means that the partition leader will wait till all ISRs got
	// the message before sending back an ACK to the sender.
	brokerConfig.Producer.RequiredAcks = sarama.WaitForAll
	// An esoteric setting required by the sarama library, see:
	// https://github.com/Shopify/sarama/issues/816
	brokerConfig.Producer.Return.Successes = true

	brokerConfig.Version = kafkaVersion

	return brokerConfig
}
