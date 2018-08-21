/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/ledger"
	"github.com/pkg/errors"
)

var namespace = "tms"

const tokenInput = "tokenInput"

// A Transactor that can transfer tokens.
type Transactor struct {
	PublicCredential []byte
	Ledger           ledger.LedgerReader
}

// RequestTransfer creates a TokenTransaction of type transfer request
//func (t *Transactor) RequestTransfer(inTokens []*token.InputId, tokensToTransfer []*token.RecipientTransferShare) (*token.TokenTransaction, error) {
func (t *Transactor) RequestTransfer(request *token.TransferRequest) (*token.TokenTransaction, error) {
	var outputs []*token.PlainOutput
	var inputs []*token.InputId
	tokenType := ""
	for _, inKeyBytes := range request.GetTokenIds() {
		// parse the composite key bytes into a string
		inKey := parseCompositeKeyBytes(inKeyBytes)

		// check whether the composite key conforms to the composite key of an output
		namespace, components, err := splitCompositeKey(inKey)
		if err != nil {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("error splitting input composite key: '%s'", err)}
		}
		if namespace != tokenOutput {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("namespace not '%s': '%s'", tokenOutput, namespace)}
		}
		if len(components) != 2 {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("not enough components in output ID composite key; expected 2, received '%s'", components)}
		}
		txID := components[0]
		index, err := strconv.Atoi(components[1])
		if err != nil {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("error parsing output index '%s': '%s'", components[1], err)}
		}

		// make sure the output exists in the ledger
		inBytes, err := t.Ledger.GetState(tokenNamespace, inKey)
		if err != nil {
			return nil, err
		}
		if inBytes == nil {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input '%s' does not exist", inKey)}
		}
		input := &token.PlainOutput{}
		err = proto.Unmarshal(inBytes, input)
		if err != nil {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("error unmarshaling input bytes: '%s'", err)}
		}

		// check the token type - only one type allowed per transfer
		if tokenType == "" {
			tokenType = input.Type
		} else if tokenType != input.Type {
			return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("two or more token types specified in input: '%s', '%s'", tokenType, input.Type)}
		}

		// add input to list of inputs
		inputs = append(inputs, &token.InputId{TxId: txID, Index: uint32(index)})
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

// ListTokens creates a TokenTransaction that lists the unspent tokens owned by owner.
func (t *Transactor) ListTokens() (*token.UnspentTokens, error) {
	iterator, err := t.Ledger.GetStateRangeScanIterator(namespace, "", "")
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

// isSpent checks whether an output token with identifier outputID has been spent.
func (t *Transactor) isSpent(outputID string) (bool, error) {
	key, err := createInputKey(outputID)
	if err != nil {
		return false, err
	}
	result, err := t.Ledger.GetState(namespace, key)
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
