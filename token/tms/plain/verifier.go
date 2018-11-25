/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/ledger"
	"github.com/pkg/errors"
)

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	tokenOutput           = "tokenOutput"
	tokenRedeem           = "tokenRedeem"
	tokenTx               = "tokenTx"
	tokenDelegatedOutput  = "tokenDelegatedOutput"
	tokenInput            = "tokenInput"
	tokenDelegatedInput   = "tokenDelegateInput"
	tokenNameSpace        = "tms"
)

var verifierLogger = flogging.MustGetLogger("token.tms.plain.verifier")

// A Verifier validates and commits token transactions.
type Verifier struct {
	IssuingValidator identity.IssuingValidator
}

// ProcessTx checks that transactions are correct wrt. the most recent ledger state.
// ProcessTx checks are ones that shall be done sequentially, since transactions within a block may introduce dependencies.
func (v *Verifier) ProcessTx(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("checking transaction with txID '%s'", txID)
	err := v.checkProcess(txID, creator, ttx, simulator)
	if err != nil {
		return err
	}

	verifierLogger.Debugf("committing transaction with txID '%s'", txID)
	err = v.commitProcess(txID, creator, ttx, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing transaction with txID '%s': %s", txID, err)
		return err
	}
	verifierLogger.Debugf("successfully processed transaction with txID '%s'", txID)
	return nil
}

func (v *Verifier) checkProcess(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerReader) error {
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

func (v *Verifier) checkAction(creator identity.PublicInfo, plainAction *token.PlainTokenAction, txID string, simulator ledger.LedgerReader) error {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		return v.checkImportAction(creator, action.PlainImport, txID, simulator)
	case *token.PlainTokenAction_PlainTransfer:
		return v.checkTransferAction(creator, action.PlainTransfer, txID, simulator)
	case *token.PlainTokenAction_PlainRedeem:
		return v.checkRedeemAction(creator, action.PlainRedeem, txID, simulator)
	case *token.PlainTokenAction_PlainApprove:
		return v.checkApproveAction(creator, action.PlainApprove, txID, simulator)
	default:
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("unknown plain token action: %T", action)}
	}
}

func (v *Verifier) checkImportAction(creator identity.PublicInfo, importAction *token.PlainImport, txID string, simulator ledger.LedgerReader) error {
	err := v.checkImportOutputs(importAction.GetOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	return v.checkImportPolicy(creator, txID, importAction)
}

func (v *Verifier) checkImportOutputs(outputs []*token.PlainOutput, txID string, simulator ledger.LedgerReader) error {
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

func (v *Verifier) checkTransferAction(creator identity.PublicInfo, transferAction *token.PlainTransfer, txID string, simulator ledger.LedgerReader) error {
	outputType, outputSum, err := v.checkTransferOutputs(transferAction.GetOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	inputType, inputSum, err := v.checkTransferInputs(creator, transferAction.GetInputs(), txID, simulator)
	if err != nil {
		return err
	}
	if outputType != inputType {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token type mismatch in inputs and outputs for transfer with ID %s (%s vs %s)", txID, outputType, inputType)}
	}
	if outputSum != inputSum {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token sum mismatch in inputs and outputs for transfer with ID %s (%d vs %d)", txID, outputSum, inputSum)}
	}
	return nil
}

func (v *Verifier) checkRedeemAction(creator identity.PublicInfo, redeemAction *token.PlainTransfer, txID string, simulator ledger.LedgerReader) error {
	// first perform the same checking as transfer
	err := v.checkTransferAction(creator, redeemAction, txID, simulator)
	if err != nil {
		return err
	}

	// then perform additional checking for redeem outputs
	// redeem transaction should not have more than 2 outputs.
	outputs := redeemAction.GetOutputs()
	if len(outputs) > 2 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("too many outputs (%d) in a redeem transaction", len(outputs))}
	}

	// output[0] should always be a redeem output - i.e., owner should be nil
	if outputs[0].Owner != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("owner should be nil in a redeem output")}
	}

	// if output[1] presents, its owner must be same as the creator
	if len(outputs) == 2 && !bytes.Equal(creator.Public(), outputs[1].Owner) {
		println(hex.EncodeToString(creator.Public()))
		println(hex.EncodeToString(outputs[1].Owner))
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("wrong owner for remaining tokens, should be original owner %s, but got %s", creator.Public(), outputs[1].Owner)}
	}

	return nil
}

