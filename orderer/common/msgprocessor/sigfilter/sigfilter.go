/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sigfilter

import (
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/common/msgprocessor/sigfilter")

type sigFilter struct {
	policyName    string
	policyManager policies.Manager
}

// New creates a new signature filter, at every evaluation, the policy manager is called
// to retrieve the latest version of the policy
func New(policyName string, policyManager policies.Manager) filter.Rule {
	return &sigFilter{
		policyName:    policyName,
		policyManager: policyManager,
	}
}

// Apply applies the policy given, resulting in Reject or Forward, never Accept
func (sf *sigFilter) Apply(message *cb.Envelope) error {
	signedData, err := message.AsSignedData()

	if err != nil {
		return fmt.Errorf("could not convert message to signedData: %s", err)
	}

	policy, ok := sf.policyManager.GetPolicy(sf.policyName)
	if !ok {
		return fmt.Errorf("could not find policy %s", sf.policyName)
	}

	return policy.Evaluate(signedData)
}
