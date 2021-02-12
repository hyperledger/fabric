/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	commonledger "github.com/hyperledger/fabric/common/ledger"
)

type QueryResponseGenerator struct {
	MaxResultLimit int
}

// BuildQueryResponse takes an iterator and fetch state to construct QueryResponse
func (q *QueryResponseGenerator) BuildQueryResponse(txContext *TransactionContext, iter commonledger.ResultsIterator,
	iterID string, isPaginated bool, totalReturnLimit int32) (*pb.QueryResponse, error) {
	pendingQueryResults := txContext.GetPendingQueryResult(iterID)
	totalReturnCount := txContext.GetTotalReturnCount(iterID)

	for {
		// if the total count has been reached, return the result and prevent the Next() being called
		if *totalReturnCount >= totalReturnLimit {
			return createQueryResponse(txContext, iterID, isPaginated, pendingQueryResults, *totalReturnCount)
		}

		queryResult, err := iter.Next()
		switch {
		case err != nil:
			chaincodeLogger.Errorf("Failed to get query result from iterator")
			txContext.CleanupQueryContext(iterID)
			return nil, err

		case queryResult == nil:

			return createQueryResponse(txContext, iterID, isPaginated, pendingQueryResults, *totalReturnCount)

		case !isPaginated && pendingQueryResults.Size() == q.MaxResultLimit:
			// if explicit pagination is not used
			// if the max number of results is queued up, cut batch, then add current result to pending batch
			// MaxResultLimit is for batching between chaincode shim and handler
			// MaxResultLimit does not limit the records returned to the client
			batch := pendingQueryResults.Cut()
			if err := pendingQueryResults.Add(queryResult); err != nil {
				txContext.CleanupQueryContext(iterID)
				return nil, err
			}
			*totalReturnCount++
			return &pb.QueryResponse{Results: batch, HasMore: true, Id: iterID}, nil

		default:
			if err := pendingQueryResults.Add(queryResult); err != nil {
				txContext.CleanupQueryContext(iterID)
				return nil, err
			}
			*totalReturnCount++
		}
	}
}

func createQueryResponse(txContext *TransactionContext, iterID string, isPaginated bool, pendingQueryResults *PendingQueryResult, totalReturnCount int32) (*pb.QueryResponse, error) {
	batch := pendingQueryResults.Cut()

	if isPaginated {
		// when explicit pagination is enabled, return the batch with the responseMetadata
		bookmark := txContext.CleanupQueryContextWithBookmark(iterID)
		responseMetadata := createResponseMetadata(totalReturnCount, bookmark)
		responseMetadataBytes, err := proto.Marshal(responseMetadata)
		if err != nil {
			return nil, err
		}
		return &pb.QueryResponse{Results: batch, HasMore: false, Id: iterID, Metadata: responseMetadataBytes}, nil
	}

	// if explicit pagination is not used, then the end of the resultset has been reached, return the batch
	txContext.CleanupQueryContext(iterID)
	return &pb.QueryResponse{Results: batch, HasMore: false, Id: iterID}, nil
}

func createResponseMetadata(returnCount int32, bookmark string) *pb.QueryResponseMetadata {
	responseMetadata := &pb.QueryResponseMetadata{}
	responseMetadata.Bookmark = bookmark
	responseMetadata.FetchedRecordsCount = int32(returnCount)
	return responseMetadata
}
