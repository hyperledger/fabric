/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/signed_data_policy_checker.go -fake-name SignedDataPolicyChecker . SignedDataPolicyChecker

type SignedDataPolicyChecker interface {
	// CheckPolicyBySignedData checks that the provided signed data is valid with respect to
	// specified policy for the specified channel.
	CheckPolicyBySignedData(channelID, policyName string, sd []*common.SignedData) error
}

// PolicyBasedAccessControl implements token command access control functions.
type PolicyBasedAccessControl struct {
	SignedDataPolicyChecker SignedDataPolicyChecker
}

func (ac *PolicyBasedAccessControl) Check(sc *token.SignedCommand, c *token.Command) error {
	switch t := c.GetPayload().(type) {

	case *token.Command_ImportRequest:
		return ac.SignedDataPolicyChecker.CheckPolicyBySignedData(
			c.Header.ChannelId,
			policies.ChannelApplicationWriters,
			[]*common.SignedData{{
				Identity:  c.Header.Creator,
				Data:      sc.Command,
				Signature: sc.Signature,
			}},
		)

	default:
		return errors.Errorf("command type not recognized: %T", t)
	}
}
