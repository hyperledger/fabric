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
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
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

func GetHeader(prop *protos.Proposal) (*protos.Header, error) {
	hdr := &protos.Header{}
	err := proto.Unmarshal(prop.Header, hdr)
	if err != nil {
		return nil, err
	}

	return hdr, nil
}

func GetChaincodeHeaderExtension(hdr *protos.Header) (*protos.ChaincodeHeaderExtension, error) {
	chaincodeHdrExt := &protos.ChaincodeHeaderExtension{}
	err := proto.Unmarshal(hdr.Extensions, chaincodeHdrExt)
	if err != nil {
		return nil, err
	}

	return chaincodeHdrExt, nil
}

func GetProposalResponse(prBytes []byte) (*protos.ProposalResponse, error) {
	proposalResponse := &protos.ProposalResponse{}
	err := proto.Unmarshal(prBytes, proposalResponse)
	if err != nil {
		return nil, err
	}

	return proposalResponse, nil
}

//getChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func GetChaincodeDeploymentSpec(code []byte) (*protos.ChaincodeDeploymentSpec, error) {
	cds := &protos.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, err
	}

	return cds, nil
}

func GetChaincodeAction(caBytes []byte) (*protos.ChaincodeAction, error) {
	chaincodeAction := &protos.ChaincodeAction{}
	err := proto.Unmarshal(caBytes, chaincodeAction)
	if err != nil {
		return nil, err
	}

	return chaincodeAction, nil
}

func GetProposalResponsePayload(prpBytes []byte) (*protos.ProposalResponsePayload, error) {
	prp := &protos.ProposalResponsePayload{}
	err := proto.Unmarshal(prpBytes, prp)
	if err != nil {
		return nil, err
	}

	return prp, nil
}

// CreateChaincodeProposal creates a proposal from given input
func CreateChaincodeProposal(cis *protos.ChaincodeInvocationSpec, creator []byte) (*protos.Proposal, error) {
	ccHdrExt := &protos.ChaincodeHeaderExtension{ChaincodeID: cis.ChaincodeSpec.ChaincodeID}
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

	// generate a random nonce
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		return nil, err
	}

	hdr := &protos.Header{Type: protos.Header_CHAINCODE,
		Extensions: ccHdrExtBytes,
		Nonce:      nonce,
		Creator:    creator}
	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	return &protos.Proposal{Header: hdrBytes, Payload: ccPropPayloadBytes}, nil
}

func GetBytesProposalResponsePayload(hash []byte, epoch []byte, result []byte, event []byte) ([]byte, error) {
	cAct := &protos.ChaincodeAction{Events: event, Results: result}
	cActBytes, err := proto.Marshal(cAct)
	if err != nil {
		return nil, err
	}

	prp := &protos.ProposalResponsePayload{Epoch: epoch, Extension: cActBytes, ProposalHash: hash}
	prpBytes, err := proto.Marshal(prp)
	if err != nil {
		return nil, err
	}

	return prpBytes, nil
}

func GetBytesChaincodeProposalPayload(cpp *protos.ChaincodeProposalPayload) ([]byte, error) {
	cppBytes, err := proto.Marshal(cpp)
	if err != nil {
		return nil, err
	}

	return cppBytes, nil
}

func GetBytesChaincodeEvent(event *protos.ChaincodeEvent) ([]byte, error) {
	eventBytes, err := proto.Marshal(event)
	if err != nil {
		return nil, err
	}

	return eventBytes, nil
}

func GetBytesProposalResponse(prpBytes []byte, endorsement *protos.Endorsement) ([]byte, error) {
	resp := &protos.ProposalResponse{
		// Timestamp: TODO!
		Version:     1, // TODO: pick right version number
		Endorsement: endorsement,
		Payload:     prpBytes,
		Response:    &protos.Response2{Status: 200, Message: "OK"}}
	respBytes, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

func GetProposalHash(header []byte, ccPropPayl []byte, visibility []byte) ([]byte, error) {
	// unmarshal the chaincode proposal payload
	cpp := &protos.ChaincodeProposalPayload{}
	err := proto.Unmarshal(ccPropPayl, cpp)
	if err != nil {
		return nil, errors.New("Failure while unmarshalling the ChaincodeProposalPayload!")
	}

	// strip the transient bytes off the payload - this needs to be done no matter the visibility mode
	cppNoTransient := &protos.ChaincodeProposalPayload{Input: cpp.Input, Transient: nil}
	cppBytes, err := GetBytesChaincodeProposalPayload(cppNoTransient)
	if err != nil {
		return nil, errors.New("Failure while marshalling the ChaincodeProposalPayload!")
	}

	// TODO: handle payload visibility - it needs to be defined first!
	// here, as an example, I'll code the visibility policy that allows the
	// full header but only the hash of the payload

	// TODO: use bccsp interfaces and providers as soon as they are ready!
	hash1 := primitives.GetDefaultHash()()
	hash1.Write(cppBytes) // hash the serialized ChaincodeProposalPayload object (stripped of the transient bytes)
	hash2 := primitives.GetDefaultHash()()
	hash2.Write(header)         // hash the serialized Header object
	hash2.Write(hash1.Sum(nil)) // hash the hash of the serialized ChaincodeProposalPayload object

	return hash2.Sum(nil), nil
}
