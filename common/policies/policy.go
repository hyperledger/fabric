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
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
)

const (
	// ChannelApplicationReaders is the label for the channel's application readers policy
	ChannelApplicationReaders = "/channel/Application/Readers"
)

var logger = logging.MustGetLogger("common/policies")

// Policy is used to determine if a signature is valid
type Policy interface {
	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
	Evaluate(signatureSet []*cb.SignedData) error
}

// Manager is a read only subset of the policy ManagerImpl
type Manager interface {
	// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
	GetPolicy(id string) (Policy, bool)

	// Manager returns the sub-policy manager for a given path and whether it exists
	Manager(path []string) (Manager, bool)

	// Basepath returns the basePath the manager was instnatiated with
	BasePath() string

	// Policies returns all policy names defined in the manager
	PolicyNames() []string
}

// Proposer is the interface used by the configtx manager for policy management
type Proposer interface {
	BeginPolicyProposals(groups []string) ([]Proposer, error)

	ProposePolicy(name string, policy *cb.ConfigPolicy) error

	RollbackProposals()

	CommitProposals()
}

// Provider provides the backing implementation of a policy
type Provider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(data []byte) (Policy, error)
}

type policyConfig struct {
	policies map[string]Policy
	managers map[string]*ManagerImpl
	imps     []*implicitMetaPolicy
}

// ManagerImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.ConfigManager
type ManagerImpl struct {
	basePath      string
	providers     map[int32]Provider
	config        *policyConfig
	pendingConfig *policyConfig
}

// NewManagerImpl creates a new ManagerImpl with the given CryptoHelper
func NewManagerImpl(basePath string, providers map[int32]Provider) *ManagerImpl {
	_, ok := providers[int32(cb.Policy_IMPLICIT_META)]
	if ok {
		logger.Panicf("ImplicitMetaPolicy type must be provider by the policy manager")
	}

	return &ManagerImpl{
		basePath:  basePath,
		providers: providers,
		config: &policyConfig{
			policies: make(map[string]Policy),
			managers: make(map[string]*ManagerImpl),
		},
	}
}

type rejectPolicy string

func (rp rejectPolicy) Evaluate(signedData []*cb.SignedData) error {
	return fmt.Errorf("No such policy type: %s", rp)
}

// Basepath returns the basePath the manager was instnatiated with
func (pm *ManagerImpl) BasePath() string {
	return pm.basePath
}

func (pm *ManagerImpl) PolicyNames() []string {
	policyNames := make([]string, len(pm.config.policies))
	i := 0
	for policyName := range pm.config.policies {
		policyNames[i] = policyName
		i++
	}
	return policyNames
}

// Manager returns the sub-policy manager for a given path and whether it exists
func (pm *ManagerImpl) Manager(path []string) (Manager, bool) {
	if len(path) == 0 {
		return pm, true
	}

	m, ok := pm.config.managers[path[0]]
	if !ok {
		return nil, false
	}

	return m.Manager(path[1:])
}

// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default reject policy
func (pm *ManagerImpl) GetPolicy(id string) (Policy, bool) {
	policy, ok := pm.config.policies[id]
	if !ok {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Returning dummy reject all policy because %s could not be found in %s", id, pm.basePath)
		}
		return rejectPolicy(id), false
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Returning policy %s for evaluation", id)
	}
	return policy, true
}

// BeginPolicies is used to start a new config proposal
func (pm *ManagerImpl) BeginPolicyProposals(groups []string) ([]Proposer, error) {
	if pm.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}

	pm.pendingConfig = &policyConfig{
		policies: make(map[string]Policy),
		managers: make(map[string]*ManagerImpl),
	}

	managers := make([]Proposer, len(groups))
	for i, group := range groups {
		newManager := NewManagerImpl(group, pm.providers)
		pm.pendingConfig.managers[group] = newManager
		managers[i] = newManager
	}
	return managers, nil
}

// RollbackProposals is used to abandon a new config proposal
func (pm *ManagerImpl) RollbackProposals() {
	pm.pendingConfig = nil
}

// CommitProposals is used to commit a new config proposal
func (pm *ManagerImpl) CommitProposals() {
	if pm.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}

	for managerPath, m := range pm.pendingConfig.managers {
		for _, policyName := range m.PolicyNames() {
			fqKey := managerPath + "/" + policyName
			pm.pendingConfig.policies[fqKey], _ = m.GetPolicy(policyName)
			logger.Debugf("In commit adding relative sub-policy %s to %s", fqKey, pm.basePath)
		}
	}

	// Now that all the policies are present, initialize the meta policies
	for _, imp := range pm.pendingConfig.imps {
		imp.initialize(pm.pendingConfig)
	}

	pm.config = pm.pendingConfig
	pm.pendingConfig = nil
}

// ProposePolicy takes key, path, and ConfigPolicy and registers it in the proposed PolicyManager, or errors
func (pm *ManagerImpl) ProposePolicy(key string, configPolicy *cb.ConfigPolicy) error {
	policy := configPolicy.Policy
	if policy == nil {
		return fmt.Errorf("Policy cannot be nil")
	}

	var cPolicy Policy

	if policy.Type == int32(cb.Policy_IMPLICIT_META) {
		imp, err := newImplicitMetaPolicy(policy.Policy)
		if err != nil {
			return err
		}
		pm.pendingConfig.imps = append(pm.pendingConfig.imps, imp)
		cPolicy = imp
	} else {
		provider, ok := pm.providers[int32(policy.Type)]
		if !ok {
			return fmt.Errorf("Unknown policy type: %v", policy.Type)
		}

		var err error
		cPolicy, err = provider.NewPolicy(policy.Policy)
		if err != nil {
			return err
		}
	}

	pm.pendingConfig.policies[key] = cPolicy

	logger.Debugf("Proposed new policy %s for %s", key, pm.basePath)
	return nil
}
