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
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

//GetChaincodeInvocationSpec get the ChaincodeInvocationSpec from the proposal
func GetChaincodeInvocationSpec(prop *peer.Proposal) (*peer.ChaincodeInvocationSpec, error) {
	txhdr := &common.Header{}
	err := proto.Unmarshal(prop.Header, txhdr)
	if err != nil {
		return nil, err
	}
	if common.HeaderType(txhdr.ChainHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		return nil, fmt.Errorf("invalid proposal type expected ENDORSER_TRANSACTION")
	}
	ccPropPayload := &peer.ChaincodeProposalPayload{}
	err = proto.Unmarshal(prop.Payload, ccPropPayload)
	if err != nil {
		return nil, err
	}
	cis := &peer.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(ccPropPayload.Input, cis)
	if err != nil {
		return nil, err
	}
	return cis, nil
}

// GetHeader from proposal
func GetHeader(prop *peer.Proposal) (*common.Header, error) {
	hdr := &common.Header{}
	err := proto.Unmarshal(prop.Header, hdr)
	if err != nil {
		return nil, err
	}

	return hdr, nil
}

// GetChaincodeHeaderExtension get chaincode header extension given header
func GetChaincodeHeaderExtension(hdr *common.Header) (*peer.ChaincodeHeaderExtension, error) {
	if common.HeaderType(hdr.ChainHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		return nil, fmt.Errorf("invalid proposal type expected ENDORSER_TRANSACTION")
	}

	chaincodeHdrExt := &peer.ChaincodeHeaderExtension{}
	err := proto.Unmarshal(hdr.ChainHeader.Extension, chaincodeHdrExt)
	if err != nil {
		return nil, err
	}

	return chaincodeHdrExt, nil
}

// GetProposalResponse given proposal in bytes
func GetProposalResponse(prBytes []byte) (*peer.ProposalResponse, error) {
	proposalResponse := &peer.ProposalResponse{}
	err := proto.Unmarshal(prBytes, proposalResponse)
	if err != nil {
		return nil, err
	}

	return proposalResponse, nil
}

//GetChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func GetChaincodeDeploymentSpec(code []byte) (*peer.ChaincodeDeploymentSpec, error) {
	cds := &peer.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, err
	}

	return cds, nil
}

// GetChaincodeAction gets the ChaincodeAction given chaicnode action bytes
func GetChaincodeAction(caBytes []byte) (*peer.ChaincodeAction, error) {
	chaincodeAction := &peer.ChaincodeAction{}
	err := proto.Unmarshal(caBytes, chaincodeAction)
	if err != nil {
		return nil, err
	}

	return chaincodeAction, nil
}

// GetProposalResponsePayload gets the proposal response payload
func GetProposalResponsePayload(prpBytes []byte) (*peer.ProposalResponsePayload, error) {
	prp := &peer.ProposalResponsePayload{}
	err := proto.Unmarshal(prpBytes, prp)
	if err != nil {
		return nil, err
	}

	return prp, nil
}

// GetProposal returns a Proposal message from its bytes
func GetProposal(propBytes []byte) (*peer.Proposal, error) {
	prop := &peer.Proposal{}
	err := proto.Unmarshal(propBytes, prop)
	if err != nil {
		return nil, err
	}

	return prop, nil
}

// CreateChaincodeProposal creates a proposal from given input
func CreateChaincodeProposal(cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, error) {
	ccHdrExt := &peer.ChaincodeHeaderExtension{ChaincodeID: cis.ChaincodeSpec.ChaincodeID}
	ccHdrExtBytes, err := proto.Marshal(ccHdrExt)
	if err != nil {
		return nil, err
	}

	cisBytes, err := proto.Marshal(cis)
	if err != nil {
		return nil, err
	}

	ccPropPayload := &peer.ChaincodeProposalPayload{Input: cisBytes}
	ccPropPayloadBytes, err := proto.Marshal(ccPropPayload)
	if err != nil {
		return nil, err
	}

	// generate a random nonce
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		return nil, err
	}

	hdr := &common.Header{ChainHeader: &common.ChainHeader{Type: int32(common.HeaderType_ENDORSER_TRANSACTION),
		Extension: ccHdrExtBytes},
		SignatureHeader: &common.SignatureHeader{Nonce: nonce,
			Creator: creator}}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	return &peer.Proposal{Header: hdrBytes, Payload: ccPropPayloadBytes}, nil
}

// GetBytesProposalResponsePayload gets proposal response payload
func GetBytesProposalResponsePayload(hash []byte, result []byte, event []byte) ([]byte, error) {
	cAct := &peer.ChaincodeAction{Events: event, Results: result}
	cActBytes, err := proto.Marshal(cAct)
	if err != nil {
		return nil, err
	}

	prp := &peer.ProposalResponsePayload{Extension: cActBytes, ProposalHash: hash}
	prpBytes, err := proto.Marshal(prp)
	if err != nil {
		return nil, err
	}

	return prpBytes, nil
}

// GetBytesChaincodeProposalPayload gets the chaincode proposal payload
func GetBytesChaincodeProposalPayload(cpp *peer.ChaincodeProposalPayload) ([]byte, error) {
	cppBytes, err := proto.Marshal(cpp)
	if err != nil {
		return nil, err
	}

	return cppBytes, nil
}

// GetBytesChaincodeEvent gets the bytes of ChaincodeEvent
func GetBytesChaincodeEvent(event *peer.ChaincodeEvent) ([]byte, error) {
	eventBytes, err := proto.Marshal(event)
	if err != nil {
		return nil, err
	}

	return eventBytes, nil
}

// GetBytesProposalResponse gets propoal bytes response
func GetBytesProposalResponse(prpBytes []byte, endorsement *peer.Endorsement) ([]byte, error) {
	resp := &peer.ProposalResponse{
		// Timestamp: TODO!
		Version:     1, // TODO: pick right version number
		Endorsement: endorsement,
		Payload:     prpBytes,
		Response:    &peer.Response2{Status: 200, Message: "OK"}}
	respBytes, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// GetBytesProposal returns the bytes of a proposal message
func GetBytesProposal(prop *peer.Proposal) ([]byte, error) {
	propBytes, err := proto.Marshal(prop)
	if err != nil {
		return nil, err
	}

	return propBytes, nil
}

// GetProposalHash gets the proposal hash
func GetProposalHash(header []byte, ccPropPayl []byte, visibility []byte) ([]byte, error) {
	// unmarshal the chaincode proposal payload
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(ccPropPayl, cpp)
	if err != nil {
		return nil, errors.New("Failure while unmarshalling the ChaincodeProposalPayload!")
	}

	// strip the transient bytes off the payload - this needs to be done no matter the visibility mode
	cppNoTransient := &peer.ChaincodeProposalPayload{Input: cpp.Input, Transient: nil}
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
