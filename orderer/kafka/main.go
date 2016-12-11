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
	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

// New creates a Kafka-backed consenter. Called by orderer's main.go.
func New(kv sarama.KafkaVersion, ro config.Retry) multichain.Consenter {
	return newConsenter(kv, ro, bfValue, pfValue, cfValue)
}

// New calls here because we need to pass additional arguments to
// the constructor and New() should only read from the config file.
func newConsenter(kv sarama.KafkaVersion, ro config.Retry, bf bfType, pf pfType, cf cfType) multichain.Consenter {
	return &consenterImpl{kv, ro, bf, pf, cf}
}

// bfType defines the signature of the broker constructor.
type bfType func([]string, ChainPartition) (Broker, error)

// pfType defines the signature of the producer constructor.
type pfType func([]string, sarama.KafkaVersion, config.Retry) Producer

// cfType defines the signature of the consumer constructor.
type cfType func([]string, sarama.KafkaVersion, ChainPartition, int64) (Consumer, error)

// bfValue holds the value for the broker constructor that's used in the non-test case.
var bfValue = func(brokers []string, cp ChainPartition) (Broker, error) {
	return newBroker(brokers, cp)
}

// pfValue holds the value for the producer constructor that's used in the non-test case.
var pfValue = func(brokers []string, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry) Producer {
	return newProducer(brokers, kafkaVersion, retryOptions)
}

// cfValue holds the value for the consumer constructor that's used in the non-test case.
var cfValue = func(brokers []string, kafkaVersion sarama.KafkaVersion, cp ChainPartition, offset int64) (Consumer, error) {
	return newConsumer(brokers, kafkaVersion, cp, offset)
}

// consenterImpl holds the implementation of type that satisfies the
// multichain.Consenter and testableConsenter interfaces. The former
// is needed because that is what the HandleChain contract requires.
// The latter is needed for testing.
type consenterImpl struct {
	kv sarama.KafkaVersion
	ro config.Retry
	bf bfType
	pf pfType
	cf cfType
}

// HandleChain creates/returns a reference to a Chain for the given set of support resources.
// Implements the multichain.Consenter interface. Called by multichain.newChainSupport(), which
// is itself called by multichain.NewManagerImpl() when ranging over the ledgerFactory's existingChains.
func (co *consenterImpl) HandleChain(cs multichain.ConsenterSupport) (multichain.Chain, error) {
	return newChain(co, cs), nil
}

// When testing we need to inject our own broker/producer/consumer.
// Therefore we need to (a) hold a reference to an object that stores
// the broker/producer/consumer constructors, and (b) refer to that
// object via its interface type, so that we can use a different
// implementation when testing. This, in turn, calls for (c) —- the
// definition of an interface (see testableConsenter below) that will
// be satisfied by both the actual and the mock object and will allow
// us to retrieve these constructors.
func newChain(consenter testableConsenter, support multichain.ConsenterSupport) *chainImpl {
	return &chainImpl{
		consenter:     consenter,
		support:       support,
		partition:     newChainPartition(support.ChainID(), rawPartition),
		lastProcessed: sarama.OffsetOldest - 1, // TODO This should be retrieved by ConsenterSupport; also see note in loop() below
		producer:      consenter.prodFunc()(support.SharedConfig().KafkaBrokers(), consenter.kafkaVersion(), consenter.retryOptions()),
		halted:        false, // Redundant as the default value for booleans is false but added for readability
		exitChan:      make(chan struct{}),
		haltedChan:    make(chan struct{}),
	}
}

// Satisfied by both chainImpl consenterImpl and mockConsenterImpl.
// Defined so as to facilitate testing.
type testableConsenter interface {
	kafkaVersion() sarama.KafkaVersion
	retryOptions() config.Retry
	brokFunc() bfType
	prodFunc() pfType
	consFunc() cfType
}

func (co *consenterImpl) kafkaVersion() sarama.KafkaVersion { return co.kv }
func (co *consenterImpl) retryOptions() config.Retry        { return co.ro }
func (co *consenterImpl) brokFunc() bfType                  { return co.bf }
func (co *consenterImpl) prodFunc() pfType                  { return co.pf }
func (co *consenterImpl) consFunc() cfType                  { return co.cf }

