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

	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// GetPayloads get's the underlying payload objects in a TransactionAction
func GetPayloads(txActions *peer.TransactionAction) (*peer.ChaincodeActionPayload, *peer.ChaincodeAction, error) {
	// TODO: pass in the tx type (in what follows we're assuming the type is ENDORSER_TRANSACTION)
	ccPayload := &peer.ChaincodeActionPayload{}
	err := proto.Unmarshal(txActions.Payload, ccPayload)
	if err != nil {
		return nil, nil, err
	}

	if ccPayload.Action == nil || ccPayload.Action.ProposalResponsePayload == nil {
		return nil, nil, fmt.Errorf("no payload in ChaincodeActionPayload")
	}
	pRespPayload := &peer.ProposalResponsePayload{}
	err = proto.Unmarshal(ccPayload.Action.ProposalResponsePayload, pRespPayload)
	if err != nil {
		return nil, nil, err
	}

	if pRespPayload.Extension == nil {
		return nil, nil, fmt.Errorf("response payload is missing extension")
	}

	respPayload := &peer.ChaincodeAction{}
	err = proto.Unmarshal(pRespPayload.Extension, respPayload)
	if err != nil {
		return ccPayload, nil, err
	}
	return ccPayload, respPayload, nil
}

// GetEnvelopeFromBlock gets an envelope from a block's Data field.
func GetEnvelopeFromBlock(data []byte) (*common.Envelope, error) {
	//Block always begins with an envelope
	var err error
	env := &common.Envelope{}
	if err = proto.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("Error getting envelope(%s)", err)
	}

	return env, nil
}

// CreateSignedEnvelope creates a signed envelope of the desired type, with marshaled dataMsg and signs it
func CreateSignedEnvelope(txType common.HeaderType, channelID string, signer crypto.LocalSigner, dataMsg proto.Message, msgVersion int32, epoch uint64) (*common.Envelope, error) {
	return CreateSignedEnvelopeWithTLSBinding(txType, channelID, signer, dataMsg, msgVersion, epoch, nil)
}

// CreateSignedEnvelopeWithTLSBinding creates a signed envelope of the desired type, with marshaled dataMsg and signs it.
// It also includes a TLS cert hash into the channel header
func CreateSignedEnvelopeWithTLSBinding(txType common.HeaderType, channelID string, signer crypto.LocalSigner, dataMsg proto.Message, msgVersion int32, epoch uint64, tlsCertHash []byte) (*common.Envelope, error) {
	payloadChannelHeader := MakeChannelHeader(txType, msgVersion, channelID, epoch)
	payloadChannelHeader.TlsCertHash = tlsCertHash
	var err error
	payloadSignatureHeader := &common.SignatureHeader{}

	if signer != nil {
		payloadSignatureHeader, err = signer.NewSignatureHeader()
		if err != nil {
			return nil, err
		}
	}

	data, err := proto.Marshal(dataMsg)
	if err != nil {
		return nil, err
	}

	paylBytes := MarshalOrPanic(&common.Payload{
		Header: MakePayloadHeader(payloadChannelHeader, payloadSignatureHeader),
		Data:   data,
	})

	var sig []byte
	if signer != nil {
		sig, err = signer.Sign(paylBytes)
		if err != nil {
			return nil, err
		}
	}

	return &common.Envelope{Payload: paylBytes, Signature: sig}, nil
}

// CreateSignedTx assembles an Envelope message from proposal, endorsements, and a signer.
// This function should be called by a client when it has collected enough endorsements
// for a proposal to create a transaction and submit it to peers for ordering
func CreateSignedTx(proposal *peer.Proposal, signer msp.SigningIdentity, resps ...*peer.ProposalResponse) (*common.Envelope, error) {
	if len(resps) == 0 {
		return nil, fmt.Errorf("At least one proposal response is necessary")
	}

	// the original header
	hdr, err := GetHeader(proposal.Header)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal the proposal header")
	}

	// the original payload
	pPayl, err := GetChaincodeProposalPayload(proposal.Payload)
	if err != nil {
		return nil, fmt.Errorf("Could not unmarshal the proposal payload")
	}

	// check that the signer is the same that is referenced in the header
	// TODO: maybe worth removing?
	signerBytes, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	shdr, err := GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	if bytes.Compare(signerBytes, shdr.Creator) != 0 {
		return nil, fmt.Errorf("The signer needs to be the same as the one referenced in the header")
	}

	// get header extensions so we have the visibility field
	hdrExt, err := GetChaincodeHeaderExtension(hdr)
	if err != nil {
		return nil, err
	}

	// ensure that all actions are bitwise equal and that they are successful
	var a1 []byte
	for n, r := range resps {
		if n == 0 {
			a1 = r.Payload
			if r.Response.Status != 200 {
				return nil, fmt.Errorf("Proposal response was not successful, error code %d, msg %s", r.Response.Status, r.Response.Message)
			}
			continue
		}

		if bytes.Compare(a1, r.Payload) != 0 {
			return nil, fmt.Errorf("ProposalResponsePayloads do not match")
		}
	}

	// fill endorsements
	endorsements := make([]*peer.Endorsement, len(resps))
	for n, r := range resps {
		endorsements[n] = r.Endorsement
	}

	// create ChaincodeEndorsedAction
	cea := &peer.ChaincodeEndorsedAction{ProposalResponsePayload: resps[0].Payload, Endorsements: endorsements}

	// obtain the bytes of the proposal payload that will go to the transaction
	propPayloadBytes, err := GetBytesProposalPayloadForTx(pPayl, hdrExt.PayloadVisibility)
	if err != nil {
		return nil, err
	}

	// serialize the chaincode action payload
	cap := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: propPayloadBytes, Action: cea}
	capBytes, err := GetBytesChaincodeActionPayload(cap)
	if err != nil {
		return nil, err
	}

	// create a transaction
	taa := &peer.TransactionAction{Header: hdr.SignatureHeader, Payload: capBytes}
	taas := make([]*peer.TransactionAction, 1)
	taas[0] = taa
	tx := &peer.Transaction{Actions: taas}

	// serialize the tx
	txBytes, err := GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	// create the payload
	payl := &common.Payload{Header: hdr, Data: txBytes}
	paylBytes, err := GetBytesPayload(payl)
	if err != nil {
		return nil, err
	}

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	// here's the envelope
	return &common.Envelope{Payload: paylBytes, Signature: sig}, nil
}

