/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getTransactionResponse(response *peer.ProposalResponse) (*peer.Response, error) {
	var retVal *peer.Response

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
			if extension.Response.Status < 200 || extension.Response.Status >= 400 {
				return nil, fmt.Errorf("error %d, %s", extension.Response.Status, extension.Response.Message)
			}
			retVal = extension.Response
		}
	}

	return retVal, nil
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

func rpcError(code codes.Code, message string, details ...proto.Message) error {
	st := status.New(code, message)
	if len(details) != 0 {
		std, err := st.WithDetails(details...)
		if err == nil {
			return std.Err()
		} // otherwise return the error without the details
	}
	return st.Err()
}
