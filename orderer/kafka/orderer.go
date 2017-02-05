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
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

// New creates a Kafka-backed consenter. Called by orderer's main.go.
func New(kv sarama.KafkaVersion, ro config.Retry, tls config.TLS) multichain.Consenter {
	return newConsenter(kv, ro, tls, bfValue, pfValue, cfValue)
}

// New calls here because we need to pass additional arguments to
// the constructor and New() should only read from the config file.
func newConsenter(kv sarama.KafkaVersion, ro config.Retry, tls config.TLS, bf bfType, pf pfType, cf cfType) multichain.Consenter {
	return &consenterImpl{kv, ro, tls, bf, pf, cf}
}

// bfType defines the signature of the broker constructor.
type bfType func([]string, ChainPartition) (Broker, error)

// pfType defines the signature of the producer constructor.
type pfType func([]string, sarama.KafkaVersion, config.Retry, config.TLS) Producer

// cfType defines the signature of the consumer constructor.
type cfType func([]string, sarama.KafkaVersion, config.TLS, ChainPartition, int64) (Consumer, error)

// bfValue holds the value for the broker constructor that's used in the non-test case.
var bfValue = func(brokers []string, cp ChainPartition) (Broker, error) {
	return newBroker(brokers, cp)
}

// pfValue holds the value for the producer constructor that's used in the non-test case.
var pfValue = func(brokers []string, kafkaVersion sarama.KafkaVersion, retryOptions config.Retry, tls config.TLS) Producer {
	return newProducer(brokers, kafkaVersion, retryOptions, tls)
}

// cfValue holds the value for the consumer constructor that's used in the non-test case.
var cfValue = func(brokers []string, kafkaVersion sarama.KafkaVersion, tls config.TLS, cp ChainPartition, offset int64) (Consumer, error) {
	return newConsumer(brokers, kafkaVersion, tls, cp, offset)
}

// consenterImpl holds the implementation of type that satisfies the
// multichain.Consenter and testableConsenter interfaces. The former
// is needed because that is what the HandleChain contract requires.
// The latter is needed for testing.
type consenterImpl struct {
	kv  sarama.KafkaVersion
	ro  config.Retry
	tls config.TLS
	bf  bfType
	pf  pfType
	cf  cfType
}

// HandleChain creates/returns a reference to a Chain for the given set of support resources.
// Implements the multichain.Consenter interface. Called by multichain.newChainSupport(), which
// is itself called by multichain.NewManagerImpl() when ranging over the ledgerFactory's existingChains.
func (co *consenterImpl) HandleChain(cs multichain.ConsenterSupport, metadata *cb.Metadata) (multichain.Chain, error) {
	return newChain(co, cs, getLastOffsetPersisted(metadata)), nil
}

func getLastOffsetPersisted(metadata *cb.Metadata) int64 {
	if metadata.Value != nil {
		// Extract orderer-related metadata from the tip of the ledger first
		kafkaMetadata := &ab.KafkaMetadata{}
		if err := proto.Unmarshal(metadata.Value, kafkaMetadata); err != nil {
			panic("Ledger may be corrupted: cannot unmarshal orderer metadata in most recent block")
		}
		return kafkaMetadata.LastOffsetPersisted
	}
	return (sarama.OffsetOldest - 1) // default
}

// When testing we need to inject our own broker/producer/consumer.
// Therefore we need to (a) hold a reference to an object that stores
// the broker/producer/consumer constructors, and (b) refer to that
// object via its interface type, so that we can use a different
// implementation when testing. This, in turn, calls for (c) —- the
// definition of an interface (see testableConsenter below) that will
// be satisfied by both the actual and the mock object and will allow
// us to retrieve these constructors.
func newChain(consenter testableConsenter, support multichain.ConsenterSupport, lastOffsetPersisted int64) *chainImpl {
	logger.Debug("Starting chain with last persisted offset:", lastOffsetPersisted)
	return &chainImpl{
		consenter:           consenter,
		support:             support,
		partition:           newChainPartition(support.ChainID(), rawPartition),
		batchTimeout:        support.SharedConfig().BatchTimeout(),
		lastOffsetPersisted: lastOffsetPersisted,
		producer:            consenter.prodFunc()(support.SharedConfig().KafkaBrokers(), consenter.kafkaVersion(), consenter.retryOptions(), consenter.tlsConfig()),
		halted:              false, // Redundant as the default value for booleans is false but added for readability
		exitChan:            make(chan struct{}),
		haltedChan:          make(chan struct{}),
		setupChan:           make(chan struct{}),
	}
}

// Satisfied by both chainImpl consenterImpl and mockConsenterImpl.
// Defined so as to facilitate testing.
type testableConsenter interface {
	kafkaVersion() sarama.KafkaVersion
	retryOptions() config.Retry
	tlsConfig() config.TLS
	brokFunc() bfType
	prodFunc() pfType
	consFunc() cfType
}