func (v *Verifier) checkOutputDoesNotExist(index int, txID string, simulator ledger.LedgerReader) error {
	outputID, err := createOutputKey(txID, index)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
	}

	existingOutputBytes, err := simulator.GetState(tokenNameSpace, outputID)
	if err != nil {
		return err
	}

	if existingOutputBytes != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("output already exists: %s", outputID)}
	}
	return nil
}

func (v *Verifier) checkTransferOutputs(outputs []*token.PlainOutput, txID string, simulator ledger.LedgerReader) (string, uint64, error) {
	tokenType := ""
	tokenSum := uint64(0)
	for i, output := range outputs {
		err := v.checkOutputDoesNotExist(i, txID, simulator)
		if err != nil {
			return "", 0, err
		}
		if tokenType == "" {
			tokenType = output.GetType()
		} else if tokenType != output.GetType() {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types ('%s', '%s') in transfer output for txID '%s'", tokenType, output.GetType(), txID)}
		}
		tokenSum += output.GetQuantity()
	}
	return tokenType, tokenSum, nil
}

func (v *Verifier) checkTransferInputs(creator identity.PublicInfo, inputIDs []*token.InputId, txID string, simulator ledger.LedgerReader) (string, uint64, error) {
	tokenType := ""
	inputSum := uint64(0)
	processedIDs := make(map[string]bool)
	for _, id := range inputIDs {
		inputKey, err := createOutputKey(id.TxId, int(id.Index))
		if err != nil {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID for transfer input: %s", err)}
		}
		input, err := v.getOutput(inputKey, simulator)
		if err != nil {
			return "", 0, err
		}
		err = v.checkInputOwner(creator, input, inputKey)
		if err != nil {
			return "", 0, err
		}
		if tokenType == "" {
			tokenType = input.GetType()
		} else if tokenType != input.GetType() {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types in transfer input for txID: %s (%s, %s)", txID, tokenType, input.GetType())}
		}
		if processedIDs[inputKey] {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("token input '%s' spent more than once in single transfer with txID '%s'", inputKey, txID)}
		}
		processedIDs[inputKey] = true
		inputSum += input.GetQuantity()
		spentKey, err := createSpentKey(id.TxId, int(id.Index))
		if err != nil {
			return "", 0, err
		}
		spent, err := v.isSpent(spentKey, simulator)
		if err != nil {
			return "", 0, err
		}
		if spent {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer has already been spent", inputKey)}
		}
	}
	return tokenType, inputSum, nil
}

func (v *Verifier) checkInputOwner(creator identity.PublicInfo, input *token.PlainOutput, inputID string) error {
	if !bytes.Equal(creator.Public(), input.Owner) {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("transfer input with ID %s not owned by creator", inputID)}
	}
	return nil
}

func (v *Verifier) checkTxDoesNotExist(txID string, simulator ledger.LedgerReader) error {
	txKey, err := createTxKey(txID)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating txID: %s", err)}
	}

	existingTx, err := simulator.GetState(tokenNameSpace, txKey)
	if err != nil {
		return err
	}

	if existingTx != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("transaction already exists: %s", txID)}
	}
	return nil
}

func (v *Verifier) checkImportPolicy(creator identity.PublicInfo, txID string, importData *token.PlainImport) error {
	for _, output := range importData.Outputs {
		err := v.IssuingValidator.Validate(creator, output.Type)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("import policy check failed: %s", err)}
		}
	}
	return nil
}

func (v *Verifier) commitProcess(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("committing action with txID '%s'", txID)
	err := v.commitAction(ttx.GetPlainAction(), txID, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing action with txID '%s': %s", txID, err)
		return err
	}

	verifierLogger.Debugf("adding transaction with txID '%s'", txID)
	err = v.addTransaction(txID, ttx, simulator)
	if err != nil {
		verifierLogger.Debugf("error adding transaction with txID '%s': %s", txID, err)
		return err
	}

	verifierLogger.Debugf("action with txID '%s' committed successfully", txID)
	return nil
}

func (v *Verifier) commitAction(plainAction *token.PlainTokenAction, txID string, simulator ledger.LedgerWriter) (err error) {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		err = v.commitImportAction(action.PlainImport, txID, simulator)
	case *token.PlainTokenAction_PlainTransfer:
		err = v.commitTransferAction(action.PlainTransfer, txID, simulator)
	case *token.PlainTokenAction_PlainRedeem:
		// call the same commit method as transfer because PlainRedeem points to the same type of outputs as transfer
		err = v.commitTransferAction(action.PlainRedeem, txID, simulator)
	case *token.PlainTokenAction_PlainApprove:
		err = v.commitApproveAction(action.PlainApprove, txID, simulator)
	}
	return
}

