/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	"github.com/golang/protobuf/proto"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
