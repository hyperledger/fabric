/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/ledger"
	"github.com/pkg/errors"
)

// A Transactor that can transfer tokens.
type Transactor struct {
	PublicCredential []byte
	Ledger           ledger.LedgerReader
}

// RequestTransfer creates a TokenTransaction of type transfer request
//func (t *Transactor) RequestTransfer(inTokens []*token.InputId, tokensToTransfer []*token.RecipientTransferShare) (*token.TokenTransaction, error) {
func (t *Transactor) RequestTransfer(request *token.TransferRequest) (*token.TokenTransaction, error) {
	var outputs []*token.PlainOutput

	inputs, tokenType, _, err := t.getInputsFromTokenIds(request.GetTokenIds())
	if err != nil {
		return nil, err
	}

	for _, ttt := range request.GetShares() {
		outputs = append(outputs, &token.PlainOutput{
			Owner:    ttt.Recipient,
			Type:     tokenType,
			Quantity: ttt.Quantity,
		})
	}

	// prepare transfer request
	transaction := &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainTransfer{
					PlainTransfer: &token.PlainTransfer{
						Inputs:  inputs,
						Outputs: outputs,
					},
				},
			},
		},
	}

	return transaction, nil
}

// RequestRedeem creates a TokenTransaction of type redeem request
func (t *Transactor) RequestRedeem(request *token.RedeemRequest) (*token.TokenTransaction, error) {
	if len(request.GetTokenIds()) == 0 {
		return nil, errors.New("no token ids in RedeemRequest")
	}
	if request.GetQuantityToRedeem() <= 0 {
		return nil, errors.Errorf("quantity to redeem [%d] must be greater than 0", request.GetQuantityToRedeem())
	}

	inputs, tokenType, quantitySum, err := t.getInputsFromTokenIds(request.GetTokenIds())
	if err != nil {
		return nil, err
	}

	if quantitySum < request.QuantityToRedeem {
		return nil, errors.Errorf("total quantity [%d] from TokenIds is less than quantity [%d] to be redeemed", quantitySum, request.QuantityToRedeem)
	}

	// add the output for redeem itself
	var outputs []*token.PlainOutput
	outputs = append(outputs, &token.PlainOutput{
		Type:     tokenType,
		Quantity: request.QuantityToRedeem,
	})

	// add another output if there is remaining quantity after redemption
	if quantitySum > request.QuantityToRedeem {
		outputs = append(outputs, &token.PlainOutput{
			Owner:    t.PublicCredential, // PublicCredential is serialized identity for the creator
			Type:     tokenType,
			Quantity: quantitySum - request.QuantityToRedeem,
		})
	}

	// PlainRedeem shares the same data structure as PlainTransfer
	transaction := &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainRedeem{
					PlainRedeem: &token.PlainTransfer{
						Inputs:  inputs,
						Outputs: outputs,
					},
				},
			},
		},
	}

	return transaction, nil
}

// read token data from ledger for each token ids and calculate the sum of quantities for all token ids
// Returns InputIds, token type, sum of token quantities, and error in the case of failure
func (t *Transactor) getInputsFromTokenIds(tokenIds [][]byte) ([]*token.InputId, string, uint64, error) {
	var inputs []*token.InputId
	var tokenType string = ""
	var quantitySum uint64 = 0
	for _, inKeyBytes := range tokenIds {
		// parse the composite key bytes into a string
		inKey := parseCompositeKeyBytes(inKeyBytes)

		// check whether the composite key conforms to the composite key of an output
		namespace, components, err := splitCompositeKey(inKey)
		if err != nil {
			return nil, "", 0, errors.New(fmt.Sprintf("error splitting input composite key: '%s'", err))
		}
		if namespace != tokenOutput {
			return nil, "", 0, errors.New(fmt.Sprintf("namespace not '%s': '%s'", tokenOutput, namespace))
		}
		if len(components) != 2 {
			return nil, "", 0, errors.New(fmt.Sprintf("not enough components in output ID composite key; expected 2, received '%s'", components))
		}
		txID := components[0]
		index, err := strconv.Atoi(components[1])
		if err != nil {
			return nil, "", 0, errors.New(fmt.Sprintf("error parsing output index '%s': '%s'", components[1], err))
		}

		// make sure the output exists in the ledger
		inBytes, err := t.Ledger.GetState(tokenNameSpace, inKey)
		if err != nil {
			return nil, "", 0, err
		}
		if inBytes == nil {
			return nil, "", 0, errors.New(fmt.Sprintf("input '%s' does not exist", inKey))
		}
		input := &token.PlainOutput{}
		err = proto.Unmarshal(inBytes, input)
		if err != nil {
			return nil, "", 0, errors.New(fmt.Sprintf("error unmarshaling input bytes: '%s'", err))
		}

		// check the owner of the token
		if !bytes.Equal(t.PublicCredential, input.Owner) {
			return nil, "", 0, errors.New(fmt.Sprintf("the requestor does not own inputs"))
		}

		// check the token type - only one type allowed per transfer
		if tokenType == "" {
			tokenType = input.Type
		} else if tokenType != input.Type {
			return nil, "", 0, errors.New(fmt.Sprintf("two or more token types specified in input: '%s', '%s'", tokenType, input.Type))
		}
		// add input to list of inputs
		inputs = append(inputs, &token.InputId{TxId: txID, Index: uint32(index)})

		// sum up the quantity
		quantitySum += input.Quantity
	}

	return inputs, tokenType, quantitySum, nil
}

