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
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	"github.com/hyperledger/fabric/orderer/common/sigfilter"
	ordererledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
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
	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() ordererledger.Reader

	// SharedConfig returns the shared config manager for this chain
	SharedConfig() sharedconfig.Manager
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
		if err != nil {
			logger.Errorf("Error reading from stream: %s", err)
			return err
		}
		payload := &cb.Payload{}
		if err = proto.Unmarshal(envelope.Payload, payload); err != nil {
			logger.Errorf("Received an envelope with no payload: %s", err)
			return err
		}

		if payload.Header == nil || payload.Header.ChainHeader == nil {
			err := fmt.Errorf("Malformed envelope recieved with bad header")
			logger.Error(err)
			return err
		}

		chain, ok := ds.sm.GetChain(payload.Header.ChainHeader.ChainID)
		if !ok {
			return sendStatusReply(srv, cb.Status_NOT_FOUND)
		}

		sf := sigfilter.New(chain.SharedConfig().EgressPolicyNames, chain.PolicyManager())
		result, _ := sf.Apply(envelope)
		if result != filter.Forward {
			return sendStatusReply(srv, cb.Status_FORBIDDEN)
		}

		seekInfo := &ab.SeekInfo{}
		if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
			logger.Errorf("Received a signed deliver request with malformed seekInfo payload: %s", err)
			return err
		}

		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Received seekInfo %v for chain %s", seekInfo, payload.Header.ChainHeader.ChainID)
		}

		cursor, number := chain.Reader().Iterator(seekInfo.Start)
		var stopNum uint64
		switch stop := seekInfo.Stop.Type.(type) {
		case *ab.SeekPosition_Oldest:
			stopNum = number
		case *ab.SeekPosition_Newest:
			stopNum = chain.Reader().Height() - 1
		case *ab.SeekPosition_Specified:
			stopNum = stop.Specified.Number
		}

		for {
			if seekInfo.Behavior == ab.SeekInfo_BLOCK_UNTIL_READY {
				<-cursor.ReadyChan()
			} else {
				select {
				case <-cursor.ReadyChan():
				default:
					return sendStatusReply(srv, cb.Status_NOT_FOUND)
				}
			}

			block, status := cursor.Next()
			if status != cb.Status_SUCCESS {
				logger.Errorf("Error reading from channel, cause was: %v", status)
				return sendStatusReply(srv, status)
			}

			logger.Debugf("Delivering block")
			if err := sendBlockReply(srv, block); err != nil {
				return err
			}

			if stopNum == block.Header.Number {
				break
			}
		}

		if err := sendStatusReply(srv, cb.Status_SUCCESS); err != nil {
			return err
		}
		logger.Debugf("Done delivering, waiting for new SeekInfo")
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
