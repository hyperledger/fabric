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

	"github.com/golang/protobuf/proto"
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
		if d != nil {
			if env, err := utils.GetEnvelopeFromBlock(d); err != nil {
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
						logger.Errorf("Error unmarshalling transaction payload for block event: %s", err)
						continue
					}
					chaincodeActionPayload := &pb.ChaincodeActionPayload{}
					err = proto.Unmarshal(tx.Actions[0].Payload, chaincodeActionPayload)
					if err != nil {
						logger.Errorf("Error unmarshalling transaction action payload for block event: %s", err)
						continue
					}

					propRespPayload := &pb.ProposalResponsePayload{}
					err = proto.Unmarshal(chaincodeActionPayload.Action.ProposalResponsePayload, propRespPayload)
					if err != nil {
						logger.Errorf("Error unmarshalling proposal response payload for block event: %s", err)
						continue
					}
					//ENDORSER_ACTION, ProposalResponsePayload.Extension field contains ChaincodeAction
					caPayload := &pb.ChaincodeAction{}
					err = proto.Unmarshal(propRespPayload.Extension, caPayload)
					if err != nil {
						logger.Errorf("Error unmarshalling chaincode action for block event: %s", err)
						continue
					}
					// Drop read write set from transaction before sending block event
					caPayload.Results = nil
					propRespPayload.Extension, err = proto.Marshal(caPayload)
					if err != nil {
						logger.Errorf("Error marshalling tx proposal extension payload for block event: %s", err)
						continue
					}
					// Marshal Transaction again and append to block to be sent
					chaincodeActionPayload.Action.ProposalResponsePayload, err = proto.Marshal(propRespPayload)
					if err != nil {
						logger.Errorf("Error marshalling tx proposal payload for block event: %s", err)
						continue
					}
					tx.Actions[0].Payload, err = proto.Marshal(chaincodeActionPayload)
					if err != nil {
						logger.Errorf("Error marshalling tx action payload for block event: %s", err)
						continue
					}
					if t, err := proto.Marshal(tx); err == nil {
						bevent.Data.Data = append(bevent.Data.Data, t)
						logger.Infof("calling sendProducerBlockEvent\n")
					} else {
						logger.Infof("Cannot marshal transaction %s\n", err)
					}
				}
			}
		}
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
