/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger/fabric/protos/token"
)

//go:generate counterfeiter -o mock/tms_tx_processor.go -fake-name TMSTxProcessor . TMSTxProcessor
//go:generate counterfeiter -o mock/tms_manager.go -fake-name TMSManager . TMSManager

// TMSTxProcessor is used to generate the read-dependencies of a token transaction
// (read-set) along with the ledger updates triggered by that transaction
// (write-set); read-write sets are returned implicitly via the simulator object
// that is passed as parameter in the Commit function
type TMSTxProcessor interface {
	// ProcessTx parses ttx to generate a RW set
	ProcessTx(txID string, creator CreatorInfo, ttx *token.TokenTransaction, simulator LedgerWriter) error
}

type TMSManager interface {
	// GetTxProcessor returns a TxProcessor for TMS transactions for the provided channel
	GetTxProcessor(channel string) (TMSTxProcessor, error)
	// SetPolicyValidator sets the policy validator for the specified channel
	SetPolicyValidator(channel string, validator PolicyValidator)
}

//go:generate counterfeiter -o mock/creator_info.go -fake-name CreatorInfo . CreatorInfo

// CreatorInfo is used to identify token owners.
type CreatorInfo interface {
	Public() []byte
}

type TxCreatorInfo struct {
	public []byte
}

func (t *TxCreatorInfo) Public() []byte {
	return t.public
}

//go:generate counterfeiter -o mock/policy_validator.go -fake-name PolicyValidator . PolicyValidator

// PolicyValidator interface, used by TMS components to validate fabric channel policies.
type PolicyValidator interface {
	// IsIssuer returns true if the creator can issue tokens of the given type, false if not
	IsIssuer(creator CreatorInfo, tokenType string) error
}

// LedgerReader interface, used to read from a ledger.
type LedgerReader interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)
}

//go:generate counterfeiter -o mock/ledger_writer.go -fake-name LedgerWriter . LedgerWriter

// LedgerWriter interface, used to read from, and write to, a ledger.
type LedgerWriter interface {
	LedgerReader
	// SetState sets the given value for the given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	SetState(namespace string, key string, value []byte) error
}
