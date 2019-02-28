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

// RequestIssue creates an issue request with the token owners, types, and quantities specified in tokensToIssue.
func (i *Issuer) RequestIssue(tokensToIssue []*token.Token) (*token.TokenTransaction, error) {
	var outputs []*token.Token
	for _, tti := range tokensToIssue {
		err := i.TokenOwnerValidator.Validate(tti.Owner)
		if err != nil {
			return nil, errors.Errorf("invalid recipient in issue request '%s'", err)
		}
		q, err := ToQuantity(tti.Quantity, Precision)
		if err != nil {
			return nil, errors.Errorf("invalid quantity in issue request '%s'", err)
		}

		outputs = append(outputs, &token.Token{
			Owner:    tti.Owner,
			Type:     tti.Type,
			Quantity: q.Hex(),
		})
	}

	return &token.TokenTransaction{
		Action: &token.TokenTransaction_TokenAction{
			TokenAction: &token.TokenAction{
				Data: &token.TokenAction_Issue{
					Issue: &token.Issue{
						Outputs: outputs,
					},
				},
			},
		},
	}, nil
}

// RequestTokenOperation returns a token transaction matching the requested issue operation
func (i *Issuer) RequestTokenOperation(request *token.TokenOperation) (*token.TokenTransaction, error) {
	if request.GetAction() == nil {
		return nil, errors.New("no action in request")
	}
	if request.GetAction().GetIssue() == nil {
		return nil, errors.New("no issue in action")
	}
	if request.GetAction().GetIssue().GetSender() == nil {
		return nil, errors.New("no sender in issue")
	}

	outputs := request.GetAction().GetIssue().GetOutputs()
	if len(outputs) == 0 {
		return nil, errors.New("no outputs in ExpectationRequest")
	}
	return &token.TokenTransaction{
		Action: &token.TokenTransaction_TokenAction{
			TokenAction: &token.TokenAction{
				Data: &token.TokenAction_Issue{
					Issue: &token.Issue{
						Outputs: outputs,
					},
				},
			},
		},
	}, nil
}
