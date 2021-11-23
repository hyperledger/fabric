/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/internal/pkg/gateway/event"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type endorserResponse struct {
	endorsementSet []*peer.ProposalResponse
	err            *gp.ErrorDetail
	timeoutExpired bool
}

// Evaluate will invoke the transaction function as specified in the SignedProposal
func (gs *Server) Evaluate(ctx context.Context, request *gp.EvaluateRequest) (*gp.EvaluateResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "an evaluate request is required")
	}
	signedProposal := request.GetProposedTransaction()
	channel, chaincodeID, hasTransientData, err := getChannelAndChaincodeFromSignedProposal(signedProposal)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to unpack transaction proposal: %s", err)
	}

	err = gs.registry.connectChannelPeers(channel, false)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s", err)
	}

	targetOrgs := request.GetTargetOrganizations()
	transientProtected := false
	if hasTransientData && targetOrgs == nil {
		targetOrgs = []string{gs.registry.localEndorser.mspid}
		transientProtected = true
	}

	plan, err := gs.registry.evaluator(channel, chaincodeID, targetOrgs)
	if err != nil {
		if transientProtected {
			return nil, status.Errorf(codes.FailedPrecondition, "no endorsers found in the gateway's organization; retry specifying target organization(s) to protect transient data: %s", err)
		}
		return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
	}

	endorser := plan.endorsers()[0]
	var response *peer.Response
	var errDetails []proto.Message
	for response == nil {
		gs.logger.Debugw("Sending to peer:", "channel", channel, "chaincode", chaincodeID, "txID", request.TransactionId, "MSPID", endorser.mspid, "endpoint", endorser.address)

		done := make(chan error)
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
			defer cancel()
			pr, err := endorser.client.ProcessProposal(ctx, signedProposal)
			code, message, retry, remove := gs.responseStatus(pr, err)
			if code == codes.OK {
				response = pr.Response
				// Prefer result from proposal response as Response.Payload is not required to be transaction result
				if result, err := getResultFromProposalResponse(pr); err == nil {
					response.Payload = result
				} else {
					logger.Warnw("Successful proposal response contained no transaction result", "error", err.Error(), "chaincode", chaincodeID, "channel", channel, "txID", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "status", response.Status, "message", response.Message)
				}
			} else {
				logger.Debugw("Evaluate call to endorser failed", "chaincode", chaincodeID, "channel", channel, "txID", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "error", message)
				errDetails = append(errDetails, errorDetail(endorser.endpointConfig, message))
				if remove {
					gs.registry.removeEndorser(endorser)
				}
				if retry {
					endorser = plan.nextPeerInGroup(endorser)
				} else {
					done <- newRpcError(code, "evaluate call to endorser returned error: "+message, errDetails...)
				}
				if endorser == nil {
					done <- newRpcError(code, "failed to evaluate transaction, see attached details for more info", errDetails...)
				}
			}
		}()
		select {
		case status := <-done:
			if status != nil {
				return nil, status
			}
		case <-ctx.Done():
			// Overall evaluation timeout expired
			logger.Warnw("Evaluate call timed out while processing request", "channel", request.ChannelId, "txID", request.TransactionId)
			return nil, newRpcError(codes.DeadlineExceeded, "evaluate timeout expired")
		}
	}

	evaluateResponse := &gp.EvaluateResponse{
		Result: response,
	}

	logger.Debugw("Evaluate call to endorser returned success", "channel", request.ChannelId, "txID", request.TransactionId, "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "status", response.GetStatus(), "message", response.GetMessage())
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

	defaultInterest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{{
			Name: chaincodeID,
		}},
	}

	var plan *plan
	var endorsements []*peer.ProposalResponse
	if len(request.EndorsingOrganizations) > 0 {
		// The client is specifying the endorsing orgs and taking responsibility for ensuring it meets the signature policy
		plan, err = gs.registry.planForOrgs(channel, chaincodeID, request.EndorsingOrganizations)
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "%s", err)
		}
	} else {
		// The client is delegating choice of endorsers to the gateway.

		// 1. Choose an endorser from the gateway's organization
		plan, err = gs.registry.planForOrgs(channel, chaincodeID, []string{gs.registry.localEndorser.mspid})
		if err != nil {
			// No local org endorsers for this channel/chaincode. If transient data is involved, return error
			if hasTransientData {
				return nil, status.Error(codes.FailedPrecondition, "no endorsers found in the gateway's organization; retry specifying endorsing organization(s) to protect transient data")
			}
			// Otherwise, just let discovery pick one.
			plan, err = gs.registry.endorsementPlan(channel, defaultInterest, nil)
			if err != nil {
				return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
			}
		}
		firstEndorser := plan.endorsers()[0]

		gs.logger.Debugw("Sending to first endorser:", "channel", channel, "chaincode", chaincodeID, "txID", request.TransactionId, "MSPID", firstEndorser.mspid, "endpoint", firstEndorser.address)

		// 2. Process the proposal on this endorser
		var firstResponse *peer.ProposalResponse
		var errDetails []proto.Message

		for firstResponse == nil && firstEndorser != nil {
			done := make(chan struct{})
			go func() {
				defer close(done)
				ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
				defer cancel()
				firstResponse, err = firstEndorser.client.ProcessProposal(ctx, signedProposal)
				code, message, _, remove := gs.responseStatus(firstResponse, err)

				if code != codes.OK {
					logger.Warnw("Endorse call to endorser failed", "channel", request.ChannelId, "txID", request.TransactionId, "endorserAddress", firstEndorser.endpointConfig.address, "endorserMspid", firstEndorser.endpointConfig.mspid, "error", message)
					errDetails = append(errDetails, errorDetail(firstEndorser.endpointConfig, message))
					if remove {
						gs.registry.removeEndorser(firstEndorser)
					}
					firstEndorser = plan.nextPeerInGroup(firstEndorser)
					firstResponse = nil
				}
			}()
			select {
			case <-done:
				// Endorser completed normally
			case <-ctx.Done():
				// Overall endorsement timeout expired
				logger.Warnw("Endorse call timed out while collecting first endorsement", "channel", request.ChannelId, "txID", request.TransactionId)
				return nil, newRpcError(codes.DeadlineExceeded, "endorsement timeout expired while collecting first endorsement")
			}
		}
		if firstEndorser == nil || firstResponse == nil {
			return nil, newRpcError(codes.Aborted, "failed to endorse transaction, see attached details for more info", errDetails...)
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
		plan, err = gs.registry.endorsementPlan(channel, interest, firstEndorser)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
		}

		// 6. Remove the gateway org's endorser, since we've already done that
		endorsements = plan.update(firstEndorser, firstResponse)
	}

	var errorDetails []proto.Message
	for endorsements == nil {
		// loop through the layouts until one gets satisfied
		endorsers := plan.endorsers()
		if endorsers == nil {
			// no more layouts
			break
		}
		var wg sync.WaitGroup
		responseCh := make(chan *endorserResponse, plan.size)
		// send to all the endorsers
		for _, e := range endorsers {
			wg.Add(1)
			go func(e *endorser) {
				defer wg.Done()
				for e != nil {
					done := make(chan *endorserResponse)
					go func() {
						defer close(done)
						gs.logger.Debugw("Sending to endorser:", "channel", channel, "chaincode", chaincodeID, "txID", request.TransactionId, "MSPID", e.mspid, "endpoint", e.address)
						ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout) // timeout of individual endorsement
						defer cancel()
						response, err := e.client.ProcessProposal(ctx, signedProposal)
						// Ignore the retry flag returned by the following responseStatus call. Endorse will retry until all endorsement layouts have been exhausted.
						// It tries to get a successful endorsement from each org and minimise the changes of a rogue peer scuppering the transaction.
						// If an org is behaving badly, it can move on to a different layout.
						code, message, _, remove := gs.responseStatus(response, err)
						if code == codes.OK {
							logger.Debugw("Endorse call to endorser returned success", "channel", request.ChannelId, "txID", request.TransactionId, "numEndorsers", len(endorsers), "endorserAddress", e.endpointConfig.address, "endorserMspid", e.endpointConfig.mspid, "status", response.Response.Status, "message", response.Response.Message)

							responseMessage := response.GetResponse()
							if responseMessage != nil {
								responseMessage.Payload = nil // Remove any duplicate response payload
							}

							endorsements := plan.update(e, response)
							done <- &endorserResponse{endorsementSet: endorsements}
						} else {
							logger.Warnw("Endorse call to endorser failed", "channel", request.ChannelId, "txID", request.TransactionId, "numEndorsers", len(endorsers), "endorserAddress", e.endpointConfig.address, "endorserMspid", e.endpointConfig.mspid, "error", message)
							if remove {
								gs.registry.removeEndorser(e)
							}
							done <- &endorserResponse{err: errorDetail(e.endpointConfig, message)}
						}
					}()
					select {
					case resp := <-done:
						// Endorser completed normally
						if resp.err != nil {
							e = plan.nextPeerInGroup(e)
						} else {
							e = nil
						}
						responseCh <- resp
					case <-ctx.Done():
						// Overall endorsement timeout expired
						responseCh <- &endorserResponse{timeoutExpired: true}
						return
					}
				}
			}(e)
		}
		wg.Wait()
		close(responseCh)

		for response := range responseCh {
			if response.timeoutExpired {
				logger.Warnw("Endorse call timed out while collecting endorsements", "channel", request.ChannelId, "txID", request.TransactionId, "numEndorsers", len(endorsers))
				return nil, newRpcError(codes.DeadlineExceeded, "endorsement timeout expired while collecting endorsements")
			}
			if response.endorsementSet != nil {
				endorsements = response.endorsementSet
				break
			}
			if response.err != nil {
				errorDetails = append(errorDetails, response.err)
			}
		}
	}

	if endorsements == nil {
		return nil, newRpcError(codes.Aborted, "failed to collect enough transaction endorsements, see attached details for more info", errorDetails...)
	}

	env, err := protoutil.CreateTx(proposal, endorsements...)
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to assemble transaction: %s", err)
	}

	endorseResponse := &gp.EndorseResponse{
		PreparedTransaction: env,
	}
	return endorseResponse, nil
}