// CreateProposalResponse creates a proposal response.
func CreateProposalResponse(hdrbytes []byte, payl []byte, response *peer.Response, results []byte, events []byte, ccid *peer.ChaincodeID, visibility []byte, signingEndorser msp.SigningIdentity) (*peer.ProposalResponse, error) {
	hdr, err := GetHeader(hdrbytes)
	if err != nil {
		return nil, err
	}

	// obtain the proposal hash given proposal header, payload and the requested visibility
	pHashBytes, err := GetProposalHash1(hdr, payl, visibility)
	if err != nil {
		return nil, fmt.Errorf("Could not compute proposal hash: err %s", err)
	}

	// get the bytes of the proposal response payload - we need to sign them
	prpBytes, err := GetBytesProposalResponsePayload(pHashBytes, response, results, events, ccid)
	if err != nil {
		return nil, errors.New("Failure while marshaling the ProposalResponsePayload")
	}

	// serialize the signing identity
	endorser, err := signingEndorser.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Could not serialize the signing identity for %s, err %s", signingEndorser.GetIdentifier(), err)
	}

	// sign the concatenation of the proposal response and the serialized endorser identity with this endorser's key
	signature, err := signingEndorser.Sign(append(prpBytes, endorser...))
	if err != nil {
		return nil, fmt.Errorf("Could not sign the proposal response payload, err %s", err)
	}

	resp := &peer.ProposalResponse{
		// Timestamp: TODO!
		Version:     1, // TODO: pick right version number
		Endorsement: &peer.Endorsement{Signature: signature, Endorser: endorser},
		Payload:     prpBytes,
		Response:    &peer.Response{Status: 200, Message: "OK"}}

	return resp, nil
}

// CreateProposalResponseFailure creates a proposal response for cases where
// endorsement proposal fails either due to a endorsement failure or a chaincode
// failure (chaincode response status >= shim.ERRORTHRESHOLD)
func CreateProposalResponseFailure(hdrbytes []byte, payl []byte, response *peer.Response, results []byte, events []byte, ccid *peer.ChaincodeID, visibility []byte) (*peer.ProposalResponse, error) {
	hdr, err := GetHeader(hdrbytes)
	if err != nil {
		return nil, err
	}

	// obtain the proposal hash given proposal header, payload and the requested visibility
	pHashBytes, err := GetProposalHash1(hdr, payl, visibility)
	if err != nil {
		return nil, fmt.Errorf("Could not compute proposal hash: err %s", err)
	}

	// get the bytes of the proposal response payload
	prpBytes, err := GetBytesProposalResponsePayload(pHashBytes, response, results, events, ccid)
	if err != nil {
		return nil, errors.New("Failure while marshaling the ProposalResponsePayload")
	}

	resp := &peer.ProposalResponse{
		// Timestamp: TODO!
		Payload:  prpBytes,
		Response: &peer.Response{Status: 500, Message: "Chaincode Error"}}

	return resp, nil
}

// GetSignedProposal returns a signed proposal given a Proposal message and a signing identity
func GetSignedProposal(prop *peer.Proposal, signer msp.SigningIdentity) (*peer.SignedProposal, error) {
	// check for nil argument
	if prop == nil || signer == nil {
		return nil, fmt.Errorf("Nil arguments")
	}

	propBytes, err := GetBytesProposal(prop)
	if err != nil {
		return nil, err
	}

	signature, err := signer.Sign(propBytes)
	if err != nil {
		return nil, err
	}

	return &peer.SignedProposal{ProposalBytes: propBytes, Signature: signature}, nil
}

