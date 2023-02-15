/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
			// Endorser completed normally
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
