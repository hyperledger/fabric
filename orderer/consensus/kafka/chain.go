/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
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

func newChain(
	consenter commonConsenter,
	support consensus.ConsenterSupport,
	lastOffsetPersisted int64,
	lastOriginalOffsetProcessed int64,
	lastResubmittedConfigOffset int64,
) (*chainImpl, error) {
	lastCutBlockNumber := getLastCutBlockNumber(support.Height())
	logger.Infof("[channel: %s] Starting chain with last persisted offset %d and last recorded block [%d]",
		support.ChainID(), lastOffsetPersisted, lastCutBlockNumber)

	doneReprocessingMsgInFlight := make(chan struct{})
	// In either one of following cases, we should unblock ingress messages:
	// - lastResubmittedConfigOffset == 0, where we've never resubmitted any config messages
	// - lastResubmittedConfigOffset == lastOriginalOffsetProcessed, where the latest config message we resubmitted
	//   has been processed already
	// - lastResubmittedConfigOffset < lastOriginalOffsetProcessed, where we've processed one or more resubmitted
	//   normal messages after the latest resubmitted config message. (we advance `lastResubmittedConfigOffset` for
	//   config messages, but not normal messages)
	if lastResubmittedConfigOffset == 0 || lastResubmittedConfigOffset <= lastOriginalOffsetProcessed {
		// If we've already caught up with the reprocessing resubmitted messages, close the channel to unblock broadcast
		close(doneReprocessingMsgInFlight)
	}

	consenter.Metrics().LastOffsetPersisted.With("channel", support.ChainID()).Set(float64(lastOffsetPersisted))

	return &chainImpl{
		consenter:                   consenter,
		ConsenterSupport:            support,
		channel:                     newChannel(support.ChainID(), defaultPartition),
		lastOffsetPersisted:         lastOffsetPersisted,
		lastOriginalOffsetProcessed: lastOriginalOffsetProcessed,
		lastResubmittedConfigOffset: lastResubmittedConfigOffset,
		lastCutBlockNumber:          lastCutBlockNumber,

		haltChan:                    make(chan struct{}),
		startChan:                   make(chan struct{}),
		doneReprocessingMsgInFlight: doneReprocessingMsgInFlight,
	}, nil
}

//go:generate counterfeiter -o mock/sync_producer.go --fake-name SyncProducer . syncProducer

type syncProducer interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
	SendMessages(msgs []*sarama.ProducerMessage) error
	Close() error
}

type chainImpl struct {
	consenter commonConsenter
	consensus.ConsenterSupport

	channel                     channel
	lastOffsetPersisted         int64
	lastOriginalOffsetProcessed int64
	lastResubmittedConfigOffset int64
	lastCutBlockNumber          uint64

	producer        syncProducer
	parentConsumer  sarama.Consumer
	channelConsumer sarama.PartitionConsumer

	// mutex used when changing the doneReprocessingMsgInFlight
	doneReprocessingMutex sync.Mutex
	// notification that there are in-flight messages need to wait for
	doneReprocessingMsgInFlight chan struct{}

	// When the partition consumer errors, close the channel. Otherwise, make
	// this an open, unbuffered channel.
	errorChan chan struct{}
	// When a Halt() request comes, close the channel. Unlike errorChan, this
	// channel never re-opens when closed. Its closing triggers the exit of the
	// processMessagesToBlock loop.
	haltChan chan struct{}
	// notification that the chain has stopped processing messages into blocks
	doneProcessingMessagesToBlocks chan struct{}
	// Close when the retriable steps in Start have completed.
	startChan chan struct{}
	// timer controls the batch timeout of cutting pending messages into block
	timer <-chan time.Time

	replicaIDs []int32
}

// Errored returns a channel which will close when a partition consumer error
// has occurred. Checked by Deliver().
func (chain *chainImpl) Errored() <-chan struct{} {
	select {
	case <-chain.startChan:
		return chain.errorChan
	default:
		// While the consenter is starting, always return an error
		dummyError := make(chan struct{})
		close(dummyError)
		return dummyError
	}
}

// Start allocates the necessary resources for staying up to date with this
// Chain. Implements the consensus.Chain interface. Called by
// consensus.NewManagerImpl() which is invoked when the ordering process is
// launched, before the call to NewServer(). Launches a goroutine so as not to
// block the consensus.Manager.
func (chain *chainImpl) Start() {
	go startThread(chain)
}

// Halt frees the resources which were allocated for this Chain. Implements the
// consensus.Chain interface.
func (chain *chainImpl) Halt() {
	select {
	case <-chain.startChan:
		// chain finished starting, so we can halt it
		select {
		case <-chain.haltChan:
			// This construct is useful because it allows Halt() to be called
			// multiple times (by a single thread) w/o panicking. Recal that a
			// receive from a closed channel returns (the zero value) immediately.
			logger.Warningf("[channel: %s] Halting of chain requested again", chain.ChainID())
		default:
			logger.Criticalf("[channel: %s] Halting of chain requested", chain.ChainID())
			// stat shutdown of chain
			close(chain.haltChan)
			// wait for processing of messages to blocks to finish shutting down
			<-chain.doneProcessingMessagesToBlocks
			// close the kafka producer and the consumer
			chain.closeKafkaObjects()
			logger.Debugf("[channel: %s] Closed the haltChan", chain.ChainID())
		}
	default:
		logger.Warningf("[channel: %s] Waiting for chain to finish starting before halting", chain.ChainID())
		<-chain.startChan
		chain.Halt()
	}
}

