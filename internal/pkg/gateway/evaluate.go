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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
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

func getChannelAndChaincodeFromSignedProposal(signedProposal *peer.SignedProposal) (string, string, bool, error) {
	if len(signedProposal.GetProposalBytes()) == 0 {
		return "", "", false, fmt.Errorf("a signed proposal is required")
	}
	proposal, err := protoutil.UnmarshalProposal(signedProposal.GetProposalBytes())
	if err != nil {
		return "", "", false, err
	}
	header, err := protoutil.UnmarshalHeader(proposal.GetHeader())
	if err != nil {
		return "", "", false, err
	}
	channelHeader, err := protoutil.UnmarshalChannelHeader(header.GetChannelHeader())
	if err != nil {
		return "", "", false, err
	}
	payload, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.GetPayload())
	if err != nil {
		return "", "", false, err
	}
	spec, err := protoutil.UnmarshalChaincodeInvocationSpec(payload.GetInput())
	if err != nil {
		return "", "", false, err
	}

	if len(channelHeader.GetChannelId()) == 0 {
		return "", "", false, fmt.Errorf("no channel id provided")
	}

	if spec.GetChaincodeSpec() == nil {
		return "", "", false, fmt.Errorf("no chaincode spec is provided, channel id [%s]", channelHeader.GetChannelId())
	}

	if spec.GetChaincodeSpec().GetChaincodeId() == nil {
		return "", "", false, fmt.Errorf("no chaincode id is provided, channel id [%s]", channelHeader.GetChannelId())
	}

	if len(spec.GetChaincodeSpec().GetChaincodeId().GetName()) == 0 {
		return "", "", false, fmt.Errorf("no chaincode name is provided, channel id [%s]", channelHeader.GetChannelId())
	}

	return channelHeader.GetChannelId(), spec.GetChaincodeSpec().GetChaincodeId().GetName(), len(payload.TransientMap) > 0, nil
}

func getResultFromProposalResponse(proposalResponse *peer.ProposalResponse) ([]byte, error) {
	responsePayload := &peer.ProposalResponsePayload{}
	if err := proto.Unmarshal(proposalResponse.GetPayload(), responsePayload); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize proposal response payload")
	}

	return getResultFromProposalResponsePayload(responsePayload)
}

func getResultFromProposalResponsePayload(responsePayload *peer.ProposalResponsePayload) ([]byte, error) {
	chaincodeAction := &peer.ChaincodeAction{}
	if err := proto.Unmarshal(responsePayload.GetExtension(), chaincodeAction); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize chaincode action")
	}

	return chaincodeAction.GetResponse().GetPayload(), nil
}
