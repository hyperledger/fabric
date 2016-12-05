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
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

type clientDelivererImpl struct {
	brokerFunc   func(*config.TopLevel) Broker
	consumerFunc func(*config.TopLevel, int64) (Consumer, error) // This resets the consumer.

	consumer Consumer
	config   *config.TopLevel
	deadChan chan struct{}

	errChan   chan error
	updChan   chan *ab.DeliverUpdate
	tokenChan chan struct{}
	lastACK   int64
	window    int64
}

func newClientDeliverer(conf *config.TopLevel, deadChan chan struct{}) Deliverer {
	brokerFunc := func(conf *config.TopLevel) Broker {
		return newBroker(conf)
	}
	consumerFunc := func(conf *config.TopLevel, seek int64) (Consumer, error) {
		return newConsumer(conf, seek)
	}

	return &clientDelivererImpl{
		brokerFunc:   brokerFunc,
		consumerFunc: consumerFunc,

		config:   conf,
		deadChan: deadChan,
		errChan:  make(chan error),
		updChan:  make(chan *ab.DeliverUpdate), // TODO Size this properly
	}
}

// Deliver receives updates from a client and returns a stream of blocks to them
func (cd *clientDelivererImpl) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	go cd.recvUpdates(stream)
	return cd.sendBlocks(stream)
}

// Close shuts down the Deliver server assigned by the orderer to a client
func (cd *clientDelivererImpl) Close() error {
	if cd.consumer != nil {
		return cd.consumer.Close()
	}
	return nil
}

func (cd *clientDelivererImpl) recvUpdates(stream ab.AtomicBroadcast_DeliverServer) {
	for {
		upd, err := stream.Recv()
		if err != nil {
			cd.errChan <- err
			return
		}
		cd.updChan <- upd
	}
}

func (cd *clientDelivererImpl) sendBlocks(stream ab.AtomicBroadcast_DeliverServer) error {
	var err error
	var reply *ab.DeliverResponse
	var upd *ab.DeliverUpdate
	block := new(cb.Block)
	for {
		select {
		case <-cd.deadChan:
			logger.Debug("sendBlocks goroutine for client-deliverer received shutdown signal")
			return nil
		case err = <-cd.errChan:
			return err
		case upd = <-cd.updChan:
			switch t := upd.GetType().(type) {
			case *ab.DeliverUpdate_Seek:
				err = cd.processSeek(t)
			case *ab.DeliverUpdate_Acknowledgement:
				err = cd.processACK(t)
			}
			if err != nil {
				var errorStatus cb.Status
				// TODO Will need to flesh this out into
				// a proper error handling system eventually.
				switch err.Error() {
				case seekOutOfRangeError:
					errorStatus = cb.Status_NOT_FOUND
				case ackOutOfRangeError, windowOutOfRangeError:
					errorStatus = cb.Status_BAD_REQUEST
				default:
					errorStatus = cb.Status_SERVICE_UNAVAILABLE
				}
				reply = new(ab.DeliverResponse)
				reply.Type = &ab.DeliverResponse_Error{Error: errorStatus}
				if err := stream.Send(reply); err != nil {
					return fmt.Errorf("Failed to send error response to the client: %s", err)
				}
				return fmt.Errorf("Failed to process received update: %s", err)
			}
		case <-cd.tokenChan:
			select {
			case data := <-cd.consumer.Recv():
				err := proto.Unmarshal(data.Value, block)
				if err != nil {
					logger.Info("Failed to unmarshal retrieved block from ordering service:", err)
				}
				reply = new(ab.DeliverResponse)
				reply.Type = &ab.DeliverResponse_Block{Block: block}
				err = stream.Send(reply)
				if err != nil {
					return fmt.Errorf("Failed to send block to the client: %s", err)
				}
				logger.Debugf("Sent block %v to client (prevHash: %v, messages: %v)\n",
					block.Header.Number, block.Header.PreviousHash, block.Data.Data)
			default:
				// Return the push token if there are no messages
				// available from the ordering service.
				cd.tokenChan <- struct{}{}
			}
		}
	}
}

func (cd *clientDelivererImpl) processSeek(msg *ab.DeliverUpdate_Seek) error {
	var err error
	var seek, window int64
	logger.Debug("Received SEEK message")

	window = int64(msg.Seek.WindowSize)
	if window <= 0 || window > int64(cd.config.General.MaxWindowSize) {
		return errors.New(windowOutOfRangeError)
	}
	cd.window = window
	logger.Debug("Requested window size set to", cd.window)

	oldestAvailable, err := cd.getOffset(int64(-2))
	if err != nil {
		return err
	}
	newestAvailable, err := cd.getOffset(int64(-1))
	if err != nil {
		return err
	}
	newestAvailable-- // Cause in the case of newest, the library actually gives us the seqNo of the *next* new block

	switch msg.Seek.Start {
	case ab.SeekInfo_OLDEST:
		seek = oldestAvailable
	case ab.SeekInfo_NEWEST:
		seek = newestAvailable
	case ab.SeekInfo_SPECIFIED:
		seek = int64(msg.Seek.SpecifiedNumber)
		if !(seek >= oldestAvailable && seek <= newestAvailable) {
			return errors.New(seekOutOfRangeError)
		}
	}

	logger.Debug("Requested seek number set to", seek)

	cd.disablePush()
	if err := cd.Close(); err != nil {
		return err
	}
	cd.lastACK = seek - 1
	logger.Debug("Set last ACK for this client's consumer to", cd.lastACK)

	cd.consumer, err = cd.consumerFunc(cd.config, seek)
	if err != nil {
		return err
	}

	cd.enablePush(cd.window)
	return nil
}

func (cd *clientDelivererImpl) getOffset(seek int64) (int64, error) {
	broker := cd.brokerFunc(cd.config)
	defer broker.Close()
	return broker.GetOffset(newOffsetReq(cd.config, seek))
}

func (cd *clientDelivererImpl) disablePush() int64 {
	// No need to add a lock to ensure these operations happen atomically.
	// The caller is the only function that can modify the tokenChan.
	remTokens := int64(len(cd.tokenChan))
	cd.tokenChan = nil
	logger.Debugf("Pushing blocks to client paused; found %v unused push token(s)", remTokens)
	return remTokens
}

func (cd *clientDelivererImpl) enablePush(newTokenCount int64) {
	cd.tokenChan = make(chan struct{}, newTokenCount)
	for i := int64(0); i < newTokenCount; i++ {
		cd.tokenChan <- struct{}{}
	}
	logger.Debugf("Pushing blocks to client resumed; %v push token(s) available", newTokenCount)
}

func (cd *clientDelivererImpl) processACK(msg *ab.DeliverUpdate_Acknowledgement) error {
	logger.Debug("Received ACK for block", msg.Acknowledgement.Number)
	remTokens := cd.disablePush()
	newACK := int64(msg.Acknowledgement.Number) // TODO Optionally mark this offset in Kafka
	if (newACK < cd.lastACK) || (newACK > cd.lastACK+cd.window) {
		return errors.New(ackOutOfRangeError)
	}
	newTokenCount := newACK - cd.lastACK + remTokens
	cd.lastACK = newACK
	cd.enablePush(newTokenCount)
	return nil
}
