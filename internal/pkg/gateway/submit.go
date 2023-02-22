/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
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
	orderers, clusterSize, err := gs.registry.orderers(request.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
	}

	if len(orderers) == 0 {
		return nil, status.Errorf(codes.Unavailable, "no orderer nodes available")
	}

	logger := logger.With("txID", request.TransactionId)
	config := gs.getChannelConfig(request.ChannelId)
	if config.ChannelConfig().Capabilities().ConsensusTypeBFT() {
		return gs.submitBFT(ctx, orderers, txn, clusterSize, logger)
	} else {
		return gs.submitNonBFT(ctx, orderers, txn, logger)
	}
}

func (gs *Server) submitBFT(ctx context.Context, orderers []*orderer, txn *common.Envelope, clusterSize int, logger *flogging.FabricLogger) (*gp.SubmitResponse, error) {
	// For BFT, we send transaction to ALL orderers
	waitCh := make(chan *gp.ErrorDetail, len(orderers))
	go gs.broadcastToAll(orderers, txn, waitCh, logger)

	quorum, _ := computeBFTQuorum(uint64(clusterSize))
	successes, failures := 0, 0
	var errDetails []proto.Message
loop:
	for i, total := 0, len(orderers); i < total; i++ {
		select {
		case osnErr := <-waitCh:
			// Broadcast completed normally
			if osnErr != nil {
				errDetails = append(errDetails, osnErr)
				failures++
				if failures > total-quorum {
					break loop
				}
			} else {
				successes++
				if successes >= quorum {
					return &gp.SubmitResponse{}, nil
				}
			}
		case <-ctx.Done():
			// Overall submit timeout expired
			logger.Warnw("Submit call timed out while broadcasting to ordering service")
			return nil, newRpcError(codes.DeadlineExceeded, "submit timeout expired while broadcasting to ordering service")
		}
	}
	logger.Warnw("Insufficient number of orderers could successfully process transaction to satisfy quorum requirement", "successes", successes, "quorum", quorum)
	return nil, newRpcError(codes.Unavailable, "insufficient number of orderers could successfully process transaction to satisfy quorum requirement", errDetails...)
}

func (gs *Server) broadcastToAll(orderers []*orderer, txn *common.Envelope, waitCh chan<- *gp.ErrorDetail, logger *flogging.FabricLogger) {
	everyoneSubmitted := make(chan struct{})
	var numFinishedSend uint32

	broadcastContext, broadcastCancel := context.WithCancel(context.Background())
	defer broadcastCancel()
	for _, o := range orderers {
		go func(ord *orderer) {
			logger.Infow("Sending transaction to orderer", "endpoint", ord.logAddress)
			ctx, cancel := context.WithCancel(broadcastContext)
			defer cancel()
			response, err := gs.broadcast(ctx, ord, txn)
			// If I'm the last to submit, notify this
			if atomic.AddUint32(&numFinishedSend, 1) == uint32(len(orderers)) {
				close(everyoneSubmitted)
			}
			if err != nil {
				logger.Warnw("Error sending transaction to orderer", "endpoint", ord.logAddress, "err", err)
				waitCh <- errorDetail(ord.endpointConfig, err.Error())
			} else if status := response.GetStatus(); status == common.Status_SUCCESS {
				logger.Infow("Successful response from orderer", "endpoint", ord.logAddress)
				waitCh <- nil
			} else {
				logger.Warnw("Unsuccessful response sending transaction to orderer", "endpoint", ord.logAddress, "status", status, "info", response.GetInfo())
				if status == common.Status_SERVICE_UNAVAILABLE && response.GetInfo() == "failed to submit request: request already exists" {
					// the orderer already has this transaction - let it continue
					waitCh <- nil
				} else {
					waitCh <- errorDetail(ord.endpointConfig, fmt.Sprintf("received unsuccessful response from orderer: status=%s, info=%s", common.Status_name[int32(status)], response.GetInfo()))
				}
			}
		}(o)
	}

	t1 := time.NewTimer(gs.options.BroadcastTimeout)
	defer t1.Stop()
	select {
	case <-everyoneSubmitted:
		return
	case <-t1.C:
		return
	}
}

// copied from the smartbft library...
// computeBFTQuorum calculates the quorums size Q, given a cluster size N.
//
// The calculation satisfies the following:
// Given a cluster size of N nodes, which tolerates f failures according to:
//
//	f = argmax ( N >= 3f+1 )
//
// Q is the size of the quorum such that:
//
//	any two subsets q1, q2 of size Q, intersect in at least f+1 nodes.
//
// Note that this is different from N-f (the number of correct nodes), when N=3f+3. That is, we have two extra nodes
// above the minimum required to tolerate f failures.
func computeBFTQuorum(N uint64) (Q int, F int) {
	F = int((int(N) - 1) / 3)
	Q = int(math.Ceil((float64(N) + float64(F) + 1) / 2.0))
	return
}

func (gs *Server) submitNonBFT(ctx context.Context, orderers []*orderer, txn *common.Envelope, logger *flogging.FabricLogger) (*gp.SubmitResponse, error) {
	// non-BFT - only need one successful response
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
