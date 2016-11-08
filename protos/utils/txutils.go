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

package utils

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos"
	"github.com/hyperledger/fabric/protos/common"
)

// GetPayloads get's the underlying payload objects in a TransactionAction
func GetPayloads(txActions *protos.TransactionAction) (*protos.ChaincodeActionPayload, *protos.ChaincodeAction, error) {
	txhdr := &protos.Header{}
	err := proto.Unmarshal(txActions.Header, txhdr)
	if err != nil {
		return nil, nil, err
	}

	switch txhdr.Type {
	case protos.Header_CHAINCODE:
		ccPayload := &protos.ChaincodeActionPayload{}
		err = proto.Unmarshal(txActions.Payload, ccPayload)
		if err != nil {
			return nil, nil, err
		}

		if ccPayload.Action == nil || ccPayload.Action.ProposalResponsePayload == nil {
			return nil, nil, fmt.Errorf("no payload in ChaincodeActionPayload")
		}
		pRespPayload := &protos.ProposalResponsePayload{}
		err = proto.Unmarshal(ccPayload.Action.ProposalResponsePayload, pRespPayload)
		if err != nil {
			return nil, nil, err
		}

		if pRespPayload.Extension == nil {
			return nil, nil, err
		}

		respPayload := &protos.ChaincodeAction{}
		err = proto.Unmarshal(pRespPayload.Extension, respPayload)
		if err != nil {
			return ccPayload, nil, err
		}
		return ccPayload, respPayload, nil

	default:
		return nil, nil, fmt.Errorf("Cannot process unknown transaction type")
	}
}

// CreateTx creates a Transaction2 from given inputs
func CreateTx(typ protos.Header_Type, ccPropPayload []byte, ccEvents []byte, simulationResults []byte, endorsements []*protos.Endorsement) (*protos.Transaction2, error) {
	if typ != protos.Header_CHAINCODE {
		panic("-----Only CHAINCODE Type is supported-----")
	}

	ext := &protos.ChaincodeAction{Results: simulationResults, Events: ccEvents}
	extBytes, err := proto.Marshal(ext)
	if err != nil {
		return nil, err
	}

	//TODO - compute epoch
	var epoch []byte

	pRespPayload := &protos.ProposalResponsePayload{ProposalHash: ccPropPayload, Epoch: epoch, Extension: extBytes}
	pRespPayloadBytes, err := proto.Marshal(pRespPayload)
	if err != nil {
		return nil, err
	}

	ceAction := &protos.ChaincodeEndorsedAction{ProposalResponsePayload: pRespPayloadBytes, Endorsements: endorsements}
	caPayload := &protos.ChaincodeActionPayload{ChaincodeProposalPayload: []byte("marshalled payload here"), Action: ceAction}
	actionBytes, err := proto.Marshal(caPayload)
	if err != nil {
		return nil, err
	}

	hdr := &protos.Header{Type: typ}
	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	tx := &protos.Transaction2{}
	tx.Actions = []*protos.TransactionAction{&protos.TransactionAction{Header: hdrBytes, Payload: actionBytes}}

	return tx, nil
}

// CreateTxFromProposalResponse create's the Transaction from just one proposal response
func CreateTxFromProposalResponse(pResp *protos.ProposalResponse) (*protos.Transaction2, error) {
	pRespPayload := &protos.ProposalResponsePayload{}
	err := proto.Unmarshal(pResp.Payload, pRespPayload)
	if err != nil {
		return nil, err
	}
	ccAction := &protos.ChaincodeAction{}
	err = proto.Unmarshal(pRespPayload.Extension, ccAction)
	if err != nil {
		return nil, err
	}
	return CreateTx(protos.Header_CHAINCODE, pRespPayload.ProposalHash, ccAction.Events, ccAction.Results, []*protos.Endorsement{pResp.Endorsement})
}

// GetEndorserTxFromBlock gets Transaction2 from Block.Data.Data
func GetEndorserTxFromBlock(data []byte) (*protos.Transaction2, error) {
	//Block always begins with an envelope
	var err error
	env := &common.Envelope{}
	if err = proto.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("Error getting envelope(%s)\n", err)
	}
	payload := &common.Payload{}
	if err = proto.Unmarshal(env.Payload, payload); err != nil {
		return nil, fmt.Errorf("Error getting payload(%s)\n", err)
	}

	if common.HeaderType(payload.Header.ChainHeader.Type) == common.HeaderType_ENDORSER_TRANSACTION {
		tx := &protos.Transaction2{}
		if err = proto.Unmarshal(payload.Data, tx); err != nil {
			return nil, fmt.Errorf("Error getting tx(%s)\n", err)
		}
		return tx, nil
	}
	return nil, nil
}
