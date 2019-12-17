/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
)

// ImplicitMetaPolicyWithSubPolicy creates an implicitmeta policy
func ImplicitMetaPolicyWithSubPolicy(subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.ConfigPolicy {
	return &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type: int32(cb.Policy_IMPLICIT_META),
			Value: protoutil.MarshalOrPanic(&cb.ImplicitMetaPolicy{
				Rule:      rule,
				SubPolicy: subPolicyName,
			}),
		},
	}
}

// TemplateImplicitMetaPolicy creates a policy at the specified path with the given policyName and subPolicyName
func TemplateImplicitMetaPolicyWithSubPolicy(path []string, policyName string, subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.ConfigGroup {
	root := protoutil.NewConfigGroup()
	group := root
	for _, element := range path {
		group.Groups[element] = protoutil.NewConfigGroup()
		group = group.Groups[element]
	}

	group.Policies[policyName] = ImplicitMetaPolicyWithSubPolicy(subPolicyName, rule)
	return root
}

// TemplateImplicitMetaPolicy creates a policy at the specified path with the given policyName
// It utilizes the policyName for the subPolicyName as well, as this is the standard usage pattern
func TemplateImplicitMetaPolicy(path []string, policyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.ConfigGroup {
	return TemplateImplicitMetaPolicyWithSubPolicy(path, policyName, policyName, rule)
}

// TempateImplicitMetaAnyPolicy returns TemplateImplicitMetaPolicy with cb.ImplicitMetaPolicy_ANY as the rule
func TemplateImplicitMetaAnyPolicy(path []string, policyName string) *cb.ConfigGroup {
	return TemplateImplicitMetaPolicy(path, policyName, cb.ImplicitMetaPolicy_ANY)
}

// TempateImplicitMetaAnyPolicy returns TemplateImplicitMetaPolicy with cb.ImplicitMetaPolicy_ALL as the rule
func TemplateImplicitMetaAllPolicy(path []string, policyName string) *cb.ConfigGroup {
	return TemplateImplicitMetaPolicy(path, policyName, cb.ImplicitMetaPolicy_ALL)
}

// TempateImplicitMetaAnyPolicy returns TemplateImplicitMetaPolicy with cb.ImplicitMetaPolicy_MAJORITY as the rule
func TemplateImplicitMetaMajorityPolicy(path []string, policyName string) *cb.ConfigGroup {
	return TemplateImplicitMetaPolicy(path, policyName, cb.ImplicitMetaPolicy_MAJORITY)
}
