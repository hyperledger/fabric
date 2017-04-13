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
	"strings"
	"sync"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
)

const (
	// Path separator is used to separate policy names in paths
	PathSeparator = "/"

	// ChannelPrefix is used in the path of standard channel policy managers
	ChannelPrefix = "Channel"

	// ApplicationPrefix is used in the path of standard application policy paths
	ApplicationPrefix = "Application"

	// OrdererPrefix is used in the path of standard orderer policy paths
	OrdererPrefix = "Orderer"

	// ChannelReaders is the label for the channel's readers policy (encompassing both orderer and application readers)
	ChannelReaders = PathSeparator + ChannelPrefix + PathSeparator + "Readers"

	// ChannelWriters is the label for the channel's writers policy (encompassing both orderer and application writers)
	ChannelWriters = PathSeparator + ChannelPrefix + PathSeparator + "Writers"

	// ChannelApplicationReaders is the label for the channel's application readers policy
	ChannelApplicationReaders = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Readers"

	// ChannelApplicationWriters is the label for the channel's application writers policy
	ChannelApplicationWriters = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Writers"

	// ChannelApplicationAdmins is the label for the channel's application admin policy
	ChannelApplicationAdmins = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Admins"

	// BlockValidation is the label for the policy which should validate the block signatures for the channel
	BlockValidation = PathSeparator + ChannelPrefix + PathSeparator + OrdererPrefix + PathSeparator + "BlockValidation"
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

	// Basepath returns the basePath the manager was instantiated with
	BasePath() string

	// Policies returns all policy names defined in the manager
	PolicyNames() []string
}

// Proposer is the interface used by the configtx manager for policy management
type Proposer interface {
	// BeginPolicyProposals starts a policy update transaction
	BeginPolicyProposals(tx interface{}, groups []string) ([]Proposer, error)

	// ProposePolicy createss a pending policy update from a ConfigPolicy and returns the deserialized
	// value of the Policy representation
	ProposePolicy(tx interface{}, name string, policy *cb.ConfigPolicy) (proto.Message, error)

	// RollbackProposals discards the pending policy updates
	RollbackProposals(tx interface{})

	// CommitProposals commits the pending policy updates
	CommitProposals(tx interface{})

	// PreCommit tests if a commit will apply
	PreCommit(tx interface{}) error
}

// Provider provides the backing implementation of a policy
type Provider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(data []byte) (Policy, proto.Message, error)
}

// ChannelPolicyManagerGetter is a support interface
// to get access to the policy manager of a given channel
type ChannelPolicyManagerGetter interface {
	// Returns the policy manager associated to the passed channel
	// and true if it was the manager requested, or false if it is the default manager
	Manager(channelID string) (Manager, bool)
}

type policyConfig struct {
	policies map[string]Policy
	managers map[string]*ManagerImpl
	imps     []*implicitMetaPolicy
}

// ManagerImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.ConfigManager
type ManagerImpl struct {
	parent        *ManagerImpl
	basePath      string
	fqPrefix      string
	providers     map[int32]Provider
	config        *policyConfig
	pendingConfig map[interface{}]*policyConfig
	pendingLock   sync.RWMutex
}

// NewManagerImpl creates a new ManagerImpl with the given CryptoHelper
func NewManagerImpl(basePath string, providers map[int32]Provider) *ManagerImpl {
	_, ok := providers[int32(cb.Policy_IMPLICIT_META)]
	if ok {
		logger.Panicf("ImplicitMetaPolicy type must be provider by the policy manager")
	}

	return &ManagerImpl{
		basePath:  basePath,
		fqPrefix:  PathSeparator + basePath + PathSeparator,
		providers: providers,
		config: &policyConfig{
			policies: make(map[string]Policy),
			managers: make(map[string]*ManagerImpl),
		},
		pendingConfig: make(map[interface{}]*policyConfig),
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
	if id == "" {
		logger.Errorf("Returning dummy reject all policy because no policy ID supplied")
		return rejectPolicy(id), false
	}
	var relpath string

	if strings.HasPrefix(id, PathSeparator) {
		if pm.parent != nil {
			return pm.parent.GetPolicy(id)
		}
		if !strings.HasPrefix(id, pm.fqPrefix) {
			if logger.IsEnabledFor(logging.DEBUG) {
				logger.Debugf("Requested policy from root manager with wrong basePath: %s, returning rejectAll", id)
			}
			return rejectPolicy(id), false
		}
		relpath = id[len(pm.fqPrefix):]
	} else {
		relpath = id
	}

	policy, ok := pm.config.policies[relpath]
	if !ok {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Returning dummy reject all policy because %s could not be found in /%s/%s", id, pm.basePath, relpath)
		}
		return rejectPolicy(relpath), false
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Returning policy %s for evaluation", relpath)
	}
	return policy, true
}

