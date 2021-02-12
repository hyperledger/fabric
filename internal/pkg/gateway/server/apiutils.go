/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type nilSigner struct {
	creator []byte
}

func (s *nilSigner) Sign([]byte) ([]byte, error) {
	return nil, nil
}

func (s *nilSigner) Serialize() ([]byte, error) {
	return s.creator, nil
}

func createUnsignedTx(
	proposal *peer.Proposal,
	resps ...*peer.ProposalResponse,
) (*common.Envelope, error) {
	// extract the Creator from the signature header
	hdr, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return nil, err
	}
	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	// TODO the creation of this dummy signer containing the serialised creator from the Proposal
	// is required because protoutil.CreateSignedTx contains a check that they match.
	// However, there is a comment there about removing that check.  If removed, the code could be
	// refactored to remove the need for this kludge.
	dummySigner := &nilSigner{
		creator: shdr.Creator,
	}

	return protoutil.CreateSignedTx(proposal, dummySigner, resps...)
}

func getValueFromResponse(response *peer.ProposalResponse) (*gateway.Result, error) {
	var retVal []byte

	if response.Payload != nil {
		payload, err := protoutil.UnmarshalProposalResponsePayload(response.Payload)
		if err != nil {
			return nil, err
		}

		extension, err := protoutil.UnmarshalChaincodeAction(payload.Extension)
		if err != nil {
			return nil, err
		}

		if extension != nil && extension.Response != nil {
			if extension.Response.Status > 200 {
				return nil, fmt.Errorf("error %d, %s", extension.Response.Status, extension.Response.Message)
			}
			retVal = extension.Response.Payload
		}
	}

	return &gateway.Result{Value: retVal}, nil
}

func getChannelAndChaincodeFromSignedProposal(signedProposal *peer.SignedProposal) (string, string, error) {
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return "", "", err
	}
	header, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return "", "", err
	}
	channelHeader, err := protoutil.UnmarshalChannelHeader(header.ChannelHeader)
	if err != nil {
		return "", "", err
	}
	payload, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload)
	if err != nil {
		return "", "", err
	}
	spec, err := protoutil.UnmarshalChaincodeInvocationSpec(payload.Input)
	if err != nil {
		return "", "", err
	}

	return channelHeader.ChannelId, spec.ChaincodeSpec.ChaincodeId.Name, nil
}
