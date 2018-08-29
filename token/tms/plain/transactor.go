/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"strings"

	"github.com/golang/protobuf/proto"
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
func (t *Transactor) RequestTransfer(request *token.TransferRequest) (*token.TokenTransaction, error) {
	panic("not implemented!")
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
								Id:       []byte(result.Key),
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
