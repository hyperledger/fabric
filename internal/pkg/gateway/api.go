/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"

	gp "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

// Evaluate will invoke the transaction function as specified in the SignedProposal
func (gs *Server) Evaluate(ctx context.Context, proposedTransaction *gp.ProposedTransaction) (*gp.Result, error) {
	if proposedTransaction == nil {
		return nil, fmt.Errorf("a proposed transaction is required")
	}
	signedProposal := proposedTransaction.Proposal
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		// TODO need to specify status codes
		return nil, fmt.Errorf("failed to unpack channel header: %w", err)
	}

	endorsers, err := gs.registry.endorsers(channel, chaincodeID)
	if err != nil {
		return nil, err
	}
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
func (gs *Server) Endorse(ctx context.Context, proposedTransaction *gp.ProposedTransaction) (*gp.PreparedTransaction, error) {
	if proposedTransaction == nil {
		return nil, fmt.Errorf("a proposed transaction is required")
	}
	signedProposal := proposedTransaction.Proposal
	if signedProposal == nil {
		return nil, fmt.Errorf("the proposed transaction must contain a signed proposal")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return nil, err
	}
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack channel header: %w", err)
	}
	endorsers, err := gs.registry.endorsers(channel, chaincodeID)
	if err != nil {
		return nil, err
	}

	var responses []*peer.ProposalResponse
	// send to all the endorsers - TODO fan out in parallel
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

	preparedTxn := &gp.PreparedTransaction{
		TxId:      proposedTransaction.TxId,
		ChannelId: channel,
		Response:  retVal,
		Envelope:  env,
	}
	return preparedTxn, nil
}

// Submit will send the signed transaction to the ordering service.  The output stream will close
// once the transaction is committed on a sufficient number of remoteEndorsers according to a defined policy.
func (gs *Server) Submit(txn *gp.PreparedTransaction, cs gp.Gateway_SubmitServer) error {
	if txn == nil {
		return fmt.Errorf("a signed prepared transaction is required")
	}
	if cs == nil {
		return fmt.Errorf("a submit server is required")
	}
	orderers, err := gs.registry.orderers(txn.ChannelId)
	if err != nil {
		return err
	}

	if len(orderers) == 0 {
		return fmt.Errorf("no broadcastClients discovered")
	}

	// send to first orderer for now
	logger.Info("Submitting txn to orderer")
	if err := orderers[0].Send(txn.Envelope); err != nil {
		return fmt.Errorf("failed to send envelope to orderer: %w", err)
	}

	response, err := orderers[0].Recv()
	if err != nil {
		return err
	}

	if response == nil {
		return fmt.Errorf("received nil response from orderer")
	}

	return cs.Send(&gp.Event{
		Value: []byte(response.Info),
	})
}
