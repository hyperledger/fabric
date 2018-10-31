/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func clock() time.Time {
	return time.Time{}
}

func prepareInputs(t *testing.T) ([]byte, []*token.TokenToIssue) {
	tokensToIssue := []*token.TokenToIssue{{Type: "type", Quantity: 1, Recipient: []byte("alice")}}

	ir := &token.ImportRequest{
		TokensToIssue: tokensToIssue,
	}

	nonce := make([]byte, 32)
	ts, err := ptypes.TimestampProto(clock())
	assert.NoError(t, err)

	command := &token.Command{
		Header: &token.Header{
			Timestamp: ts,
			Nonce:     nonce,
			Creator:   []byte("Alice"),
			ChannelId: "myChannel",
		},
		Payload: &token.Command_ImportRequest{
			ImportRequest: ir,
		},
	}

	raw, err := proto.Marshal(command)
	assert.NoError(t, err)

	return raw, tokensToIssue
}

func TestProver_RequestImport(t *testing.T) {

	t.Parallel()

	identity := &mocks.Identity{}
	signingIdentity := &mocks.SigningIdentity{}
	randomnessReader := &mocks.Reader{}
	proverClient := &mocks.ProverClient{}

	resetMocks := func() {
		identity.Mock = mock.Mock{}
		signingIdentity.Mock = mock.Mock{}
		randomnessReader.Mock = mock.Mock{}
		proverClient.Mock = mock.Mock{}
	}

	raw, tokensToIssue := prepareInputs(t)
	sc := &token.SignedCommand{
		Command:   raw,
		Signature: []byte("pineapple"),
	}

	scr := &token.SignedCommandResponse{
		Response:  []byte("carrots"),
		Signature: []byte("apple"),
	}
	nonce := make([]byte, 32)

	for _, testCase := range []struct {
		name                  string
		readReturns           []interface{}
		serializeReturns      []interface{}
		signReturns           []interface{}
		processCommandReturns []interface{}
		expectedErr           string
	}{
		{
			name:        "Read() fails",
			readReturns: []interface{}{20, errors.New("wild potato")},
			expectedErr: "wild potato",
		},
		{
			name:             "Serialize() fails",
			readReturns:      []interface{}{32, nil},
			serializeReturns: []interface{}{nil, errors.New("wild apples")},
			expectedErr:      "wild apples",
		},
		{
			name:             "Sign() fails",
			readReturns:      []interface{}{32, nil},
			serializeReturns: []interface{}{[]byte("Alice"), nil},
			signReturns:      []interface{}{nil, errors.New("wild banana")},
			expectedErr:      "wild banana",
		},
		{
			name:                  "ProcessCommand() fails",
			readReturns:           []interface{}{32, nil},
			serializeReturns:      []interface{}{[]byte("Alice"), nil},
			signReturns:           []interface{}{[]byte("pineapple"), nil},
			processCommandReturns: []interface{}{nil, errors.New("wild carrots")},
			expectedErr:           "wild carrots",
		},
		{
			name:                  "RequestImport() Succeeds",
			readReturns:           []interface{}{32, nil},
			serializeReturns:      []interface{}{[]byte("Alice"), nil},
			signReturns:           []interface{}{[]byte("pineapple"), nil},
			processCommandReturns: []interface{}{scr, nil},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			defer resetMocks()

			randomnessReader.On("Read", nonce).Return(testCase.readReturns[0], testCase.readReturns[1])
			signingIdentity.On("GetPublicVersion").Return(identity)
			if testCase.serializeReturns != nil {
				identity.On("Serialize").Return(testCase.serializeReturns[0], testCase.serializeReturns[1])
				if testCase.signReturns != nil {
					signingIdentity.On("Sign", raw).Return(testCase.signReturns[0], testCase.signReturns[1])
					if testCase.processCommandReturns != nil {
						proverClient.On(
							"ProcessCommand", context.Background(), sc).Return(
							testCase.processCommandReturns[0], testCase.processCommandReturns[1])
					}
				}
			}

			prover := ProverPeer{RandomnessReader: randomnessReader, ProverClient: proverClient, ChannelID: "myChannel", Time: clock}

			response, err := prover.RequestImport(tokensToIssue, signingIdentity)

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, scr.Response, response)
			} else {
				assert.Error(t, err)
				assert.Nil(t, response)
				assert.EqualError(t, err, testCase.expectedErr)
			}

			if testCase.readReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertNumberOfCalls(t, "GetPublicVersion", 0)
				identity.AssertNumberOfCalls(t, "Serialize", 0)
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.serializeReturns != nil && testCase.serializeReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.signReturns != nil && testCase.signReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertCalled(t, "Sign", raw)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.processCommandReturns != nil && testCase.processCommandReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertCalled(t, "Sign", raw)
				proverClient.AssertCalled(t, "ProcessCommand", context.Background(), sc)
			}

		})
	}
}

