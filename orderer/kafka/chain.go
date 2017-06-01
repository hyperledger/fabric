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
	indexRecvPass
	indexProcessConnectPass
	indexProcessTimeToCutError
	indexProcessTimeToCutPass
	indexPprocessRegularError
	indexProcessRegularPass
	indexSendTimeToCutError
	indexSendTimeToCutPass
	indexExitChanPass
)

func newChain(consenter commonConsenter, support multichain.ConsenterSupport, lastOffsetPersisted int64) (*chainImpl, error) {
	lastCutBlockNumber := getLastCutBlockNumber(support.Height())
	logger.Infof("[channel: %s] Starting chain with last persisted offset %d and last recorded block %d",
		support.ChainID(), lastOffsetPersisted, lastCutBlockNumber)

	return &chainImpl{
		consenter:           consenter,
		support:             support,
		channel:             newChannel(support.ChainID(), defaultPartition),
		lastOffsetPersisted: lastOffsetPersisted,
		lastCutBlockNumber:  lastCutBlockNumber,
		halted:              false, // Redundant as the default value for booleans is false but added for readability
		exitChan:            make(chan struct{}),
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

	halted   bool          // For the Enqueue() calls
	exitChan chan struct{} // For the Chain's Halt() method

	startCompleted bool // For testing
}

// Start allocates the necessary resources for staying up to date with this
// Chain. Implements the multichain.Chain interface. Called by
// multichain.NewManagerImpl() which is invoked when the ordering process is
// launched, before the call to NewServer().
func (chain *chainImpl) Start() {
	var err error

	// Set up the producer
	chain.producer, err = setupProducerForChannel(chain.support.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel, chain.consenter.retryOptions())
	if err != nil {
		logger.Criticalf("[channel: %s] Cannot set up producer = %s", chain.channel.topic(), err)
		close(chain.exitChan)
		chain.halted = true
		return
	}
	logger.Infof("[channel: %s] Producer set up successfully", chain.support.ChainID())

	// Have the producer post the CONNECT message
	if err = sendConnectMessage(chain.producer, chain.channel); err != nil {
		logger.Criticalf("[channel: %s] Cannot post CONNECT message = %s", chain.channel.topic(), err)
		close(chain.exitChan)
		chain.halted = true
		chain.producer.Close()
		return
	}
	logger.Infof("[channel: %s] CONNECT message posted successfully", chain.channel.topic())

	// Set up the consumer
	chain.parentConsumer, chain.channelConsumer, err = setupConsumerForChannel(chain.support.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel, chain.lastOffsetPersisted+1)
	if err != nil {
		logger.Criticalf("[channel: %s] Cannot set up consumer = %s", chain.channel.topic(), err)
		close(chain.exitChan)
		chain.halted = true
		chain.producer.Close()
		return
	}
	logger.Infof("[channel: %s] Consumer set up successfully", chain.channel.topic())
	go listenForErrors(chain.channelConsumer.Errors(), chain.exitChan)

	// Keep up to date with the channel
	go processMessagesToBlock(chain.support, chain.producer, chain.parentConsumer, chain.channelConsumer,
		chain.channel, &chain.lastCutBlockNumber, &chain.halted, &chain.exitChan)

	chain.startCompleted = true
}

// Halt frees the resources which were allocated for this Chain. Implements the
// multichain.Chain interface.
func (chain *chainImpl) Halt() {
	select {
	case <-chain.exitChan:
		// This construct is useful because it allows Halt() to be called
		// multiple times w/o panicking. Recal that a receive from a closed
		// channel returns (the zero value) immediately.
		logger.Warningf("[channel: %s] Halting of chain requested again", chain.support.ChainID())
	default:
		logger.Criticalf("[channel: %s] Halting of chain requested", chain.support.ChainID())
		close(chain.exitChan)
	}
}

// Enqueue accepts a message and returns true on acceptance, or false on
// shutdown. Implements the multichain.Chain interface. Called by Broadcast.
func (chain *chainImpl) Enqueue(env *cb.Envelope) bool {
	if chain.halted {
		logger.Warningf("[channel: %s] Will not enqueue cause the chain has been halted", chain.support.ChainID())
		return false
	}

	logger.Debugf("[channel: %s] Enqueueing envelope...", chain.support.ChainID())
	marshaledEnv, err := utils.Marshal(env)
	if err != nil {
		return false
	}
	payload := utils.MarshalOrPanic(newRegularMessage(marshaledEnv))
	message := newProducerMessage(chain.channel, payload)
	if _, _, err := chain.producer.SendMessage(message); err != nil {
		logger.Errorf("[channel: %s] cannot enqueue envelope = %s", chain.support.ChainID(), err)
		return false
	}
	logger.Debugf("[channel: %s] Envelope enqueued successfully", chain.support.ChainID())

	return !chain.halted // If ch.halted has been set to true while sending, we should return false
}

// processMessagesToBlocks drains the Kafka consumer for the given channel, and
// takes care of converting the stream of ordered messages into blocks for the
// channel's ledger. NOTE: May need to rethink the model here, and turn this
// into a method. For the time being, we optimize for testability.
func processMessagesToBlock(support multichain.ConsenterSupport, producer sarama.SyncProducer,
	parentConsumer sarama.Consumer, channelConsumer sarama.PartitionConsumer,
	chn channel, lastCutBlockNumber *uint64, haltedFlag *bool, exitChan *chan struct{}) ([]uint64, error) {
	msg := new(ab.KafkaMessage)
	var timer <-chan time.Time

	counts := make([]uint64, 10) // For metrics and tests

	defer func() {
		_ = closeLoop(chn.topic(), producer, parentConsumer, channelConsumer, haltedFlag)
		logger.Infof("[channel: %s] Closed producer/consumer threads for channel and exiting loop", chn.topic())
	}()

	for {
		select {
		case in := <-channelConsumer.Messages():
			if err := proto.Unmarshal(in.Value, msg); err != nil {
				// This shouldn't happen, it should be filtered at ingress
				logger.Criticalf("[channel: %s] Unable to unmarshal consumed message = %s", chn.topic(), err)
				counts[indexRecvError]++
			} else {
				logger.Debugf("[channel: %s] Successfully unmarshalled consumed message, offset is %d. Inspecting type...", chn.topic(), in.Offset)
				counts[indexRecvPass]++
			}
			switch msg.Type.(type) {
			case *ab.KafkaMessage_Connect:
				_ = processConnect(chn.topic())
				counts[indexProcessConnectPass]++
			case *ab.KafkaMessage_TimeToCut:
				if err := processTimeToCut(msg.GetTimeToCut(), support, lastCutBlockNumber, &timer, in.Offset); err != nil {
					logger.Warningf("[channel: %s] %s", chn.topic(), err)
					logger.Criticalf("[channel: %s] Consenter for channel exiting", chn.topic())
					counts[indexProcessTimeToCutError]++
					return counts, err // TODO Revisit whether we should indeed stop processing the chain at this point
				}
				counts[indexProcessTimeToCutPass]++
			case *ab.KafkaMessage_Regular:
				if err := processRegular(msg.GetRegular(), support, &timer, in.Offset, lastCutBlockNumber); err != nil {
					logger.Warningf("[channel: %s] Error when processing incoming message of type REGULAR = %s", chn.topic(), err)
					counts[indexPprocessRegularError]++
				} else {
					counts[indexProcessRegularPass]++
				}
			}
		case <-timer:
			if err := sendTimeToCut(producer, chn, (*lastCutBlockNumber)+1, &timer); err != nil {
				logger.Errorf("[channel: %s] cannot post time-to-cut message = %s", chn.topic(), err)
				// Do not return though
				counts[indexSendTimeToCutError]++
			} else {
				counts[indexSendTimeToCutPass]++
			}
		case <-*exitChan: // When Halt() is called
			logger.Warningf("[channel: %s] Consenter for channel exiting", chn.topic())
			counts[indexExitChanPass]++
			return counts, nil
		}
	}
}

// Helper functions

func closeLoop(channelName string, producer sarama.SyncProducer, parentConsumer sarama.Consumer, channelConsumer sarama.PartitionConsumer, haltedFlag *bool) []error {
	var errs []error

	*haltedFlag = true

	err := channelConsumer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close channelConsumer cleanly = %s", channelName, err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the channel consumer", channelName)
	}

	err = parentConsumer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close parentConsumer cleanly = %s", channelName, err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the parent consumer", channelName)
	}

	err = producer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close producer cleanly = %s", channelName, err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the producer", channelName)
	}

	return errs
}

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

