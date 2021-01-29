/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric/protoutil"

	pb "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
)

// Evaluate will invoke the transaction function as specified in the SignedProposal
func (gs *Server) Evaluate(ctx context.Context, proposedTransaction *pb.ProposedTransaction) (*pb.Result, error) {
	signedProposal := proposedTransaction.Proposal
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack channel header: %w", err)
	}

	endorsers := gs.registry.Endorsers(channel, chaincodeID)
	if len(endorsers) == 0 {
		return nil, fmt.Errorf("no endorsing peers found for channel: %s", proposedTransaction.ChannelId)
	}
	response, err := endorsers[0].ProcessProposal(ctx, signedProposal) // TODO choose suitable peer based on block height, etc (future user story)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate transaction: %w", err)
	}

	return getValueFromResponse(response)
}

// Endorse will collect endorsements by invoking the transaction function specified in the SignedProposal against
// sufficient Peers to satisfy the endorsement policy.
func (gs *Server) Endorse(ctx context.Context, proposedTransaction *pb.ProposedTransaction) (*pb.PreparedTransaction, error) {
	signedProposal := proposedTransaction.Proposal
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return nil, err
	}
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack channel header: %w", err)
	}
	endorsers := gs.registry.Endorsers(channel, chaincodeID)

	var responses []*peer.ProposalResponse
	// send to all the endorsers
	for _, endorser := range endorsers {
		response, err := endorser.ProcessProposal(ctx, signedProposal)
		if err != nil {
			return nil, fmt.Errorf("failed to process proposal: %w", err)
		}
		responses = append(responses, response)
	}

	env, err := createUnsignedTx(proposal, responses...)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble transaction: %w", err)
	}

	retVal, err := getValueFromResponse(responses[0])
	if err != nil {
		return nil, fmt.Errorf("failed to extract value from response payload: %w", err)
	}

	preparedTxn := &pb.PreparedTransaction{
		TxId:      proposedTransaction.TxId,
		ChannelId: proposedTransaction.ChannelId,
		Response:  retVal,
		Envelope:  env,
	}
	return preparedTxn, nil
}

// Submit will send the signed transaction to the ordering service.  The output stream will close
// once the transaction is committed on a sufficient number of peers according to a defined policy.
func (gs *Server) Submit(txn *pb.PreparedTransaction, cs pb.Gateway_SubmitServer) error {
	// not yet implemented in embedded gateway

	return fmt.Errorf("Submit() not implemented")
}
