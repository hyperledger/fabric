/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
)

type policyRouter struct {
	channelPolicyManager   policies.Manager
	resourcesPolicyManager policies.Manager
}

var minPathLength int

var (
	channelPolicyMatcher   = regexp.MustCompile(fmt.Sprintf("^/%s/", channelconfig.RootGroupKey))
	resourcesPolicyMatcher = regexp.MustCompile(fmt.Sprintf("^/%s/", RootGroupKey))
)

// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
// It appropriately routes the request to the resource or channel policy manager, as required by the invocation.
func (pr *policyRouter) GetPolicy(id string) (policies.Policy, bool) {
	switch {
	case channelPolicyMatcher.MatchString(id):
		return pr.channelPolicyManager.GetPolicy(id)
	case resourcesPolicyMatcher.MatchString(id):
		return pr.resourcesPolicyManager.GetPolicy(id)
	default:
		return nil, false
	}
}

// Manager returns the sub-policy manager for a given path and whether it exists
func (pr *policyRouter) Manager(path []string) (policies.Manager, bool) {
	if len(path) == 0 {
		return pr, true
	}
	switch path[0] {
	case channelconfig.RootGroupKey:
		if pr.channelPolicyManager == nil {
			return nil, false
		}
		return pr.channelPolicyManager.Manager(path[1:])
	case RootGroupKey:
		if pr.resourcesPolicyManager == nil {
			return nil, false
		}
		return pr.resourcesPolicyManager.Manager(path[1:])
	default:
		return nil, false
	}
}
