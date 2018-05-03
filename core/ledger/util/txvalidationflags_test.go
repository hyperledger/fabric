/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"testing"

	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestTransactionValidationFlags(t *testing.T) {
	txFlags := NewTxValidationFlagsSetValue(10, peer.TxValidationCode_VALID)
	assert.Equal(t, 10, len(txFlags))

	txFlags.SetFlag(0, peer.TxValidationCode_VALID)
	assert.Equal(t, peer.TxValidationCode_VALID, txFlags.Flag(0))
	assert.Equal(t, true, txFlags.IsValid(0))

	txFlags.SetFlag(1, peer.TxValidationCode_MVCC_READ_CONFLICT)
	assert.Equal(t, true, txFlags.IsInvalid(1))
}
