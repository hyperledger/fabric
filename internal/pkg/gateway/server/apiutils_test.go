/*
Copyright 2020 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"

	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestGetValueFromResponse(t *testing.T) {
	response := createProposalResponse("MyResult", t)

	result, err := getValueFromResponse(response)
	require.NoError(t, err, "Failed to extract value from response")
	require.Equal(t, []byte("MyResult"), result.Value, "Incorrect value")
}

func TestCreateUnsignedTx(t *testing.T) {
	proposal := createProposal(t)
	response1 := createProposalResponse("MyResult", t)
	response2 := createProposalResponse("MyResult", t)
	tx, err := createUnsignedTx(proposal, response1, response2)

	require.NoError(t, err, "Failed to create unsigned tx")
	payload, err := protoutil.UnmarshalPayload(tx.Payload)
	require.NoError(t, err, "Failed to unmarshal the tx payload")
	transaction, err := protoutil.UnmarshalTransaction(payload.Data)
	require.NoError(t, err, "Failed to unmarshal the payload data")
	require.Len(t, transaction.Actions, 1, "Should be one transaction action")
	tap, err := protoutil.UnmarshalChaincodeActionPayload(transaction.Actions[0].Payload)
	require.NoError(t, err, "Failed to unmarshal the action response")
	require.Len(t, tap.Action.Endorsements, 2)
	require.Equal(t, response1.Payload, tap.Action.ProposalResponsePayload, "Incorrect response payload")
}
