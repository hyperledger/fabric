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

package broadcast

import (
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"io"

	"github.com/golang/protobuf/proto"
)

var logger = logging.MustGetLogger("orderer/common/broadcast")

// Handler defines an interface which handles broadcasts
type Handler interface {
	// Handle starts a service thread for a given gRPC connection and services the broadcast connection
	Handle(srv ab.AtomicBroadcast_BroadcastServer) error
}

// SupportManager provides a way for the Handler to look up the Support for a chain
type SupportManager interface {
	// GetChain gets the chain support for a given ChainID
	GetChain(chainID string) (Support, bool)

	// ProposeChain accepts a configuration transaction for a chain which does not already exists
	// The status returned is whether the proposal is accepted for consideration, only after consensus
	// occurs will the proposal be committed or rejected
	ProposeChain(env *cb.Envelope) cb.Status
}

// Support provides the backing resources needed to support broadcast on a chain
type Support interface {
	// Enqueue accepts a message and returns true on acceptance, or false on shutdown
	Enqueue(env *cb.Envelope) bool

	// Filters returns the set of broadcast filters for this chain
	Filters() *filter.RuleSet
}

type handlerImpl struct {
	sm SupportManager
}

// NewHandlerImpl constructs a new implementation of the Handler interface
func NewHandlerImpl(sm SupportManager) Handler {
	return &handlerImpl{
		sm: sm,
	}
}

// Handle starts a service thread for a given gRPC connection and services the broadcast connection
func (bh *handlerImpl) Handle(srv ab.AtomicBroadcast_BroadcastServer) error {
	for {
		msg, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		payload := &cb.Payload{}
		err = proto.Unmarshal(msg.Payload, payload)
		if payload.Header == nil || payload.Header.ChainHeader == nil || payload.Header.ChainHeader.ChainID == "" {
			logger.Debugf("Received malformed message, dropping connection")
			return srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
		}

		support, ok := bh.sm.GetChain(payload.Header.ChainHeader.ChainID)
		if !ok {
			// Chain not found, maybe create one?
			if payload.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
				return srv.Send(&ab.BroadcastResponse{Status: cb.Status_NOT_FOUND})
			}

			logger.Debugf("Proposing new chain")
			err = srv.Send(&ab.BroadcastResponse{Status: bh.sm.ProposeChain(msg)})
			if err != nil {
				return err
			}
			continue
		}

		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Broadcast is filtering message for chain %s", payload.Header.ChainHeader.ChainID)
		}

		// Normal transaction for existing chain
		_, filterErr := support.Filters().Apply(msg)

		if filterErr != nil {
			logger.Debugf("Rejecting broadcast message")
			return srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
		}

		if !support.Enqueue(msg) {
			logger.Debugf("Consenter instructed us to shut down")
			return srv.Send(&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE})
		}

		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Broadcast is successfully enqueued message for chain %s", payload.Header.ChainHeader.ChainID)
		}

		err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SUCCESS})

		if err != nil {
			return err
		}
	}
}