func (chain *chainImpl) WaitReady() error {
	select {
	case <-chain.startChan: // The Start phase has completed
		select {
		case <-chain.haltChan: // The chain has been halted, stop here
			return fmt.Errorf("consenter for this channel has been halted")
		case <-chain.doneReprocessing(): // Block waiting for all re-submitted messages to be reprocessed
			return nil
		}
	default: // Not ready yet
		return fmt.Errorf("backing Kafka cluster has not completed booting; try again later")
	}
}

func (chain *chainImpl) doneReprocessing() <-chan struct{} {
	chain.doneReprocessingMutex.Lock()
	defer chain.doneReprocessingMutex.Unlock()
	return chain.doneReprocessingMsgInFlight
}

func (chain *chainImpl) reprocessConfigComplete() {
	chain.doneReprocessingMutex.Lock()
	defer chain.doneReprocessingMutex.Unlock()
	close(chain.doneReprocessingMsgInFlight)
}

func (chain *chainImpl) reprocessConfigPending() {
	chain.doneReprocessingMutex.Lock()
	defer chain.doneReprocessingMutex.Unlock()
	chain.doneReprocessingMsgInFlight = make(chan struct{})
}

// Implements the consensus.Chain interface. Called by Broadcast().
func (chain *chainImpl) Order(env *cb.Envelope, configSeq uint64) error {
	return chain.order(env, configSeq, int64(0))
}

func (chain *chainImpl) order(env *cb.Envelope, configSeq uint64, originalOffset int64) error {
	marshaledEnv, err := utils.Marshal(env)
	if err != nil {
		return errors.Errorf("cannot enqueue, unable to marshal envelope: %s", err)
	}
	if !chain.enqueue(newNormalMessage(marshaledEnv, configSeq, originalOffset)) {
		return errors.Errorf("cannot enqueue")
	}
	return nil
}

// Implements the consensus.Chain interface. Called by Broadcast().
func (chain *chainImpl) Configure(config *cb.Envelope, configSeq uint64) error {
	return chain.configure(config, configSeq, int64(0))
}

func (chain *chainImpl) configure(config *cb.Envelope, configSeq uint64, originalOffset int64) error {
	marshaledConfig, err := utils.Marshal(config)
	if err != nil {
		return fmt.Errorf("cannot enqueue, unable to marshal config because %s", err)
	}
	if !chain.enqueue(newConfigMessage(marshaledConfig, configSeq, originalOffset)) {
		return fmt.Errorf("cannot enqueue")
	}
	return nil
}

// enqueue accepts a message and returns true on acceptance, or false otherwise.
func (chain *chainImpl) enqueue(kafkaMsg *ab.KafkaMessage) bool {
	logger.Debugf("[channel: %s] Enqueueing envelope...", chain.ChainID())
	select {
	case <-chain.startChan: // The Start phase has completed
		select {
		case <-chain.haltChan: // The chain has been halted, stop here
			logger.Warningf("[channel: %s] consenter for this channel has been halted", chain.ChainID())
			return false
		default: // The post path
			payload, err := utils.Marshal(kafkaMsg)
			if err != nil {
				logger.Errorf("[channel: %s] unable to marshal Kafka message because = %s", chain.ChainID(), err)
				return false
			}
			message := newProducerMessage(chain.channel, payload)
			if _, _, err = chain.producer.SendMessage(message); err != nil {
				logger.Errorf("[channel: %s] cannot enqueue envelope because = %s", chain.ChainID(), err)
				return false
			}
			logger.Debugf("[channel: %s] Envelope enqueued successfully", chain.ChainID())
			return true
		}
	default: // Not ready yet
		logger.Warningf("[channel: %s] Will not enqueue, consenter for this channel hasn't started yet", chain.ChainID())
		return false
	}
}

func (chain *chainImpl) HealthCheck(ctx context.Context) error {
	var err error

	payload := utils.MarshalOrPanic(newConnectMessage())
	message := newProducerMessage(chain.channel, payload)

	_, _, err = chain.producer.SendMessage(message)
	if err != nil {
		logger.Warnf("[channel %s] Cannot post CONNECT message = %s", chain.channel.topic(), err)
		if err == sarama.ErrNotEnoughReplicas {
			errMsg := fmt.Sprintf("[replica ids: %d]", chain.replicaIDs)
			return errors.WithMessage(err, errMsg)
		}
	}
	return nil
}

