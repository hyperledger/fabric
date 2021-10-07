/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getChannelAndChaincodeFromSignedProposal(signedProposal *peer.SignedProposal) (string, string, bool, error) {
	if signedProposal == nil {
		return "", "", false, fmt.Errorf("a signed proposal is required")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return "", "", false, err
	}
	header, err := protoutil.UnmarshalHeader(proposal.Header)
	if err != nil {
		return "", "", false, err
	}
	channelHeader, err := protoutil.UnmarshalChannelHeader(header.ChannelHeader)
	if err != nil {
		return "", "", false, err
	}
	payload, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload)
	if err != nil {
		return "", "", false, err
	}
	spec, err := protoutil.UnmarshalChaincodeInvocationSpec(payload.Input)
	if err != nil {
		return "", "", false, err
	}

	return channelHeader.ChannelId, spec.ChaincodeSpec.ChaincodeId.Name, len(payload.TransientMap) > 0, nil
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

func wrappedRpcError(err error, message string, details ...proto.Message) error {
	statusErr := status.Convert(err)
	return rpcError(statusErr.Code(), message+": "+statusErr.Message(), details...)
}

func errorDetail(e *endpointConfig, err error) *gp.ErrorDetail {
	return &gp.ErrorDetail{Address: e.address, MspId: e.mspid, Message: err.Error()}
}
