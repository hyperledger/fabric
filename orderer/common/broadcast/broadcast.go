/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast

import (
	"io"

	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/broadcast")

// Handler defines an interface which handles broadcasts
type Handler interface {
	// Handle starts a service thread for a given gRPC connection and services the broadcast connection
	Handle(srv ab.AtomicBroadcast_BroadcastServer) error
}

// ChannelSupportRegistrar provides a way for the Handler to look up the Support for a chain
type ChannelSupportRegistrar interface {
	// BroadcastChannelSupport returns the message channel header, whether the message is a config update
	// and the channel resources for a message or an error if the message is not a message which can
	// be processed directly (like CONFIG and ORDERER_TRANSACTION messages)
	BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, ChannelSupport, error)
}

// ChannelSupport provides the backing resources needed to support broadcast on a channel
type ChannelSupport interface {
	msgprocessor.Processor
	Consenter
}

// Consenter provides methods to send messages through consensus
type Consenter interface {
	// Order accepts a message or returns an error indicating the cause of failure
	// It ultimately passes through to the consensus.Chain interface
	Order(env *cb.Envelope, configSeq uint64) error

	// Configure accepts a reconfiguration or returns an error indicating the cause of failure
	// It ultimately passes through to the consensus.Chain interface
	Configure(configUpdateMsg *cb.Envelope, config *cb.Envelope, configSeq uint64) error
}

type handlerImpl struct {
	sm ChannelSupportRegistrar
}

// NewHandlerImpl constructs a new implementation of the Handler interface
func NewHandlerImpl(sm ChannelSupportRegistrar) Handler {
	return &handlerImpl{
		sm: sm,
	}
}

// Handle starts a service thread for a given gRPC connection and services the broadcast connection
func (bh *handlerImpl) Handle(srv ab.AtomicBroadcast_BroadcastServer) error {
	logger.Debugf("Starting new broadcast loop")
	for {
		msg, err := srv.Recv()
		if err == io.EOF {
			logger.Debugf("Received EOF, hangup")
			return nil
		}
		if err != nil {
			logger.Warningf("Error reading from stream: %s", err)
			return err
		}

		chdr, isConfig, processor, err := bh.sm.BroadcastChannelSupport(msg)
		if err != nil {
			logger.Warningf("[channel: %s] Could not get message processor: %s", chdr.ChannelId, err)
			return srv.Send(&ab.BroadcastResponse{Status: cb.Status_INTERNAL_SERVER_ERROR, Info: err.Error()})
		}

		if !isConfig {
			logger.Debugf("[channel: %s] Broadcast is processing normal message of type %s", chdr.ChannelId, cb.HeaderType_name[chdr.Type])

			configSeq, err := processor.ProcessNormalMsg(msg)
			if err != nil {
				logger.Warningf("[channel: %s] Rejecting broadcast of normal message because of error: %s", chdr.ChannelId, err)
				return srv.Send(&ab.BroadcastResponse{Status: ClassifyError(err), Info: err.Error()})
			}

			err = processor.Order(msg, configSeq)
			if err != nil {
				logger.Warningf("[channel: %s] Rejecting broadcast of normal message with SERVICE_UNAVAILABLE: rejected by Order: %s", chdr.ChannelId, err)
				return srv.Send(&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()})
			}
		} else { // isConfig
			logger.Debugf("[channel: %s] Broadcast is processing config update message", chdr.ChannelId)

			config, configSeq, err := processor.ProcessConfigUpdateMsg(msg)
			if err != nil {
				logger.Warningf("[channel: %s] Rejecting broadcast of config message because of error: %s", chdr.ChannelId, err)
				return srv.Send(&ab.BroadcastResponse{Status: ClassifyError(err), Info: err.Error()})
			}

			err = processor.Configure(msg, config, configSeq)
			if err != nil {
				logger.Warningf("[channel: %s] Rejecting broadcast of config message with SERVICE_UNAVAILABLE: rejected by Configure: %s", chdr.ChannelId, err)
				return srv.Send(&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()})
			}
		}

		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("[channel: %s] Broadcast has successfully enqueued message of type %s", chdr.ChannelId, cb.HeaderType_name[chdr.Type])
		}

		err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SUCCESS})
		if err != nil {
			logger.Warningf("[channel: %s] Error sending to stream: %s", chdr.ChannelId, err)
			return err
		}
	}
}

// ClassifyError converts an error type into a status code.
func ClassifyError(err error) cb.Status {
	switch err {
	case msgprocessor.ErrChannelDoesNotExist:
		return cb.Status_NOT_FOUND
	default:
		return cb.Status_BAD_REQUEST
	}
}
