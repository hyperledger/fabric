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

package deliver

import (
	"io"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/utils"
)

var logger = logging.MustGetLogger("orderer/common/deliver")

// Handler defines an interface which handles Deliver requests
type Handler interface {
	Handle(srv ab.AtomicBroadcast_DeliverServer) error
}

// SupportManager provides a way for the Handler to look up the Support for a chain
type SupportManager interface {
	GetChain(chainID string) (Support, bool)
}

// Support provides the backing resources needed to support deliver on a chain
type Support interface {
	// Sequence returns the current config sequence number, can be used to detect config changes
	Sequence() uint64

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() ledger.Reader

	// Errored returns a channel which closes when the backing consenter has errored
	Errored() <-chan struct{}
}

type deliverServer struct {
	sm SupportManager
}

// NewHandlerImpl creates an implementation of the Handler interface
func NewHandlerImpl(sm SupportManager) Handler {
	return &deliverServer{
		sm: sm,
	}
}

func (ds *deliverServer) Handle(srv ab.AtomicBroadcast_DeliverServer) error {
	logger.Debugf("Starting new deliver loop")
	for {
		logger.Debugf("Attempting to read seek info message")
		envelope, err := srv.Recv()
		if err == io.EOF {
			logger.Debugf("Received EOF, hangup")
			return nil
		}

		if err != nil {
			logger.Warningf("Error reading from stream: %s", err)
			return err
		}

		payload, err := utils.UnmarshalPayload(envelope.Payload)
		if err != nil {
			logger.Warningf("Received an envelope with no payload: %s", err)
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}

		if payload.Header == nil {
			logger.Warningf("Malformed envelope received with bad header")
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}

		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			logger.Warningf("Failed to unmarshal channel header: %s", err)
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}

		chain, ok := ds.sm.GetChain(chdr.ChannelId)
		if !ok {
			// Note, we log this at DEBUG because SDKs will poll waiting for channels to be created
			// So we would expect our log to be somewhat flooded with these
			logger.Debugf("Rejecting deliver because channel %s not found", chdr.ChannelId)
			return sendStatusReply(srv, cb.Status_NOT_FOUND)
		}

		erroredChan := chain.Errored()
		select {
		case <-erroredChan:
			logger.Warningf("[channel: %s] Rejecting deliver request because of consenter error", chdr.ChannelId)
			return sendStatusReply(srv, cb.Status_SERVICE_UNAVAILABLE)
		default:

		}

		lastConfigSequence := chain.Sequence()

		sf := sigfilter.New(policies.ChannelReaders, chain.PolicyManager())
		result, _ := sf.Apply(envelope)
		if result != filter.Forward {
			logger.Warningf("[channel: %s] Received unauthorized deliver request", chdr.ChannelId)
			return sendStatusReply(srv, cb.Status_FORBIDDEN)
		}

		seekInfo := &ab.SeekInfo{}
		if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
			logger.Warningf("[channel: %s] Received a signed deliver request with malformed seekInfo payload: %s", chdr.ChannelId, err)
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}

		if seekInfo.Start == nil || seekInfo.Stop == nil {
			logger.Warningf("[channel: %s] Received seekInfo message with missing start or stop %v, %v", chdr.ChannelId, seekInfo.Start, seekInfo.Stop)
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}

		logger.Debugf("[channel: %s] Received seekInfo (%p) %v", chdr.ChannelId, seekInfo, seekInfo)

		cursor, number := chain.Reader().Iterator(seekInfo.Start)
		var stopNum uint64
		switch stop := seekInfo.Stop.Type.(type) {
		case *ab.SeekPosition_Oldest:
			stopNum = number
		case *ab.SeekPosition_Newest:
			stopNum = chain.Reader().Height() - 1
		case *ab.SeekPosition_Specified:
			stopNum = stop.Specified.Number
			if stopNum < number {
				logger.Warningf("[channel: %s] Received invalid seekInfo message: start number %d greater than stop number %d", chdr.ChannelId, number, stopNum)
				return sendStatusReply(srv, cb.Status_BAD_REQUEST)
			}
		}

		for {
			if seekInfo.Behavior == ab.SeekInfo_BLOCK_UNTIL_READY {
				select {
				case <-erroredChan:
					logger.Warningf("[channel: %s] Aborting deliver request because of consenter error", chdr.ChannelId)
					return sendStatusReply(srv, cb.Status_SERVICE_UNAVAILABLE)
				case <-cursor.ReadyChan():
				}
			} else {
				select {
				case <-cursor.ReadyChan():
				default:
					return sendStatusReply(srv, cb.Status_NOT_FOUND)
				}
			}

			currentConfigSequence := chain.Sequence()
			if currentConfigSequence > lastConfigSequence {
				lastConfigSequence = currentConfigSequence
				sf := sigfilter.New(policies.ChannelReaders, chain.PolicyManager())
				result, _ := sf.Apply(envelope)
				if result != filter.Forward {
					logger.Warningf("[channel: %s] Client authorization revoked for deliver request", chdr.ChannelId)
					return sendStatusReply(srv, cb.Status_FORBIDDEN)
				}
			}

			block, status := cursor.Next()
			if status != cb.Status_SUCCESS {
				logger.Errorf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
				return sendStatusReply(srv, status)
			}

			logger.Debugf("[channel: %s] Delivering block for (%p)", chdr.ChannelId, seekInfo)

			if err := sendBlockReply(srv, block); err != nil {
				logger.Warningf("[channel: %s] Error sending to stream: %s", chdr.ChannelId, err)
				return err
			}

			if stopNum == block.Header.Number {
				break
			}
		}

		if err := sendStatusReply(srv, cb.Status_SUCCESS); err != nil {
			logger.Warningf("[channel: %s] Error sending to stream: %s", chdr.ChannelId, err)
			return err
		}

		logger.Debugf("[channel: %s] Done delivering for (%p), waiting for new SeekInfo", chdr.ChannelId, seekInfo)
	}
}

func sendStatusReply(srv ab.AtomicBroadcast_DeliverServer, status cb.Status) error {
	return srv.Send(&ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: status},
	})

}

func sendBlockReply(srv ab.AtomicBroadcast_DeliverServer, block *cb.Block) error {
	return srv.Send(&ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{Block: block},
	})
}
