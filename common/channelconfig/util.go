/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/config"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/pkg/errors"
)

// InitializeConfigValues takes a config group and uses it to initialize a config Root
// XXX This goes away by the end of the CR series so no test logic
func InitializeConfigValues(vp config.ValueProposer, group *cb.ConfigGroup) error {
	subGroups := make([]string, len(group.Groups))
	i := 0
	for subGroup := range group.Groups {
		subGroups[i] = subGroup
		i++
	}

	valueDeserializer, subValueHandlers, err := vp.BeginValueProposals("", subGroups)
	if err != nil {
		return err
	}

	for key, value := range group.Values {
		_, err := valueDeserializer.Deserialize(key, value.Value)
		if err != nil {
			return errors.Wrapf(err, "failed to deserialize key %s", key)
		}
	}

	for i := range subGroups {
		if err := InitializeConfigValues(subValueHandlers[i], group.Groups[subGroups[i]]); err != nil {
			return errors.Wrapf(err, "failed initializing values for subgroup %s", subGroups[i])
		}
	}

	err = vp.PreCommit("")
	if err != nil {
		return errors.Wrapf(err, "precommit failed for group")
	}

	vp.CommitProposals("")
	return nil
}
