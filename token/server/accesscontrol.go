/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/acl_provider.go -fake-name ACLProvider . ACLProvider

type ACLProvider interface {
	// CheckACL checks access control for the resource for the given channel.
	// idinfo is an object such as []*protoutil.SignedData from which
	// an id can be extracted for testing against a policy
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

type ACLResources struct {
	IssueTokens    string
	TransferTokens string
	ListTokens     string
}

// PolicyBasedAccessControl implements token command access control functions.
type PolicyBasedAccessControl struct {
	ACLProvider  ACLProvider
	ACLResources *ACLResources
}

func (ac *PolicyBasedAccessControl) Check(sc *token.SignedCommand, c *token.Command) error {
	signedData := []*protoutil.SignedData{{
		Identity:  c.Header.Creator,
		Data:      sc.Command,
		Signature: sc.Signature,
	}}

	switch t := c.GetPayload().(type) {

	case *token.Command_IssueRequest:
		return ac.ACLProvider.CheckACL(
			ac.ACLResources.IssueTokens,
			c.Header.ChannelId,
			signedData,
		)
	case *token.Command_ListRequest:
		return ac.ACLProvider.CheckACL(
			ac.ACLResources.ListTokens,
			c.Header.ChannelId,
			signedData,
		)
	case *token.Command_TransferRequest:
		return ac.ACLProvider.CheckACL(
			ac.ACLResources.TransferTokens,
			c.Header.ChannelId,
			signedData,
		)
	case *token.Command_RedeemRequest:
		// Redeem has same policy as transfer
		return ac.ACLProvider.CheckACL(
			ac.ACLResources.TransferTokens,
			c.Header.ChannelId,
			signedData,
		)
	case *token.Command_TokenOperationRequest:
		request := c.GetTokenOperationRequest()
		if request == nil {
			return errors.New("command has no token operation request")
		}
		return ac.checkTokenOperationRequest(request.Operations, signedData, c)
	default:
		return errors.Errorf("command type not recognized: %T", t)
	}
}

// checkTokenOperationRequest checks either issue policy or transfer policy depending on the payload type of the token operation
func (ac *PolicyBasedAccessControl) checkTokenOperationRequest(ops []*token.TokenOperation, signedData []*protoutil.SignedData, c *token.Command) error {
	if len(ops) == 0 {
		return errors.New("TokenOperationRequest has no operations")
	}
	for _, op := range ops {
		if op.GetAction() == nil {
			return errors.New("no action in request")
		}
		if op.GetAction().GetPayload() == nil {
			return errors.New("no payload in action")
		}
		switch t := op.GetAction().GetPayload().(type) {
		case *token.TokenOperationAction_Issue:
			return ac.ACLProvider.CheckACL(
				ac.ACLResources.IssueTokens,
				c.Header.ChannelId,
				signedData,
			)
		case *token.TokenOperationAction_Transfer:
			return ac.ACLProvider.CheckACL(
				ac.ACLResources.TransferTokens,
				c.Header.ChannelId,
				signedData,
			)
		default:
			return errors.Errorf("operation payload type not recognized: %T", t)
		}
	}
	return nil
}
