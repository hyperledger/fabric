/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

func getValueFromResponse(response *peer.ProposalResponse) (*gateway.Result, error) {
	var retVal []byte

	if response != nil && response.Payload != nil {
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
	if signedProposal == nil {
		return "", "", fmt.Errorf("a signed proposal is required")
	}
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
