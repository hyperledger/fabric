/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"math"
	"testing"

	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetQueryResponse(t *testing.T) {

	queryResult := &queryresult.KV{
		Key:       "key",
		Namespace: "namespace",
		Value:     []byte("value"),
	}

	// test various boundry cases around maxResultLimit
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
		handler := &Handler{}
		transactionContext := &transactionContext{
			queryIteratorMap:    make(map[string]ledger.ResultsIterator),
			pendingQueryResults: make(map[string]*pendingQueryResult),
		}
		queryID := "test"
		t.Run(fmt.Sprintf("%d", tc.expectedResultCount), func(t *testing.T) {
			resultsIterator := &MockResultsIterator{}
			handler.initializeQueryContext(transactionContext, queryID, resultsIterator)
			if tc.expectedResultCount > 0 {
				resultsIterator.On("Next").Return(queryResult, nil).Times(tc.expectedResultCount)
			}
			resultsIterator.On("Next").Return(nil, nil).Once()
			resultsIterator.On("Close").Return().Once()
			totalResultCount := 0
			for hasMoreCount := 0; hasMoreCount <= tc.expectedHasMoreCount; hasMoreCount++ {
				queryResponse, _ := getQueryResponse(handler, transactionContext, resultsIterator, queryID)
				assert.NotNil(t, queryResponse.GetResults())
				if queryResponse.GetHasMore() {
					t.Logf("Got %d results and more are expected.", len(queryResponse.GetResults()))
				} else {
					t.Logf("Got %d results and no more are expected.", len(queryResponse.GetResults()))
				}

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
			resultsIterator.AssertExpectations(t)
		})
	}

}

type MockResultsIterator struct {
	mock.Mock
}

func (m *MockResultsIterator) Next() (ledger.QueryResult, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ledger.QueryResult), args.Error(1)
}

func (m *MockResultsIterator) Close() {
	m.Called()
}
