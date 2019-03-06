/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/ledger"
	"github.com/pkg/errors"
)

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	tokenKeyPrefix        = "token"
	tokenNameSpace        = "_fabtoken"
	numComponentsInKey    = 3 // 3 components: owner, txid, index, excluding tokenKeyPrefix
	ownerSeparator        = "/"
)

var verifierLogger = flogging.MustGetLogger("token.tms.plain.verifier")

// A Verifier validates and commits token transactions.
type Verifier struct {
	IssuingValidator    identity.IssuingValidator
	TokenOwnerValidator identity.TokenOwnerValidator
}

// ProcessTx checks that transactions are correct wrt. the most recent ledger state.
// ProcessTx checks are ones that shall be done sequentially, since transactions within a block may introduce dependencies.
func (v *Verifier) ProcessTx(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("checking transaction with txID '%s'", txID)

	// create TokenOwner from creator and pass it through so that we don't have to create it multiple times
	tokenOwner := &token.TokenOwner{Type: token.TokenOwner_MSP_IDENTIFIER, Raw: creator.Public()}

	err := v.checkProcess(txID, creator, tokenOwner, ttx, simulator)
	if err != nil {
		return err
	}

	verifierLogger.Debugf("committing transaction with txID '%s'", txID)
	err = v.commitProcess(txID, tokenOwner, ttx, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing transaction with txID '%s': %s", txID, err)
		return err
	}
	verifierLogger.Debugf("successfully processed transaction with txID '%s'", txID)
	return nil
}

func (v *Verifier) checkProcess(txID string, creator identity.PublicInfo, tokenOwner *token.TokenOwner, ttx *token.TokenTransaction, simulator ledger.LedgerReader) error {
	action := ttx.GetTokenAction()
	if action == nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("check process failed for transaction '%s': missing token action", txID)}
	}

	err := v.checkAction(creator, tokenOwner, action, txID, simulator)
	if err != nil {
		return err
	}

	return nil
}

func (v *Verifier) checkAction(creator identity.PublicInfo, tokenOwner *token.TokenOwner, tokenAction *token.TokenAction, txID string, simulator ledger.LedgerReader) error {
	switch action := tokenAction.Data.(type) {
	case *token.TokenAction_Issue:
		return v.checkIssueAction(creator, action.Issue, txID, simulator)
	case *token.TokenAction_Transfer:
		return v.checkTransferAction(tokenOwner, action.Transfer, txID, simulator)
	case *token.TokenAction_Redeem:
		return v.checkRedeemAction(tokenOwner, action.Redeem, txID, simulator)
	default:
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("unknown plain token action: %T", action)}
	}
}

func (v *Verifier) checkIssueAction(creator identity.PublicInfo, issueAction *token.Issue, txID string, simulator ledger.LedgerReader) error {
	err := v.checkIssueOutputs(issueAction.GetOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	return v.checkIssuePolicy(creator, txID, issueAction)
}

func (v *Verifier) checkIssueOutputs(outputs []*token.Token, txID string, simulator ledger.LedgerReader) error {
	if len(outputs) == 0 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("no outputs in transaction: %s", txID)}
	}
	for i, output := range outputs {
		err := v.checkTokenDoesNotExist(output, i, txID, simulator)
		if err != nil {
			return err
		}

		_, err = ToQuantity(output.Quantity, Precision)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("output %d quantity is invalid in transaction: %s", i, txID)}
		}

		err = v.TokenOwnerValidator.Validate(output.Owner)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err '%s'", txID, err)}
		}
	}
	return nil
}

func (v *Verifier) checkTransferAction(tokenOwner *token.TokenOwner, transferAction *token.Transfer, txID string, simulator ledger.LedgerReader) error {
	return v.checkInputsAndOutputs(tokenOwner, transferAction.GetInputs(), transferAction.GetOutputs(), txID, simulator, true)
}

func (v *Verifier) checkRedeemAction(tokenOwner *token.TokenOwner, redeemAction *token.Transfer, txID string, simulator ledger.LedgerReader) error {
	err := v.checkInputsAndOutputs(tokenOwner, redeemAction.GetInputs(), redeemAction.GetOutputs(), txID, simulator, false)
	if err != nil {
		return err
	}

	// perform additional checking for redeem outputs
	// redeem transaction should not have more than 2 outputs.
	outputs := redeemAction.GetOutputs()
	if len(outputs) > 2 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("too many outputs in a redeem transaction")}
	}

	// output[0] should always be a redeem output - i.e., owner should be nil
	if outputs[0].Owner != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("owner should be nil in a redeem output")}
	}

	// if output[1] presents, its owner must be same as the tokenOwner (creator)
	if len(outputs) == 2 && !proto.Equal(tokenOwner, outputs[1].Owner) {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("wrong owner for remaining tokens, should be original owner %s, but got %s", tokenOwner.Raw, outputs[1].Owner.Raw)}
	}

	return nil
}

