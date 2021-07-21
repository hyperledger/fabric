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
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
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
	channel, chaincodeID, _, err := getChannelAndChaincodeFromSignedProposal(signedProposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}

	err = gs.registry.registerChannel(channel)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	endorser, err := gs.registry.evaluator(channel, chaincodeID, request.GetTargetOrganizations())
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
	defer cancel()

	response, err := endorser.client.ProcessProposal(ctx, signedProposal)
	if err != nil {
		logger.Debugw("Evaluate call to endorser failed", "channel", request.ChannelId, "txid", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "error", err)
		return nil, rpcError(
			codes.Aborted,
			"failed to evaluate transaction",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}

	retVal, err := getTransactionResponse(response)
	if err != nil {
		logger.Debugw("Evaluate call to endorser returned failure", "channel", request.ChannelId, "txid", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "error", err)
		return nil, rpcError(
			codes.Aborted,
			"transaction evaluation error",
			&gp.EndpointError{Address: endorser.address, MspId: endorser.mspid, Message: err.Error()},
		)
	}
	evaluateResponse := &gp.EvaluateResponse{
		Result: retVal,
	}

	logger.Debugw("Evaluate call to endorser returned success", "channel", request.ChannelId, "txid", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "status", retVal.GetStatus(), "message", retVal.GetMessage())
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
	channel, chaincodeID, hasTransientData, err := getChannelAndChaincodeFromSignedProposal(signedProposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}

	err = gs.registry.registerChannel(channel)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
	defer cancel()

	defaultInterest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{{
			Name: chaincodeID,
		}},
	}

	var endorsers []*endorser
	var responses []*peer.ProposalResponse
	if len(request.EndorsingOrganizations) > 0 {
		// The client is specifying the endorsing orgs and taking responsibility for ensuring it meets the signature policy
		endorsers, err = gs.registry.endorsersForOrgs(channel, chaincodeID, request.EndorsingOrganizations)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "%s", err)
		}
	} else {
		// The client is delegating choice of endorsers to the gateway.

		// 1. Choose an endorser from the gateway's organization
		var firstEndorser *endorser
		es, ok := gs.registry.endorsersByOrg(channel, chaincodeID)[gs.registry.localEndorser.mspid]
		if !ok {
			// No local org endorsers for this channel/chaincode. If transient data is involved, return error
			if hasTransientData {
				return nil, status.Error(codes.FailedPrecondition, "no endorsers found in the gateway's organization; retry specifying endorsing organization(s) to protect transient data")
			}
			// Otherwise, just let discovery pick one.
			endorsers, err = gs.registry.endorsers(channel, defaultInterest, "")
			if err != nil {
				return nil, status.Errorf(codes.Unavailable, "%s", err)
			}
			firstEndorser = endorsers[0]
		} else {
			firstEndorser = es[0].endorser
		}

		gs.logger.Debugw("Sending to first endorser:", "channel", channel, "chaincode", chaincodeID, "MSPID", firstEndorser.mspid, "endpoint", firstEndorser.address)

		// 2. Process the proposal on this endorser
		firstResponse, err := firstEndorser.client.ProcessProposal(ctx, signedProposal)
		if err != nil {
			return nil, rpcError(codes.Aborted, "failed to endorse transaction", endpointError(firstEndorser, err))
		}
		if firstResponse.Response.Status < 200 || firstResponse.Response.Status >= 400 {
			return nil, rpcError(codes.Aborted, "failed to endorse transaction", endpointError(firstEndorser, fmt.Errorf("error %d, %s", firstResponse.Response.Status, firstResponse.Response.Message)))
		}

		// 3. Extract ChaincodeInterest and SBE policies
		// The chaincode interest could be nil for legacy peers and for chaincode functions that don't produce a read-write set
		interest := firstResponse.Interest
		if len(interest.GetChaincodes()) == 0 {
			interest = defaultInterest
		}

		// 4. If transient data is involved, then we need to ensure that discovery only returns orgs which own the collections involved.
		// Do this by setting NoPrivateReads to false on each collection
		if hasTransientData {
			for _, call := range interest.GetChaincodes() {
				call.NoPrivateReads = false
			}
		}

		// 5. Get a set of endorsers from discovery via the registry
		// The preferred discovery layout will contain the firstEndorser's Org.
		endorsers, err = gs.registry.endorsers(channel, interest, firstEndorser.mspid)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "%s", err)
		}

		// 6. Remove the gateway org's endorser, since we've already done that
		for i, e := range endorsers {
			if e.mspid == firstEndorser.mspid {
				endorsers = append(endorsers[:i], endorsers[i+1:]...)
				responses = append(responses, firstResponse)
				break
			}
		}
	}

	var wg sync.WaitGroup
	responseCh := make(chan *endorserResponse, len(endorsers))
	// send to all the endorsers
	for _, e := range endorsers {
		wg.Add(1)
		go func(e *endorser) {
			defer wg.Done()
			gs.logger.Debugw("Sending to endorser:", "channel", channel, "chaincode", chaincodeID, "MSPID", e.mspid, "endpoint", e.address)
			response, err := e.client.ProcessProposal(ctx, signedProposal)
			switch {
			case err != nil:
				logger.Debugw("Endorse call to endorser failed", "channel", request.ChannelId, "txid", request.TransactionId, "numEndorsers", len(endorsers), "endorserAddress", e.endpointConfig.address, "endorserMspid", e.endpointConfig.mspid, "error", err)
				responseCh <- &endorserResponse{err: endpointError(e, err)}
			case response.Response.Status < 200 || response.Response.Status >= 400:
				// this is an error case and will be returned in the error details to the client
				logger.Debugw("Endorse call to endorser returned failure", "channel", request.ChannelId, "txid", request.TransactionId, "numEndorsers", len(endorsers), "endorserAddress", e.endpointConfig.address, "endorserMspid", e.endpointConfig.mspid, "status", response.Response.Status, "message", response.Response.Message)
				responseCh <- &endorserResponse{err: endpointError(e, fmt.Errorf("error %d, %s", response.Response.Status, response.Response.Message))}
			default:
				logger.Debugw("Endorse call to endorser returned success", "channel", request.ChannelId, "txid", request.TransactionId, "numEndorsers", len(endorsers), "endorserAddress", e.endpointConfig.address, "endorserMspid", e.endpointConfig.mspid, "status", response.Response.Status, "message", response.Response.Message)
				responseCh <- &endorserResponse{pr: response}
			}
		}(e)
	}
	wg.Wait()
	close(responseCh)

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
			"failed to create BroadcastClient",
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
func (gs *Server) CommitStatus(ctx context.Context, signedRequest *gp.SignedCommitStatusRequest) (*gp.CommitStatusResponse, error) {
	if signedRequest == nil {
		return nil, status.Error(codes.InvalidArgument, "a commit status request is required")
	}

	request := &gp.CommitStatusRequest{}
	if err := proto.Unmarshal(signedRequest.Request, request); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid status request")
	}

	signedData := &protoutil.SignedData{
		Data:      signedRequest.Request,
		Identity:  request.Identity,
		Signature: signedRequest.Signature,
	}
	if err := gs.policy.CheckACL(resources.Gateway_CommitStatus, request.ChannelId, signedData); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	txStatus, err := gs.commitFinder.TransactionStatus(ctx, request.ChannelId, request.TransactionId)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	response := &gp.CommitStatusResponse{
		Result:      txStatus.Code,
		BlockNumber: txStatus.BlockNumber,
	}
	return response, nil
}

