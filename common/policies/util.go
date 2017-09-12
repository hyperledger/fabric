/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// ConfigPolicy defines a common representation for different *cb.ConfigPolicy values.
type ConfigPolicy interface {
	// Key is the key this value should be stored in the *cb.ConfigGroup.Policies map.
	Key() string

	// Value is the backing policy implementation for this ConfigPolicy
	Value() *cb.Policy
}

// StandardConfigValue implements the ConfigValue interface.
type StandardConfigPolicy struct {
	key   string
	value *cb.Policy
}

// Key is the key this value should be stored in the *cb.ConfigGroup.Values map.
func (scv *StandardConfigPolicy) Key() string {
	return scv.key
}

// Value is the *cb.Policy which should be stored as the *cb.ConfigPolicy.Policy.
func (scv *StandardConfigPolicy) Value() *cb.Policy {
	return scv.value
}

func makeImplicitMetaPolicy(subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.Policy {
	return &cb.Policy{
		Type: int32(cb.Policy_IMPLICIT_META),
		Value: utils.MarshalOrPanic(&cb.ImplicitMetaPolicy{
			Rule:      rule,
			SubPolicy: subPolicyName,
		}),
	}
}

// ImplicitMetaAllPolicy defines an implicit meta policy whose sub_policy and key is policyname with rule ALL.
func ImplicitMetaAllPolicy(policyName string) *StandardConfigPolicy {
	return &StandardConfigPolicy{
		key:   policyName,
		value: makeImplicitMetaPolicy(policyName, cb.ImplicitMetaPolicy_ALL),
	}
}

// ImplicitMetaAnyPolicy defines an implicit meta policy whose sub_policy and key is policyname with rule ANY.
func ImplicitMetaAnyPolicy(policyName string) *StandardConfigPolicy {
	return &StandardConfigPolicy{
		key:   policyName,
		value: makeImplicitMetaPolicy(policyName, cb.ImplicitMetaPolicy_ANY),
	}
}

// ImplicitMetaMajorityPolicy defines an implicit meta policy whose sub_policy and key is policyname with rule MAJORITY.
func ImplicitMetaMajorityPolicy(policyName string) *StandardConfigPolicy {
	return &StandardConfigPolicy{
		key:   policyName,
		value: makeImplicitMetaPolicy(policyName, cb.ImplicitMetaPolicy_MAJORITY),
	}
}

// ImplicitMetaMajorityPolicy defines a policy with key policyName and the given signature policy.
func SignaturePolicy(policyName string, sigPolicy *cb.SignaturePolicyEnvelope) *StandardConfigPolicy {
	return &StandardConfigPolicy{
		key: policyName,
		value: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: utils.MarshalOrPanic(sigPolicy),
		},
	}
}
