/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestClient_Issue(t *testing.T) {

	t.Parallel()

	prover := &mocks.Prover{}
	txSubmitter := &mocks.FabricTxSubmitter{}
	signingIdentity := &mocks.SigningIdentity{}

	resetMocks := func() {
		prover.Mock = mock.Mock{}
		txSubmitter.Mock = mock.Mock{}
		signingIdentity.Mock = mock.Mock{}
	}

	tokensToIssue := []*token.TokenToIssue{
		{Type: "type",
			Quantity:  1,
			Recipient: []byte("alice")},
	}

	for _, testCase := range []struct {
		name                 string
		requestImportReturns []interface{}
		signReturns          []interface{}
		submitReturns        interface{}
		expectedErr          string
	}{
		{
			name:                 "RequestImport() fails",
			requestImportReturns: []interface{}{nil, errors.New("wild banana")},
			expectedErr:          "wild banana",
		},
		{
			name:                 "Sign() fails",
			requestImportReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:          []interface{}{nil, errors.New("wild carrots")},
			expectedErr:          "wild carrots",
		},
		{
			name:                 "Submit() fails",
			requestImportReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:          []interface{}{[]byte("carrots"), nil},
			submitReturns:        errors.New("wild apples"),
			expectedErr:          "wild apples",
		},
		{
			name:                 "Issue() Succeeds",
			requestImportReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:          []interface{}{[]byte("carrots"), nil},
			submitReturns:        nil,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			defer resetMocks()
			payload := &common.Payload{Data: []byte("pineapple")}
			payloadBytes, err := proto.Marshal(payload)
			assert.NoError(t, err)
			envelope := &common.Envelope{Payload: payloadBytes, Signature: []byte("carrots")}
			tx, err := proto.Marshal(envelope)
			assert.NoError(t, err)

			prover.On("RequestImport", tokensToIssue, signingIdentity).Return(
				testCase.requestImportReturns[0], testCase.requestImportReturns[1])
			if testCase.signReturns != nil {
				signingIdentity.On("Sign", payloadBytes).Return(
					testCase.signReturns[0], testCase.signReturns[1])
			}
			txSubmitter.On("Submit", tx).Return(testCase.submitReturns)

			c := Client{Prover: prover, TxSubmitter: txSubmitter, SigningIdentity: signingIdentity}
			_, err = c.Issue(tokensToIssue)

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.EqualError(t, err, testCase.expectedErr)
			}

			prover.AssertCalled(t, "RequestImport", tokensToIssue, signingIdentity)
			if testCase.requestImportReturns[1] != nil {
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				txSubmitter.AssertNumberOfCalls(t, "Submit", 0)
			} else {
				payload = &common.Payload{Data: []byte("pineapple")}
				payloadBytes, err = proto.Marshal(payload)
				assert.NoError(t, err)

				if testCase.signReturns != nil && testCase.signReturns[1] != nil {
					signingIdentity.AssertCalled(t, "Sign", payloadBytes)
					txSubmitter.AssertNumberOfCalls(t, "Submit", 0)
				}

				if testCase.submitReturns != nil {
					envelope := &common.Envelope{Payload: payloadBytes, Signature: []byte("carrots")}
					tx, err := proto.Marshal(envelope)
					assert.NoError(t, err)
					signingIdentity.AssertCalled(t, "Sign", payloadBytes)
					txSubmitter.AssertCalled(t, "Submit", tx)
				}
			}

		})
	}
}

func TestClient_Transfer(t *testing.T) {

	t.Parallel()

	prover := &mocks.Prover{}
	txSubmitter := &mocks.FabricTxSubmitter{}
	signingIdentity := &mocks.SigningIdentity{}

	resetMocks := func() {
		prover.Mock = mock.Mock{}
		txSubmitter.Mock = mock.Mock{}
		signingIdentity.Mock = mock.Mock{}
	}

	tokenIDs, shares := prepareInputsForTransfer(t)

	for _, testCase := range []struct {
		name                   string
		requestTransferReturns []interface{}
		signReturns            []interface{}
		submitReturns          interface{}
		expectedErr            string
	}{
		{
			name:                   "RequestTransfer() fails",
			requestTransferReturns: []interface{}{nil, errors.New("wild banana")},
			expectedErr:            "wild banana",
		},
		{
			name:                   "Sign() fails",
			requestTransferReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:            []interface{}{nil, errors.New("wild carrots")},
			expectedErr:            "wild carrots",
		},
		{
			name:                   "Submit() fails",
			requestTransferReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:            []interface{}{[]byte("carrots"), nil},
			submitReturns:          errors.New("wild banana"),
			expectedErr:            "wild banana",
		},
		{
			name:                   "Transfer() Succeeds",
			requestTransferReturns: []interface{}{[]byte("pineapple"), nil},
			signReturns:            []interface{}{[]byte("carrots"), nil},
			submitReturns:          nil,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			defer resetMocks()

			payload := &common.Payload{Data: []byte("pineapple")}
			payloadBytes, err := proto.Marshal(payload)
			assert.NoError(t, err)
			envelope := &common.Envelope{Payload: payloadBytes, Signature: []byte("carrots")}
			tx, err := proto.Marshal(envelope)
			assert.NoError(t, err)

			prover.On("RequestTransfer", tokenIDs, shares, signingIdentity).Return(
				testCase.requestTransferReturns[0], testCase.requestTransferReturns[1])
			if testCase.signReturns != nil {
				signingIdentity.On("Sign", payloadBytes).Return(
					testCase.signReturns[0], testCase.signReturns[1])
			}
			txSubmitter.On("Submit", tx).Return(testCase.submitReturns)

			c := Client{Prover: prover, TxSubmitter: txSubmitter, SigningIdentity: signingIdentity}
			_, err = c.Transfer(tokenIDs, shares)

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.EqualError(t, err, testCase.expectedErr)
			}

			prover.AssertCalled(t, "RequestTransfer", tokenIDs, shares, signingIdentity)
			if testCase.requestTransferReturns[1] != nil {
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				txSubmitter.AssertNumberOfCalls(t, "Submit", 0)
			} else {
				payload = &common.Payload{Data: []byte("pineapple")}
				payloadBytes, err = proto.Marshal(payload)
				assert.NoError(t, err)

				if testCase.signReturns != nil && testCase.signReturns[1] != nil {
					signingIdentity.AssertCalled(t, "Sign", payloadBytes)
					txSubmitter.AssertNumberOfCalls(t, "Submit", 0)
				}

				if testCase.submitReturns != nil {
					tx, err := proto.Marshal(envelope)
					assert.NoError(t, err)
					signingIdentity.AssertCalled(t, "Sign", payloadBytes)
					txSubmitter.AssertCalled(t, "Submit", tx)
				}
			}
		})
	}
}

func prepareInputsForTransfer(t *testing.T) ([][]byte, []*token.RecipientTransferShare) {
	tokenIDs := [][]byte{[]byte("id1"), []byte("id2")}
	shares := []*token.RecipientTransferShare{
		{Recipient: []byte("alice"), Quantity: 100},
		{Recipient: []byte("Bob"), Quantity: 50},
	}
	return tokenIDs, shares
}
