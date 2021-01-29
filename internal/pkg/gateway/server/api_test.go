/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	cp "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/gateway"
	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	idmocks "github.com/hyperledger/fabric/internal/pkg/identity/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestEvaluate(t *testing.T) {
	proposal := createProposal(t)

	mockSigner := &idmocks.SignerSerializer{}
	mockSigner.SignReturns([]byte("my_signature"), nil)

	server := &Server{
		registry: newMockRegistry(t),
	}

	sp, err := protoutil.GetSignedProposal(proposal, mockSigner)
	require.NoError(t, err, "Failed to sign the proposal")

	ptx := &pb.ProposedTransaction{
		Proposal: sp,
	}

	result, err := server.Evaluate(context.TODO(), ptx)
	require.NoError(t, err, "Failed to evaluate the proposal")
	require.Equal(t, []byte("MyResult"), result.Value, "Incorrect result")
}

func TestEndorse(t *testing.T) {
	proposal := createProposal(t)

	mockSigner := &idmocks.SignerSerializer{}
	mockSigner.SignReturns([]byte("my_signature"), nil)

	server := &Server{
		registry: newMockRegistry(t),
	}

	sp, err := protoutil.GetSignedProposal(proposal, mockSigner)
	require.NoError(t, err, "Failed to sign the proposal")

	ptx := &pb.ProposedTransaction{
		Proposal: sp,
	}

	result, err := server.Endorse(context.TODO(), ptx)
	require.NoError(t, err, "Failed to prepare the transaction")
	require.Equal(t, []byte("MyResult"), result.Response.Value, "Incorrect response")
}

func newMockRegistry(t *testing.T) *mocks.Registry {
	endorser1 := &mocks.Endorser{}
	endorser1.ProcessProposalReturns(createProposalResponse("MyResult", t), nil)
	endorsers := []gateway.Endorser{endorser1}

	mockRegistry := &mocks.Registry{}
	mockRegistry.EndorsersReturns(endorsers)
	return mockRegistry
}

func createProposal(t *testing.T) *peer.Proposal {
	invocationSpec := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_NODE,
			ChaincodeId: &peer.ChaincodeID{Name: "my_chaincode"},
			Input:       &peer.ChaincodeInput{Args: nil},
		},
	}

	proposal, _, err := protoutil.CreateChaincodeProposal(
		cp.HeaderType_ENDORSER_TRANSACTION,
		"mychannel",
		invocationSpec,
		[]byte{},
	)

	require.NoError(t, err, "Failed to create the proposal")

	return proposal
}

func createProposalResponse(value string, t *testing.T) *peer.ProposalResponse {
	response := &peer.Response{
		Status:  200,
		Payload: []byte(value),
	}
	action := &peer.ChaincodeAction{
		Response: response,
	}
	payload := &peer.ProposalResponsePayload{
		ProposalHash: []byte{},
		Extension:    marshal(action, t),
	}
	endorsement := &peer.Endorsement{}

	return &peer.ProposalResponse{
		Payload:     marshal(payload, t),
		Response:    response,
		Endorsement: endorsement,
	}
}

func marshal(msg proto.Message, t *testing.T) []byte {
	buf, err := proto.Marshal(msg)
	require.NoError(t, err, "Failed to marshal message")
	return buf
}
