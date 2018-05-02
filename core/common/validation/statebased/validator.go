/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"github.com/hyperledger/fabric/protos/peer"
)

// StateBasedValidator is used to validate a transaction that performs changes to
// KVS keys that use key-level endorsement policies. This interface is supposed to be called
// by any validator plugin (including the default validator plugin). The functions of this
// interface are to be called as follows:
// 1) the validator plugin extracts the read-write set from the transaction and calls PreValidate
//    (even before determining whether the transaction is valid)
// 2) the validator plugin calls Validate before or after having determined the validity of the
//    transaction based on other considerations
// 3) the validator plugin determines the overall validity of the transaction and then calls
//    PostValidate
type StateBasedValidator interface {
	// PreValidate sets the internal data structures of the validator needed before validation
	// of `rwset` on the specified channel at the specified height
	PreValidate(ch, cc string, blockNum, txNum uint64, rwset []byte) error
	// TODO: remove the channel argument as part of FAB-9908

	// Validate determines whether the transaction on the specified channel at the specified height
	// is valid according to its chaincode-level endorsement policy and any key-level validation
	// parametres
	Validate(ch, cc string, blockNum, txNum uint64, rwset, prp, ep []byte, endorsements []*peer.Endorsement) error
	// TODO: remove the channel argument as part of FAB-9908

	// PostValidate sets the internal data structures of the validator needed after the validation
	// code was determined for a transaction on the specified channel at the specified height
	PostValidate(ch, cc string, blockNum, txNum uint64, vc peer.TxValidationCode) error
	// TODO: remove the channel argument as part of FAB-9908
}
