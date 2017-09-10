/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	localconfig "github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

// Used for capturing metrics -- see processMessagesToBlocks
const (
	indexRecvError = iota
	indexUnmarshalError
	indexRecvPass
	indexProcessConnectPass
	indexProcessTimeToCutError
	indexProcessTimeToCutPass
	indexProcessRegularError
	indexProcessRegularPass
	indexSendTimeToCutError
	indexSendTimeToCutPass
	indexExitChanPass
)

func newChain(consenter commonConsenter, support multichain.ConsenterSupport, lastOffsetPersisted int64) (*chainImpl, error) {
	lastCutBlockNumber := getLastCutBlockNumber(support.Height())
	logger.Infof("[channel: %s] Starting chain with last persisted offset %d and last recorded block %d",
		support.ChainID(), lastOffsetPersisted, lastCutBlockNumber)

	errorChan := make(chan struct{})
	close(errorChan) // We need this closed when starting up

	return &chainImpl{
		consenter:           consenter,
		support:             support,
		channel:             newChannel(support.ChainID(), defaultPartition),
		lastOffsetPersisted: lastOffsetPersisted,
		lastCutBlockNumber:  lastCutBlockNumber,

		errorChan: errorChan,
		haltChan:  make(chan struct{}),
		startChan: make(chan struct{}),
	}, nil
}

type chainImpl struct {
	consenter commonConsenter
	support   multichain.ConsenterSupport

	channel             channel
	lastOffsetPersisted int64
	lastCutBlockNumber  uint64

	producer        sarama.SyncProducer
	parentConsumer  sarama.Consumer
	channelConsumer sarama.PartitionConsumer

	// When the partition consumer errors, close the channel. Otherwise, make
	// this an open, unbuffered channel.
	errorChan chan struct{}
	// When a Halt() request comes, close the channel. Unlike errorChan, this
	// channel never re-opens when closed. Its closing triggers the exit of the
	// processMessagesToBlock loop.
	haltChan chan struct{}
	// // Close when the retriable steps in Start have completed.
	startChan chan struct{}
}

// Errored returns a channel which will close when a partition consumer error
// has occurred. Checked by Deliver().
func (chain *chainImpl) Errored() <-chan struct{} {
	return chain.errorChan
}

// Start allocates the necessary resources for staying up to date with this
// Chain. Implements the multichain.Chain interface. Called by
// multichain.NewManagerImpl() which is invoked when the ordering process is
// launched, before the call to NewServer(). Launches a goroutine so as not to
// block the multichain.Manager.
func (chain *chainImpl) Start() {
	go startThread(chain)
}

// Halt frees the resources which were allocated for this Chain. Implements the
// multichain.Chain interface.
func (chain *chainImpl) Halt() {
	select {
	case <-chain.haltChan:
		// This construct is useful because it allows Halt() to be called
		// multiple times (by a single thread) w/o panicking. Recal that a
		// receive from a closed channel returns (the zero value) immediately.
		logger.Warningf("[channel: %s] Halting of chain requested again", chain.support.ChainID())
	default:
		logger.Criticalf("[channel: %s] Halting of chain requested", chain.support.ChainID())
		close(chain.haltChan)
		chain.closeKafkaObjects() // Also close the producer and the consumer
		logger.Debugf("[channel: %s] Closed the haltChan", chain.support.ChainID())
	}
}

// Enqueue accepts a message and returns true on acceptance, or false otheriwse.
// Implements the multichain.Chain interface. Called by Broadcast().
func (chain *chainImpl) Enqueue(env *cb.Envelope) bool {
	logger.Debugf("[channel: %s] Enqueueing envelope...", chain.support.ChainID())
	select {
	case <-chain.startChan: // The Start phase has completed
		select {
		case <-chain.haltChan: // The chain has been halted, stop here
			logger.Warningf("[channel: %s] Will not enqueue, consenter for this channel has been halted", chain.support.ChainID())
			return false
		default: // The post path
			marshaledEnv, err := utils.Marshal(env)
			if err != nil {
				logger.Errorf("[channel: %s] cannot enqueue, unable to marshal envelope = %s", chain.support.ChainID(), err)
				return false
			}
			// We're good to go
			payload := utils.MarshalOrPanic(newRegularMessage(marshaledEnv))
			message := newProducerMessage(chain.channel, payload)
			if _, _, err := chain.producer.SendMessage(message); err != nil {
				logger.Errorf("[channel: %s] cannot enqueue envelope = %s", chain.support.ChainID(), err)
				return false
			}
			logger.Debugf("[channel: %s] Envelope enqueued successfully", chain.support.ChainID())
			return true
		}
	default: // Not ready yet
		logger.Warningf("[channel: %s] Will not enqueue, consenter for this channel hasn't started yet", chain.support.ChainID())
		return false
	}
}

