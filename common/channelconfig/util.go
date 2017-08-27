/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

// InitializePolicyManager takes a config group and uses it to initialize a PolicyManager
// XXX This goes away by the end of the CR series so no test logic
func InitializePolicyManager(pm policies.Proposer, group *cb.ConfigGroup) error {
	subGroups := make([]string, len(group.Groups))
	i := 0
	for subGroup := range group.Groups {
		subGroups[i] = subGroup
		i++
	}

	subPolicyHandlers, err := pm.BeginPolicyProposals("", subGroups)
	if err != nil {
		return err
	}

	for key, policy := range group.Policies {
		_, err := pm.ProposePolicy("", key, policy)
		if err != nil {
			return err
		}
	}

	for i := range subGroups {
		if err := InitializePolicyManager(subPolicyHandlers[i], group.Groups[subGroups[i]]); err != nil {
			return errors.Wrapf(err, "failed initializing subgroup %s", subGroups[i])
		}
	}

	pm.CommitProposals("")
	return nil
}