// GetSignedEvent returns a signed event given an Event message and a signing identity
func GetSignedEvent(evt *peer.Event, signer msp.SigningIdentity) (*peer.SignedEvent, error) {
	// check for nil argument
	if evt == nil || signer == nil {
		return nil, errors.New("nil arguments")
	}

	evtBytes, err := proto.Marshal(evt)
	if err != nil {
		return nil, err
	}

	signature, err := signer.Sign(evtBytes)
	if err != nil {
		return nil, err
	}

	return &peer.SignedEvent{EventBytes: evtBytes, Signature: signature}, nil
}

// MockSignedEndorserProposalOrPanic creates a SignedProposal with the passed arguments
func MockSignedEndorserProposalOrPanic(chainID string, cs *peer.ChaincodeSpec, creator, signature []byte) (*peer.SignedProposal, *peer.Proposal) {
	prop, _, err := CreateChaincodeProposal(
		common.HeaderType_ENDORSER_TRANSACTION,
		chainID,
		&peer.ChaincodeInvocationSpec{ChaincodeSpec: cs},
		creator)
	if err != nil {
		panic(err)
	}

	propBytes, err := GetBytesProposal(prop)
	if err != nil {
		panic(err)
	}

	return &peer.SignedProposal{ProposalBytes: propBytes, Signature: signature}, prop
}

func MockSignedEndorserProposal2OrPanic(chainID string, cs *peer.ChaincodeSpec, signer msp.SigningIdentity) (*peer.SignedProposal, *peer.Proposal) {
	serializedSigner, err := signer.Serialize()
	if err != nil {
		panic(err)
	}

	prop, _, err := CreateChaincodeProposal(
		common.HeaderType_ENDORSER_TRANSACTION,
		chainID,
		&peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{}},
		serializedSigner)
	if err != nil {
		panic(err)
	}

	sProp, err := GetSignedProposal(prop, signer)
	if err != nil {
		panic(err)
	}

	return sProp, prop
}

// GetBytesProposalPayloadForTx takes a ChaincodeProposalPayload and returns its serialized
// version according to the visibility field
func GetBytesProposalPayloadForTx(payload *peer.ChaincodeProposalPayload, visibility []byte) ([]byte, error) {
	// check for nil argument
	if payload == nil /* || visibility == nil */ {
		return nil, fmt.Errorf("Nil arguments")
	}

	// strip the transient bytes off the payload - this needs to be done no matter the visibility mode
	cppNoTransient := &peer.ChaincodeProposalPayload{Input: payload.Input, TransientMap: nil}
	cppBytes, err := GetBytesChaincodeProposalPayload(cppNoTransient)
	if err != nil {
		return nil, errors.New("Failure while marshalling the ChaincodeProposalPayload!")
	}

	// currently the fabric only supports full visibility: this means that
	// there are no restrictions on which parts of the proposal payload will
	// be visible in the final transaction; this default approach requires
	// no additional instructions in the PayloadVisibility field; however
	// the fabric may be extended to encode more elaborate visibility
	// mechanisms that shall be encoded in this field (and handled
	// appropriately by the peer)

	return cppBytes, nil
}

// GetProposalHash2 gets the proposal hash - this version
// is called by the committer where the visibility policy
// has already been enforced and so we already get what
// we have to get in ccPropPayl
func GetProposalHash2(header *common.Header, ccPropPayl []byte) ([]byte, error) {
	// check for nil argument
	if header == nil ||
		header.ChannelHeader == nil ||
		header.SignatureHeader == nil ||
		ccPropPayl == nil {
		return nil, fmt.Errorf("Nil arguments")
	}

	hash, err := factory.GetDefault().GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		return nil, fmt.Errorf("Failed instantiating hash function [%s]", err)
	}
	hash.Write(header.ChannelHeader)   // hash the serialized Channel Header object
	hash.Write(header.SignatureHeader) // hash the serialized Signature Header object
	hash.Write(ccPropPayl)             // hash the bytes of the chaincode proposal payload that we are given

	return hash.Sum(nil), nil
}

// GetProposalHash1 gets the proposal hash bytes after sanitizing the
// chaincode proposal payload according to the rules of visibility
func GetProposalHash1(header *common.Header, ccPropPayl []byte, visibility []byte) ([]byte, error) {
	// check for nil argument
	if header == nil ||
		header.ChannelHeader == nil ||
		header.SignatureHeader == nil ||
		ccPropPayl == nil /* || visibility == nil */ {
		return nil, fmt.Errorf("Nil arguments")
	}

	// unmarshal the chaincode proposal payload
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(ccPropPayl, cpp)
	if err != nil {
		return nil, errors.New("Failure while unmarshalling the ChaincodeProposalPayload!")
	}

	ppBytes, err := GetBytesProposalPayloadForTx(cpp, visibility)
	if err != nil {
		return nil, err
	}

	hash2, err := factory.GetDefault().GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		return nil, fmt.Errorf("Failed instantiating hash function [%s]", err)
	}
	hash2.Write(header.ChannelHeader)   // hash the serialized Channel Header object
	hash2.Write(header.SignatureHeader) // hash the serialized Signature Header object
	hash2.Write(ppBytes)                // hash of the part of the chaincode proposal payload that will go to the tx

	return hash2.Sum(nil), nil
}
