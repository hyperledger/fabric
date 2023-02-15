/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"io"

	"github.com/golang/protobuf/proto"
	gp "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/internal/pkg/gateway/event"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