func (v *Verifier) commitImportAction(importAction *token.PlainImport, txID string, simulator ledger.LedgerWriter) error {
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

// commitTransferAction is called for both transfer and redeem transactions
// Check the owner of each output to determine how to generate the key
func (v *Verifier) commitTransferAction(transferAction *token.PlainTransfer, txID string, simulator ledger.LedgerWriter) error {
	var outputID string
	var err error
	for i, output := range transferAction.GetOutputs() {
		if output.Owner != nil {
			outputID, err = createOutputKey(txID, i)
		} else {
			outputID, err = createRedeemKey(txID, i)
		}
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = v.addOutput(outputID, output, simulator)
		if err != nil {
			return err
		}
	}
	return v.markInputsSpent(txID, transferAction.GetInputs(), simulator)
}

func (v *Verifier) checkApproveAction(creator identity.PublicInfo, approveAction *token.PlainApprove, txID string, simulator ledger.LedgerReader) error {
	outputType, outputSum, err := v.checkApproveOutputs(creator, approveAction.GetOutput(), approveAction.GetDelegatedOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	inputType, inputSum, err := v.checkTransferInputs(creator, approveAction.GetInputs(), txID, simulator)
	if err != nil {
		return err
	}
	if outputType != inputType {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token type mismatch in inputs and outputs for approve with ID %s (%s vs %s)", txID, outputType, inputType)}
	}
	if outputSum != inputSum {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token sum mismatch in inputs and outputs for approve with ID %s (%d vs %d)", txID, outputSum, inputSum)}
	}
	return nil
}

func (v *Verifier) commitApproveAction(approveAction *token.PlainApprove, txID string, simulator ledger.LedgerWriter) error {
	if approveAction.GetOutput() != nil {
		outputID, err := createOutputKey(txID, 0)
		if err != nil {
			return err
		}
		err = v.addOutput(outputID, approveAction.GetOutput(), simulator)
		if err != nil {
			return err
		}
	}
	for i, delegatedOutput := range approveAction.GetDelegatedOutputs() {
		// createDelegatedOutputKey() error already checked in checkDelegatedOutputsDoesNotExist
		outputID, _ := createDelegatedOutputKey(txID, i)
		err := v.addDelegatedOutput(outputID, delegatedOutput, simulator)
		if err != nil {
			return err
		}
	}
	return v.markInputsSpent(txID, approveAction.GetInputs(), simulator)
}

func (v *Verifier) checkApproveOutputs(creator identity.PublicInfo, output *token.PlainOutput, delegatedOutputs []*token.PlainDelegatedOutput, txID string, simulator ledger.LedgerReader) (string, uint64, error) {
	tokenType := ""
	tokenSum := uint64(0)

	// output is optional in approve tx
	// check whether this tx contains an output
	if output != nil {
		// check that the owner is not an empty slice
		if !bytes.Equal(output.Owner, creator.Public()) {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the owner of the output is not valid")}
		}
		tokenType = output.GetType()
		err := v.checkOutputDoesNotExist(0, txID, simulator)
		if err != nil {
			return "", 0, err
		}
		// check that a spent key can be created for the output in the future
		spentKey, err := createSpentKey(txID, 0)
		if err != nil {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the output for approve txID '%s' is invalid: cannot create a spent key", txID)}
		}
		spent, err := v.isSpent(spentKey, simulator)
		if err != nil {
			return "", 0, errors.New(fmt.Sprintf("the output for approve txID '%s' is invalid: cannot check spent status", txID))
		}
		if spent {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the output for approve txID '%s' is invalid: cannot create a spent key", txID)}
		}
		tokenSum = output.GetQuantity()
	}

	// check consistency of delegated outputs
	for i, delegatedOutput := range delegatedOutputs {
		// check that delegated outputs have the creator as one of the owners
		if !bytes.Equal(delegatedOutput.Owner, creator.Public()) {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the owner of the delegated output is invalid")}
		}
		// check consistency of type
		if tokenType == "" {
			tokenType = delegatedOutput.GetType()
		} else if tokenType != delegatedOutput.GetType() {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types ('%s', '%s') in approve outputs for txID '%s'", tokenType, delegatedOutput.GetType(), txID)}
		}
		// each delegated output should have one delegatees
		if len(delegatedOutput.Delegatees) != 1 {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the number of delegates in approve txID '%s' is not 1, it is [%d]", txID, len(delegatedOutput.Delegatees))}
		}
		// check that the delegatee is not an empty slice
		if len(delegatedOutput.Delegatees[0]) == 0 {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the delegated output for approve txID '%s' does not have a delegatee", txID)}
		}

		err := v.checkDelegatedOutputDoesNotExist(i, txID, simulator)
		if err != nil {
			return "", 0, errors.New(fmt.Sprintf("the delegated output for approve txID '%s' is invalid: the ID exists", txID))
		}
		// check that a spent key can be created for the delegate output in the future
		spentKey, err := createSpentDelegatedOutputKey(txID, int(i))
		if err != nil {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the delegated output for approve txID '%s' is invalid: cannot create a spent key", txID)}
		}
		spent, err := v.isSpent(spentKey, simulator)
		if err != nil {
			return "", 0, errors.New(fmt.Sprintf("the delegated output for approve txID '%s' is invalid: cannot check spent status", txID))
		}
		if spent {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("the delegated output for approve txID '%s' is invalid: cannot create a spent key", txID)}
		}
		tokenSum += delegatedOutput.GetQuantity()
	}

	return tokenType, tokenSum, nil
}

func (v *Verifier) addOutput(outputID string, output *token.PlainOutput, simulator ledger.LedgerWriter) error {
	outputBytes := utils.MarshalOrPanic(output)

	return simulator.SetState(tokenNameSpace, outputID, outputBytes)
}

func (v *Verifier) addDelegatedOutput(outputID string, delegatedOutput *token.PlainDelegatedOutput, simulator ledger.LedgerWriter) error {
	outputBytes := utils.MarshalOrPanic(delegatedOutput)

	return simulator.SetState(tokenNameSpace, outputID, outputBytes)
}

func (v *Verifier) addTransaction(txID string, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	ttxBytes := utils.MarshalOrPanic(ttx)

	ttxID, err := createTxKey(txID)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating txID: %s", err)}
	}

	return simulator.SetState(tokenNameSpace, ttxID, ttxBytes)
}

