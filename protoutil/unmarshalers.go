/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
)

// the implicit contract of all these unmarshalers is that they
// will return a non-nil pointer whenever the error is nil

// UnmarshalBlock unmarshals bytes to a Block
func UnmarshalBlock(encoded []byte) (*common.Block, error) {
	block := &common.Block{}
	err := proto.Unmarshal(encoded, block)
	return block, errors.Wrap(err, "error unmarshalling Block")
}

// UnmarshalChaincodeDeploymentSpec unmarshals bytes to a ChaincodeDeploymentSpec
func UnmarshalChaincodeDeploymentSpec(code []byte) (*peer.ChaincodeDeploymentSpec, error) {
	cds := &peer.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(code, cds)
	return cds, errors.Wrap(err, "error unmarshalling ChaincodeDeploymentSpec")
}

// UnmarshalChaincodeInvocationSpec unmarshals bytes to a ChaincodeInvocationSpec
func UnmarshalChaincodeInvocationSpec(encoded []byte) (*peer.ChaincodeInvocationSpec, error) {
	cis := &peer.ChaincodeInvocationSpec{}
	err := proto.Unmarshal(encoded, cis)
	return cis, errors.Wrap(err, "error unmarshalling ChaincodeInvocationSpec")
}

// UnmarshalPayload unmarshals bytes to a Payload
func UnmarshalPayload(encoded []byte) (*common.Payload, error) {
	payload := &common.Payload{}
	err := proto.Unmarshal(encoded, payload)
	return payload, errors.Wrap(err, "error unmarshalling Payload")
}

// UnmarshalEnvelope unmarshals bytes to a Envelope
func UnmarshalEnvelope(encoded []byte) (*common.Envelope, error) {
	envelope := &common.Envelope{}
	err := proto.Unmarshal(encoded, envelope)
	return envelope, errors.Wrap(err, "error unmarshalling Envelope")
}

// UnmarshalChannelHeader unmarshals bytes to a ChannelHeader
func UnmarshalChannelHeader(bytes []byte) (*common.ChannelHeader, error) {
	chdr := &common.ChannelHeader{}
	err := proto.Unmarshal(bytes, chdr)
	return chdr, errors.Wrap(err, "error unmarshalling ChannelHeader")
}

// UnmarshalChaincodeID unmarshals bytes to a ChaincodeID
func UnmarshalChaincodeID(bytes []byte) (*peer.ChaincodeID, error) {
	ccid := &peer.ChaincodeID{}
	err := proto.Unmarshal(bytes, ccid)
	return ccid, errors.Wrap(err, "error unmarshalling ChaincodeID")
}

// UnmarshalSignatureHeader unmarshals bytes to a SignatureHeader
func UnmarshalSignatureHeader(bytes []byte) (*common.SignatureHeader, error) {
	sh := &common.SignatureHeader{}
	err := proto.Unmarshal(bytes, sh)
	return sh, errors.Wrap(err, "error unmarshalling SignatureHeader")
}

// UnmarshalIdentifierHeader unmarshals bytes to an IdentifierHeader
func UnmarshalIdentifierHeader(bytes []byte) (*common.IdentifierHeader, error) {
	ih := &common.IdentifierHeader{}
	err := proto.Unmarshal(bytes, ih)
	return ih, errors.Wrap(err, "error unmarshalling IdentifierHeader")
}

func UnmarshalSerializedIdentity(bytes []byte) (*msp.SerializedIdentity, error) {
	sid := &msp.SerializedIdentity{}
	err := proto.Unmarshal(bytes, sid)
	return sid, errors.Wrap(err, "error unmarshalling SerializedIdentity")
}

// UnmarshalHeader unmarshals bytes to a Header
func UnmarshalHeader(bytes []byte) (*common.Header, error) {
	hdr := &common.Header{}
	err := proto.Unmarshal(bytes, hdr)
	return hdr, errors.Wrap(err, "error unmarshalling Header")
}

// UnmarshalConfigEnvelope unmarshals bytes to a ConfigEnvelope
func UnmarshalConfigEnvelope(bytes []byte) (*common.ConfigEnvelope, error) {
	cfg := &common.ConfigEnvelope{}
	err := proto.Unmarshal(bytes, cfg)
	return cfg, errors.Wrap(err, "error unmarshalling ConfigEnvelope")
}

// UnmarshalChaincodeHeaderExtension unmarshals bytes to a ChaincodeHeaderExtension
func UnmarshalChaincodeHeaderExtension(hdrExtension []byte) (*peer.ChaincodeHeaderExtension, error) {
	chaincodeHdrExt := &peer.ChaincodeHeaderExtension{}
	err := proto.Unmarshal(hdrExtension, chaincodeHdrExt)
	return chaincodeHdrExt, errors.Wrap(err, "error unmarshalling ChaincodeHeaderExtension")
}

// UnmarshalProposalResponse unmarshals bytes to a ProposalResponse
func UnmarshalProposalResponse(prBytes []byte) (*peer.ProposalResponse, error) {
	proposalResponse := &peer.ProposalResponse{}
	err := proto.Unmarshal(prBytes, proposalResponse)
	return proposalResponse, errors.Wrap(err, "error unmarshalling ProposalResponse")
}

// UnmarshalChaincodeAction unmarshals bytes to a ChaincodeAction
func UnmarshalChaincodeAction(caBytes []byte) (*peer.ChaincodeAction, error) {
	chaincodeAction := &peer.ChaincodeAction{}
	err := proto.Unmarshal(caBytes, chaincodeAction)
	return chaincodeAction, errors.Wrap(err, "error unmarshalling ChaincodeAction")
}