// Called by Start().
func startThread(chain *chainImpl) {
	var err error

	// Create topic if it does not exist (requires Kafka v0.10.1.0)
	err = setupTopicForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.consenter.topicDetail(), chain.channel)
	if err != nil {
		// log for now and fallback to auto create topics setting for broker
		logger.Infof("[channel: %s]: failed to create Kafka topic = %s", chain.channel.topic(), err)
	}

	// Set up the producer
	chain.producer, err = setupProducerForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel)
	if err != nil {
		logger.Panicf("[channel: %s] Cannot set up producer = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] Producer set up successfully", chain.ChainID())

	// Have the producer post the CONNECT message
	if err = sendConnectMessage(chain.consenter.retryOptions(), chain.haltChan, chain.producer, chain.channel); err != nil {
		logger.Panicf("[channel: %s] Cannot post CONNECT message = %s", chain.channel.topic(), err)
	}
	logger.Infof("[channel: %s] CONNECT message posted successfully", chain.channel.topic())

	// Set up the parent consumer
	chain.parentConsumer, err = setupParentConsumerForChannel(chain.consenter.retryOptions(), chain.haltChan, chain.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel)
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

	chain.replicaIDs, err = getHealthyClusterReplicaInfo(chain.consenter.retryOptions(), chain.haltChan, chain.SharedConfig().KafkaBrokers(), chain.consenter.brokerConfig(), chain.channel)
	if err != nil {
		logger.Panicf("[channel: %s] failed to get replica IDs = %s", chain.channel.topic(), err)
	}

	chain.doneProcessingMessagesToBlocks = make(chan struct{})

	chain.errorChan = make(chan struct{}) // Deliver requests will also go through
	close(chain.startChan)                // Broadcast requests will now go through

	logger.Infof("[channel: %s] Start phase completed successfully", chain.channel.topic())

	chain.processMessagesToBlocks() // Keep up to date with the channel
}

