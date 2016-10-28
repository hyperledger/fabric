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
)

//GetChaincodeInvocationSpec get the ChaincodeInvocationSpec from the proposal
func GetChaincodeInvocationSpec(prop *protos.Proposal) (*protos.ChaincodeInvocationSpec, error) {
	txhdr := &protos.Header{}
	err := proto.Unmarshal(prop.Header, txhdr)
	if err != nil {
		return nil, err
	}
	if txhdr.Type != protos.Header_CHAINCODE {
		return nil, fmt.Errorf("only CHAINCODE Type proposals supported")
	}
	ccPropPayload := &protos.ChaincodeProposalPayload{}
	err = proto.Unmarshal(prop.Payload, ccPropPayload)
	if err != nil {
		return nil, err
	}
	cis := &protos.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(ccPropPayload.Input, cis)
	if err != nil {
		return nil, err
	}
	return cis, nil
}

// CreateChaincodeProposal creates a proposal from given input
func CreateChaincodeProposal(cis *protos.ChaincodeInvocationSpec) (*protos.Proposal, error) {
	ccHdrExt := &protos.ChaincodeHeaderExtension{}
	ccHdrExtBytes, err := proto.Marshal(ccHdrExt)
	if err != nil {
		return nil, err
	}

	cisBytes, err := proto.Marshal(cis)
	if err != nil {
		return nil, err
	}

	ccPropPayload := &protos.ChaincodeProposalPayload{Input: cisBytes}
	ccPropPayloadBytes, err := proto.Marshal(ccPropPayload)
	if err != nil {
		return nil, err
	}

	hdr := &protos.Header{Type: protos.Header_CHAINCODE, Extensions: ccHdrExtBytes}
	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	return &protos.Proposal{Header: hdrBytes, Payload: ccPropPayloadBytes}, nil
}

// CreateProposalResponse create's the underlying payload objects in a TransactionAction
func CreateProposalResponse(typ protos.Header_Type, prop *protos.Proposal, ccEvents *protos.ChaincodeEvent, simulationResults []byte, endorsement *protos.Endorsement) (*protos.ProposalResponse, error) {
	if typ != protos.Header_CHAINCODE {
		panic("-----Only CHAINCODE Type is supported-----")
	}

	var err error
	var cceventsBytes []byte

	if ccEvents != nil {
		cceventsBytes, err = proto.Marshal(ccEvents)
		if err != nil {
			return nil, err
		}
	}

	ext := &protos.ChaincodeAction{Results: simulationResults, Events: cceventsBytes}
	extBytes, err := proto.Marshal(ext)
	if err != nil {
		return nil, err
	}

	pRespPayload := &protos.ProposalResponsePayload{ProposalHash: []byte("TODO-use proposal to generate hash"), Epoch: []byte("TODO-compute epoch"), Extension: extBytes}

	pRespPayloadBytes, err := proto.Marshal(pRespPayload)
	if err != nil {
		return nil, err
	}

	propResp := &protos.ProposalResponse{Payload: pRespPayloadBytes, Endorsement: endorsement}

	return propResp, nil
}
