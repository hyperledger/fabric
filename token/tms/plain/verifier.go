/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/creatorinfo.go -fake-name CreatorInfo . CreatorInfo

// CreatorInfo is used to identify token owners.
type CreatorInfo interface {
	Public() []byte
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
	// SetState sets the given value for the given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId.
	SetState(namespace string, key string, value []byte) error
}

// A Verifier validates and commits token transactions.
type Verifier struct {
	PolicyValidator transaction.PolicyValidator
}

// ProcessTx checks that transactions are correct wrt. the most recent ledger state.
// ProcessTx checks are ones that shall be done sequentially, since transactions within a block may introduce dependencies.
func (v *Verifier) ProcessTx(txID string, creator transaction.CreatorInfo, ttx *token.TokenTransaction, simulator transaction.LedgerWriter) error {
	err := v.checkProcess(txID, creator, ttx, simulator)
	if err != nil {
		return err
	}

	err = v.commitProcess(txID, creator, ttx, simulator)
	if err != nil {
		return err
	}
	return nil
}

func (v *Verifier) checkProcess(txID string, creator transaction.CreatorInfo, ttx *token.TokenTransaction, simulator transaction.LedgerReader) error {
	action := ttx.GetPlainAction()
	if action == nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("check process failed for transaction '%s': missing token action", txID)}
	}

	err := v.checkAction(creator, action, txID, simulator)
	if err != nil {
		return err
	}

	err = v.checkTxDoesNotExist(txID, simulator)
	if err != nil {
		return err
	}
	return nil
}

func (v *Verifier) checkAction(creator transaction.CreatorInfo, plainAction *token.PlainTokenAction, txID string, simulator transaction.LedgerReader) error {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		return v.checkImportAction(creator, action.PlainImport, txID, simulator)
	default:
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("unknown plain token action: %T", action)}
	}
}

func (v *Verifier) checkImportAction(creator transaction.CreatorInfo, importAction *token.PlainImport, txID string, simulator transaction.LedgerReader) error {
	err := v.checkImportOutputs(importAction.GetOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	return v.checkImportPolicy(creator, txID, importAction)
}

func (v *Verifier) checkImportOutputs(outputs []*token.PlainOutput, txID string, simulator transaction.LedgerReader) error {
	if len(outputs) == 0 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("no outputs in transaction: %s", txID)}
	}
	for i, output := range outputs {
		err := v.checkOutputDoesNotExist(i, txID, simulator)
		if err != nil {
			return err
		}

		if output.Quantity == 0 {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("output %d quantity is 0 in transaction: %s", i, txID)}
		}
	}
	return nil
}

func (v *Verifier) checkOutputDoesNotExist(index int, txID string, simulator transaction.LedgerReader) error {
	outputID, err := createOutputKey(txID, index)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
	}

	existingOutputBytes, err := simulator.GetState(tokenNamespace, outputID)
	if err != nil {
		return err
	}

	if existingOutputBytes != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("output already exists: %s", outputID)}
	}
	return nil
}

func (v *Verifier) checkTxDoesNotExist(txID string, simulator transaction.LedgerReader) error {
	txKey, err := createTxKey(txID)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating txID: %s", err)}
	}

	existingTx, err := simulator.GetState(tokenNamespace, txKey)
	if err != nil {
		return err
	}

	if existingTx != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("transaction already exists: %s", txID)}
	}
	return nil
}

func (v *Verifier) checkImportPolicy(creator transaction.CreatorInfo, txID string, importData *token.PlainImport) error {
	for _, output := range importData.Outputs {
		err := v.PolicyValidator.IsIssuer(creator, output.Type)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("import policy check failed: %s", err)}
		}
	}
	return nil
}

// Namespace under which token composite keys are stored
const tokenNamespace = "tms"

func (v *Verifier) commitProcess(txID string, creator transaction.CreatorInfo, ttx *token.TokenTransaction, simulator transaction.LedgerWriter) error {
	err := v.commitAction(ttx.GetPlainAction(), txID, simulator)
	if err != nil {
		return err
	}

	err = v.addTransaction(txID, ttx, simulator)
	if err != nil {
		return err
	}

	return nil
}

func (v *Verifier) commitAction(plainAction *token.PlainTokenAction, txID string, simulator transaction.LedgerWriter) (err error) {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		err = v.commitImportAction(action.PlainImport, txID, simulator)
	}
	return
}

func (v *Verifier) commitImportAction(importAction *token.PlainImport, txID string, simulator transaction.LedgerWriter) error {
	for i, output := range importAction.GetOutputs() {
		outputID, err := createOutputKey(txID, i)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = v.addOutput(outputID, output, simulator)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Verifier) addOutput(outputID string, output *token.PlainOutput, simulator transaction.LedgerWriter) error {
	outputBytes := utils.MarshalOrPanic(output)

	return simulator.SetState(tokenNamespace, outputID, outputBytes)
}

func (v *Verifier) addTransaction(txID string, ttx *token.TokenTransaction, simulator transaction.LedgerWriter) error {
	ttxBytes := utils.MarshalOrPanic(ttx)

	ttxID, err := createTxKey(txID)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating txID: %s", err)}
	}

	return simulator.SetState(tokenNamespace, ttxID, ttxBytes)
}

// For composite keys for outputs
const tokenOutputKey = "tokenOutput"

// Create a ledger key for an individual output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenOutputKey, []string{txID, strconv.Itoa(index)})
}

// For composite keys for transactions
const tokenTxKey = "tokenTx"

// Create a ledger key for a token transaction, as a function of the transaction ID
func createTxKey(txID string) (string, error) {
	return createCompositeKey(tokenTxKey, []string{txID})
}

// createCompositeKey and its related functions and consts copied from core/chaincode/shim/chaincode.go
func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
)

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}