// UnmarshalResponse unmarshals bytes to a Response
func UnmarshalResponse(resBytes []byte) (*peer.Response, error) {
	response := &peer.Response{}
	err := proto.Unmarshal(resBytes, response)
	return response, errors.Wrap(err, "error unmarshalling Response")
}

// UnmarshalChaincodeEvents unmarshals bytes to a ChaincodeEvent
func UnmarshalChaincodeEvents(eBytes []byte) (*peer.ChaincodeEvent, error) {
	chaincodeEvent := &peer.ChaincodeEvent{}
	err := proto.Unmarshal(eBytes, chaincodeEvent)
	return chaincodeEvent, errors.Wrap(err, "error unmarshalling ChaicnodeEvent")
}

// UnmarshalProposalResponsePayload unmarshals bytes to a ProposalResponsePayload
func UnmarshalProposalResponsePayload(prpBytes []byte) (*peer.ProposalResponsePayload, error) {
	prp := &peer.ProposalResponsePayload{}
	err := proto.Unmarshal(prpBytes, prp)
	return prp, errors.Wrap(err, "error unmarshalling ProposalResponsePayload")
}

// UnmarshalProposal unmarshals bytes to a Proposal
func UnmarshalProposal(propBytes []byte) (*peer.Proposal, error) {
	prop := &peer.Proposal{}
	err := proto.Unmarshal(propBytes, prop)
	return prop, errors.Wrap(err, "error unmarshalling Proposal")
}

// UnmarshalTransaction unmarshals bytes to a Transaction
func UnmarshalTransaction(txBytes []byte) (*peer.Transaction, error) {
	tx := &peer.Transaction{}
	err := proto.Unmarshal(txBytes, tx)
	return tx, errors.Wrap(err, "error unmarshalling Transaction")
}

// UnmarshalChaincodeActionPayload unmarshals bytes to a ChaincodeActionPayload
func UnmarshalChaincodeActionPayload(capBytes []byte) (*peer.ChaincodeActionPayload, error) {
	cap := &peer.ChaincodeActionPayload{}
	err := proto.Unmarshal(capBytes, cap)
	return cap, errors.Wrap(err, "error unmarshalling ChaincodeActionPayload")
}

// UnmarshalChaincodeProposalPayload unmarshals bytes to a ChaincodeProposalPayload
func UnmarshalChaincodeProposalPayload(bytes []byte) (*peer.ChaincodeProposalPayload, error) {
	cpp := &peer.ChaincodeProposalPayload{}
	err := proto.Unmarshal(bytes, cpp)
	return cpp, errors.Wrap(err, "error unmarshalling ChaincodeProposalPayload")
}

// UnmarshalTxReadWriteSet unmarshals bytes to a TxReadWriteSet
func UnmarshalTxReadWriteSet(bytes []byte) (*rwset.TxReadWriteSet, error) {
	rws := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(bytes, rws)
	return rws, errors.Wrap(err, "error unmarshalling TxReadWriteSet")
}

// UnmarshalKVRWSet unmarshals bytes to a KVRWSet
func UnmarshalKVRWSet(bytes []byte) (*kvrwset.KVRWSet, error) {
	rws := &kvrwset.KVRWSet{}
	err := proto.Unmarshal(bytes, rws)
	return rws, errors.Wrap(err, "error unmarshalling KVRWSet")
}

// UnmarshalHashedRWSet unmarshals bytes to a HashedRWSet
func UnmarshalHashedRWSet(bytes []byte) (*kvrwset.HashedRWSet, error) {
	hrws := &kvrwset.HashedRWSet{}
	err := proto.Unmarshal(bytes, hrws)
	return hrws, errors.Wrap(err, "error unmarshalling HashedRWSet")
}

// UnmarshalSignaturePolicy unmarshals bytes to a SignaturePolicyEnvelope
func UnmarshalSignaturePolicy(bytes []byte) (*common.SignaturePolicyEnvelope, error) {
	sp := &common.SignaturePolicyEnvelope{}
	err := proto.Unmarshal(bytes, sp)
	return sp, errors.Wrap(err, "error unmarshalling SignaturePolicyEnvelope")
}

// UnmarshalPayloadOrPanic unmarshals bytes to a Payload structure or panics
// on error
func UnmarshalPayloadOrPanic(encoded []byte) *common.Payload {
	payload, err := UnmarshalPayload(encoded)
	if err != nil {
		panic(err)
	}
	return payload
}

// UnmarshalEnvelopeOrPanic unmarshals bytes to an Envelope structure or panics
// on error
func UnmarshalEnvelopeOrPanic(encoded []byte) *common.Envelope {
	envelope, err := UnmarshalEnvelope(encoded)
	if err != nil {
		panic(err)
	}
	return envelope
}

// UnmarshalBlockOrPanic unmarshals bytes to an Block or panics
// on error
func UnmarshalBlockOrPanic(encoded []byte) *common.Block {
	block, err := UnmarshalBlock(encoded)
	if err != nil {
		panic(err)
	}
	return block
}

// UnmarshalChannelHeaderOrPanic unmarshals bytes to a ChannelHeader or panics
// on error
func UnmarshalChannelHeaderOrPanic(bytes []byte) *common.ChannelHeader {
	chdr, err := UnmarshalChannelHeader(bytes)
	if err != nil {
		panic(err)
	}
	return chdr
}

// UnmarshalSignatureHeaderOrPanic unmarshals bytes to a SignatureHeader or panics
// on error
func UnmarshalSignatureHeaderOrPanic(bytes []byte) *common.SignatureHeader {
	sighdr, err := UnmarshalSignatureHeader(bytes)
	if err != nil {
		panic(err)
	}
	return sighdr
}
