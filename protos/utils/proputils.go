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
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// GetChaincodeInvocationSpec get the ChaincodeInvocationSpec from the proposal
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

// GetHeader Get Header from bytes
func GetHeader(bytes []byte) (*common.Header, error) {
	hdr := &common.Header{}
	err := proto.Unmarshal(bytes, hdr)
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

// GetChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
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

// GetPayload Get Payload from Envelope message
func GetPayload(e *common.Envelope) (*common.Payload, error) {
	payload := &common.Payload{}
	err := proto.Unmarshal(e.Payload, payload)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

// GetTransaction Get Transaction from bytes
func GetTransaction(txBytes []byte) (*peer.Transaction, error) {
	tx := &peer.Transaction{}
	err := proto.Unmarshal(txBytes, tx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// GetChaincodeActionPayload Get ChaincodeActionPayload from bytes
func GetChaincodeActionPayload(capBytes []byte) (*peer.ChaincodeActionPayload, error) {
	cap := &peer.ChaincodeActionPayload{}
	err := proto.Unmarshal(capBytes, cap)
	if err != nil {
		return nil, err
	}

	return cap, nil
}

// GetChaincodeProposalPayload Get ChaincodeProposalPayload from bytes
func GetChaincodeProposalPayload(bytes []byte) (*peer.ChaincodeProposalPayload, error) {
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(bytes, cpp)
	if err != nil {
		return nil, err
	}

	return cpp, nil
}

// GetSignatureHeader Get SignatureHeader from bytes
func GetSignatureHeader(bytes []byte) (*common.SignatureHeader, error) {
	sh := &common.SignatureHeader{}
	err := proto.Unmarshal(bytes, sh)
	if err != nil {
		return nil, err
	}

	return sh, nil
}

// GetEnvelope Get Envelope from bytes
func GetEnvelope(bytes []byte) (*common.Envelope, error) {
	env := &common.Envelope{}
	err := proto.Unmarshal(bytes, env)
	if err != nil {
		return nil, err
	}

	return env, nil
}

// CreateChaincodeProposal creates a proposal from given input
func CreateChaincodeProposal(txid string, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, error) {
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
		TxID:      txid,
		ChainID:   chainID,
		Extension: ccHdrExtBytes},
		SignatureHeader: &common.SignatureHeader{Nonce: nonce, Creator: creator}}

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

// GetBytesChaincodeActionPayload get the bytes of ChaincodeActionPayload from the message
func GetBytesChaincodeActionPayload(cap *peer.ChaincodeActionPayload) ([]byte, error) {
	capBytes, err := proto.Marshal(cap)
	if err != nil {
		return nil, err
	}

	return capBytes, nil
}

// GetBytesProposalResponse gets propoal bytes response
func GetBytesProposalResponse(pr *peer.ProposalResponse) ([]byte, error) {
	respBytes, err := proto.Marshal(pr)
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

// GetBytesHeader get the bytes of Header from the message
func GetBytesHeader(hdr *common.Header) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// GetBytesSignatureHeader get the bytes of SignatureHeader from the message
func GetBytesSignatureHeader(hdr *common.SignatureHeader) ([]byte, error) {
	bytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// GetBytesTransaction get the bytes of Transaction from the message
func GetBytesTransaction(tx *peer.Transaction) ([]byte, error) {
	bytes, err := proto.Marshal(tx)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// GetBytesPayload get the bytes of Payload from the message
func GetBytesPayload(payl *common.Payload) ([]byte, error) {
	bytes, err := proto.Marshal(payl)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// GetBytesEnvelope get the bytes of Envelope from the message
func GetBytesEnvelope(env *common.Envelope) ([]byte, error) {
	bytes, err := proto.Marshal(env)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// GetActionFromEnvelope extracts a ChaincodeAction message from a serialized Envelope
func GetActionFromEnvelope(envBytes []byte) (*peer.ChaincodeAction, error) {
	env, err := GetEnvelope(envBytes)
	if err != nil {
		return nil, err
	}

	payl, err := GetPayload(env)
	if err != nil {
		return nil, err
	}

	tx, err := GetTransaction(payl.Data)
	if err != nil {
		return nil, err
	}

	_, respPayload, err := GetPayloads(tx.Actions[0])
	return respPayload, err
}

// CreateProposalFromCIS returns a proposal given a serialized identity and a ChaincodeInvocationSpec
func CreateProposalFromCIS(txid string, chainID string, cis *peer.ChaincodeInvocationSpec, creator []byte) (*peer.Proposal, error) {
	return CreateChaincodeProposal(txid, chainID, cis, creator)
}

// CreateProposalFromCDS returns a proposal given a serialized identity and a ChaincodeDeploymentSpec
func CreateProposalFromCDS(txid string, chainID string, cds *peer.ChaincodeDeploymentSpec, creator []byte) (*peer.Proposal, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	//wrap the deployment in an invocation spec to lccc...
	lcccSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeID: &peer.ChaincodeID{Name: "lccc"},
			CtorMsg:     &peer.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte(chainID), b}}}}

	//...and get the proposal for it
	return CreateProposalFromCIS(txid, chainID, lcccSpec, creator)
}
