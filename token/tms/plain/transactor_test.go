/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/tms/plain"
	"github.com/stretchr/testify/assert"
)

type nextReturns struct {
	result interface{}
	err    error
}

type getStateRangeScanIteratorReturns struct {
	iterator ledger.ResultsIterator
	err      error
}

type getStateReturns struct {
	value []byte
	err   error
}

func TestTransactor_ListTokens(t *testing.T) {
	t.Parallel()

	var err error

	ledger := &mock.LedgerReader{}
	iterator := &mock.ResultsIterator{}

	transactor := &plain.Transactor{PublicCredential: []byte("Alice"), Ledger: ledger}

	outputs := make([][]byte, 3)
	keys := make([]string, 3)
	results := make([]*queryresult.KV, 4)

	outputs[0], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK1", Quantity: 100})
	assert.NoError(t, err)
	outputs[1], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Bob"), Type: "TOK2", Quantity: 200})
	assert.NoError(t, err)
	outputs[2], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK3", Quantity: 300})
	assert.NoError(t, err)

	keys[0], err = plain.GenerateKeyForTest("1", 0)
	assert.NoError(t, err)
	keys[1], err = plain.GenerateKeyForTest("1", 1)
	assert.NoError(t, err)
	keys[2], err = plain.GenerateKeyForTest("2", 0)
	assert.NoError(t, err)

	results[0] = &queryresult.KV{Key: keys[0], Value: outputs[0]}
	results[1] = &queryresult.KV{Key: keys[1], Value: outputs[1]}
	results[2] = &queryresult.KV{Key: keys[2], Value: outputs[2]}
	results[3] = &queryresult.KV{Key: "123", Value: []byte("not an output")}

	for _, testCase := range []struct {
		name                             string
		getStateRangeScanIteratorReturns getStateRangeScanIteratorReturns
		nextReturns                      []nextReturns
		getStateReturns                  []getStateReturns
		expectedErr                      string
	}{
		{
			name:                             "getStateRangeScanIterator() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{nil, errors.New("wild potato")},
			expectedErr:                      "wild potato",
		},
		{
			name:                             "next() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			nextReturns:                      []nextReturns{{queryresult.KV{}, errors.New("wild banana")}},
			expectedErr:                      "wild banana",
		},
		{
			name:                             "getStateReturns() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			nextReturns:                      []nextReturns{{results[0], nil}},
			getStateReturns:                  []getStateReturns{{nil, errors.New("wild apple")}},
			expectedErr:                      "wild apple",
		},
		{
			name:                             "Success",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			getStateReturns: []getStateReturns{
				{nil, nil},
				{[]byte("value"), nil},
			},
			nextReturns: []nextReturns{
				{results[0], nil},
				{results[1], nil},
				{results[2], nil},
				{results[3], nil},
				{nil, nil},
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			ledger.GetStateRangeScanIteratorReturns(testCase.getStateRangeScanIteratorReturns.iterator, testCase.getStateRangeScanIteratorReturns.err)
			if testCase.getStateRangeScanIteratorReturns.iterator != nil {
				if len(testCase.nextReturns) == 1 {
					iterator.NextReturns(testCase.nextReturns[0].result, testCase.nextReturns[0].err)
					if testCase.nextReturns[0].err == nil {
						ledger.GetStateReturns(testCase.getStateReturns[0].value, testCase.getStateReturns[0].err)
					}
				} else {
					iterator.NextReturnsOnCall(2, testCase.nextReturns[0].result, testCase.nextReturns[0].err)
					iterator.NextReturnsOnCall(3, testCase.nextReturns[1].result, testCase.nextReturns[1].err)
					iterator.NextReturnsOnCall(4, testCase.nextReturns[2].result, testCase.nextReturns[2].err)
					iterator.NextReturnsOnCall(5, testCase.nextReturns[3].result, testCase.nextReturns[3].err)
					iterator.NextReturnsOnCall(6, testCase.nextReturns[4].result, testCase.nextReturns[4].err)

					ledger.GetStateReturnsOnCall(1, testCase.getStateReturns[0].value, testCase.getStateReturns[0].err)
					ledger.GetStateReturnsOnCall(2, testCase.getStateReturns[1].value, testCase.getStateReturns[1].err)
				}

			}
			expectedTokens := &token.UnspentTokens{Tokens: []*token.TokenOutput{{Type: "TOK1", Quantity: 100, Id: []byte(keys[0])}}}
			tokens, err := transactor.ListTokens()

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
				assert.NotNil(t, tokens)
				assert.Equal(t, expectedTokens, tokens)
			} else {
				assert.Error(t, err)
				assert.Nil(t, tokens)
				assert.EqualError(t, err, testCase.expectedErr)
			}
			if testCase.getStateRangeScanIteratorReturns.err != nil {
				assert.Equal(t, 1, ledger.GetStateRangeScanIteratorCallCount())
				assert.Equal(t, 0, ledger.GetStateCallCount())
				assert.Equal(t, 0, iterator.NextCallCount())
			} else {
				if testCase.nextReturns[0].err != nil {
					assert.Equal(t, 2, ledger.GetStateRangeScanIteratorCallCount())
					assert.Equal(t, 0, ledger.GetStateCallCount())
					assert.Equal(t, 1, iterator.NextCallCount())
				} else {
					if testCase.getStateReturns[0].err != nil {
						assert.Equal(t, 3, ledger.GetStateRangeScanIteratorCallCount())
						assert.Equal(t, 1, ledger.GetStateCallCount())
						assert.Equal(t, 2, iterator.NextCallCount())
					} else {
						assert.Equal(t, 4, ledger.GetStateRangeScanIteratorCallCount())
						assert.Equal(t, 3, ledger.GetStateCallCount())
						assert.Equal(t, 7, iterator.NextCallCount())
					}

				}
			}

		})

	}
}
