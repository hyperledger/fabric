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

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/util"
	"github.com/hyperledger/fabric/protos/utils"
)

const pkgLogID = "orderer/common/deliver"

var logger *logging.Logger

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

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
	addr := util.ExtractRemoteAddress(srv.Context())
	logger.Debugf("Starting new deliver loop for %s", addr)
	for {
		logger.Debugf("Attempting to read seek info message from %s", addr)
		envelope, err := srv.Recv()
		if err == io.EOF {
			logger.Debugf("Received EOF from %s, hangup", addr)
			return nil
		}

		if err != nil {
			logger.Warningf("Error reading from %s: %s", addr, err)
			return err
		}

		if err := ds.deliverBlocks(srv, envelope); err != nil {
			return err
		}

		logger.Debugf("Waiting for new SeekInfo from %s", addr)
	}
}

func (ds *deliverServer) deliverBlocks(srv ab.AtomicBroadcast_DeliverServer, envelope *cb.Envelope) error {
	addr := util.ExtractRemoteAddress(srv.Context())
	payload, err := utils.UnmarshalPayload(envelope.Payload)
	if err != nil {
		logger.Warningf("Received an envelope from %s with no payload: %s", addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	if payload.Header == nil {
		logger.Warningf("Malformed envelope received from %s with bad header", addr)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Warningf("Failed to unmarshal channel header from %s: %s", addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	chain, ok := ds.sm.GetChain(chdr.ChannelId)
	if !ok {
		// Note, we log this at DEBUG because SDKs will poll waiting for channels to be created
		// So we would expect our log to be somewhat flooded with these
		logger.Debugf("Rejecting deliver for %s because channel %s not found", addr, chdr.ChannelId)
		return sendStatusReply(srv, cb.Status_NOT_FOUND)
	}

	erroredChan := chain.Errored()
	select {
	case <-erroredChan:
		logger.Warningf("[channel: %s] Rejecting deliver request for %s because of consenter error", chdr.ChannelId, addr)
		return sendStatusReply(srv, cb.Status_SERVICE_UNAVAILABLE)
	default:

	}

	lastConfigSequence := chain.Sequence()

	sf := msgprocessor.NewSigFilter(policies.ChannelReaders, chain)
	if err := sf.Apply(envelope); err != nil {
		logger.Warningf("[channel: %s] Received unauthorized deliver request from %s: %s", chdr.ChannelId, addr, err)
		return sendStatusReply(srv, cb.Status_FORBIDDEN)
	}

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	logger.Debugf("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)

	cursor, number := chain.Reader().Iterator(seekInfo.Start)
	defer cursor.Close()
	var stopNum uint64
	switch stop := seekInfo.Stop.Type.(type) {
	case *ab.SeekPosition_Oldest:
		stopNum = number
	case *ab.SeekPosition_Newest:
		stopNum = chain.Reader().Height() - 1
	case *ab.SeekPosition_Specified:
		stopNum = stop.Specified.Number
		if stopNum < number {
			logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}
	}

	for {
		if seekInfo.Behavior == ab.SeekInfo_BLOCK_UNTIL_READY {
			select {
			case <-erroredChan:
				logger.Warningf("[channel: %s] Aborting deliver for request because of consenter error", chdr.ChannelId, addr)
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
			if err := sf.Apply(envelope); err != nil {
				logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
				return sendStatusReply(srv, cb.Status_FORBIDDEN)
			}
		}

		block, status := cursor.Next()
		if status != cb.Status_SUCCESS {
			logger.Errorf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return sendStatusReply(srv, status)
		}

		logger.Debugf("[channel: %s] Delivering block for (%p) for %s", chdr.ChannelId, seekInfo, addr)

		if err := sendBlockReply(srv, block); err != nil {
			logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return err
		}

		if stopNum == block.Header.Number {
			break
		}
	}

	if err := sendStatusReply(srv, cb.Status_SUCCESS); err != nil {
		logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
		return err
	}

	logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return nil

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