// Called by Start().
func startThread(chain *chainImpl) {
	var err error

	// Set up the producer
	chain.producer, err = setupProducerForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.support.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel)
	if err != nil {
		logger.Panicf("[channel: %s] Cannot set up producer = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] Producer set up successfully", chain.support.ChainID())

	// Have the producer post the CONNECT message
	if err = sendConnectMessage(chain.consenter.retryOptions(), chain.haltChan, chain.producer, chain.channel); err != nil {
		logger.Panicf("[channel: %s] Cannot post CONNECT message = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] CONNECT message posted successfully", chain.channel.topic())

	// Set up the parent consumer
	chain.parentConsumer, err = setupParentConsumerForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.support.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel)
	if err != nil {
		logger.Panicf("[channel: %s] Cannot set up parent consumer = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] Parent consumer set up successfully", chain.channel.topic())

	// Set up the channel consumer
	chain.channelConsumer, err = setupChannelConsumerForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.parentConsumer, chain.channel, chain.lastOffsetPersisted+1)
	if err != nil {
		logger.Panicf("[channel: %s] Cannot set up channel consumer = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] Channel consumer set up successfully", chain.channel.topic())

	close(chain.startChan)                // Broadcast requests will now go through
	chain.errorChan = make(chan struct{}) // Deliver requests will also go through

	logger.Infof("[channel: %s] Start phase completed successfully", chain.channel.topic())

	chain.processMessagesToBlocks() // Keep up to date with the channel
}

// processMessagesToBlocks drains the Kafka consumer for the given channel, and
// takes care of converting the stream of ordered messages into blocks for the
// channel's ledger.
func (chain *chainImpl) processMessagesToBlocks() ([]uint64, error) {
	counts := make([]uint64, 11) // For metrics and tests
	msg := new(ab.KafkaMessage)
	var timer <-chan time.Time

	defer func() { // When Halt() is called
		select {
		case <-chain.errorChan: // If already closed, don't do anything
		default:
			close(chain.errorChan)
		}
	}()

	for {
		select {
		case <-chain.haltChan:
			logger.Warningf("[channel: %s] Consenter for channel exiting", chain.support.ChainID())
			counts[indexExitChanPass]++
			return counts, nil
		case kafkaErr := <-chain.channelConsumer.Errors():
			logger.Errorf("[channel: %s] Error during consumption: %s", chain.support.ChainID(), kafkaErr)
			counts[indexRecvError]++
			select {
			case <-chain.errorChan: // If already closed, don't do anything
			default:
				close(chain.errorChan)
			}
			logger.Warningf("[channel: %s] Closed the errorChan", chain.support.ChainID())
			// This covers the edge case where (1) a consumption error has
			// closed the errorChan and thus rendered the chain unavailable to
			// deliver clients, (2) we're already at the newest offset, and (3)
			// there are no new Broadcast requests coming in. In this case,
			// there is no trigger that can recreate the errorChan again and
			// mark the chain as available, so we have to force that trigger via
			// the emission of a CONNECT message. TODO Consider rate limiting
			go sendConnectMessage(chain.consenter.retryOptions(), chain.haltChan, chain.producer, chain.channel)
		case in, ok := <-chain.channelConsumer.Messages():
			if !ok {
				logger.Criticalf("[channel: %s] Kafka consumer closed.", chain.support.ChainID())
				return counts, nil
			}
			select {
			case <-chain.errorChan: // If this channel was closed...
				chain.errorChan = make(chan struct{}) // ...make a new one.
				logger.Infof("[channel: %s] Marked consenter as available again", chain.support.ChainID())
			default:
			}
			if err := proto.Unmarshal(in.Value, msg); err != nil {
				// This shouldn't happen, it should be filtered at ingress
				logger.Criticalf("[channel: %s] Unable to unmarshal consumed message = %s", chain.support.ChainID(), err)
				counts[indexUnmarshalError]++
				continue
			} else {
				logger.Debugf("[channel: %s] Successfully unmarshalled consumed message, offset is %d. Inspecting type...", chain.support.ChainID(), in.Offset)
				counts[indexRecvPass]++
			}
			switch msg.Type.(type) {
			case *ab.KafkaMessage_Connect:
				_ = processConnect(chain.support.ChainID())
				counts[indexProcessConnectPass]++
			case *ab.KafkaMessage_TimeToCut:
				if err := processTimeToCut(msg.GetTimeToCut(), chain.support, &chain.lastCutBlockNumber, &timer, in.Offset); err != nil {
					logger.Warningf("[channel: %s] %s", chain.support.ChainID(), err)
					logger.Criticalf("[channel: %s] Consenter for channel exiting", chain.support.ChainID())
					counts[indexProcessTimeToCutError]++
					return counts, err // TODO Revisit whether we should indeed stop processing the chain at this point
				}
				counts[indexProcessTimeToCutPass]++
			case *ab.KafkaMessage_Regular:
				if err := processRegular(msg.GetRegular(), chain.support, &timer, in.Offset, &chain.lastCutBlockNumber); err != nil {
					logger.Warningf("[channel: %s] Error when processing incoming message of type REGULAR = %s", chain.support.ChainID(), err)
					counts[indexProcessRegularError]++
				} else {
					counts[indexProcessRegularPass]++
				}
			}
		case <-timer:
			if err := sendTimeToCut(chain.producer, chain.channel, chain.lastCutBlockNumber+1, &timer); err != nil {
				logger.Errorf("[channel: %s] cannot post time-to-cut message = %s", chain.support.ChainID(), err)
				// Do not return though
				counts[indexSendTimeToCutError]++
			} else {
				counts[indexSendTimeToCutPass]++
			}
		}
	}
}

func (chain *chainImpl) closeKafkaObjects() []error {
	var errs []error

	err := chain.channelConsumer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close channelConsumer cleanly = %s", chain.support.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the channel consumer", chain.support.ChainID())
	}

	err = chain.parentConsumer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close parentConsumer cleanly = %s", chain.support.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the parent consumer", chain.support.ChainID())
	}

	err = chain.producer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close producer cleanly = %s", chain.support.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the producer", chain.support.ChainID())
	}

	return errs
}

