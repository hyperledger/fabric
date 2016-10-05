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
	"time"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/config"
)

// Producer allows the caller to send blocks to the orderer
type Producer interface {
	Send(payload []byte) error
	Closeable
}

type producerImpl struct {
	producer sarama.SyncProducer
	topic    string
}

func newProducer(conf *config.TopLevel) Producer {
	brokerConfig := newBrokerConfig(conf)
	var p sarama.SyncProducer
	var err error

	repeatTick := time.NewTicker(conf.Kafka.Retry.Period)
	panicTick := time.NewTicker(conf.Kafka.Retry.Stop)
	defer repeatTick.Stop()
	defer panicTick.Stop()

loop:
	for {
		select {
		case <-panicTick.C:
			panic(fmt.Errorf("Failed to create Kafka producer: %v", err))
		case <-repeatTick.C:
			logger.Debug("Connecting to Kafka brokers:", conf.Kafka.Brokers)
			p, err = sarama.NewSyncProducer(conf.Kafka.Brokers, brokerConfig)
			if err == nil {
				break loop
			}
		}
	}

	logger.Debug("Connected to Kafka brokers")
	return &producerImpl{producer: p, topic: conf.Kafka.Topic}
}

func (p *producerImpl) Close() error {
	return p.producer.Close()
}

func (p *producerImpl) Send(payload []byte) error {
	_, offset, err := p.producer.SendMessage(newMsg(payload, p.topic))
	if err == nil {
		logger.Debugf("Forwarded block %v to ordering service", offset)
	} else {
		logger.Info("Failed to send to Kafka brokers:", err)
	}
	return err
}