// processMessagesToBlocks drains the Kafka consumer for the given channel, and
// takes care of converting the stream of ordered messages into blocks for the
// channel's ledger.
func (chain *chainImpl) processMessagesToBlocks() ([]uint64, error) {
	counts := make([]uint64, 11) // For metrics and tests
	msg := new(ab.KafkaMessage)

	defer func() {
		// notify that we are not processing messages to blocks
		close(chain.doneProcessingMessagesToBlocks)
	}()

	defer func() { // When Halt() is called
		select {
		case <-chain.errorChan: // If already closed, don't do anything
		default:
			close(chain.errorChan)
		}
	}()

	subscription := fmt.Sprintf("added subscription to %s/%d", chain.channel.topic(), chain.channel.partition())
	var topicPartitionSubscriptionResumed <-chan string
	var deliverSessionTimer *time.Timer
	var deliverSessionTimedOut <-chan time.Time

	for {
		select {
		case <-chain.haltChan:
			logger.Warningf("[channel: %s] Consenter for channel exiting", chain.ChainID())
			counts[indexExitChanPass]++
			return counts, nil
		case kafkaErr := <-chain.channelConsumer.Errors():
			logger.Errorf("[channel: %s] Error during consumption: %s", chain.ChainID(), kafkaErr)
			counts[indexRecvError]++
			select {
			case <-chain.errorChan: // If already closed, don't do anything
			default:

				switch kafkaErr.Err {
				case sarama.ErrOffsetOutOfRange:
					// the kafka consumer will auto retry for all errors except for ErrOffsetOutOfRange
					logger.Errorf("[channel: %s] Unrecoverable error during consumption: %s", chain.ChainID(), kafkaErr)
					close(chain.errorChan)
				default:
					if topicPartitionSubscriptionResumed == nil {
						// register listener
						topicPartitionSubscriptionResumed = saramaLogger.NewListener(subscription)
						// start session timout timer
						deliverSessionTimer = time.NewTimer(chain.consenter.retryOptions().NetworkTimeouts.ReadTimeout)
						deliverSessionTimedOut = deliverSessionTimer.C
					}
				}
			}
			select {
			case <-chain.errorChan: // we are not ignoring the error
				logger.Warningf("[channel: %s] Closed the errorChan", chain.ChainID())
				// This covers the edge case where (1) a consumption error has
				// closed the errorChan and thus rendered the chain unavailable to
				// deliver clients, (2) we're already at the newest offset, and (3)
				// there are no new Broadcast requests coming in. In this case,
				// there is no trigger that can recreate the errorChan again and
				// mark the chain as available, so we have to force that trigger via
				// the emission of a CONNECT message. TODO Consider rate limiting
				go sendConnectMessage(chain.consenter.retryOptions(), chain.haltChan, chain.producer, chain.channel)
			default: // we are ignoring the error
				logger.Warningf("[channel: %s] Deliver sessions will be dropped if consumption errors continue.", chain.ChainID())
			}
		case <-topicPartitionSubscriptionResumed:
			// stop listening for subscription message
			saramaLogger.RemoveListener(subscription, topicPartitionSubscriptionResumed)
			// disable subscription event chan
			topicPartitionSubscriptionResumed = nil

			// stop timeout timer
			if !deliverSessionTimer.Stop() {
				<-deliverSessionTimer.C
			}
			logger.Warningf("[channel: %s] Consumption will resume.", chain.ChainID())

		case <-deliverSessionTimedOut:
			// stop listening for subscription message
			saramaLogger.RemoveListener(subscription, topicPartitionSubscriptionResumed)
			// disable subscription event chan
			topicPartitionSubscriptionResumed = nil

			close(chain.errorChan)
			logger.Warningf("[channel: %s] Closed the errorChan", chain.ChainID())

			// make chain available again via CONNECT message trigger
			go sendConnectMessage(chain.consenter.retryOptions(), chain.haltChan, chain.producer, chain.channel)

		case in, ok := <-chain.channelConsumer.Messages():
			if !ok {
				logger.Criticalf("[channel: %s] Kafka consumer closed.", chain.ChainID())
				return counts, nil
			}

			// catch the possibility that we missed a topic subscription event before
			// we registered the event listener
			if topicPartitionSubscriptionResumed != nil {
				// stop listening for subscription message
				saramaLogger.RemoveListener(subscription, topicPartitionSubscriptionResumed)
				// disable subscription event chan
				topicPartitionSubscriptionResumed = nil
				// stop timeout timer
				if !deliverSessionTimer.Stop() {
					<-deliverSessionTimer.C
				}
			}

			select {
			case <-chain.errorChan: // If this channel was closed...
				chain.errorChan = make(chan struct{}) // ...make a new one.
				logger.Infof("[channel: %s] Marked consenter as available again", chain.ChainID())
			default:
			}
			if err := proto.Unmarshal(in.Value, msg); err != nil {
				// This shouldn't happen, it should be filtered at ingress
				logger.Criticalf("[channel: %s] Unable to unmarshal consumed message = %s", chain.ChainID(), err)
				counts[indexUnmarshalError]++
				continue
			} else {
				logger.Debugf("[channel: %s] Successfully unmarshalled consumed message, offset is %d. Inspecting type...", chain.ChainID(), in.Offset)
				counts[indexRecvPass]++
			}
			switch msg.Type.(type) {
			case *ab.KafkaMessage_Connect:
				_ = chain.processConnect(chain.ChainID())
				counts[indexProcessConnectPass]++
			case *ab.KafkaMessage_TimeToCut:
				if err := chain.processTimeToCut(msg.GetTimeToCut(), in.Offset); err != nil {
					logger.Warningf("[channel: %s] %s", chain.ChainID(), err)
					logger.Criticalf("[channel: %s] Consenter for channel exiting", chain.ChainID())
					counts[indexProcessTimeToCutError]++
					return counts, err // TODO Revisit whether we should indeed stop processing the chain at this point
				}
				counts[indexProcessTimeToCutPass]++
			case *ab.KafkaMessage_Regular:
				if err := chain.processRegular(msg.GetRegular(), in.Offset); err != nil {
					logger.Warningf("[channel: %s] Error when processing incoming message of type REGULAR = %s", chain.ChainID(), err)
					counts[indexProcessRegularError]++
				} else {
					counts[indexProcessRegularPass]++
				}
			}
		case <-chain.timer:
			if err := sendTimeToCut(chain.producer, chain.channel, chain.lastCutBlockNumber+1, &chain.timer); err != nil {
				logger.Errorf("[channel: %s] cannot post time-to-cut message = %s", chain.ChainID(), err)
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
		logger.Errorf("[channel: %s] could not close channelConsumer cleanly = %s", chain.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the channel consumer", chain.ChainID())
	}

	err = chain.parentConsumer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close parentConsumer cleanly = %s", chain.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the parent consumer", chain.ChainID())
	}

	err = chain.producer.Close()
	if err != nil {
		logger.Errorf("[channel: %s] could not close producer cleanly = %s", chain.ChainID(), err)
		errs = append(errs, err)
	} else {
		logger.Debugf("[channel: %s] Closed the producer", chain.ChainID())
	}

	return errs
}

// Helper functions

func getLastCutBlockNumber(blockchainHeight uint64) uint64 {
	return blockchainHeight - 1
}

func getOffsets(metadataValue []byte, chainID string) (persisted int64, processed int64, resubmitted int64) {
	if metadataValue != nil {
		// Extract orderer-related metadata from the tip of the ledger first
		kafkaMetadata := &ab.KafkaMetadata{}
		if err := proto.Unmarshal(metadataValue, kafkaMetadata); err != nil {
			logger.Panicf("[channel: %s] Ledger may be corrupted:"+
				"cannot unmarshal orderer metadata in most recent block", chainID)
		}
		return kafkaMetadata.LastOffsetPersisted,
			kafkaMetadata.LastOriginalOffsetProcessed,
			kafkaMetadata.LastResubmittedConfigOffset
	}
	return sarama.OffsetOldest - 1, int64(0), int64(0) // default
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

func newNormalMessage(payload []byte, configSeq uint64, originalOffset int64) *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_Regular{
			Regular: &ab.KafkaMessageRegular{
				Payload:        payload,
				ConfigSeq:      configSeq,
				Class:          ab.KafkaMessageRegular_NORMAL,
				OriginalOffset: originalOffset,
			},
		},
	}
}