// Helper functions

func getLastCutBlockNumber(blockchainHeight uint64) uint64 {
	return blockchainHeight - 1
}

func getLastOffsetPersisted(metadataValue []byte, chainID string) int64 {
	if metadataValue != nil {
		// Extract orderer-related metadata from the tip of the ledger first
		kafkaMetadata := &ab.KafkaMetadata{}
		if err := proto.Unmarshal(metadataValue, kafkaMetadata); err != nil {
			logger.Panicf("[channel: %s] Ledger may be corrupted:"+
				"cannot unmarshal orderer metadata in most recent block", chainID)
		}
		return kafkaMetadata.LastOffsetPersisted
	}
	return (sarama.OffsetOldest - 1) // default
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

func newProducerMessage(channel channel, pld []byte) *sarama.ProducerMessage {
	return &sarama.ProducerMessage{
		Topic: channel.topic(),
		Key:   sarama.StringEncoder(strconv.Itoa(int(channel.partition()))), // TODO Consider writing an IntEncoder?
		Value: sarama.ByteEncoder(pld),
	}
}

func processConnect(channelName string) error {
	logger.Debugf("[channel: %s] It's a connect message - ignoring", channelName)
	return nil
}

func processRegular(regularMessage *ab.KafkaMessageRegular, support multichain.ConsenterSupport, timer *<-chan time.Time, receivedOffset int64, lastCutBlockNumber *uint64) error {
	env := new(cb.Envelope)
	if err := proto.Unmarshal(regularMessage.Payload, env); err != nil {
		// This shouldn't happen, it should be filtered at ingress
		return fmt.Errorf("unmarshal/%s", err)
	}
	batches, committers, ok, pending := support.BlockCutter().Ordered(env)
	logger.Debugf("[channel: %s] Ordering results: items in batch = %d, ok = %v, pending = %v", support.ChainID(), len(batches), ok, pending)
	if ok && len(batches) == 0 && *timer == nil {
		*timer = time.After(support.SharedConfig().BatchTimeout())
		logger.Debugf("[channel: %s] Just began %s batch timer", support.ChainID(), support.SharedConfig().BatchTimeout().String())
		return nil
	}

	offset := receivedOffset
	if pending || len(batches) == 2 {
		// If the newest envelope is not encapsulated into the first batch,
		// the LastOffsetPersisted of first block should be receivedOffset-1.
		offset--
	}

	// If !ok, batches == nil, so this will be skipped
	for i, batch := range batches {
		block := support.CreateNextBlock(batch)
		encodedLastOffsetPersisted := utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: offset})
		support.WriteBlock(block, committers[i], encodedLastOffsetPersisted)
		*lastCutBlockNumber++
		logger.Debugf("[channel: %s] Batch filled, just cut block %d - last persisted offset is now %d", support.ChainID(), *lastCutBlockNumber, offset)
		offset++
	}

	if len(batches) > 0 {
		*timer = nil
	}
	return nil
}

