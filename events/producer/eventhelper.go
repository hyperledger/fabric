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

package producer

import (
	"fmt"

	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("eventhub_producer")
}

// SendProducerBlockEvent sends block event to clients
func SendProducerBlockEvent(block *common.Block) error {
	bevent := &common.Block{}
	bevent.Header = block.Header
	bevent.Metadata = block.Metadata
	bevent.Data = &common.BlockData{}
	for _, d := range block.Data.Data {
		ebytes := d
		if ebytes != nil {
			if env, err := utils.GetEnvelopeFromBlock(ebytes); err != nil {
				logger.Errorf("Error getting tx from block(%s)\n", err)
			} else if env != nil {
				// get the payload from the envelope
				payload, err := utils.GetPayload(env)
				if err != nil {
					return fmt.Errorf("Could not extract payload from envelope, err %s", err)
				}

				if common.HeaderType(payload.Header.ChainHeader.Type) == common.HeaderType_ENDORSER_TRANSACTION {
					tx, err := utils.GetTransaction(payload.Data)
					if err != nil {
						return fmt.Errorf("Error unmarshalling transaction payload for block event: %s", err)
					}
					chaincodeActionPayload, err := utils.GetChaincodeActionPayload(tx.Actions[0].Payload)
					if err != nil {
						return fmt.Errorf("Error unmarshalling transaction action payload for block event: %s", err)
					}
					propRespPayload, err := utils.GetProposalResponsePayload(chaincodeActionPayload.Action.ProposalResponsePayload)
					if err != nil {
						return fmt.Errorf("Error unmarshalling proposal response payload for block event: %s", err)
					}
					//ENDORSER_ACTION, ProposalResponsePayload.Extension field contains ChaincodeAction
					caPayload, err := utils.GetChaincodeAction(propRespPayload.Extension)
					if err != nil {
						return fmt.Errorf("Error unmarshalling chaincode action for block event: %s", err)
					}
					// Drop read write set from transaction before sending block event
					// Performance issue with chaincode deploy txs and causes nodejs grpc
					// to hit max message size bug
					// Dropping the read write set may cause issues for security and
					// we will need to revist when event security is addressed
					caPayload.Results = nil
					chaincodeActionPayload.Action.ProposalResponsePayload, err = utils.GetBytesProposalResponsePayload(propRespPayload.ProposalHash, caPayload.Response, caPayload.Results, caPayload.Events)
					if err != nil {
						return fmt.Errorf("Error marshalling tx proposal payload for block event: %s", err)
					}
					tx.Actions[0].Payload, err = utils.GetBytesChaincodeActionPayload(chaincodeActionPayload)
					if err != nil {
						return fmt.Errorf("Error marshalling tx action payload for block event: %s", err)
					}
					payload.Data, err = utils.GetBytesTransaction(tx)
					if err != nil {
						return fmt.Errorf("Error marshalling payload for block event: %s", err)
					}
					env.Payload, err = utils.GetBytesPayload(payload)
					if err != nil {
						return fmt.Errorf("Error marshalling tx envelope for block event: %s", err)
					}
					ebytes, err = utils.GetBytesEnvelope(env)
					if err != nil {
						return fmt.Errorf("Cannot marshal transaction %s", err)
					}
				}
			}
		}
		bevent.Data.Data = append(bevent.Data.Data, ebytes)
	}
	return Send(CreateBlockEvent(bevent))
}

//CreateBlockEvent creates a Event from a Block
func CreateBlockEvent(te *common.Block) *pb.Event {
	return &pb.Event{Event: &pb.Event_Block{Block: te}}
}

//CreateChaincodeEvent creates a Event from a ChaincodeEvent
func CreateChaincodeEvent(te *pb.ChaincodeEvent) *pb.Event {
	return &pb.Event{Event: &pb.Event_ChaincodeEvent{ChaincodeEvent: te}}
}

//CreateRejectionEvent creates an Event from TxResults
func CreateRejectionEvent(tx *pb.Transaction, errorMsg string) *pb.Event {
	return &pb.Event{Event: &pb.Event_Rejection{Rejection: &pb.Rejection{Tx: tx, ErrorMsg: errorMsg}}}
}