// ListTokens creates a TokenTransaction that lists the unspent tokens owned by owner.
func (t *Transactor) ListTokens() (*token.UnspentTokens, error) {
	iterator, err := t.Ledger.GetStateRangeScanIterator(tokenNameSpace, "", "")
	if err != nil {
		return nil, err
	}

	tokens := make([]*token.TokenOutput, 0)
	prefix, err := createPrefix(tokenOutput)
	if err != nil {
		return nil, err
	}
	for {
		next, err := iterator.Next()

		switch {
		case err != nil:
			return nil, err

		case next == nil:
			// nil response from iterator indicates end of query results
			return &token.UnspentTokens{Tokens: tokens}, nil

		default:
			result, ok := next.(*queryresult.KV)
			if !ok {
				return nil, errors.New("failed to retrieve unspent tokens: casting error")
			}
			if strings.HasPrefix(result.Key, prefix) {
				output := &token.PlainOutput{}
				err = proto.Unmarshal(result.Value, output)
				if err != nil {
					return nil, errors.New("failed to retrieve unspent tokens: casting error")
				}
				if string(output.Owner) == string(t.PublicCredential) {
					spent, err := t.isSpent(result.Key)
					if err != nil {
						return nil, err
					}
					if !spent {
						tokens = append(tokens,
							&token.TokenOutput{
								Type:     output.Type,
								Quantity: output.Quantity,
								Id:       getCompositeKeyBytes(result.Key),
							})
					}
				}
			}
		}
	}

}

func (t *Transactor) RequestApprove(request *token.ApproveRequest) (*token.TokenTransaction, error) {
	if len(request.GetTokenIds()) == 0 {
		return nil, errors.New("no token ids in ApproveAllowanceRequest")
	}

	if len(request.AllowanceShares) == 0 {
		return nil, errors.New("no recipient shares in ApproveAllowanceRequest")
	}

	var delegatedOutputs []*token.PlainDelegatedOutput

	inputs, tokenType, sumQuantity, err := t.getInputsFromTokenIds(request.GetTokenIds())
	if err != nil {
		return nil, err
	}

	// prepare approve tx

	delegatedQuantity := uint64(0)
	for _, share := range request.GetAllowanceShares() {
		if len(share.Recipient) == 0 {
			return nil, errors.Errorf("the recipient in approve must be specified")
		}
		if share.Quantity <= 0 {
			return nil, errors.Errorf("the quantity to approve [%d] must be greater than 0", share.GetQuantity())
		}
		delegatedOutputs = append(delegatedOutputs, &token.PlainDelegatedOutput{
			Owner:      []byte(request.Credential),
			Delegatees: [][]byte{share.Recipient},
			Type:       tokenType,
			Quantity:   share.Quantity,
		})
		delegatedQuantity = delegatedQuantity + share.Quantity
	}
	if sumQuantity < delegatedQuantity {
		return nil, errors.Errorf("insufficient funds: %v < %v", sumQuantity, delegatedQuantity)

	}
	var output *token.PlainOutput
	if sumQuantity != delegatedQuantity {
		output = &token.PlainOutput{
			Owner:    request.Credential,
			Type:     tokenType,
			Quantity: sumQuantity - delegatedQuantity,
		}
	}

	transaction := &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainApprove{
					PlainApprove: &token.PlainApprove{
						Inputs:           inputs,
						DelegatedOutputs: delegatedOutputs,
						Output:           output,
					},
				},
			},
		},
	}

	return transaction, nil
}

func (t *Transactor) RequestTransferFrom(request *token.TransferRequest) (*token.TokenTransaction, error) {
	panic("implement me!")
}

// RequestExpectation allows indirect transfer based on the expectation.
// It creates a token transaction based on the outputs as specified in the expectation.
func (t *Transactor) RequestExpectation(request *token.ExpectationRequest) (*token.TokenTransaction, error) {
	panic("not implemented yet")
}

// Done releases any resources held by this transactor
func (t *Transactor) Done() {
	if t.Ledger != nil {
		t.Ledger.Done()
	}
}

// isSpent checks whether an output token with identifier outputID has been spent.
func (t *Transactor) isSpent(outputID string) (bool, error) {
	key, err := createInputKey(outputID)
	if err != nil {
		return false, err
	}
	result, err := t.Ledger.GetState(tokenNameSpace, key)
	if err != nil {
		return false, err
	}
	if result == nil {
		return false, nil
	}
	return true, nil
}

// Create a ledger key for an individual input in a token transaction, as a function of
// the outputID
func createInputKey(outputID string) (string, error) {
	att := strings.Split(outputID, string(minUnicodeRuneValue))
	return createCompositeKey(tokenInput, att[1:])
}

// Create a prefix as a function of the string passed as argument
func createPrefix(keyword string) (string, error) {
	return createCompositeKey(keyword, nil)
}

// GenerateKeyForTest is here only for testing purposes, to be removed later.
func GenerateKeyForTest(txID string, index int) (string, error) {
	return createOutputKey(txID, index)
}

func splitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 1
	components := []string{}
	for i := 1; i < len(compositeKey); i++ {
		if compositeKey[i] == minUnicodeRuneValue {
			components = append(components, compositeKey[componentIndex:i])
			componentIndex = i + 1
		}
	}
	if len(components) < 2 {
		return "", nil, errors.New("invalid composite key - no components found")
	}
	return components[0], components[1:], nil
}