// checkInputsAndOutputs checks that inputs and outputs are valid and have same type and sum of quantity
func (v *Verifier) checkInputsAndOutputs(
	tokenOwner *token.TokenOwner,
	tokenIds []*token.TokenId,
	outputs []*token.Token,
	txID string,
	simulator ledger.LedgerReader,
	ownerRequired bool) error {

	outputType, outputSum, err := v.checkOutputs(outputs, txID, simulator, ownerRequired)
	if err != nil {
		return err
	}
	inputType, inputSum, err := v.checkInputs(tokenOwner, tokenIds, txID, simulator)
	if err != nil {
		return err
	}
	if outputType != inputType {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token type mismatch in inputs and outputs for transaction ID %s (%s vs %s)", txID, outputType, inputType)}
	}
	cmp, err := outputSum.Cmp(inputSum)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("cannot compare quantities '%s'", err)}
	}
	if cmp != 0 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token sum mismatch in inputs and outputs for transaction ID %s (%d vs %d)", txID, outputSum, inputSum)}
	}
	return nil
}

func (v *Verifier) checkTokenDoesNotExist(token *token.Token, index int, txID string, simulator ledger.LedgerReader) error {
	// when tokens are redeemed we generate an output without an owner.
	// Skip checking because this output is not stored in ledger
	if token.Owner == nil {
		return nil
	}

	ownerString, err := GetTokenOwnerString(token.Owner)
	if err != nil {
		return err
	}

	tokenKey, err := createTokenKey(ownerString, txID, index)
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
	}

	outputBytes, err := simulator.GetState(tokenNameSpace, tokenKey)
	if err != nil {
		return err
	}
	if len(outputBytes) != 0 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token already exists: %s", tokenKey)}
	}
	return nil
}

func (v *Verifier) checkOutputs(outputs []*token.Token, txID string, simulator ledger.LedgerReader, ownerRequired bool) (string, Quantity, error) {
	tokenType := ""
	tokenSum := NewZeroQuantity(Precision)
	for i, output := range outputs {
		err := v.checkTokenDoesNotExist(output, i, txID, simulator)
		if err != nil {
			return "", nil, err
		}
		if tokenType == "" {
			tokenType = output.GetType()
		} else if tokenType != output.GetType() {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types ('%s', '%s') in output for txID '%s'", tokenType, output.GetType(), txID)}
		}
		if ownerRequired {
			err = v.TokenOwnerValidator.Validate(output.GetOwner())
			if err != nil {
				return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err '%s'", txID, err)}
			}
		}
		quantity, err := ToQuantity(output.GetQuantity(), Precision)
		if err != nil {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("quantity in output [%s] is invalid, err '%s'", output.GetQuantity(), err)}
		}

		tokenSum, err = tokenSum.Add(quantity)
		if err != nil {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("failed adding up output quantities, err '%s'", err)}
		}

	}
	return tokenType, tokenSum, nil
}

func (v *Verifier) checkInputs(tokenOwner *token.TokenOwner, tokenIds []*token.TokenId, txID string, simulator ledger.LedgerReader) (string, Quantity, error) {
	if len(tokenIds) == 0 {
		return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("no tokenIds in transaction: %s", txID)}
	}

	tokenType := ""
	inputSum := NewZeroQuantity(Precision)

	tokenKeys, err := createTokenKeys(tokenOwner, tokenIds)
	if err != nil {
		return "", nil, err
	}

	for _, inputKey := range tokenKeys {
		input, err := v.getToken(inputKey, simulator)
		if err != nil {
			return "", nil, err
		}

		err = v.checkInputOwner(tokenOwner, input, inputKey)
		if err != nil {
			return "", nil, err
		}
		if tokenType == "" {
			tokenType = input.GetType()
		} else if tokenType != input.GetType() {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types in input for txID: %s (%s, %s)", txID, tokenType, input.GetType())}
		}

		inputQuantity, err := ToQuantity(input.GetQuantity(), Precision)
		if err != nil {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("quantity in output [%s] is invalid, err '%s'", input.GetQuantity(), err)}
		}
		inputSum, err = inputSum.Add(inputQuantity)
		if err != nil {
			return "", nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("failed adding up input quantities, err '%s'", err)}
		}
	}
	return tokenType, inputSum, nil
}

func (v *Verifier) checkInputOwner(tokenOwner *token.TokenOwner, input *token.Token, tokenId string) error {
	if !proto.Equal(tokenOwner, input.Owner) {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("transfer input with ID %s not owned by creator", tokenId)}
	}
	return nil
}

func (v *Verifier) checkIssuePolicy(creator identity.PublicInfo, txID string, issueData *token.Issue) error {
	for _, output := range issueData.Outputs {
		err := v.IssuingValidator.Validate(creator, output.Type)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("issue policy check failed: %s", err)}
		}
	}
	return nil
}

func (v *Verifier) commitProcess(txID string, tokenOwner *token.TokenOwner, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("committing action with txID '%s'", txID)
	err := v.commitAction(tokenOwner, ttx.GetTokenAction(), txID, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing action with txID '%s': %s", txID, err)
		return err
	}

	verifierLogger.Debugf("action with txID '%s' committed successfully", txID)
	return nil
}