func (co *consenterImpl) kafkaVersion() sarama.KafkaVersion { return co.kv }
func (co *consenterImpl) retryOptions() config.Retry        { return co.ro }
func (co *consenterImpl) tlsConfig() config.TLS             { return co.tls }
func (co *consenterImpl) brokFunc() bfType                  { return co.bf }
func (co *consenterImpl) prodFunc() pfType                  { return co.pf }
func (co *consenterImpl) consFunc() cfType                  { return co.cf }

type chainImpl struct {
	consenter testableConsenter
	support   multichain.ConsenterSupport

	partition           ChainPartition
	batchTimeout        time.Duration
	lastOffsetPersisted int64
	lastCutBlock        uint64

	producer Producer
	consumer Consumer

	halted   bool          // For the Enqueue() calls
	exitChan chan struct{} // For the Chain's Halt() method

	// Hooks for testing
	haltedChan chan struct{}
	setupChan  chan struct{}
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
	consumer, err := ch.consenter.consFunc()(ch.support.SharedConfig().KafkaBrokers(), ch.consenter.kafkaVersion(), ch.consenter.tlsConfig(), ch.partition, ch.lastOffsetPersisted+1)
	if err != nil {
		logger.Criticalf("Cannot retrieve required offset from Kafka cluster for chain %s: %s", ch.partition, err)
		close(ch.exitChan)
		ch.halted = true
		return
	}
	ch.consumer = consumer
	close(ch.setupChan)

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
	var timer <-chan time.Time
	var ttcNumber uint64
	var encodedLastOffsetPersisted []byte

	defer close(ch.haltedChan)
	defer ch.producer.Close()
	defer func() { ch.halted = true }()
	defer ch.consumer.Close()

	for {
		select {
		case in := <-ch.consumer.Recv():
			if err := proto.Unmarshal(in.Value, msg); err != nil {
				// This shouldn't happen, it should be filtered at ingress
				logger.Critical("Unable to unmarshal consumed message:", err)
			}
			logger.Debug("Received:", msg)
			switch msg.Type.(type) {
			case *ab.KafkaMessage_Connect:
				logger.Debug("It's a connect message - ignoring")
				continue
			case *ab.KafkaMessage_TimeToCut:
				ttcNumber = msg.GetTimeToCut().BlockNumber
				logger.Debug("It's a time-to-cut message for block", ttcNumber)
				if ttcNumber == ch.lastCutBlock+1 {
					timer = nil
					logger.Debug("Nil'd the timer")
					batch, committers := ch.support.BlockCutter().Cut()
					if len(batch) == 0 {
						logger.Warningf("Got right time-to-cut message (%d) but no pending requests - this might indicate a bug", ch.lastCutBlock)
						logger.Infof("Consenter for chain %s exiting", ch.partition.Topic())
						return
					}
					block := ch.support.CreateNextBlock(batch)
					encodedLastOffsetPersisted = utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: in.Offset})
					ch.support.WriteBlock(block, committers, encodedLastOffsetPersisted)
					ch.lastCutBlock++
					logger.Debug("Proper time-to-cut received, just cut block", ch.lastCutBlock)
					continue
				} else if ttcNumber > ch.lastCutBlock+1 {
					logger.Warningf("Got larger time-to-cut message (%d) than allowed (%d) - this might indicate a bug", ttcNumber, ch.lastCutBlock+1)
					logger.Infof("Consenter for chain %s exiting", ch.partition.Topic())
					return
				}
				logger.Debug("Ignoring stale time-to-cut-message for", ch.lastCutBlock)
			case *ab.KafkaMessage_Regular:
				env := new(cb.Envelope)
				if err := proto.Unmarshal(msg.GetRegular().Payload, env); err != nil {
					// This shouldn't happen, it should be filtered at ingress
					logger.Critical("Unable to unmarshal consumed regular message:", err)
					continue
				}
				batches, committers, ok := ch.support.BlockCutter().Ordered(env)
				logger.Debugf("Ordering results: batches: %v, ok: %v", batches, ok)
				if ok && len(batches) == 0 && timer == nil {
					timer = time.After(ch.batchTimeout)
					logger.Debugf("Just began %s batch timer", ch.batchTimeout.String())
					continue
				}
				// If !ok, batches == nil, so this will be skipped
				for i, batch := range batches {
					block := ch.support.CreateNextBlock(batch)
					encodedLastOffsetPersisted = utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: in.Offset})
					ch.support.WriteBlock(block, committers[i], encodedLastOffsetPersisted)
					ch.lastCutBlock++
					logger.Debug("Batch filled, just cut block", ch.lastCutBlock)
				}
				if len(batches) > 0 {
					timer = nil
				}
			}
		case <-timer:
			logger.Debugf("Time-to-cut block %d timer expired", ch.lastCutBlock+1)
			timer = nil
			if err := ch.producer.Send(ch.partition, utils.MarshalOrPanic(newTimeToCutMessage(ch.lastCutBlock+1))); err != nil {
				logger.Errorf("Couldn't post to %s: %s", ch.partition, err)
				// Do not exit
			}
		case <-ch.exitChan: // When Halt() is called
			logger.Infof("Consenter for chain %s exiting", ch.partition.Topic())
			return
		}
	}
}

// Closeable allows the shut down of the calling resource.
type Closeable interface {
	Close() error
}
