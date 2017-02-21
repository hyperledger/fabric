/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

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

package configtx

import (
	"fmt"

	"github.com/hyperledger/fabric/common/configtx/api"
	configvaluesapi "github.com/hyperledger/fabric/common/configvalues"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
)

type configResult struct {
	handler       api.Transactional
	policyHandler api.Transactional
	subResults    []*configResult
}

func (cr *configResult) preCommit() error {
	for _, subResult := range cr.subResults {
		err := subResult.preCommit()
		if err != nil {
			return err
		}
	}
	return cr.handler.PreCommit()
}

func (cr *configResult) commit() {
	for _, subResult := range cr.subResults {
		subResult.commit()
	}
	cr.handler.CommitProposals()
	cr.policyHandler.CommitProposals()
}

func (cr *configResult) rollback() {
	for _, subResult := range cr.subResults {
		subResult.rollback()
	}
	cr.handler.RollbackProposals()
	cr.policyHandler.RollbackProposals()
}

// proposeGroup proposes a group configuration with a given handler
// it will in turn recursively call itself until all groups have been exhausted
// at each call, it returns the handler that was passed in, plus any handlers returned
// by recursive calls into proposeGroup
func (cm *configManager) proposeGroup(name string, group *cb.ConfigGroup, handler configvaluesapi.ValueProposer, policyHandler policies.Proposer) (*configResult, error) {
	subGroups := make([]string, len(group.Groups))
	i := 0
	for subGroup := range group.Groups {
		subGroups[i] = subGroup
		i++
	}

	logger.Debugf("Beginning new config for channel %s and group %s", cm.chainID, name)
	subHandlers, err := handler.BeginValueProposals(subGroups)
	if err != nil {
		return nil, err
	}

	subPolicyHandlers, err := policyHandler.BeginPolicyProposals(subGroups)
	if err != nil {
		return nil, err
	}

	if len(subHandlers) != len(subGroups) || len(subPolicyHandlers) != len(subGroups) {
		return nil, fmt.Errorf("Programming error, did not return as many handlers as groups %d vs %d vs %d", len(subHandlers), len(subGroups), len(subPolicyHandlers))
	}

	result := &configResult{
		handler:       handler,
		policyHandler: policyHandler,
		subResults:    make([]*configResult, 0, len(subGroups)),
	}

	for i, subGroup := range subGroups {
		subResult, err := cm.proposeGroup(name+"/"+subGroup, group.Groups[subGroup], subHandlers[i], subPolicyHandlers[i])
		if err != nil {
			result.rollback()
			return nil, err
		}
		result.subResults = append(result.subResults, subResult)
	}

	for key, value := range group.Values {
		if err := handler.ProposeValue(key, value); err != nil {
			result.rollback()
			return nil, err
		}
	}

	for key, policy := range group.Policies {
		if err := policyHandler.ProposePolicy(key, policy); err != nil {
			result.rollback()
			return nil, err
		}
	}

	err = result.preCommit()
	if err != nil {
		result.rollback()
		return nil, err
	}

	return result, nil
}

func (cm *configManager) processConfig(channelGroup *cb.ConfigGroup) (*configResult, error) {
	helperGroup := cb.NewConfigGroup()
	helperGroup.Groups[RootGroupKey] = channelGroup
	groupResult, err := cm.proposeGroup("", helperGroup, cm.initializer.ValueProposer(), cm.initializer.PolicyProposer())
	if err != nil {
		return nil, err
	}

	return groupResult, nil
}