// BeginPolicies is used to start a new config proposal
func (pm *ManagerImpl) BeginPolicyProposals(tx interface{}, groups []string) ([]Proposer, error) {
	pm.pendingLock.Lock()
	defer pm.pendingLock.Unlock()
	pendingConfig, ok := pm.pendingConfig[tx]
	if ok {
		logger.Panicf("Serious Programming error: cannot call begin mulitply for the same proposal")
	}

	pendingConfig = &policyConfig{
		policies: make(map[string]Policy),
		managers: make(map[string]*ManagerImpl),
	}
	pm.pendingConfig[tx] = pendingConfig

	managers := make([]Proposer, len(groups))
	for i, group := range groups {
		newManager := NewManagerImpl(group, pm.providers)
		newManager.parent = pm
		pendingConfig.managers[group] = newManager
		managers[i] = newManager
	}
	return managers, nil
}

// RollbackProposals is used to abandon a new config proposal
func (pm *ManagerImpl) RollbackProposals(tx interface{}) {
	pm.pendingLock.Lock()
	defer pm.pendingLock.Unlock()
	delete(pm.pendingConfig, tx)
}

// PreCommit is currently a no-op for the policy manager and always returns nil
func (pm *ManagerImpl) PreCommit(tx interface{}) error {
	return nil
}

// CommitProposals is used to commit a new config proposal
func (pm *ManagerImpl) CommitProposals(tx interface{}) {
	pm.pendingLock.Lock()
	defer pm.pendingLock.Unlock()
	pendingConfig, ok := pm.pendingConfig[tx]
	if !ok {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}

	if pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}

	for managerPath, m := range pendingConfig.managers {
		for _, policyName := range m.PolicyNames() {
			fqKey := managerPath + PathSeparator + policyName
			pendingConfig.policies[fqKey], _ = m.GetPolicy(policyName)
			logger.Debugf("In commit adding relative sub-policy %s to %s", fqKey, pm.basePath)
		}
	}

	// Now that all the policies are present, initialize the meta policies
	for _, imp := range pendingConfig.imps {
		imp.initialize(pendingConfig)
	}

	pm.config = pendingConfig
	delete(pm.pendingConfig, tx)

	if pm.parent == nil && pm.basePath == ChannelPrefix {
		for _, policyName := range []string{ChannelReaders, ChannelWriters} {
			_, ok := pm.GetPolicy(policyName)
			if !ok {
				logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
			} else {
				logger.Debugf("As expected, current configuration has policy '%s'", policyName)
			}
		}
		if _, ok := pm.config.managers[ApplicationPrefix]; ok {
			// Check for default application policies if the application component is defined
			for _, policyName := range []string{
				ChannelApplicationReaders,
				ChannelApplicationWriters,
				ChannelApplicationAdmins} {
				_, ok := pm.GetPolicy(policyName)
				if !ok {
					logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
				} else {
					logger.Debugf("As expected, current configuration has policy '%s'", policyName)
				}
			}
		}
		if _, ok := pm.config.managers[OrdererPrefix]; ok {
			for _, policyName := range []string{BlockValidation} {
				_, ok := pm.GetPolicy(policyName)
				if !ok {
					logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
				} else {
					logger.Debugf("As expected, current configuration has policy '%s'", policyName)
				}
			}
		}
	}
}

// ProposePolicy takes key, path, and ConfigPolicy and registers it in the proposed PolicyManager, or errors
// It also returns the deserialized policy value for tracking and inspection at the invocation side.
func (pm *ManagerImpl) ProposePolicy(tx interface{}, key string, configPolicy *cb.ConfigPolicy) (proto.Message, error) {
	pm.pendingLock.RLock()
	pendingConfig, ok := pm.pendingConfig[tx]
	pm.pendingLock.RUnlock()
	if !ok {
		logger.Panicf("Serious Programming error: called Propose without Begin")
	}

	policy := configPolicy.Policy
	if policy == nil {
		return nil, fmt.Errorf("Policy cannot be nil")
	}

	var cPolicy Policy
	var deserialized proto.Message

	if policy.Type == int32(cb.Policy_IMPLICIT_META) {
		imp, err := newImplicitMetaPolicy(policy.Policy)
		if err != nil {
			return nil, err
		}
		pendingConfig.imps = append(pendingConfig.imps, imp)
		cPolicy = imp
		deserialized = imp.conf
	} else {
		provider, ok := pm.providers[int32(policy.Type)]
		if !ok {
			return nil, fmt.Errorf("Unknown policy type: %v", policy.Type)
		}

		var err error
		cPolicy, deserialized, err = provider.NewPolicy(policy.Policy)
		if err != nil {
			return nil, err
		}
	}

	pendingConfig.policies[key] = cPolicy

	logger.Debugf("Proposed new policy %s for %s", key, pm.basePath)
	return deserialized, nil
}