// ChaincodeEvents supplies a stream of responses, each containing all the events emitted by the requested chaincode
// for a specific block. The streamed responses are ordered by ascending block number. Responses are only returned for
// blocks that contain the requested events, while blocks not containing any of the requested events are skipped. The
// events within each response message are presented in the same order that the transactions that emitted them appear
// within the block.
func (gs *Server) ChaincodeEvents(signedRequest *gp.SignedChaincodeEventsRequest, stream gp.Gateway_ChaincodeEventsServer) error {
	if signedRequest == nil {
		return status.Error(codes.InvalidArgument, "a chaincode events request is required")
	}

	request := &gp.ChaincodeEventsRequest{}
	if err := proto.Unmarshal(signedRequest.Request, request); err != nil {
		return status.Error(codes.InvalidArgument, "invalid chaincode events request")
	}

	signedData := &protoutil.SignedData{
		Data:      signedRequest.Request,
		Identity:  request.Identity,
		Signature: signedRequest.Signature,
	}
	if err := gs.policy.CheckACL(resources.Gateway_ChaincodeEvents, request.ChannelId, signedData); err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	events, err := gs.eventer.ChaincodeEvents(stream.Context(), request.ChannelId, request.ChaincodeId)
	if err != nil {
		return status.Error(codes.FailedPrecondition, err.Error())
	}

	for event := range events {
		response := &gp.ChaincodeEventsResponse{
			BlockNumber: event.BlockNumber,
			Events:      event.Events,
		}
		if err := stream.Send(response); err != nil {
			return err // Likely stream closed by the client
		}
	}

	// If stream is still open, this was a server-side error; otherwise client won't see it anyway
	return status.Error(codes.Unavailable, "failed to read events")
}

func endpointError(e *endorser, err error) *gp.EndpointError {
	return &gp.EndpointError{Address: e.address, MspId: e.mspid, Message: err.Error()}
}
