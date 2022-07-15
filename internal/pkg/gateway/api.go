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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/internal/pkg/gateway/event"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		gs.logger.Debugw("Sending to peer:", "channel", channel, "chaincode", chaincodeID, "txID", request.GetTransactionId(), "MSPID", endorser.mspid, "endpoint", endorser.address)

		done := make(chan error)
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout)
			defer cancel()
			pr, err := endorser.client.ProcessProposal(ctx, signedProposal)
			code, message, retry, remove := responseStatus(pr, err)
			if code == codes.OK {
				response = pr.Response
				// Prefer result from proposal response as Response.Payload is not required to be transaction result
				if result, err := getResultFromProposalResponse(pr); err == nil {
					response.Payload = result
				} else {
					logger.Warnw("Successful proposal response contained no transaction result", "error", err.Error(), "chaincode", chaincodeID, "channel", channel, "txID", request.GetTransactionId(), "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "status", response.GetStatus(), "message", response.GetMessage())
				}
			} else {
				logger.Debugw("Evaluate call to endorser failed", "chaincode", chaincodeID, "channel", channel, "txID", request.GetTransactionId(), "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "error", message)
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
			logger.Warnw("Evaluate call timed out while processing request", "channel", request.GetChannelId(), "txID", request.GetTransactionId())
			return nil, newRpcError(codes.DeadlineExceeded, "evaluate timeout expired")
		}
	}

	evaluateResponse := &gp.EvaluateResponse{
		Result: response,
	}

	logger.Debugw("Evaluate call to endorser returned success", "channel", request.GetChannelId(), "txID", request.GetTransactionId(), "endorserAddress", endorser.endpointConfig.address, "endorserMspid", endorser.endpointConfig.mspid, "status", response.GetStatus(), "message", response.GetMessage())
	return evaluateResponse, nil
}

// Endorse will collect endorsements by invoking the transaction function specified in the SignedProposal against
// sufficient Peers to satisfy the endorsement policy.
func (gs *Server) Endorse(ctx context.Context, request *gp.EndorseRequest) (*gp.EndorseResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "an endorse request is required")
	}
	signedProposal := request.GetProposedTransaction()
	if len(signedProposal.GetProposalBytes()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "the proposed transaction must contain a signed proposal")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.GetProposalBytes())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	header, err := protoutil.UnmarshalHeader(proposal.GetHeader())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	channelHeader, err := protoutil.UnmarshalChannelHeader(header.GetChannelHeader())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	payload, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.GetPayload())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	spec, err := protoutil.UnmarshalChaincodeInvocationSpec(payload.GetInput())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	channel := channelHeader.GetChannelId()
	chaincodeID := spec.GetChaincodeSpec().GetChaincodeId().GetName()
	hasTransientData := len(payload.GetTransientMap()) > 0

	logger := gs.logger.With("channel", channel, "chaincode", chaincodeID, "txID", request.GetTransactionId())

	var plan *plan
	var action *peer.ChaincodeEndorsedAction
	if len(request.GetEndorsingOrganizations()) > 0 {
		// The client is specifying the endorsing orgs and taking responsibility for ensuring it meets the signature policy
		plan, err = gs.registry.planForOrgs(channel, chaincodeID, request.GetEndorsingOrganizations())
		if err != nil {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
	} else {
		// The client is delegating choice of endorsers to the gateway.
		plan, err = gs.planFromFirstEndorser(ctx, channel, chaincodeID, hasTransientData, signedProposal, logger)
		if err != nil {
			return nil, err
		}
	}

	for plan.completedLayout == nil {
		// loop through the layouts until one gets satisfied
		endorsers := plan.endorsers()
		if endorsers == nil {
			// no more layouts
			break
		}
		// send to all the endorsers
		waitCh := make(chan bool, len(endorsers))
		for _, e := range endorsers {
			go func(e *endorser) {
				for e != nil {
					if gs.processProposal(ctx, plan, e, signedProposal, logger) {
						break
					}
					e = plan.nextPeerInGroup(e)
				}
				waitCh <- true
			}(e)
		}
		for i := 0; i < len(endorsers); i++ {
			select {
			case <-waitCh:
				// Endorser completedLayout normally
			case <-ctx.Done():
				logger.Warnw("Endorse call timed out while collecting endorsements", "numEndorsers", len(endorsers))
				return nil, newRpcError(codes.DeadlineExceeded, "endorsement timeout expired while collecting endorsements")
			}
		}

	}

	if plan.completedLayout == nil {
		return nil, newRpcError(codes.Aborted, "failed to collect enough transaction endorsements, see attached details for more info", plan.errorDetails...)
	}

	action = &peer.ChaincodeEndorsedAction{ProposalResponsePayload: plan.responsePayload, Endorsements: uniqueEndorsements(plan.completedLayout.endorsements)}

	preparedTransaction, err := prepareTransaction(header, payload, action)
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "failed to assemble transaction: %s", err)
	}

	return &gp.EndorseResponse{PreparedTransaction: preparedTransaction}, nil
}