type chainImpl struct {
	consenter testableConsenter
	support   multichain.ConsenterSupport

	partition     ChainPartition
	lastProcessed int64

	producer Producer
	consumer Consumer

	halted   bool          // For the Enqueue() calls
	exitChan chan struct{} // For the Chain's Halt() method

	haltedChan chan struct{} // Hook for testing
}

// Start allocates the necessary resources for staying up to date with this Chain.
// Implements the multichain.Chain interface. Called by multichain.NewManagerImpl()
// which is invoked when the ordering process is launched, before the call to NewServer().
func (ch *chainImpl) Start() {
	// 1. Post the CONNECT message to prevent panicking that occurs
	// when seeking on a partition that hasn't been created yet.
	logger.Debug("Posting the CONNECT message...")
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newConnectMessage())); err != nil {
		logger.Criticalf("Couldn't post CONNECT message to %s: %s", ch.partition, err)
		close(ch.exitChan)
		ch.halted = true
		return
	}

	// 2. Set up the listener/consumer for this partition.
	// TODO When restart support gets added to the common components level, start
	// the consumer from lastProcessed. For now, hard-code to oldest available.
	consumer, err := ch.consenter.consFunc()(ch.support.SharedConfig().KafkaBrokers(), ch.consenter.kafkaVersion(), ch.partition, ch.lastProcessed+1)
	if err != nil {
		logger.Criticalf("Cannot retrieve required offset from Kafka cluster for chain %s: %s", ch.partition, err)
		close(ch.exitChan)
		ch.halted = true
		return
	}
	ch.consumer = consumer

	// 3. Set the loop the keep up to date with the chain.
	go ch.loop()
}

// Halt frees the resources which were allocated for this Chain.
// Implements the multichain.Chain interface.
func (ch *chainImpl) Halt() {
	select {
	case <-ch.exitChan:
		// This construct is useful because it allows Halt() to be
		// called multiple times w/o panicking. Recal that a receive
		// from a closed channel returns (the zero value) immediately.
	default:
		close(ch.exitChan)
	}
}

// Enqueue accepts a message and returns true on acceptance, or false on shutdown.
// Implements the multichain.Chain interface. Called by the drainQueue goroutine,
// which is spawned when the broadcast handler's Handle() function is invoked.
func (ch *chainImpl) Enqueue(env *cb.Envelope) bool {
	if ch.halted {
		return false
	}

	logger.Debug("Enqueueing:", env)
	if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newRegularMessage(utils.MarshalOrPanic(env)))); err != nil {
		logger.Errorf("Couldn't post to %s: %s", ch.partition, err)
		return false
	}

	return !ch.halted // If ch.halted has been set to true while sending, we should return false
}

func (ch *chainImpl) loop() {
	msg := new(ab.KafkaMessage)

	defer close(ch.haltedChan)
	defer ch.producer.Close()
	defer func() { ch.halted = true }()
	defer ch.consumer.Close()

	// TODO Add support for time-based block cutting

	for {
		select {
		case in := <-ch.consumer.Recv():
			logger.Debug("Received:", in)
			if err := proto.Unmarshal(in.Value, msg); err != nil {
				// This shouldn't happen, it should be filtered at ingress
				logger.Critical("Unable to unmarshal consumed message:", err)
			}
			logger.Debug("Unmarshaled to:", msg)
			switch msg.Type.(type) {
			case *ab.KafkaMessage_Connect, *ab.KafkaMessage_TimeToCut:
				logger.Debugf("Ignoring message")
				continue
			case *ab.KafkaMessage_Regular:
				env := new(cb.Envelope)
				if err := proto.Unmarshal(msg.GetRegular().Payload, env); err != nil {
					// This shouldn't happen, it should be filtered at ingress
					logger.Critical("Unable to unmarshal consumed message:", err)
					continue
				}
				batches, committers, ok := ch.support.BlockCutter().Ordered(env)
				logger.Debugf("Ordering results: batches: %v, ok: %v", batches, ok)
				if ok && len(batches) == 0 {
					continue
				}
				// If !ok, batches == nil, so this will be skipped
				for i, batch := range batches {
					ch.support.WriteBlock(batch, nil, committers[i])
				}
			}
		case <-ch.exitChan: // when Halt() is called
			logger.Infof("Consenter for chain %s exiting", ch.partition.Topic())
			return
		}
	}
}

// Closeable allows the shut down of the calling resource.
type Closeable interface {
	Close() error
}
