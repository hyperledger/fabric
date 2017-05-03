/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"testing"

	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestTransactionValidationFlags(t *testing.T) {
	txFlags := NewTxValidationFlags(10)
	assert.Equal(t, 10, len(txFlags))

	txFlags.SetFlag(0, peer.TxValidationCode_VALID)
	assert.Equal(t, peer.TxValidationCode_VALID, txFlags.Flag(0))
	assert.Equal(t, true, txFlags.IsValid(0))

	txFlags.SetFlag(1, peer.TxValidationCode_MVCC_READ_CONFLICT)
	assert.Equal(t, true, txFlags.IsInvalid(1))
}