// responseStatus unpacks the proposal response and error values that are returned from ProcessProposal and
// determines how the gateway should react (retry?, close connection?).
// Uses the grpc canonical status error codes and their recommended actions.
// Returns:
// - response status code, with codes.OK indicating success and other values indicating likely error type
// - error message extracted from the err or generated from 500 proposal response (string)
// - should the gateway retry (only the Evaluate() uses this) (bool)
// - should the gateway close the connection and remove the peer from its registry (bool)
func (gs *Server) responseStatus(response *peer.ProposalResponse, err error) (statusCode codes.Code, message string, retry bool, remove bool) {
	if err != nil {
		if response == nil {
			// there is no ProposalResponse, so this must have been generated by grpc in response to an unavailable peer
			// - close the connection and retry on another
			return codes.Unavailable, err.Error(), true, true
		}
		// there is a response and an err, so it must have been from the unpackProposal() or preProcess() stages
		// preProcess does all the signature and ACL checking. In either case, no point retrying, or closing the connection (it's a client error)
		return codes.FailedPrecondition, err.Error(), false, false
	}
	if response.Response.Status < 200 || response.Response.Status >= 400 {
		if response.Payload == nil && response.Response.Status == 500 {
			// there's a error 500 response but no payload, so the response was generated in the peer rather than the chaincode
			if strings.HasSuffix(response.Response.Message, chaincode.ErrorStreamTerminated) {
				// chaincode container crashed probably. Close connection and retry on another peer
				return codes.Aborted, response.Response.Message, true, true
			}
			// some other error - retry on another peer
			return codes.Aborted, response.Response.Message, true, false
		} else {
			// otherwise it must be an error response generated by the chaincode
			return codes.Unknown, fmt.Sprintf("chaincode response %d, %s", response.Response.Status, response.Response.Message), false, false
		}
	}
	// anything else is a success
	return codes.OK, "", false, false
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
		return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
	}

	if len(orderers) == 0 {
		return nil, status.Errorf(codes.Unavailable, "no orderer nodes available")
	}

	// try each orderer in random order
	var errDetails []proto.Message
	for _, index := range rand.Perm(len(orderers)) {
		orderer := orderers[index]
		logger.Infow("Sending transaction to orderer", "txID", request.TransactionId, "endpoint", orderer.address)
		err := gs.broadcast(ctx, orderer, txn)
		if err == nil {
			return &gp.SubmitResponse{}, nil
		}

		logger.Warnw("Error sending transaction to orderer", "txID", request.TransactionId, "endpoint", orderer.address, "err", err)
		errDetails = append(errDetails, errorDetail(orderer.endpointConfig, err.Error()))

		errStatus := toRpcStatus(err)
		if errStatus.Code() != codes.Unavailable {
			return nil, newRpcError(errStatus.Code(), errStatus.Message(), errDetails...)
		}
	}

	return nil, newRpcError(codes.Unavailable, "no orderers could successfully process transaction", errDetails...)
}