func prepareInputsForRequestTransfer(t *testing.T) ([][]byte, []*token.RecipientTransferShare, []byte) {
	tokenIDs, shares := prepareInputsForTransfer(t)

	tr := &token.TransferRequest{
		Shares:   shares,
		TokenIds: tokenIDs,
	}

	nonce := make([]byte, 32)
	ts, err := ptypes.TimestampProto(clock())
	assert.NoError(t, err)

	command := &token.Command{
		Header: &token.Header{
			Timestamp: ts,
			Nonce:     nonce,
			Creator:   []byte("Alice"),
			ChannelId: "myChannel",
		},
		Payload: &token.Command_TransferRequest{
			TransferRequest: tr,
		},
	}

	raw, err := proto.Marshal(command)
	assert.NoError(t, err)

	return tokenIDs, shares, raw
}

func TestProver_RequestTransfer(t *testing.T) {

	t.Parallel()

	identity := &mocks.Identity{}
	signingIdentity := &mocks.SigningIdentity{}
	randomnessReader := &mocks.Reader{}
	proverClient := &mocks.ProverClient{}

	resetMocks := func() {
		identity.Mock = mock.Mock{}
		signingIdentity.Mock = mock.Mock{}
		randomnessReader.Mock = mock.Mock{}
		proverClient.Mock = mock.Mock{}
	}

	tokenIDs, shares, raw := prepareInputsForRequestTransfer(t)
	sc := &token.SignedCommand{
		Command:   raw,
		Signature: []byte("pineapple"),
	}

	scr := &token.SignedCommandResponse{
		Response:  []byte("carrots"),
		Signature: []byte("apple"),
	}
	nonce := make([]byte, 32)

	for _, testCase := range []struct {
		name                  string
		readReturns           []interface{}
		serializeReturns      []interface{}
		signReturns           []interface{}
		processCommandReturns []interface{}
		expectedErr           string
	}{
		{
			name:        "Read() fails",
			readReturns: []interface{}{20, errors.New("wild potato")},
			expectedErr: "wild potato",
		},
		{
			name:             "Serialize() fails",
			readReturns:      []interface{}{32, nil},
			serializeReturns: []interface{}{nil, errors.New("wild apples")},
			expectedErr:      "wild apples",
		},
		{
			name:             "Sign() fails",
			readReturns:      []interface{}{32, nil},
			serializeReturns: []interface{}{[]byte("Alice"), nil},
			signReturns:      []interface{}{nil, errors.New("wild banana")},
			expectedErr:      "wild banana",
		},
		{
			name:                  "ProcessCommand() fails",
			readReturns:           []interface{}{32, nil},
			serializeReturns:      []interface{}{[]byte("Alice"), nil},
			signReturns:           []interface{}{[]byte("pineapple"), nil},
			processCommandReturns: []interface{}{nil, errors.New("wild carrots")},
			expectedErr:           "wild carrots",
		},
		{
			name:                  "RequestTransfer() Succeeds",
			readReturns:           []interface{}{32, nil},
			serializeReturns:      []interface{}{[]byte("Alice"), nil},
			signReturns:           []interface{}{[]byte("pineapple"), nil},
			processCommandReturns: []interface{}{scr, nil},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			defer resetMocks()

			randomnessReader.On("Read", nonce).Return(testCase.readReturns[0], testCase.readReturns[1])
			signingIdentity.On("GetPublicVersion").Return(identity)
			if testCase.serializeReturns != nil {
				identity.On("Serialize").Return(testCase.serializeReturns[0], testCase.serializeReturns[1])
				if testCase.signReturns != nil {
					signingIdentity.On("Sign", raw).Return(testCase.signReturns[0], testCase.signReturns[1])
					if testCase.processCommandReturns != nil {
						proverClient.On(
							"ProcessCommand", context.Background(), sc).Return(
							testCase.processCommandReturns[0], testCase.processCommandReturns[1])
					}
				}
			}

			prover := ProverPeer{RandomnessReader: randomnessReader, ProverClient: proverClient, ChannelID: "myChannel", Time: clock}

			response, err := prover.RequestTransfer(tokenIDs, shares, signingIdentity)

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, scr.Response, response)
			} else {
				assert.Error(t, err)
				assert.Nil(t, response)
				assert.EqualError(t, err, testCase.expectedErr)
			}

			if testCase.readReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertNumberOfCalls(t, "GetPublicVersion", 0)
				identity.AssertNumberOfCalls(t, "Serialize", 0)
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.serializeReturns != nil && testCase.serializeReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertNumberOfCalls(t, "Sign", 0)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.signReturns != nil && testCase.signReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertCalled(t, "Sign", raw)
				proverClient.AssertNumberOfCalls(t, "ProcessCommand", 0)
			}

			if testCase.processCommandReturns != nil && testCase.processCommandReturns[1] != nil {
				randomnessReader.AssertCalled(t, "Read", nonce)
				signingIdentity.AssertCalled(t, "GetPublicVersion")
				identity.AssertCalled(t, "Serialize")
				signingIdentity.AssertCalled(t, "Sign", raw)
				proverClient.AssertCalled(t, "ProcessCommand", context.Background(), sc)
			}

		})
	}
}
