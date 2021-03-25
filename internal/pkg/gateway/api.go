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
	"github.com/hyperledger/fabric-protos-go/common"
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
func (gs *Server) Evaluate(ctx context.Context, request *gp.EvaluateRequest) (*gp.EvaluateResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "an evaluate request is required")
	}
	signedProposal := request.GetProposedTransaction()
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(signedProposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}

	endorsers, err := gs.registry.endorsers(channel, chaincodeID)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}
	if len(endorsers) == 0 {
		return nil, status.Errorf(codes.NotFound, "no endorsing peers found for channel: %s", request.ChannelId)
	}

	endorser := endorsers[0] // The peer with highest ledger height is first in the list

	ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
	defer cancel()

	response, err := endorser.client.ProcessProposal(ctx, signedProposal)
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"failed to evaluate transaction",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}

	retVal, err := getTransactionResponse(response)
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"transaction evaluation error",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}
	evaluateResponse := &gp.EvaluateResponse{
		Result: retVal,
	}
	return evaluateResponse, nil
}

// Endorse will collect endorsements by invoking the transaction function specified in the SignedProposal against
// sufficient Peers to satisfy the endorsement policy.
func (gs *Server) Endorse(ctx context.Context, request *gp.EndorseRequest) (*gp.EndorseResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "an endorse request is required")
	}
	signedProposal := request.GetProposedTransaction()
	if signedProposal == nil {
		return nil, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.ProposalBytes)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}
	channel, chaincodeID, err := getChannelAndChaincodeFromSignedProposal(signedProposal)
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
			switch {
			case err != nil:
				responseCh <- &endorserResponse{err: endpointError(e, err)}
			case response.Response.Status < 200 || response.Response.Status >= 400:
				// this is an error case and will be returned in the error details to the client
				responseCh <- &endorserResponse{err: endpointError(e, fmt.Errorf("error %d, %s", response.Response.Status, response.Response.Message))}
			default:
				responseCh <- &endorserResponse{pr: response}
			}
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

	retVal, err := getTransactionResponse(responses[0])
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to extract transaction response: %s", err)
	}

	endorseResponse := &gp.EndorseResponse{
		Result:              retVal,
		PreparedTransaction: env,
	}
	return endorseResponse, nil
}

// Submit will send the signed transaction to the ordering service. The response indicates whether the transaction was
// successfully received by the orderer. This does not imply successful commit of the transaction, only that is has
// been delivered to the orderer.
func (gs *Server) Submit(ctx context.Context, request *gp.SubmitRequest) (*gp.SubmitResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "a submit request is required")
	}
	txn := request.GetPreparedTransaction()
	if txn == nil {
		return nil, status.Error(codes.InvalidArgument, "a prepared transaction is required")
	}
	if len(txn.Signature) == 0 {
		return nil, status.Error(codes.InvalidArgument, "prepared transaction must be signed")
	}
	orderers, err := gs.registry.orderers(request.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	if len(orderers) == 0 {
		return nil, status.Errorf(codes.NotFound, "no broadcastClients discovered")
	}

	orderer := orderers[0] // send to first orderer for now

	broadcast, err := orderer.client.Broadcast(ctx)
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"failed to send transaction to orderer",
			&gp.EndpointError{Address: orderer.address, MspId: orderer.mspid, Message: err.Error()},
		)
	}
	logger.Info("Submitting txn to orderer")
	if err := broadcast.Send(txn); err != nil {
		return nil, rpcError(
			codes.Aborted,
			"failed to send transaction to orderer",
			&gp.EndpointError{Address: orderer.address, MspId: orderer.mspid, Message: err.Error()},
		)
	}

	response, err := broadcast.Recv()
	if err != nil {
		return nil, rpcError(
			codes.Aborted,
			"failed to receive response from orderer",
			&gp.EndpointError{Address: orderer.address, MspId: orderer.mspid, Message: err.Error()},
		)
	}

	if response == nil {
		return nil, status.Error(codes.Aborted, "received nil response from orderer")
	}

	if response.Status != common.Status_SUCCESS {
		return nil, status.Errorf(codes.Aborted, "received unsuccessful response from orderer: %s", common.Status_name[int32(response.Status)])
	}

	return &gp.SubmitResponse{}, nil
}

// CommitStatus returns the validation code for a specific transaction on a specific channel. If the transaction is
// already committed, the status will be returned immediately; otherwise this call will block and return only when
// the transaction commits or the context is cancelled.
//
// If the transaction commit status cannot be returned, for example if the specified channel does not exist, a
// FailedPrecondition error will be returned.
func (gs *Server) CommitStatus(ctx context.Context, request *gp.CommitStatusRequest) (*gp.CommitStatusResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "a commit status request is required")
	}

	txStatus, err := gs.commitFinder.TransactionStatus(ctx, request.ChannelId, request.TransactionId)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	response := &gp.CommitStatusResponse{
		Result: txStatus,
	}
	return response, nil
}

func endpointError(e *endorser, err error) *gp.EndpointError {
	return &gp.EndpointError{Address: e.address, MspId: e.mspid, Message: err.Error()}
}
