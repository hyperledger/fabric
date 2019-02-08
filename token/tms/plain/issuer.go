/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/pkg/errors"
)

// An Issuer that can import new tokens
type Issuer struct {
	TokenOwnerValidator identity.TokenOwnerValidator
}

// RequestImport creates an import request with the token owners, types, and quantities specified in tokensToIssue.
func (i *Issuer) RequestImport(tokensToIssue []*token.TokenToIssue) (*token.TokenTransaction, error) {
	var outputs []*token.PlainOutput
	for _, tti := range tokensToIssue {
		err := i.TokenOwnerValidator.Validate(tti.Recipient)
		if err != nil {
			return nil, errors.Errorf("invalid recipient in issue request '%s'", err)
		}
		outputs = append(outputs, &token.PlainOutput{
			Owner:    tti.Recipient,
			Type:     tti.Type,
			Quantity: tti.Quantity,
		})
	}

	return &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainImport{
					PlainImport: &token.PlainImport{
						Outputs: outputs,
					},
				},
			},
		},
	}, nil
}

// RequestExpectation allows indirect import based on the expectation.
// It creates a token transaction with the outputs as specified in the expectation.
func (i *Issuer) RequestExpectation(request *token.ExpectationRequest) (*token.TokenTransaction, error) {
	if request.GetExpectation() == nil {
		return nil, errors.New("no token expectation in ExpectationRequest")
	}
	if request.GetExpectation().GetPlainExpectation() == nil {
		return nil, errors.New("no plain expectation in ExpectationRequest")
	}
	if request.GetExpectation().GetPlainExpectation().GetImportExpectation() == nil {
		return nil, errors.New("no import expectation in ExpectationRequest")
	}

	outputs := request.GetExpectation().GetPlainExpectation().GetImportExpectation().GetOutputs()
	if len(outputs) == 0 {
		return nil, errors.New("no outputs in ExpectationRequest")
	}
	return &token.TokenTransaction{
		Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainImport{
					PlainImport: &token.PlainImport{
						Outputs: outputs,
					},
				},
			},
		},
	}, nil
}
