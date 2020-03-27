/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txflags

import (
	"github.com/hyperledger/fabric-protos-go/peer"
)

// ValidationFlags is array of transaction validation codes. It is used when committer validates block.
type ValidationFlags []uint8

// New create new object-array of validation codes with target size.
// Default values: TxValidationCode_NOT_VALIDATED
func New(size int) ValidationFlags {
	return newWithValues(size, peer.TxValidationCode_NOT_VALIDATED)
}

// NewWithValues creates new object-array of validation codes with target size
// and the supplied value
func NewWithValues(size int, value peer.TxValidationCode) ValidationFlags {
	return newWithValues(size, value)
}

func newWithValues(size int, value peer.TxValidationCode) ValidationFlags {
	inst := make(ValidationFlags, size)
	for i := range inst {
		inst[i] = uint8(value)
	}

	return inst
}

// SetFlag assigns validation code to specified transaction
func (obj ValidationFlags) SetFlag(txIndex int, flag peer.TxValidationCode) {
	obj[txIndex] = uint8(flag)
}

// Flag returns validation code at specified transaction
func (obj ValidationFlags) Flag(txIndex int) peer.TxValidationCode {
	return peer.TxValidationCode(obj[txIndex])
}

// IsValid checks if specified transaction is valid
func (obj ValidationFlags) IsValid(txIndex int) bool {
	return obj.IsSetTo(txIndex, peer.TxValidationCode_VALID)
}

// IsInvalid checks if specified transaction is invalid
func (obj ValidationFlags) IsInvalid(txIndex int) bool {
	return !obj.IsValid(txIndex)
}

// IsSetTo returns true if the specified transaction equals flag; false otherwise.
func (obj ValidationFlags) IsSetTo(txIndex int, flag peer.TxValidationCode) bool {
	return obj.Flag(txIndex) == flag
}
