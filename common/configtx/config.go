/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
)

type configResult struct {
	tx            interface{}
	groupName     string
	groupKey      string
	group         *cb.ConfigGroup
	valueHandler  config.ValueProposer
	policyHandler policies.Proposer
	subResults    []*configResult
}

func (cr *configResult) preCommit() error {
	for _, subResult := range cr.subResults {
		err := subResult.preCommit()
		if err != nil {
			return err
		}
	}
	return cr.valueHandler.PreCommit(cr.tx)
}

func (cr *configResult) commit() {
	for _, subResult := range cr.subResults {
		subResult.commit()
	}
	cr.valueHandler.CommitProposals(cr.tx)
	cr.policyHandler.CommitProposals(cr.tx)
}

func (cr *configResult) rollback() {
	for _, subResult := range cr.subResults {
		subResult.rollback()
	}
	cr.valueHandler.RollbackProposals(cr.tx)
	cr.policyHandler.RollbackProposals(cr.tx)
}

// proposeGroup proposes a group configuration with a given handler
// it will in turn recursively call itself until all groups have been exhausted
// at each call, it updates the configResult to contain references to the handlers
// which have been invoked so that calling result.commit() or result.rollback() will
// appropriately cleanup
func proposeGroup(result *configResult) error {
	subGroups := make([]string, len(result.group.Groups))
	i := 0
	for subGroup := range result.group.Groups {
		subGroups[i] = subGroup
		i++
	}

	valueDeserializer, subValueHandlers, err := result.valueHandler.BeginValueProposals(result.tx, subGroups)
	if err != nil {
		return err
	}

	subPolicyHandlers, err := result.policyHandler.BeginPolicyProposals(result.tx, subGroups)
	if err != nil {
		return err
	}

	if len(subValueHandlers) != len(subGroups) || len(subPolicyHandlers) != len(subGroups) {
		return fmt.Errorf("Programming error, did not return as many handlers as groups %d vs %d vs %d", len(subValueHandlers), len(subGroups), len(subPolicyHandlers))
	}

	for key, value := range result.group.Values {
		_, err := valueDeserializer.Deserialize(key, value.Value)
		if err != nil {
			result.rollback()
			return fmt.Errorf("Error deserializing key %s for group %s: %s", key, result.groupName, err)
		}
	}

	for key, policy := range result.group.Policies {
		_, err := result.policyHandler.ProposePolicy(result.tx, key, policy)
		if err != nil {
			result.rollback()
			return err
		}
	}

	result.subResults = make([]*configResult, 0, len(subGroups))

	for i, subGroup := range subGroups {
		result.subResults = append(result.subResults, &configResult{
			tx:            result.tx,
			groupKey:      subGroup,
			groupName:     result.groupName + "/" + subGroup,
			group:         result.group.Groups[subGroup],
			valueHandler:  subValueHandlers[i],
			policyHandler: subPolicyHandlers[i],
		})

		if err := proposeGroup(result.subResults[i]); err != nil {
			result.rollback()
			return err
		}
	}

	return nil
}

func processConfig(channelGroup *cb.ConfigGroup, proposer api.Proposer) (*configResult, error) {
	helperGroup := cb.NewConfigGroup()
	helperGroup.Groups[proposer.RootGroupKey()] = channelGroup

	configResult := &configResult{
		group:         helperGroup,
		valueHandler:  proposer.ValueProposer(),
		policyHandler: proposer.PolicyProposer(),
	}
	err := proposeGroup(configResult)
	if err != nil {
		return nil, err
	}

	return configResult, nil
}

func (cm *configManager) processConfig(channelGroup *cb.ConfigGroup) (*configResult, error) {
	logger.Debugf("Beginning new config for channel %s", cm.current.channelID)
	configResult, err := processConfig(channelGroup, cm.initializer)
	if err != nil {
		return nil, err
	}

	err = configResult.preCommit()
	if err != nil {
		configResult.rollback()
		return nil, err
	}

	return configResult, nil
}
