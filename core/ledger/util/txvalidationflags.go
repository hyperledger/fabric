/*
Copyright IBM Corp. 2016 All Rights Reserved.

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
	"github.com/hyperledger/fabric/protos/peer"
)

// TxValidationFlags is array of transaction validation codes. It is used when commiter validates block.
type TxValidationFlags []uint8

// NewTxValidationFlags Create new object-array of validation codes with target size. Default values: valid.
func NewTxValidationFlags(size int) TxValidationFlags {
	inst := make(TxValidationFlags, size)
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
