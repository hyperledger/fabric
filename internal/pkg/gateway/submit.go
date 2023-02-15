/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

	logger := logger.With("txID", request.TransactionId)

	// try each orderer in random order
	var errDetails []proto.Message
	for _, index := range rand.Perm(len(orderers)) {
		orderer := orderers[index]
		logger.Infow("Sending transaction to orderer", "endpoint", orderer.logAddress)

		var response *ab.BroadcastResponse
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(ctx, gs.options.BroadcastTimeout)
			defer cancel()

			response, err = gs.broadcast(ctx, orderer, txn)
		}()
		select {
		case <-done:
			// Broadcast completed normally
		case <-ctx.Done():
			// Overall submit timeout expired
			logger.Warnw("Submit call timed out while broadcasting to ordering service")
			return nil, newRpcError(codes.DeadlineExceeded, "submit timeout expired while broadcasting to ordering service")
		}

		if err != nil {
			errDetails = append(errDetails, errorDetail(orderer.endpointConfig, err.Error()))
			logger.Warnw("Error sending transaction to orderer", "endpoint", orderer.logAddress, "err", err)
			continue
		}

		status := response.GetStatus()
		if status == common.Status_SUCCESS {
			return &gp.SubmitResponse{}, nil
		}

		logger.Warnw("Unsuccessful response sending transaction to orderer", "endpoint", orderer.logAddress, "status", status, "info", response.GetInfo())

		if status >= 400 && status < 500 {
			// client error - don't retry
			return nil, newRpcError(codes.Aborted, fmt.Sprintf("received unsuccessful response from orderer: status=%s, info=%s", common.Status_name[int32(status)], response.GetInfo()))
		}
	}
	return nil, newRpcError(codes.Unavailable, "no orderers could successfully process transaction", errDetails...)
}

func (gs *Server) broadcast(ctx context.Context, orderer *orderer, txn *common.Envelope) (*ab.BroadcastResponse, error) {
	broadcast, err := orderer.client.Broadcast(ctx)
	if err != nil {
		return nil, err
	}

	if err := broadcast.Send(txn); err != nil {
		return nil, err
	}

	response, err := broadcast.Recv()
	if err != nil {
		return nil, err
	}

	return response, nil
}

func prepareTransaction(header *common.Header, payload *peer.ChaincodeProposalPayload, action *peer.ChaincodeEndorsedAction) (*common.Envelope, error) {
	cppNoTransient := &peer.ChaincodeProposalPayload{Input: payload.Input, TransientMap: nil}
	cppBytes, err := protoutil.GetBytesChaincodeProposalPayload(cppNoTransient)
	if err != nil {
		return nil, err
	}

	cap := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: cppBytes, Action: action}
	capBytes, err := protoutil.GetBytesChaincodeActionPayload(cap)
	if err != nil {
		return nil, err
	}

	tx := &peer.Transaction{Actions: []*peer.TransactionAction{{Header: header.SignatureHeader, Payload: capBytes}}}
	txBytes, err := protoutil.GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	payl := &common.Payload{Header: header, Data: txBytes}
	paylBytes, err := protoutil.GetBytesPayload(payl)
	if err != nil {
		return nil, err
	}

	return &common.Envelope{Payload: paylBytes}, nil
}