func listenForErrors(errChan <-chan *sarama.ConsumerError, exitChan <-chan struct{}) error {
	select {
	case <-exitChan:
		return nil
	case err := <-errChan:
		logger.Error(err)
		return err
	}
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

func newProducerMessage(chn channel, pld []byte) *sarama.ProducerMessage {
	return &sarama.ProducerMessage{
		Topic: chn.topic(),
		Key:   sarama.StringEncoder(strconv.Itoa(int(chn.partition()))), // TODO Consider writing an IntEncoder?
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
	batches, committers, ok := support.BlockCutter().Ordered(env)
	logger.Debugf("[channel: %s] Ordering results: items in batch = %d, ok = %v", support.ChainID(), len(batches), ok)
	if ok && len(batches) == 0 && *timer == nil {
		*timer = time.After(support.SharedConfig().BatchTimeout())
		logger.Debugf("[channel: %s] Just began %s batch timer", support.ChainID(), support.SharedConfig().BatchTimeout().String())
		return nil
	}
	// If !ok, batches == nil, so this will be skipped
	for i, batch := range batches {
		// If more than one batch is produced, exactly 2 batches are produced.
		// The receivedOffset for the first batch is one less than the supplied
		// offset to this function.
		offset := receivedOffset - int64(len(batches)-i-1)
		block := support.CreateNextBlock(batch)
		encodedLastOffsetPersisted := utils.MarshalOrPanic(&ab.KafkaMetadata{LastOffsetPersisted: offset})
		support.WriteBlock(block, committers[i], encodedLastOffsetPersisted)
		*lastCutBlockNumber++
		logger.Debugf("[channel: %s] Batch filled, just cut block %d - last persisted offset is now %d", support.ChainID(), *lastCutBlockNumber, offset)
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

// Post a CONNECT message to the channel. This prevents the panicking that would
// occur if we were to set up a consumer and seek on a partition that hadn't
// been written to yet.
func sendConnectMessage(producer sarama.SyncProducer, channel channel) error {
	logger.Infof("[channel: %s] Posting the CONNECT message...", channel.topic())
	payload := utils.MarshalOrPanic(newConnectMessage())
	message := newProducerMessage(channel, payload)
	_, _, err := producer.SendMessage(message)
	return err
}

func sendTimeToCut(producer sarama.SyncProducer, channel channel, timeToCutBlockNumber uint64, timer *<-chan time.Time) error {
	logger.Debugf("[channel: %s] Time-to-cut block %d timer expired", channel.topic(), timeToCutBlockNumber)
	*timer = nil
	payload := utils.MarshalOrPanic(newTimeToCutMessage(timeToCutBlockNumber))
	message := newProducerMessage(channel, payload)
	_, _, err := producer.SendMessage(message)
	return err
}

// Sets up the listener/consumer for a channel.
func setupConsumerForChannel(brokers []string, brokerConfig *sarama.Config, channel channel, startFrom int64) (sarama.Consumer, sarama.PartitionConsumer, error) {
	logger.Infof("[channel: %s] Setting up the consumer for this channel...", channel.topic())

	parentConsumer, err := sarama.NewConsumer(brokers, brokerConfig)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("[channel: %s] Created new parent consumer", channel.topic())

	channelConsumer, err := parentConsumer.ConsumePartition(channel.topic(), channel.partition(), startFrom)
	if err != nil {
		_ = parentConsumer.Close()
		return nil, nil, err
	}
	logger.Debugf("[channel: %s] Created new channel consumer", channel.topic())

	return parentConsumer, channelConsumer, nil
}

// Sets up the writer/producer for a channel.
func setupProducerForChannel(brokers []string, brokerConfig *sarama.Config, channel channel, retryOptions localconfig.Retry) (sarama.SyncProducer, error) {
	var err error
	var producer sarama.SyncProducer

	// This will be revised in: https://jira.hyperledger.org/browse/FAB-4136
	repeatTick := time.NewTicker(retryOptions.Period)
	panicTick := time.NewTicker(retryOptions.Stop)
	logger.Debugf("[channel: %s] Retrying every %s for a total of %s", channel.topic(), retryOptions.Period.String(), retryOptions.Stop.String())
	defer repeatTick.Stop()
	defer panicTick.Stop()

loop:
	for {
		select {
		case <-panicTick.C:
			return nil, err
		case <-repeatTick.C:
			logger.Debugf("[channel: %s] Connecting to Kafka cluster: %s", channel.topic(), brokers)
			if producer, err = sarama.NewSyncProducer(brokers, brokerConfig); err == nil {
				break loop
			}
		}
	}

	return producer, err
}
