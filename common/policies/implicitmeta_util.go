/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package policies

import (
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// ImplicitMetaPolicyWithSubPolicy creates an implicitmeta policy
func ImplicitMetaPolicyWithSubPolicy(subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.ConfigPolicy {
	return &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type: int32(cb.Policy_IMPLICIT_META),
			Value: utils.MarshalOrPanic(&cb.ImplicitMetaPolicy{
				Rule:      rule,
				SubPolicy: subPolicyName,
			}),
		},
	}
}

// TemplateImplicitMetaPolicy creates a policy at the specified path with the given policyName and subPolicyName
func TemplateImplicitMetaPolicyWithSubPolicy(path []string, policyName string, subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) *cb.ConfigGroup {
	root := cb.NewConfigGroup()
	group := root
	for _, element := range path {
		group.Groups[element] = cb.NewConfigGroup()
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