type ppResponse struct {
	response *peer.ProposalResponse
	err      error
}

// processProposal will invoke the given endorsing peer to process the signed proposal, and will update the plan accordingly.
// This function will timeout and return false if the given context timeout or the EndorsementTimeout option expires.
// Returns boolean true if the endorsement was successful.
func (gs *Server) processProposal(ctx context.Context, plan *plan, endorser *endorser, signedProposal *peer.SignedProposal, logger *flogging.FabricLogger) bool {
	var response *peer.ProposalResponse
	done := make(chan *ppResponse)
	go func() {
		defer close(done)
		logger.Debugw("Sending to endorser:", "MSPID", endorser.mspid, "endpoint", endorser.address)
		ctx, cancel := context.WithTimeout(ctx, gs.options.EndorsementTimeout) // timeout of individual endorsement
		defer cancel()
		response, err := endorser.client.ProcessProposal(ctx, signedProposal)
		done <- &ppResponse{response: response, err: err}
	}()
	select {
	case resp := <-done:
		// Endorser completedLayout normally
		code, message, _, remove := responseStatus(resp.response, resp.err)
		if code != codes.OK {
			logger.Warnw("Endorse call to endorser failed", "MSPID", endorser.mspid, "endpoint", endorser.address, "error", message)
			if remove {
				gs.registry.removeEndorser(endorser)
			}
			plan.addError(errorDetail(endorser.endpointConfig, message))
			return false
		}
		response = resp.response
		logger.Debugw("Endorse call to endorser returned success", "MSPID", endorser.mspid, "endpoint", endorser.address, "status", response.Response.Status, "message", response.Response.Message)

		responseMessage := response.GetResponse()
		if responseMessage != nil {
			responseMessage.Payload = nil // Remove any duplicate response payload
		}

		return plan.processEndorsement(endorser, response)
	case <-ctx.Done():
		// Overall endorsement timeout expired
		return false
	}
}

// planFromFirstEndorser implements the gateway's strategy of processing the proposal on a single (preferably local) peer
// and using the ChaincodeInterest from the response to invoke discovery and build an endorsement plan.
// Returns the endorsement plan which can be used to request further endorsements, if required.
func (gs *Server) planFromFirstEndorser(ctx context.Context, channel string, chaincodeID string, hasTransientData bool, signedProposal *peer.SignedProposal, logger *flogging.FabricLogger) (*plan, error) {
	defaultInterest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{{
			Name: chaincodeID,
		}},
	}

	// 1. Choose an endorser from the gateway's organization
	plan, err := gs.registry.planForOrgs(channel, chaincodeID, []string{gs.registry.localEndorser.mspid})
	if err != nil {
		// No local org endorsers for this channel/chaincode. If transient data is involved, return error
		if hasTransientData {
			return nil, status.Error(codes.FailedPrecondition, "no endorsers found in the gateway's organization; retry specifying endorsing organization(s) to protect transient data")
		}
		// Otherwise, just let discovery pick one.
		plan, err = gs.registry.endorsementPlan(channel, defaultInterest, nil)
		if err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
	}
	firstEndorser := plan.endorsers()[0]

	gs.logger.Debugw("Sending to first endorser:", "MSPID", firstEndorser.mspid, "endpoint", firstEndorser.address)

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
			code, message, _, remove := responseStatus(firstResponse, err)

			if code != codes.OK {
				logger.Warnw("Endorse call to endorser failed", "endorserAddress", firstEndorser.address, "endorserMspid", firstEndorser.mspid, "error", message)
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
			// Endorser completedLayout normally
		case <-ctx.Done():
			// Overall endorsement timeout expired
			logger.Warn("Endorse call timed out while collecting first endorsement")
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
	originalInterest := &peer.ChaincodeInterest{}
	var protectedCollections []string
	if hasTransientData {
		for _, call := range interest.GetChaincodes() {
			ccc := *call // shallow copy
			originalInterest.Chaincodes = append(originalInterest.Chaincodes, &ccc)
			if call.NoPrivateReads {
				call.NoPrivateReads = false
				protectedCollections = append(protectedCollections, call.CollectionNames...)
			}
		}
	}

	// 5. Get a set of endorsers from discovery via the registry
	// The preferred discovery layout will contain the firstEndorser's Org.
	plan, err = gs.registry.endorsementPlan(channel, interest, firstEndorser)
	if err != nil {
		if len(protectedCollections) > 0 {
			// may have failed because of the cautious approach we are taking with transient data - check
			_, err = gs.registry.endorsementPlan(channel, originalInterest, firstEndorser)
			if err == nil {
				return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("requires endorsement from organisation(s) that are not in the distribution policy of the private data collection(s): %v; retry specifying trusted endorsing organizations to protect transient data", protectedCollections))
			}
		}
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	// 6. Remove the gateway org's endorser, since we've already done that
	plan.processEndorsement(firstEndorser, firstResponse)

	return plan, nil
}

