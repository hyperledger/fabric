/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"github.com/hyperledger/fabric/protos/peer"
)

// TxValidationFlags is array of transaction validation codes. It is used when committer validates block.
type TxValidationFlags []uint8

// NewTxValidationFlags Create new object-array of validation codes with target size.
// Default values: TxValidationCode_NOT_VALIDATED
func NewTxValidationFlags(size int) TxValidationFlags {
	return newTxValidationFlagsSetValue(size, peer.TxValidationCode_NOT_VALIDATED)
}

// NewTxValidationFlagsSetValue Creates new object-array of validation codes with target size
// and the supplied value
func NewTxValidationFlagsSetValue(size int, value peer.TxValidationCode) TxValidationFlags {
	return newTxValidationFlagsSetValue(size, value)
}

func newTxValidationFlagsSetValue(size int, value peer.TxValidationCode) TxValidationFlags {
	inst := make(TxValidationFlags, size)
	for i := range inst {
		inst[i] = uint8(value)
	}

	return inst
}

// SetFlag assigns validation code to specified transaction
func (obj TxValidationFlags) SetFlag(txIndex int, flag peer.TxValidationCode) {
	obj[txIndex] = uint8(flag)
}

// Flag returns validation code at specified transaction
func (obj TxValidationFlags) Flag(txIndex int) peer.TxValidationCode {
	return peer.TxValidationCode(obj[txIndex])
}

// IsValid checks if specified transaction is valid
func (obj TxValidationFlags) IsValid(txIndex int) bool {
	return obj.IsSetTo(txIndex, peer.TxValidationCode_VALID)
}

// IsInvalid checks if specified transaction is invalid
func (obj TxValidationFlags) IsInvalid(txIndex int) bool {
	return !obj.IsValid(txIndex)
}

// IsSetTo returns true if the specified transaction equals flag; false otherwise.
func (obj TxValidationFlags) IsSetTo(txIndex int, flag peer.TxValidationCode) bool {
	return obj.Flag(txIndex) == flag
}
