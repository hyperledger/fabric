/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap/zapcore"
)

type ImplicitMetaPolicy struct {
	Threshold   int
	SubPolicies []Policy

	// Only used for logging
	managers      map[string]*ManagerImpl
	SubPolicyName string
}

// NewPolicy creates a new policy based on the policy bytes
func NewImplicitMetaPolicy(data []byte, managers map[string]*ManagerImpl) (*ImplicitMetaPolicy, error) {
	definition := &cb.ImplicitMetaPolicy{}
	if err := proto.Unmarshal(data, definition); err != nil {
		return nil, fmt.Errorf("Error unmarshalling to ImplicitMetaPolicy: %s", err)
	}

	subPolicies := make([]Policy, len(managers))

	i := 0
	for _, manager := range managers {
		subPolicies[i], _ = manager.GetPolicy(definition.SubPolicy)
		i++
	}

	var threshold int

	switch definition.Rule {
	case cb.ImplicitMetaPolicy_ANY:
		threshold = 1
	case cb.ImplicitMetaPolicy_ALL:
		threshold = len(subPolicies)
	case cb.ImplicitMetaPolicy_MAJORITY:
		threshold = len(subPolicies)/2 + 1
	}

	// In the special case that there are no policies, consider 0 to be a majority or any
	if len(subPolicies) == 0 {
		threshold = 0
	}

	return &ImplicitMetaPolicy{
		SubPolicies:   subPolicies,
		Threshold:     threshold,
		managers:      managers,
		SubPolicyName: definition.SubPolicy,
	}, nil
}

// EvaluateSignedData takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (imp *ImplicitMetaPolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	logger.Debugf("This is an implicit meta policy, it will trigger other policy evaluations, whose failures may be benign")
	remaining := imp.Threshold

	defer func() {
		if remaining != 0 {
			// This log message may be large and expensive to construct, so worth checking the log level
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				var b bytes.Buffer
				b.WriteString(fmt.Sprintf("Evaluation Failed: Only %d policies were satisfied, but needed %d of [ ", imp.Threshold-remaining, imp.Threshold))
				for m := range imp.managers {
					b.WriteString(m)
					b.WriteString("/")
					b.WriteString(imp.SubPolicyName)
					b.WriteString(" ")
				}
				b.WriteString("]")
				logger.Debugf(b.String())
			}
		}
	}()

	for _, policy := range imp.SubPolicies {
		if policy.EvaluateSignedData(signatureSet) == nil {
			remaining--
			if remaining == 0 {
				return nil
			}
		}
	}
	if remaining == 0 {
		return nil
	}
	return fmt.Errorf("implicit policy evaluation failed - %d sub-policies were satisfied, but this policy requires %d of the '%s' sub-policies to be satisfied", (imp.Threshold - remaining), imp.Threshold, imp.SubPolicyName)
}

// EvaluateIdentities takes an array of identities and evaluates whether
// they satisfy the policy
func (imp *ImplicitMetaPolicy) EvaluateIdentities(identities []msp.Identity) error {
	logger.Debugf("This is an implicit meta policy, it will trigger other policy evaluations, whose failures may be benign")
	remaining := imp.Threshold

	defer func() {
		// This log message may be large and expensive to construct, so worth checking the log level
		if remaining == 0 {
			return
		}
		if !logger.IsEnabledFor(zapcore.DebugLevel) {
			return
		}

		var b bytes.Buffer
		b.WriteString(fmt.Sprintf("Evaluation Failed: Only %d policies were satisfied, but needed %d of [ ", imp.Threshold-remaining, imp.Threshold))
		for m := range imp.managers {
			b.WriteString(m)
			b.WriteString("/")
			b.WriteString(imp.SubPolicyName)
			b.WriteString(" ")
		}
		b.WriteString("]")
		logger.Debugf(b.String())
	}()

	for _, policy := range imp.SubPolicies {
		if policy.EvaluateIdentities(identities) == nil {
			remaining--
			if remaining == 0 {
				return nil
			}
		}
	}
	if remaining == 0 {
		return nil
	}
	return fmt.Errorf("implicit policy evaluation failed - %d sub-policies were satisfied, but this policy requires %d of the '%s' sub-policies to be satisfied", (imp.Threshold - remaining), imp.Threshold, imp.SubPolicyName)
}
