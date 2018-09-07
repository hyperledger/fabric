/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"sync"

	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/credential.go -fake-name Credential . Credential

// The Credential is used to identify token owners.
type Credential interface {
	Public() []byte
	Private() []byte
}

//go:generate counterfeiter -o mock/policy_validator.go -fake-name PolicyValidator . PolicyValidator

// A PolicyValidator interface, used by TMS components to validate fabric channel policies
type PolicyValidator interface {
	// IsIssuer returns true if the creator can issue tokens of the given type, false if not
	IsIssuer(creator Credential, tokenType string) error
}

//go:generate counterfeiter -o mock/pool.go -fake-name Pool . Pool

// A Pool implements a UTXO pool
type Pool interface {
	CommitUpdate(transactionData []tms.TransactionData) error
}

// A Verifier validates and commits token transactions.
type Verifier struct {
	Pool            Pool
	PolicyValidator PolicyValidator

	mutex sync.Mutex
}

// Validate checks transactions to see if they are well-formed.
// Validation checks can be done in parallel for all of the token transactions within a block.
func (v *Verifier) Validate(creator Credential, data tms.TransactionData) error {
	plainAction := data.Tx.GetPlainAction()
	if plainAction == nil {
		return errors.Errorf("validation failed: unknown action: %T", data.Tx.GetAction())
	}

	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		return nil
	default:
		return errors.Errorf("validation failed: unknown plain token action: %T", action)
	}
}

// Commit checks that transactions are correct wrt. the most recent ledger state.
// Commit checks shall be done in parallel, since transactions within a block may introduce dependencies.
func (v *Verifier) Commit(creator Credential, transactionData []tms.TransactionData) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	for _, data := range transactionData {
		plainAction := data.Tx.GetPlainAction()
		if plainAction == nil {
			return errors.Errorf("commit failed: unknown action: %T", data.Tx.GetAction())
		}

		switch action := plainAction.Data.(type) {
		case *token.PlainTokenAction_PlainImport:
			err := v.commitCheckImport(creator, data.TxID, action.PlainImport)
			if err != nil {
				return errors.WithMessage(err, "commit failed")
			}
		default:
			return errors.Errorf("commit failed: unknown plain token action: %T", action)
		}
	}

	err := v.Pool.CommitUpdate(transactionData)
	if err != nil {
		return errors.Wrap(err, "commit failed")
	}
	return nil
}

func (v *Verifier) commitCheckImport(creator Credential, txID string, importData *token.PlainImport) error {
	for _, output := range importData.Outputs {
		err := v.PolicyValidator.IsIssuer(creator, output.Type)
		if err != nil {
			return err
		}
	}
	return nil
}
