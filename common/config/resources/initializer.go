/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

const RootGroupKey = "Resources"

type policyProposerRoot struct {
	policyManager *policies.ManagerImpl
}

// BeginPolicyProposals is used to start a new config proposal
func (p *policyProposerRoot) BeginPolicyProposals(tx interface{}, groups []string) ([]policies.Proposer, error) {
	if len(groups) != 1 {
		logger.Panicf("Initializer only supports having one root group")
	}
	return []policies.Proposer{p.policyManager}, nil
}

func (i *policyProposerRoot) ProposePolicy(tx interface{}, key string, policy *cb.ConfigPolicy) (proto.Message, error) {
	return nil, fmt.Errorf("Programming error, this should never be invoked")
}

// PreCommit is a no-op and returns nil
func (i *policyProposerRoot) PreCommit(tx interface{}) error {
	return nil
}

// RollbackConfig is used to abandon a new config proposal
func (i *policyProposerRoot) RollbackProposals(tx interface{}) {}

// CommitConfig is used to commit a new config proposal
func (i *policyProposerRoot) CommitProposals(tx interface{}) {}

type Bundle struct {
	ppr *policyProposerRoot
	vpr *valueProposerRoot
	cm  configtxapi.Manager
	pm  policies.Manager
}

// New creates a new resources config bundle
func New(envConfig *cb.Envelope, mspManager msp.MSPManager, channelPolicyManager policies.Manager) (*Bundle, error) {
	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(mspManager)
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	b := &Bundle{
		vpr: newValueProposerRoot(),
		ppr: &policyProposerRoot{
			policyManager: policies.NewManagerImpl(RootGroupKey, policyProviderMap),
		},
	}
	b.pm = &policyRouter{
		channelPolicyManager:   channelPolicyManager,
		resourcesPolicyManager: b.ppr.policyManager,
	}

	var err error
	b.cm, err = configtx.NewManagerImpl(envConfig, b, nil)
	if err != nil {
		return nil, err
	}
	return b, err
}

func (b *Bundle) RootGroupKey() string {
	return RootGroupKey
}

func (b *Bundle) PolicyProposer() policies.Proposer {
	logger.Debug("Boo", b)
	logger.Debug("boo", b.ppr)
	return b.ppr
}

func (b *Bundle) ValueProposer() config.ValueProposer {
	return b.vpr
}

func (b *Bundle) ConfigtxManager() configtxapi.Manager {
	return b.cm
}

func (b *Bundle) PolicyManager() policies.Manager {
	return b.pm
}

func (b *Bundle) ResourcePolicyMapper() PolicyMapper {
	return b.vpr
}
