/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type endorserResponse struct {
	pr  *peer.ProposalResponse
	err *gp.EndpointError
}

// Evaluate will invoke the transaction function as specified in the SignedProposal
func (gs *Server) Evaluate(ctx context.Context, proposedTransaction *gp.ProposedTransaction) (*gp.Result, error) {
	if proposedTransaction == nil {
		return nil, status.Error(codes.InvalidArgument, "a proposed transaction is required")
	}
	signedProposal := proposedTransaction.Proposal
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}

	endorsers, err := gs.registry.endorsers(channel, chaincodeID)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}
	if len(endorsers) == 0 {
		return nil, status.Errorf(codes.NotFound, "no endorsing peers found for channel: %s", proposedTransaction.ChannelId)
	}

	endorser := endorsers[0] // TODO choose suitable peer based on block height, etc (future user story)
	response, err := endorser.client.ProcessProposal(ctx, signedProposal)
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"failed to evaluate transaction",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}

	value, err := getValueFromResponse(response)
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"transaction evaluation error",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}
	return value, nil
}

// Endorse will collect endorsements by invoking the transaction function specified in the SignedProposal against
// sufficient Peers to satisfy the endorsement policy.
func (gs *Server) Endorse(ctx context.Context, proposedTransaction *gp.ProposedTransaction) (*gp.PreparedTransaction, error) {
	if proposedTransaction == nil {
		return nil, status.Error(codes.InvalidArgument, "a proposed transaction is required")
	}
	signedProposal := proposedTransaction.Proposal
	if signedProposal == nil {
		return nil, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(proposedTransaction.Proposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}
	endorsers, err := gs.registry.endorsers(channel, chaincodeID)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
	defer cancel()

	var wg sync.WaitGroup
	responseCh := make(chan *endorserResponse, len(endorsers))
	// send to all the endorsers
	for _, e := range endorsers {
		wg.Add(1)
		go func(e *endorser) {
			defer wg.Done()
			response, err := e.client.ProcessProposal(ctx, signedProposal)
			var detail *gp.EndpointError
			if err != nil {
				detail = &gp.EndpointError{Address: e.address, MspId: e.mspid, Message: err.Error()}
				responseCh <- &endorserResponse{err: detail}
				return
			}
			if response.Response.Status < 200 || response.Response.Status >= 400 {
				// this is an error case and will be returned in the error details to the client
				detail = &gp.EndpointError{Address: e.address, MspId: e.mspid, Message: fmt.Sprintf("error %d, %s", response.Response.Status, response.Response.Message)}
			}
			responseCh <- &endorserResponse{pr: response, err: detail}
		}(e)
	}
	wg.Wait()
	close(responseCh)

	var responses []*peer.ProposalResponse
	var errorDetails []proto.Message
	for response := range responseCh {
		if response.err != nil {
			errorDetails = append(errorDetails, response.err)
		} else {
			responses = append(responses, response.pr)
		}
	}

	if len(errorDetails) != 0 {
		return nil, rpcError(codes.Aborted, "failed to endorse transaction", errorDetails...)
	}

	env, err := protoutil.CreateTx(proposal, responses...)
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to assemble transaction: %s", err)
	}

	retVal, err := getValueFromResponse(responses[0])
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to extract value from response payload: %s", err)
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
		return status.Error(codes.InvalidArgument, "a signed prepared transaction is required")
	}
	if cs == nil {
		return status.Error(codes.Internal, "a submit server is required")
	}
	orderers, err := gs.registry.orderers(txn.ChannelId)
	if err != nil {
		return status.Errorf(codes.Unavailable, "%s", err)
	}

	if len(orderers) == 0 {
		return status.Errorf(codes.NotFound, "no broadcastClients discovered")
	}

	orderer := orderers[0] // send to first orderer for now
	logger.Info("Submitting txn to orderer")
	if err := orderer.client.Send(txn.Envelope); err != nil {
		return rpcError(
			codes.Aborted,
			"failed to send transaction to orderer",
			&gp.EndpointError{Address: orderer.address, MspId: orderer.mspid, Message: err.Error()},
		)
	}

	response, err := orderer.client.Recv()
	if err != nil {
		return rpcError(
			codes.Aborted,
			"failed to receive response from orderer",
			&gp.EndpointError{Address: orderer.address, MspId: orderer.mspid, Message: err.Error()},
		)
	}

	if response == nil {
		return status.Error(codes.Aborted, "received nil response from orderer")
	}

	return cs.Send(&gp.Event{
		Value: []byte(response.Info),
	})
}
