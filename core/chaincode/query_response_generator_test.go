/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/stretchr/testify/require"
)

const totalQueryLimit = 103

func TestBuildQueryResponse(t *testing.T) {
	queryResult := &queryresult.KV{
		Key:       "key",
		Namespace: "namespace",
		Value:     []byte("value"),
	}

	// test various boundary cases around maxResultLimit
	const maxResultLimit = 10
	testCases := []struct {
		recordCount          int
		expectedResultCount  int
		expectedHasMoreCount int
		isPaginated          bool
		maxResultLimit       int
		totalQueryLimit      int
	}{
		{0, 0, 0, false, maxResultLimit, totalQueryLimit},
		{1, 1, 0, false, maxResultLimit, totalQueryLimit},
		{10, 10, 0, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit - 2, maxResultLimit - 2, 0, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit - 1, maxResultLimit - 1, 0, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit, maxResultLimit, 0, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit + 1, maxResultLimit + 1, 1, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit + 2, maxResultLimit + 2, 1, false, maxResultLimit, totalQueryLimit},
		{int(math.Floor(maxResultLimit * 1.5)), int(math.Floor(maxResultLimit * 1.5)), 1, false, maxResultLimit, totalQueryLimit},
		{maxResultLimit * 2, maxResultLimit * 2, 1, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit - 2, 10*maxResultLimit - 2, 9, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit - 1, 10*maxResultLimit - 1, 9, false, maxResultLimit, totalQueryLimit},
		{10 * maxResultLimit, 10 * maxResultLimit, 9, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 1, 10*maxResultLimit + 1, 10, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 2, 10*maxResultLimit + 2, 10, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 3, 10*maxResultLimit + 3, 10, false, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 5, 10*maxResultLimit + 3, 10, false, maxResultLimit, totalQueryLimit},
		{10, 5, 1, false, 4, 5},
		{10, 5, 0, false, 5, 5},
		{10, 5, 0, false, 6, 5},
		{0, 0, 0, true, maxResultLimit, totalQueryLimit},
		{1, 1, 0, true, maxResultLimit, totalQueryLimit},
		{10, 10, 0, true, maxResultLimit, totalQueryLimit},
		{maxResultLimit, maxResultLimit, 0, true, maxResultLimit, totalQueryLimit},
		{maxResultLimit + 1, maxResultLimit + 1, 0, true, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 2, 10*maxResultLimit + 2, 0, true, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 3, totalQueryLimit, 0, true, maxResultLimit, totalQueryLimit},
		{10*maxResultLimit + 4, totalQueryLimit, 0, true, maxResultLimit, totalQueryLimit},
		{10, 5, 0, true, 4, 5},
		{10, 5, 0, false, 5, 5},
		{10, 5, 0, false, 6, 5},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d", tc.expectedResultCount), func(t *testing.T) {
			txSimulator := &mock.TxSimulator{}
			transactionContext := &chaincode.TransactionContext{
				TXSimulator: txSimulator,
			}

			resultsIterator := &mock.QueryResultsIterator{}
			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			for i := 0; i < tc.recordCount; i++ {
				resultsIterator.NextReturnsOnCall(i, queryResult, nil)
			}
			resultsIterator.NextReturnsOnCall(tc.recordCount, nil, nil)
			responseGenerator := &chaincode.QueryResponseGenerator{
				MaxResultLimit: tc.maxResultLimit,
			}
			totalResultCount := 0
			for hasMoreCount := 0; hasMoreCount <= tc.expectedHasMoreCount; hasMoreCount++ {
				queryResponse, err := responseGenerator.BuildQueryResponse(transactionContext, resultsIterator, "query-id", tc.isPaginated, int32(tc.totalQueryLimit))
				require.NoError(t, err)

				switch {
				case hasMoreCount < tc.expectedHasMoreCount:
					// max limit sized batch retrieved, more expected
					require.True(t, queryResponse.GetHasMore())
					require.Len(t, queryResponse.GetResults(), tc.maxResultLimit)
				default:
					// remainder retrieved, no more expected
					require.Len(t, queryResponse.GetResults(), tc.expectedResultCount-totalResultCount)
					require.False(t, queryResponse.GetHasMore())

				}
				totalResultCount += len(queryResponse.GetResults())
			}

			// assert the total number of records is correct
			require.Equal(t, tc.expectedResultCount, totalResultCount)

			if tc.isPaginated {
				// this case checks if the expected method was called to close the recordset
				require.Equal(t, 1, resultsIterator.GetBookmarkAndCloseCallCount())
			} else {
				require.Equal(t, 1, resultsIterator.CloseCallCount())
			}
		})
	}
}

func TestBuildQueryResponseErrors(t *testing.T) {
	validResult := &queryresult.KV{Key: "key-name"}
	invalidResult := brokenProto{}

	tests := []struct {
		errorOnNextCall        int
		brokenResultOnNextCall int
		expectedErrValue       string
	}{
		{-1, 2, "marshal-failed"},
		{-1, 3, "marshal-failed"},
		{2, -1, "next-failed"},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			txSimulator := &mock.TxSimulator{}
			transactionContext := &chaincode.TransactionContext{TXSimulator: txSimulator}
			resultsIterator := &mock.QueryResultsIterator{}
			resultsIterator.NextReturns(validResult, nil)
			if tc.errorOnNextCall >= 0 {
				resultsIterator.NextReturnsOnCall(tc.errorOnNextCall, nil, errors.New("next-failed"))
			}
			if tc.brokenResultOnNextCall >= 0 {
				resultsIterator.NextReturnsOnCall(tc.brokenResultOnNextCall, invalidResult, nil)
			}

			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			responseGenerator := &chaincode.QueryResponseGenerator{
				MaxResultLimit: 3,
			}

			resp, err := responseGenerator.BuildQueryResponse(transactionContext, resultsIterator, "query-id", false, totalQueryLimit)
			if tc.expectedErrValue == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErrValue)
			}
			require.Nil(t, resp)
			require.Equal(t, 1, resultsIterator.CloseCallCount())
		})
	}
}