func processTimeToCut(ttcMessage *ab.KafkaMessageTimeToCut, support multichain.ConsenterSupport, lastCutBlockNumber *uint64, timer *<-chan time.Time, receivedOffset int64) error {
	ttcNumber := ttcMessage.GetBlockNumber()
	logger.Debugf("[channel: %s] It's a time-to-cut message for block %d", support.ChainID(), ttcNumber)
	if ttcNumber == *lastCutBlockNumber+1 {
		*timer = nil
		logger.Debugf("[channel: %s] Nil'd the timer", support.ChainID())
		batch, committers := support.BlockCutter().Cut()
		if len(batch) == 0 {
			return fmt.Errorf("got right time-to-cut message (for block %d),"+
				" no pending requests though; this might indicate a bug", *lastCutBlockNumber+1)
		}
		block := support.CreateNextBlock(batch)
		encodedLastOffsetPersisted := utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: receivedOffset})
		support.WriteBlock(block, committers, encodedLastOffsetPersisted)
		*lastCutBlockNumber++
		logger.Debugf("[channel: %s] Proper time-to-cut received, just cut block %d", support.ChainID(), *lastCutBlockNumber)
		return nil
	} else if ttcNumber > *lastCutBlockNumber+1 {
		return fmt.Errorf("got larger time-to-cut message (%d) than allowed/expected (%d)"+
			" - this might indicate a bug", ttcNumber, *lastCutBlockNumber+1)
	}
	logger.Debugf("[channel: %s] Ignoring stale time-to-cut-message for block %d", support.ChainID(), ttcNumber)
	return nil
}

// Post a CONNECT message to the channel using the given retry options. This
// prevents the panicking that would occur if we were to set up a consumer and
// seek on a partition that hadn't been written to yet.
func sendConnectMessage(retryOptions localconfig.Retry, exitChan chan struct{}, producer sarama.SyncProducer, channel channel) error {
	logger.Infof("[channel: %s] About to post the CONNECT message...", channel.topic())

	payload := utils.MarshalOrPanic(newConnectMessage())
	message := newProducerMessage(channel, payload)

	retryMsg := "Attempting to post the CONNECT message..."
	postConnect := newRetryProcess(retryOptions, exitChan, channel, retryMsg, func() error {
		_, _, err := producer.SendMessage(message)
		return err
	})

	return postConnect.retry()
}

func sendTimeToCut(producer sarama.SyncProducer, channel channel, timeToCutBlockNumber uint64, timer *<-chan time.Time) error {
	logger.Debugf("[channel: %s] Time-to-cut block %d timer expired", channel.topic(), timeToCutBlockNumber)
	*timer = nil
	payload := utils.MarshalOrPanic(newTimeToCutMessage(timeToCutBlockNumber))
	message := newProducerMessage(channel, payload)
	_, _, err := producer.SendMessage(message)
	return err
}

// Sets up the partition consumer for a channel using the given retry options.
func setupChannelConsumerForChannel(retryOptions localconfig.Retry, haltChan chan struct{}, parentConsumer sarama.Consumer, channel channel, startFrom int64) (sarama.PartitionConsumer, error) {
	var err error
	var channelConsumer sarama.PartitionConsumer

	logger.Infof("[channel: %s] Setting up the channel consumer for this channel (start offset: %d)...", channel.topic(), startFrom)

	retryMsg := "Connecting to the Kafka cluster"
	setupChannelConsumer := newRetryProcess(retryOptions, haltChan, channel, retryMsg, func() error {
		channelConsumer, err = parentConsumer.ConsumePartition(channel.topic(), channel.partition(), startFrom)
		return err
	})

	return channelConsumer, setupChannelConsumer.retry()
}

// Sets up the parent consumer for a channel using the given retry options.
func setupParentConsumerForChannel(retryOptions localconfig.Retry, haltChan chan struct{}, brokers []string, brokerConfig *sarama.Config, channel channel) (sarama.Consumer, error) {
	var err error
	var parentConsumer sarama.Consumer

	logger.Infof("[channel: %s] Setting up the parent consumer for this channel...", channel.topic())

	retryMsg := "Connecting to the Kafka cluster"
	setupParentConsumer := newRetryProcess(retryOptions, haltChan, channel, retryMsg, func() error {
		parentConsumer, err = sarama.NewConsumer(brokers, brokerConfig)
		return err
	})

	return parentConsumer, setupParentConsumer.retry()
}

// Sets up the writer/producer for a channel using the given retry options.
func setupProducerForChannel(retryOptions localconfig.Retry, haltChan chan struct{}, brokers []string, brokerConfig *sarama.Config, channel channel) (sarama.SyncProducer, error) {
	var err error
	var producer sarama.SyncProducer

	logger.Infof("[channel: %s] Setting up the producer for this channel...", channel.topic())

	retryMsg := "Connecting to the Kafka cluster"
	setupProducer := newRetryProcess(retryOptions, haltChan, channel, retryMsg, func() error {
		producer, err = sarama.NewSyncProducer(brokers, brokerConfig)
		return err
	})

	return producer, setupProducer.retry()
}
