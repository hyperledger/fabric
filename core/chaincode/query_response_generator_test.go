/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestBuildQueryResponse(t *testing.T) {
	queryResult := &queryresult.KV{
		Key:       "key",
		Namespace: "namespace",
		Value:     []byte("value"),
	}

	// test various boundry cases around maxResultLimit
	const maxResultLimit = 10
	testCases := []struct {
		expectedResultCount  int
		expectedHasMoreCount int
	}{
		{0, 0},
		{1, 0},
		{10, 0},
		{maxResultLimit - 2, 0},
		{maxResultLimit - 1, 0},
		{maxResultLimit, 0},
		{maxResultLimit + 1, 1},
		{maxResultLimit + 2, 1},
		{int(math.Floor(maxResultLimit * 1.5)), 1},
		{maxResultLimit * 2, 1},
		{10*maxResultLimit - 2, 9},
		{10*maxResultLimit - 1, 9},
		{10 * maxResultLimit, 9},
		{10*maxResultLimit + 1, 10},
		{10*maxResultLimit + 2, 10},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d", tc.expectedResultCount), func(t *testing.T) {
			txSimulator := &mock.TxSimulator{}
			transactionContext := &chaincode.TransactionContext{
				TXSimulator: txSimulator,
			}

			resultsIterator := &mock.ResultsIterator{}
			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			for i := 0; i < tc.expectedResultCount; i++ {
				resultsIterator.NextReturnsOnCall(i, queryResult, nil)
			}
			resultsIterator.NextReturnsOnCall(tc.expectedResultCount, nil, nil)
			responseGenerator := &chaincode.QueryResponseGenerator{
				MaxResultLimit: maxResultLimit,
			}

			for totalResultCount, hasMoreCount := 0, 0; hasMoreCount <= tc.expectedHasMoreCount; hasMoreCount++ {
				queryResponse, err := responseGenerator.BuildQueryResponse(transactionContext, resultsIterator, "query-id")
				assert.NoError(t, err)

				switch {
				case hasMoreCount < tc.expectedHasMoreCount:
					// max limit sized batch retrieved, more expected
					assert.True(t, queryResponse.GetHasMore())
					assert.Len(t, queryResponse.GetResults(), maxResultLimit)
				default:
					// remainder retrieved, no more expected
					assert.Len(t, queryResponse.GetResults(), tc.expectedResultCount-totalResultCount)
					assert.False(t, queryResponse.GetHasMore())

				}
				totalResultCount += len(queryResponse.GetResults())
			}
			assert.Equal(t, tc.expectedResultCount+1, resultsIterator.NextCallCount())
			assert.Equal(t, 1, resultsIterator.CloseCallCount())
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
			resultsIterator := &mock.ResultsIterator{}
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

			resp, err := responseGenerator.BuildQueryResponse(transactionContext, resultsIterator, "query-id")
			if tc.expectedErrValue == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedErrValue)
			}
			assert.Nil(t, resp)
			assert.Equal(t, 1, resultsIterator.CloseCallCount())
		})
	}
}
