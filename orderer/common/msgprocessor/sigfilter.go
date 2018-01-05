/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

// SigFilterSupport provides the resources required for the signature filter
type SigFilterSupport interface {
	// PolicyManager returns a reference to the current policy manager
	PolicyManager() policies.Manager
}

// SigFilter stores the name of the policy to apply to deliver requests to
// determine whether a client is authorized
type SigFilter struct {
	policyName string
	support    SigFilterSupport
}

// NewSigFilter creates a new signature filter, at every evaluation, the policy manager is called
// to retrieve the latest version of the policy
func NewSigFilter(policyName string, support SigFilterSupport) *SigFilter {
	return &SigFilter{
		policyName: policyName,
		support:    support,
	}
}

// Apply applies the policy given, resulting in Reject or Forward, never Accept
func (sf *SigFilter) Apply(message *cb.Envelope) error {
	signedData, err := message.AsSignedData()

	if err != nil {
		return fmt.Errorf("could not convert message to signedData: %s", err)
	}

	policy, ok := sf.support.PolicyManager().GetPolicy(sf.policyName)
	if !ok {
		return fmt.Errorf("could not find policy %s", sf.policyName)
	}

	err = policy.Evaluate(signedData)
	if err != nil {
		return errors.Wrap(errors.WithStack(ErrPermissionDenied), err.Error())
	}
	return nil
}