var TokenInputSpentMarker = []byte{1}

func (v *Verifier) markInputsSpent(txID string, inputs []*token.InputId, simulator ledger.LedgerWriter) error {
	for _, id := range inputs {
		inputID, err := createSpentKey(id.TxId, int(id.Index))
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating spent key: %s", err)}
		}
		verifierLogger.Debugf("marking input '%s' as spent", inputID)
		err = simulator.SetState(tokenNameSpace, inputID, TokenInputSpentMarker)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Verifier) getOutput(outputID string, simulator ledger.LedgerReader) (*token.PlainOutput, error) {
	outputBytes, err := simulator.GetState(tokenNameSpace, outputID)
	if err != nil {
		return nil, err
	}
	if outputBytes == nil {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer does not exist", outputID)}
	}
	if len(outputBytes) == 0 {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer does not exist", outputID)}
	}
	output := &token.PlainOutput{}
	err = proto.Unmarshal(outputBytes, output)
	if err != nil {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("unmarshaling error: %s", err)}
	}
	return output, nil
}

// isSpent checks whether an output token with identifier outputID has been spent.
func (v *Verifier) isSpent(spentKey string, simulator ledger.LedgerReader) (bool, error) {
	verifierLogger.Debugf("checking if input with ID '%s' has been spent", spentKey)
	result, err := simulator.GetState(tokenNameSpace, spentKey)
	return result != nil, err
}

// Create a ledger key for an individual output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenOutput, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a redeem output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createRedeemKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenRedeem, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a token transaction, as a function of the transaction ID
func createTxKey(txID string) (string, error) {
	return createCompositeKey(tokenTx, []string{txID})
}

// Create a ledger key for a spent individual output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createSpentKey(txID string, index int) (string, error) {
	return createCompositeKey("tokenInput", []string{txID, strconv.Itoa(index)})
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

func parseCompositeKeyBytes(keyBytes []byte) string {
	return string(keyBytes)
}

func getCompositeKeyBytes(compositeKey string) []byte {
	return []byte(compositeKey)
}

// Create a ledger key for an individual delegated output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createDelegatedOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenDelegatedOutput, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a spent individual delegated output in a token transaction, as a function of
// the transaction ID, and the index of the delegated output
func createSpentDelegatedOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenDelegatedInput, []string{txID, strconv.Itoa(index)})
}

func (v *Verifier) checkDelegatedOutputDoesNotExist(index int, txID string, simulator ledger.LedgerReader) error {
	outputID, err := createDelegatedOutputKey(txID, index)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
	}

	existingOutputBytes, err := simulator.GetState(tokenNameSpace, outputID)
	if err != nil {
		return err
	}

	if existingOutputBytes != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("output already exists: %s", outputID)}
	}
	return nil
}