func newConfigMessage(config []byte, configSeq uint64, originalOffset int64) *ab.KafkaMessage {
	return &ab.KafkaMessage{
		Type: &ab.KafkaMessage_Regular{
			Regular: &ab.KafkaMessageRegular{
				Payload:        config,
				ConfigSeq:      configSeq,
				Class:          ab.KafkaMessageRegular_CONFIG,
				OriginalOffset: originalOffset,
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

func (chain *chainImpl) processConnect(channelName string) error {
	logger.Debugf("[channel: %s] It's a connect message - ignoring", channelName)
	return nil
}

func (chain *chainImpl) processRegular(regularMessage *ab.KafkaMessageRegular, receivedOffset int64) error {
	// When committing a normal message, we also update `lastOriginalOffsetProcessed` with `newOffset`.
	// It is caller's responsibility to deduce correct value of `newOffset` based on following rules:
	// - if Resubmission is switched off, it should always be zero
	// - if the message is committed on first pass, meaning it's not re-validated and re-ordered, this value
	//   should be the same as current `lastOriginalOffsetProcessed`
	// - if the message is re-validated and re-ordered, this value should be the `OriginalOffset` of that
	//   Kafka message, so that `lastOriginalOffsetProcessed` is advanced
	commitNormalMsg := func(message *cb.Envelope, newOffset int64) {
		batches, pending := chain.BlockCutter().Ordered(message)
		logger.Debugf("[channel: %s] Ordering results: items in batch = %d, pending = %v", chain.ChainID(), len(batches), pending)

		switch {
		case chain.timer != nil && !pending:
			// Timer is already running but there are no messages pending, stop the timer
			chain.timer = nil
		case chain.timer == nil && pending:
			// Timer is not already running and there are messages pending, so start it
			chain.timer = time.After(chain.SharedConfig().BatchTimeout())
			logger.Debugf("[channel: %s] Just began %s batch timer", chain.ChainID(), chain.SharedConfig().BatchTimeout().String())
		default:
			// Do nothing when:
			// 1. Timer is already running and there are messages pending
			// 2. Timer is not set and there are no messages pending
		}

		if len(batches) == 0 {
			// If no block is cut, we update the `lastOriginalOffsetProcessed`, start the timer if necessary and return
			chain.lastOriginalOffsetProcessed = newOffset
			return
		}

		offset := receivedOffset
		if pending || len(batches) == 2 {
			// If the newest envelope is not encapsulated into the first batch,
			// the `LastOffsetPersisted` should be `receivedOffset` - 1.
			offset--
		} else {
			// We are just cutting exactly one block, so it is safe to update
			// `lastOriginalOffsetProcessed` with `newOffset` here, and then
			// encapsulate it into this block. Otherwise, if we are cutting two
			// blocks, the first one should use current `lastOriginalOffsetProcessed`
			// and the second one should use `newOffset`, which is also used to
			// update `lastOriginalOffsetProcessed`
			chain.lastOriginalOffsetProcessed = newOffset
		}

		// Commit the first block
		block := chain.CreateNextBlock(batches[0])
		metadata := &ab.KafkaMetadata{
			LastOffsetPersisted:         offset,
			LastOriginalOffsetProcessed: chain.lastOriginalOffsetProcessed,
			LastResubmittedConfigOffset: chain.lastResubmittedConfigOffset,
		}
		chain.WriteBlock(block, metadata)
		chain.lastCutBlockNumber++
		logger.Debugf("[channel: %s] Batch filled, just cut block [%d] - last persisted offset is now %d", chain.ChainID(), chain.lastCutBlockNumber, offset)

		// Commit the second block if exists
		if len(batches) == 2 {
			chain.lastOriginalOffsetProcessed = newOffset
			offset++

			block := chain.CreateNextBlock(batches[1])
			metadata := &ab.KafkaMetadata{
				LastOffsetPersisted:         offset,
				LastOriginalOffsetProcessed: newOffset,
				LastResubmittedConfigOffset: chain.lastResubmittedConfigOffset,
			}
			chain.WriteBlock(block, metadata)
			chain.lastCutBlockNumber++
			logger.Debugf("[channel: %s] Batch filled, just cut block [%d] - last persisted offset is now %d", chain.ChainID(), chain.lastCutBlockNumber, offset)
		}
	}

	// When committing a config message, we also update `lastOriginalOffsetProcessed` with `newOffset`.
	// It is caller's responsibility to deduce correct value of `newOffset` based on following rules:
	// - if Resubmission is switched off, it should always be zero
	// - if the message is committed on first pass, meaning it's not re-validated and re-ordered, this value
	//   should be the same as current `lastOriginalOffsetProcessed`
	// - if the message is re-validated and re-ordered, this value should be the `OriginalOffset` of that
	//   Kafka message, so that `lastOriginalOffsetProcessed` is advanced
	commitConfigMsg := func(message *cb.Envelope, newOffset int64) {
		logger.Debugf("[channel: %s] Received config message", chain.ChainID())
		batch := chain.BlockCutter().Cut()

		if batch != nil {
			logger.Debugf("[channel: %s] Cut pending messages into block", chain.ChainID())
			block := chain.CreateNextBlock(batch)
			metadata := &ab.KafkaMetadata{
				LastOffsetPersisted:         receivedOffset - 1,
				LastOriginalOffsetProcessed: chain.lastOriginalOffsetProcessed,
				LastResubmittedConfigOffset: chain.lastResubmittedConfigOffset,
			}
			chain.WriteBlock(block, metadata)
			chain.lastCutBlockNumber++
		}

		logger.Debugf("[channel: %s] Creating isolated block for config message", chain.ChainID())
		chain.lastOriginalOffsetProcessed = newOffset
		block := chain.CreateNextBlock([]*cb.Envelope{message})
		metadata := &ab.KafkaMetadata{
			LastOffsetPersisted:         receivedOffset,
			LastOriginalOffsetProcessed: chain.lastOriginalOffsetProcessed,
			LastResubmittedConfigOffset: chain.lastResubmittedConfigOffset,
		}
		chain.WriteConfigBlock(block, metadata)
		chain.lastCutBlockNumber++
		chain.timer = nil
	}

	seq := chain.Sequence()

	env := &cb.Envelope{}
	if err := proto.Unmarshal(regularMessage.Payload, env); err != nil {
		// This shouldn't happen, it should be filtered at ingress
		return fmt.Errorf("failed to unmarshal payload of regular message because = %s", err)
	}

	logger.Debugf("[channel: %s] Processing regular Kafka message of type %s", chain.ChainID(), regularMessage.Class.String())

	// If we receive a message from a pre-v1.1 orderer, or resubmission is explicitly disabled, every orderer
	// should operate as the pre-v1.1 ones: validate again and not attempt to reorder. That is because the
	// pre-v1.1 orderers cannot identify re-ordered messages and resubmissions could lead to committing
	// the same message twice.
	//
	// The implicit assumption here is that the resubmission capability flag is set only when there are no more
	// pre-v1.1 orderers on the network. Otherwise it is unset, and this is what we call a compatibility mode.
	if regularMessage.Class == ab.KafkaMessageRegular_UNKNOWN || !chain.SharedConfig().Capabilities().Resubmission() {
		// Received regular message of type UNKNOWN or resubmission if off, indicating an OSN network with v1.0.x orderer
		logger.Warningf("[channel: %s] This orderer is running in compatibility mode", chain.ChainID())

		chdr, err := utils.ChannelHeader(env)
		if err != nil {
			return fmt.Errorf("discarding bad config message because of channel header unmarshalling error = %s", err)
		}

		class := chain.ClassifyMsg(chdr)
		switch class {
		case msgprocessor.ConfigMsg:
			if _, _, err := chain.ProcessConfigMsg(env); err != nil {
				return fmt.Errorf("discarding bad config message because = %s", err)
			}

			commitConfigMsg(env, chain.lastOriginalOffsetProcessed)

		case msgprocessor.NormalMsg:
			if _, err := chain.ProcessNormalMsg(env); err != nil {
				return fmt.Errorf("discarding bad normal message because = %s", err)
			}

			commitNormalMsg(env, chain.lastOriginalOffsetProcessed)

		case msgprocessor.ConfigUpdateMsg:
			return fmt.Errorf("not expecting message of type ConfigUpdate")

		default:
			logger.Panicf("[channel: %s] Unsupported message classification: %v", chain.ChainID(), class)
		}

		return nil
	}

	switch regularMessage.Class {
	case ab.KafkaMessageRegular_UNKNOWN:
		logger.Panicf("[channel: %s] Kafka message of type UNKNOWN should have been processed already", chain.ChainID())

	case ab.KafkaMessageRegular_NORMAL:
		// This is a message that is re-validated and re-ordered
		if regularMessage.OriginalOffset != 0 {
			logger.Debugf("[channel: %s] Received re-submitted normal message with original offset %d", chain.ChainID(), regularMessage.OriginalOffset)

			// But we've reprocessed it already
			if regularMessage.OriginalOffset <= chain.lastOriginalOffsetProcessed {
				logger.Debugf(
					"[channel: %s] OriginalOffset(%d) <= LastOriginalOffsetProcessed(%d), message has been consumed already, discard",
					chain.ChainID(), regularMessage.OriginalOffset, chain.lastOriginalOffsetProcessed)
				return nil
			}

			logger.Debugf(
				"[channel: %s] OriginalOffset(%d) > LastOriginalOffsetProcessed(%d), "+
					"this is the first time we receive this re-submitted normal message",
				chain.ChainID(), regularMessage.OriginalOffset, chain.lastOriginalOffsetProcessed)

			// In case we haven't reprocessed the message, there's no need to differentiate it from those
			// messages that will be processed for the first time.
		}

		// The config sequence has advanced
		if regularMessage.ConfigSeq < seq {
			logger.Debugf("[channel: %s] Config sequence has advanced since this normal message got validated, re-validating", chain.ChainID())
			configSeq, err := chain.ProcessNormalMsg(env)
			if err != nil {
				return fmt.Errorf("discarding bad normal message because = %s", err)
			}

			logger.Debugf("[channel: %s] Normal message is still valid, re-submit", chain.ChainID())

			// For both messages that are ordered for the first time or re-ordered, we set original offset
			// to current received offset and re-order it.
			if err := chain.order(env, configSeq, receivedOffset); err != nil {
				return fmt.Errorf("error re-submitting normal message because = %s", err)
			}

			return nil
		}

		// Any messages coming in here may or may not have been re-validated
		// and re-ordered, BUT they are definitely valid here

		// advance lastOriginalOffsetProcessed iff message is re-validated and re-ordered
		offset := regularMessage.OriginalOffset
		if offset == 0 {
			offset = chain.lastOriginalOffsetProcessed
		}

		commitNormalMsg(env, offset)

	case ab.KafkaMessageRegular_CONFIG:
		// This is a message that is re-validated and re-ordered
		if regularMessage.OriginalOffset != 0 {
			logger.Debugf("[channel: %s] Received re-submitted config message with original offset %d", chain.ChainID(), regularMessage.OriginalOffset)

			// But we've reprocessed it already
			if regularMessage.OriginalOffset <= chain.lastOriginalOffsetProcessed {
				logger.Debugf(
					"[channel: %s] OriginalOffset(%d) <= LastOriginalOffsetProcessed(%d), message has been consumed already, discard",
					chain.ChainID(), regularMessage.OriginalOffset, chain.lastOriginalOffsetProcessed)
				return nil
			}

			logger.Debugf(
				"[channel: %s] OriginalOffset(%d) > LastOriginalOffsetProcessed(%d), "+
					"this is the first time we receive this re-submitted config message",
				chain.ChainID(), regularMessage.OriginalOffset, chain.lastOriginalOffsetProcessed)

			if regularMessage.OriginalOffset == chain.lastResubmittedConfigOffset && // This is very last resubmitted config message
				regularMessage.ConfigSeq == seq { // AND we don't need to resubmit it again
				logger.Debugf("[channel: %s] Config message with original offset %d is the last in-flight resubmitted message"+
					"and it does not require revalidation, unblock ingress messages now", chain.ChainID(), regularMessage.OriginalOffset)
				chain.reprocessConfigComplete() // Therefore, we could finally unblock broadcast
			}

			// Somebody resubmitted message at offset X, whereas we didn't. This is due to non-determinism where
			// that message was considered invalid by us during revalidation, however somebody else deemed it to
			// be valid, and resubmitted it. We need to advance lastResubmittedConfigOffset in this case in order
			// to enforce consistency across the network.
			if chain.lastResubmittedConfigOffset < regularMessage.OriginalOffset {
				chain.lastResubmittedConfigOffset = regularMessage.OriginalOffset
			}
		}

		// The config sequence has advanced
		if regularMessage.ConfigSeq < seq {
			logger.Debugf("[channel: %s] Config sequence has advanced since this config message got validated, re-validating", chain.ChainID())
			configEnv, configSeq, err := chain.ProcessConfigMsg(env)
			if err != nil {
				return fmt.Errorf("rejecting config message because = %s", err)
			}

			// For both messages that are ordered for the first time or re-ordered, we set original offset
			// to current received offset and re-order it.
			if err := chain.configure(configEnv, configSeq, receivedOffset); err != nil {
				return fmt.Errorf("error re-submitting config message because = %s", err)
			}

			logger.Debugf("[channel: %s] Resubmitted config message with offset %d, block ingress messages", chain.ChainID(), receivedOffset)
			chain.lastResubmittedConfigOffset = receivedOffset // Keep track of last resubmitted message offset
			chain.reprocessConfigPending()                     // Begin blocking ingress messages

			return nil
		}

		// Any messages coming in here may or may not have been re-validated
		// and re-ordered, BUT they are definitely valid here

		// advance lastOriginalOffsetProcessed iff message is re-validated and re-ordered
		offset := regularMessage.OriginalOffset
		if offset == 0 {
			offset = chain.lastOriginalOffsetProcessed
		}

		commitConfigMsg(env, offset)

	default:
		return errors.Errorf("unsupported regular kafka message type: %v", regularMessage.Class.String())
	}

	return nil
}

func (chain *chainImpl) processTimeToCut(ttcMessage *ab.KafkaMessageTimeToCut, receivedOffset int64) error {
	ttcNumber := ttcMessage.GetBlockNumber()
	logger.Debugf("[channel: %s] It's a time-to-cut message for block [%d]", chain.ChainID(), ttcNumber)
	if ttcNumber == chain.lastCutBlockNumber+1 {
		chain.timer = nil
		logger.Debugf("[channel: %s] Nil'd the timer", chain.ChainID())
		batch := chain.BlockCutter().Cut()
		if len(batch) == 0 {
			return fmt.Errorf("got right time-to-cut message (for block [%d]),"+
				" no pending requests though; this might indicate a bug", chain.lastCutBlockNumber+1)
		}
		block := chain.CreateNextBlock(batch)
		metadata := &ab.KafkaMetadata{
			LastOffsetPersisted:         receivedOffset,
			LastOriginalOffsetProcessed: chain.lastOriginalOffsetProcessed,
		}
		chain.WriteBlock(block, metadata)
		chain.lastCutBlockNumber++
		logger.Debugf("[channel: %s] Proper time-to-cut received, just cut block [%d]", chain.ChainID(), chain.lastCutBlockNumber)
		return nil
	} else if ttcNumber > chain.lastCutBlockNumber+1 {
		return fmt.Errorf("got larger time-to-cut message (%d) than allowed/expected (%d)"+
			" - this might indicate a bug", ttcNumber, chain.lastCutBlockNumber+1)
	}
	logger.Debugf("[channel: %s] Ignoring stale time-to-cut-message for block [%d]", chain.ChainID(), ttcNumber)
	return nil
}

// WriteBlock acts as a wrapper around the consenter support WriteBlock, encoding the metadata,
// and updating the metrics.
func (chain *chainImpl) WriteBlock(block *cb.Block, metadata *ab.KafkaMetadata) {
	chain.ConsenterSupport.WriteBlock(block, utils.MarshalOrPanic(metadata))
	chain.consenter.Metrics().LastOffsetPersisted.With("channel", chain.ChainID()).Set(float64(metadata.LastOffsetPersisted))
}

// WriteBlock acts as a wrapper around the consenter support WriteConfigBlock, encoding the metadata,
// and updating the metrics.
func (chain *chainImpl) WriteConfigBlock(block *cb.Block, metadata *ab.KafkaMetadata) {
	chain.ConsenterSupport.WriteConfigBlock(block, utils.MarshalOrPanic(metadata))
	chain.consenter.Metrics().LastOffsetPersisted.With("channel", chain.ChainID()).Set(float64(metadata.LastOffsetPersisted))
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
		select {
		case <-exitChan:
			logger.Debugf("[channel: %s] Consenter for channel exiting, aborting retry", channel)
			return nil
		default:
			_, _, err := producer.SendMessage(message)
			return err
		}
	})

	return postConnect.retry()
}

func sendTimeToCut(producer sarama.SyncProducer, channel channel, timeToCutBlockNumber uint64, timer *<-chan time.Time) error {
	logger.Debugf("[channel: %s] Time-to-cut block [%d] timer expired", channel.topic(), timeToCutBlockNumber)
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

// Creates the Kafka topic for the channel if it does not already exist
func setupTopicForChannel(retryOptions localconfig.Retry, haltChan chan struct{}, brokers []string, brokerConfig *sarama.Config, topicDetail *sarama.TopicDetail, channel channel) error {

	// requires Kafka v0.10.1.0 or higher
	if !brokerConfig.Version.IsAtLeast(sarama.V0_10_1_0) {
		return nil
	}

	logger.Infof("[channel: %s] Setting up the topic for this channel...",
		channel.topic())

	retryMsg := fmt.Sprintf("Creating Kafka topic [%s] for channel [%s]",
		channel.topic(), channel.String())

	setupTopic := newRetryProcess(
		retryOptions,
		haltChan,
		channel,
		retryMsg,
		func() error {

			var err error
			clusterMembers := map[int32]*sarama.Broker{}
			var controllerId int32

			// loop through brokers to access metadata
			for _, address := range brokers {
				broker := sarama.NewBroker(address)
				err = broker.Open(brokerConfig)

				if err != nil {
					continue
				}

				var ok bool
				ok, err = broker.Connected()
				if !ok {
					continue
				}
				defer broker.Close()

				// metadata request which includes the topic
				var apiVersion int16
				if brokerConfig.Version.IsAtLeast(sarama.V0_11_0_0) {
					// use API version 4 to disable auto topic creation for
					// metadata requests
					apiVersion = 4
				} else {
					apiVersion = 1
				}
				metadata, err := broker.GetMetadata(&sarama.MetadataRequest{
					Version:                apiVersion,
					Topics:                 []string{channel.topic()},
					AllowAutoTopicCreation: false})

				if err != nil {
					continue
				}

				controllerId = metadata.ControllerID
				for _, broker := range metadata.Brokers {
					clusterMembers[broker.ID()] = broker
				}

				for _, topic := range metadata.Topics {
					if topic.Name == channel.topic() {
						if topic.Err != sarama.ErrUnknownTopicOrPartition {
							// auto create topics must be enabled so return
							return nil
						}
					}
				}
				break
			}

			// check to see if we got any metadata from any of the brokers in the list
			if len(clusterMembers) == 0 {
				return fmt.Errorf(
					"error creating topic [%s]; failed to retrieve metadata for the cluster",
					channel.topic())
			}

			// get the controller
			controller := clusterMembers[controllerId]
			err = controller.Open(brokerConfig)

			if err != nil {
				return err
			}

			var ok bool
			ok, err = controller.Connected()
			if !ok {
				return err
			}
			defer controller.Close()

			// create the topic
			req := &sarama.CreateTopicsRequest{
				Version: 0,
				TopicDetails: map[string]*sarama.TopicDetail{
					channel.topic(): topicDetail},
				Timeout: 3 * time.Second}
			resp := &sarama.CreateTopicsResponse{}
			resp, err = controller.CreateTopics(req)
			if err != nil {
				return err
			}

			// check the response
			if topicErr, ok := resp.TopicErrors[channel.topic()]; ok {
				// treat no error and topic exists error as success
				if topicErr.Err == sarama.ErrNoError ||
					topicErr.Err == sarama.ErrTopicAlreadyExists {
					return nil
				}
				if topicErr.Err == sarama.ErrInvalidTopic {
					// topic is invalid so abort
					logger.Warningf("[channel: %s] Failed to set up topic = %s",
						channel.topic(), topicErr.Err.Error())
					go func() {
						haltChan <- struct{}{}
					}()
				}
				return fmt.Errorf("error creating topic: [%s]",
					topicErr.Err.Error())
			}

			return nil
		})

	return setupTopic.retry()
}

// Replica ID information can accurately be retrieved only when the cluster
// is healthy. Otherwise, the replica request does not return the full set
// of initial replicas. This information is needed to provide context when
// a health check returns an error.
func getHealthyClusterReplicaInfo(retryOptions localconfig.Retry, haltChan chan struct{}, brokers []string, brokerConfig *sarama.Config, channel channel) ([]int32, error) {
	var replicaIDs []int32

	retryMsg := "Getting list of Kafka brokers replicating the channel"
	getReplicaInfo := newRetryProcess(retryOptions, haltChan, channel, retryMsg, func() error {
		client, err := sarama.NewClient(brokers, brokerConfig)
		if err != nil {
			return err
		}
		defer client.Close()

		replicaIDs, err = client.Replicas(channel.topic(), channel.partition())
		if err != nil {
			return err
		}
		return nil
	})

	return replicaIDs, getReplicaInfo.retry()
}
