/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v13

import (
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// StateBasedValidator is used to validate a transaction that performs changes to
// KVS keys that use key-level endorsement policies. This interface is supposed to be called
// by any validator plugin (including the default validator plugin). The functions of this
// interface are to be called as follows:
// 1) the validator plugin calls PreValidate (even before determining whether the transaction is
//    valid)
// 2) the validator plugin calls Validate before or after having determined the validity of the
//    transaction based on other considerations
// 3) the validator plugin determines the overall validity of the transaction and then calls
//    PostValidate
type StateBasedValidator interface {
	// PreValidate sets the internal data structures of the validator needed before validation
	// of transaction `txNum` in the specified block can proceed
	PreValidate(txNum uint64, block *common.Block)

	// Validate determines whether the transaction on the specified channel at the specified height
	// is valid according to its chaincode-level endorsement policy and any key-level validation
	// parametres
	Validate(cc string, blockNum, txNum uint64, rwset, prp, ep []byte, endorsements []*peer.Endorsement) commonerrors.TxValidationError

	// PostValidate sets the internal data structures of the validator needed after the validation
	// code was determined for a transaction on the specified channel at the specified height
	PostValidate(cc string, blockNum, txNum uint64, err error)
}