// responseStatus unpacks the proposal response and error values that are returned from ProcessProposal and
// determines how the gateway should react (retry?, close connection?).
// Uses the grpc canonical status error codes and their recommended actions.
// Returns:
// - response status code, with codes.OK indicating success and other values indicating likely error type
// - error message extracted from the err or generated from 500 proposal response (string)
// - should the gateway retry (only the Evaluate() uses this) (bool)
// - should the gateway close the connection and remove the peer from its registry (bool)
func responseStatus(response *peer.ProposalResponse, err error) (statusCode codes.Code, message string, retry bool, remove bool) {
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
	if len(signedRequest.GetRequest()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "a commit status request is required")
	}

	request := &gp.CommitStatusRequest{}
	if err := proto.Unmarshal(signedRequest.GetRequest(), request); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid status request: %v", err)
	}

	signedData := &protoutil.SignedData{
		Data:      signedRequest.GetRequest(),
		Identity:  request.GetIdentity(),
		Signature: signedRequest.GetSignature(),
	}
	if err := gs.policy.CheckACL(resources.Gateway_CommitStatus, request.GetChannelId(), signedData); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	txStatus, err := gs.commitFinder.TransactionStatus(ctx, request.GetChannelId(), request.GetTransactionId())
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
	if len(signedRequest.GetRequest()) == 0 {
		return status.Error(codes.InvalidArgument, "a chaincode events request is required")
	}

	request := &gp.ChaincodeEventsRequest{}
	if err := proto.Unmarshal(signedRequest.GetRequest(), request); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid chaincode events request: %v", err)
	}

	signedData := &protoutil.SignedData{
		Data:      signedRequest.GetRequest(),
		Identity:  request.GetIdentity(),
		Signature: signedRequest.GetSignature(),
	}
	if err := gs.policy.CheckACL(resources.Gateway_ChaincodeEvents, request.GetChannelId(), signedData); err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	ledger, err := gs.ledgerProvider.Ledger(request.GetChannelId())
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	startBlock, err := chaincodeEventsStartBlock(ledger, request)
	if err != nil {
		return err
	}

	isMatch := chaincodeEventMatcher(request)

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
		for _, event := range response.GetEvents() {
			if isMatch(event) {
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

func chaincodeEventMatcher(request *gp.ChaincodeEventsRequest) func(event *peer.ChaincodeEvent) bool {
	chaincodeID := request.GetChaincodeId()
	previousTransactionID := request.GetAfterTransactionId()

	if len(previousTransactionID) == 0 {
		return func(event *peer.ChaincodeEvent) bool {
			return event.GetChaincodeId() == chaincodeID
		}
	}

	passedPreviousTransaction := false

	return func(event *peer.ChaincodeEvent) bool {
		if !passedPreviousTransaction {
			if event.TxId == previousTransactionID {
				passedPreviousTransaction = true
			}
			return false
		}

		return event.GetChaincodeId() == chaincodeID
	}
}

func chaincodeEventsStartBlock(ledger ledger.Ledger, request *gp.ChaincodeEventsRequest) (uint64, error) {
	afterTransactionID := request.GetAfterTransactionId()
	if len(afterTransactionID) > 0 {
		if block, err := ledger.GetBlockByTxID(afterTransactionID); err == nil {
			return block.GetHeader().GetNumber(), nil
		}
	}

	return startBlockFromLedgerPosition(ledger, request.GetStartPosition())
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