func (gs *Server) broadcast(ctx context.Context, orderer *orderer, txn *common.Envelope) error {
	broadcast, err := orderer.client.Broadcast(ctx)
	if err != nil {
		return err
	}

	if err := broadcast.Send(txn); err != nil {
		return err
	}

	response, err := broadcast.Recv()
	if err != nil {
		return err
	}

	if response.GetStatus() != common.Status_SUCCESS {
		return status.Errorf(codes.Aborted, "received unsuccessful response from orderer: %s", common.Status_name[int32(response.GetStatus())])
	}

	return nil
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
		return nil, status.Errorf(codes.InvalidArgument, "invalid status request: %v", err)
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
		return nil, toRpcError(err, codes.Aborted)
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
		return status.Errorf(codes.InvalidArgument, "invalid chaincode events request: %v", err)
	}

	signedData := &protoutil.SignedData{
		Data:      signedRequest.Request,
		Identity:  request.Identity,
		Signature: signedRequest.Signature,
	}
	if err := gs.policy.CheckACL(resources.Gateway_ChaincodeEvents, request.ChannelId, signedData); err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	ledger, err := gs.ledgerProvider.Ledger(request.GetChannelId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	startBlock, err := startBlockFromLedgerPosition(ledger, request.GetStartPosition())
	if err != nil {
		return err
	}

	ledgerIter, err := ledger.GetBlocksIterator(startBlock)
	if err != nil {
		return status.Error(codes.Aborted, err.Error())
	}

	eventsIter := event.NewChaincodeEventsIterator(ledgerIter)
	defer eventsIter.Close()

	for {
		response, err := eventsIter.Next()
		if err != nil {
			return status.Error(codes.Aborted, err.Error())
		}

		var matchingEvents []*peer.ChaincodeEvent

		for _, event := range response.Events {
			if event.GetChaincodeId() == request.GetChaincodeId() {
				matchingEvents = append(matchingEvents, event)
			}
		}

		if len(matchingEvents) == 0 {
			continue
		}

		response.Events = matchingEvents

		if err := stream.Send(response); err != nil {
			if err == io.EOF {
				// Stream closed by the client
				return status.Error(codes.Canceled, err.Error())
			}
			return err
		}
	}
}

func startBlockFromLedgerPosition(ledger ledger.Ledger, position *ab.SeekPosition) (uint64, error) {
	switch seek := position.GetType().(type) {
	case nil:
	case *ab.SeekPosition_NextCommit:
	case *ab.SeekPosition_Specified:
		return seek.Specified.GetNumber(), nil
	default:
		return 0, status.Errorf(codes.InvalidArgument, "invalid start position type: %T", seek)
	}

	ledgerInfo, err := ledger.GetBlockchainInfo()
	if err != nil {
		return 0, status.Error(codes.Aborted, err.Error())
	}

	return ledgerInfo.GetHeight(), nil
}
