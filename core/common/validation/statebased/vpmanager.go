/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
)

// ValidationParameterUpdatedErr is returned whenever
// Validation Parameters for a key could not be
// supplied because they are being updated
type ValidationParameterUpdatedError struct {
	CC     string
	Coll   string
	Key    string
	Height uint64
	Txnum  uint64
}

func (f *ValidationParameterUpdatedError) Error() string {
	return fmt.Sprintf("validation parameters for key [%s] in namespace [%s:%s] have been changed in transaction %d of block %d", f.Key, f.CC, f.Coll, f.Txnum, f.Height)
}

// KeyLevelValidationParameterManager is used by validation plugins in order
// to retrieve validation parameters for individual KVS keys.
// The functions are supposed to be called in the following order:
//  1. the validation plugin called to validate a certain tx calls ExtractValidationParameterDependency
//     in order for the manager to be able to determine whether validation parameters from the ledger
//     can be used or whether they are being updated by a transaction in this block.
//  2. the validation plugin issues 0 or more calls to GetValidationParameterForKey.
//  3. the validation plugin determines the validation code for the tx and calls SetTxValidationCode.
type KeyLevelValidationParameterManager interface {
	// GetValidationParameterForKey returns the validation parameter for the
	// supplied KVS key identified by (cc, coll, key) at the specified block
	// height h. The function returns the validation parameter and no error in case of
	// success, or nil and an error otherwise. One particular error that may be
	// returned is ValidationParameterUpdatedErr, which is returned in case the
	// validation parameters for the given KVS key have been changed by a transaction
	// with txNum smaller than the one supplied by the caller. This protects from a
	// scenario where a transaction changing validation parameters is marked as valid
	// by VSCC and is later invalidated by the committer for other reasons (e.g. MVCC
	// conflicts).  This function may be blocking until sufficient information has
	// been passed (by calling ApplyRWSetUpdates and ApplyValidatedRWSetUpdates) for
	// all txes with txNum smaller than the one supplied by the caller.
	GetValidationParameterForKey(cc, coll, key string, blockNum, txNum uint64) ([]byte, error)

	// ExtractValidationParameterDependency is used to determine which validation parameters are
	// updated by transaction at height `blockNum, txNum`. This is needed
	// to determine which txes have dependencies for specific validation parameters and will
	// determine whether GetValidationParameterForKey may block.
	ExtractValidationParameterDependency(blockNum, txNum uint64, rwset []byte)

	// SetTxValidationResult sets the validation result for transaction at height
	// `blockNum, txNum` for the specified chaincode `cc`.
	// This is used to determine whether the dependencies set by
	// ExtractValidationParameterDependency matter or not.
	SetTxValidationResult(cc string, blockNum, txNum uint64, err error)
}
