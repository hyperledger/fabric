/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/stretchr/testify/assert"
)

func TestActiveTransactions(t *testing.T) {
	activeTx := chaincode.NewActiveTransactions()
	assert.NotNil(t, activeTx)

	// Add unique transactions
	ok := activeTx.Add("channel-id", "tx-id")
	assert.True(t, ok, "a new transaction should return true")
	ok = activeTx.Add("channel-id", "tx-id-2")
	assert.True(t, ok, "adding a different transaction id should return true")
	ok = activeTx.Add("channel-id-2", "tx-id")
	assert.True(t, ok, "adding a different channel-id should return true")

	// Attempt to add a transaction that already exists
	ok = activeTx.Add("channel-id", "tx-id")
	assert.False(t, ok, "attempting to an existing transaction should return false")

	// Remove existing and make sure the ID can be reused
	activeTx.Remove("channel-id", "tx-id")
	ok = activeTx.Add("channel-id", "tx-id")
	assert.True(t, ok, "using a an id that has been removed should return true")
}

func TestNewTxKey(t *testing.T) {
	tests := []struct {
		channelID string
		txID      string
		result    string
	}{
		{"", "", ""},
		{"", "tx-1", "tx-1"},
		{"chan-1", "", "chan-1"},
		{"chan-1", "tx-1", "chan-1tx-1"},
	}
	for _, tc := range tests {
		result := chaincode.NewTxKey(tc.channelID, tc.txID)
		assert.Equal(t, tc.result, result)
	}
}