func (v *Verifier) commitAction(tokenOwner *token.TokenOwner, tokenAction *token.TokenAction, txID string, simulator ledger.LedgerWriter) (err error) {
	switch action := tokenAction.Data.(type) {
	case *token.TokenAction_Issue:
		err = v.commitIssueAction(action.Issue, txID, simulator)
	case *token.TokenAction_Transfer:
		err = v.commitTransferAction(tokenOwner, action.Transfer, txID, simulator)
	case *token.TokenAction_Redeem:
		// call the same commit method as transfer because Redeem points to the same type of outputs as transfer
		err = v.commitTransferAction(tokenOwner, action.Redeem, txID, simulator)
	}
	return
}

func (v *Verifier) commitIssueAction(issueAction *token.Issue, txID string, simulator ledger.LedgerWriter) error {
	for i, output := range issueAction.GetOutputs() {
		ownerString, err := GetTokenOwnerString(output.Owner)
		if err != nil {
			return err
		}

		outputID, err := createTokenKey(ownerString, txID, i)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = simulator.SetState(tokenNameSpace, outputID, protoutil.MarshalOrPanic(output))
		if err != nil {
			return err
		}
	}
	return nil
}

// commitTransferAction is called for both transfer and redeem transactions
// Check the owner of each output to determine how to generate the key
func (v *Verifier) commitTransferAction(tokenOwner *token.TokenOwner, transferAction *token.Transfer, txID string, simulator ledger.LedgerWriter) error {
	for i, output := range transferAction.GetOutputs() {
		// when tokens are redeemed we generate an output without an owner.
		// however we do not want this output to be committed on the ledger
		if output.Owner == nil {
			continue
		}

		ownerString, err := GetTokenOwnerString(output.Owner)
		if err != nil {
			return err
		}

		outputID, err := createTokenKey(ownerString, txID, i)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = simulator.SetState(tokenNameSpace, outputID, protoutil.MarshalOrPanic(output))
		if err != nil {
			return err
		}
	}
	return v.spendTokens(tokenOwner, transferAction.GetInputs(), simulator)
}

func (v *Verifier) spendTokens(tokenOwner *token.TokenOwner, tokenIds []*token.TokenId, simulator ledger.LedgerWriter) error {
	// get tokenOwner string to reuse in the loop below
	ownerString, err := GetTokenOwnerString(tokenOwner)
	if err != nil {
		return err
	}

	for _, id := range tokenIds {
		tokenKey, err := createTokenKey(ownerString, id.TxId, int(id.Index))
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating spent key: %s", err)}
		}

		verifierLogger.Debugf("Delete %s\n", tokenKey)
		err = simulator.DeleteState(tokenNameSpace, tokenKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Verifier) getToken(tokenKey string, simulator ledger.LedgerReader) (*token.Token, error) {
	outputBytes, err := simulator.GetState(tokenNameSpace, tokenKey)
	if err != nil {
		return nil, err
	}
	if len(outputBytes) == 0 {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("token with ID %s does not exist", tokenKey)}
	}

	output := &token.Token{}
	err = proto.Unmarshal(outputBytes, output)
	if err != nil {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("unmarshaling error: %s", err)}
	}
	return output, nil
}

// Create a ledger key for an individual output in a token transaction, as a function of
// the token owner, transaction ID, and index of the output
func createTokenKey(tokenOwnerString, txID string, index int) (string, error) {
	return createCompositeKey(tokenKeyPrefix, []string{tokenOwnerString, txID, strconv.Itoa(index)})
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

// this method creates valid tokenKeys from list of input tokenIds and checks for duplicates
func createTokenKeys(tokenOwner *token.TokenOwner, tokenIds []*token.TokenId) ([]string, error) {
	// get the owner string to reuse in the loop below
	ownerString, err := GetTokenOwnerString(tokenOwner)
	if err != nil {
		return nil, err
	}

	tokenKeys := make([]string, len(tokenIds))
	inputKeys := make(map[string]bool, len(tokenIds))

	for i, id := range tokenIds {
		key, err := createTokenKey(ownerString, id.TxId, int(id.Index))
		if err != nil {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID for transfer input: %s", err)}
		}

		if inputKeys[key] {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("Input duplicates found")}
		} else {
			inputKeys[key] = true
			tokenKeys[i] = key
		}
	}

	return tokenKeys, nil
}

// GetTokenOwnerString returns a hex encoded string of the token owner.
// The owner is hashed to reduce the length.
func GetTokenOwnerString(owner *token.TokenOwner) (string, error) {
	if owner == nil {
		verifierLogger.Debug("GetTokenOwnerString: owner is nil")
		return "", nil
	}

	// Hash owner.Type/owner.Raw.
	// Get []byte for owner.Type/, owner.Raw is already []byte.
	typeBytes := []byte(strconv.Itoa(int(owner.Type)) + ownerSeparator)
	hashedOwner, err := factory.GetDefault().Hash(append(typeBytes, owner.Raw...), &bccsp.SHA256Opts{})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hashedOwner), nil
}
