/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/config"
	oldchannelconfig "github.com/hyperledger/fabric/common/config/channel"
	oldmspconfig "github.com/hyperledger/fabric/common/config/channel/msp"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common/channelconfig")

// RootGroupKey is the key for namespacing the channel config, especially for
// policy evaluation.
const RootGroupKey = "Channel"

// Bundle is a collection of resources which will always have a consistent
// view of the channel configuration.  In particular, for a given bundle reference,
// the config sequence, the policy manager etc. will always return exactly the
// same value.  The Bundle structure is immutable and will always be replaced in its
// entirety, with new backing memory.
type Bundle struct {
	policyManager   policies.Manager
	mspManager      msp.MSPManager
	rootConfig      *oldchannelconfig.Root
	configtxManager configtxapi.Manager
}

// PolicyManager returns the policy manager constructed for this config
func (b *Bundle) PolicyManager() policies.Manager {
	return b.policyManager
}

// MSPManager returns the MSP manager constructed for this config
func (b *Bundle) MSPManager() msp.MSPManager {
	return b.mspManager
}

// ChannelConfig returns the config.Channel for the chain
func (b *Bundle) ChannelConfig() oldchannelconfig.Channel {
	return b.rootConfig.Channel()
}

// OrdererConfig returns the config.Orderer for the channel
// and whether the Orderer config exists
func (b *Bundle) OrdererConfig() (oldchannelconfig.Orderer, bool) {
	result := b.rootConfig.Orderer()
	return result, result != nil
}

// ConsortiumsConfig() returns the config.Consortiums for the channel
// and whether the consortiums config exists
func (b *Bundle) ConsortiumsConfig() (oldchannelconfig.Consortiums, bool) {
	result := b.rootConfig.Consortiums()
	return result, result != nil
}

// ApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists
func (b *Bundle) ApplicationConfig() (oldchannelconfig.Application, bool) {
	result := b.rootConfig.Application()
	return result, result != nil
}

// ConfigtxManager returns the configtx.Manager for the channel
func (b *Bundle) ConfigtxManager() configtxapi.Manager {
	return b.configtxManager
}

type noopDeserializer struct{}

// Deserializer returns nil, nil
func (nd noopDeserializer) Deserialize(key string, value []byte) (proto.Message, error) {
	return nil, nil
}

type noopProposer struct {
	policyManager policies.Manager
}

// BeginValueProposals returns a noop deserializer, and noop proposers for subgroups
func (np noopProposer) BeginValueProposals(tx interface{}, groups []string) (config.ValueDeserializer, []config.ValueProposer, error) {
	proposers := make([]config.ValueProposer, len(groups))
	for i := range proposers {
		proposers[i] = noopProposer{}
	}
	return noopDeserializer{}, proposers, nil
}

// BeginValueProposals returns a noop deserializer, and noop proposers for subgroups
func (np noopProposer) BeginPolicyProposals(tx interface{}, groups []string) ([]policies.Proposer, error) {
	proposers := make([]policies.Proposer, len(groups))
	for i := range proposers {
		proposers[i] = noopProposer{}
	}
	return proposers, nil
}

// ProposePolicy returns nil, nil
func (np noopProposer) ProposePolicy(tx interface{}, name string, policy *cb.ConfigPolicy) (proto.Message, error) {
	return nil, nil
}

// RollbackProposals is a no-op
func (np noopProposer) RollbackProposals(tx interface{}) {}

// PreCommit returns nil
func (np noopProposer) PreCommit(tx interface{}) error { return nil }

// CommitProposals is a no-op
func (np noopProposer) CommitProposals(tx interface{}) {}

// ValueProposer returns noopProposer
func (np noopProposer) ValueProposer() config.ValueProposer {
	return np
}

// PolicyProposer returns noopProposer
func (np noopProposer) PolicyProposer() policies.Proposer {
	return np
}

// RootGroupKey returns RootGroupKey constant
func (np noopProposer) RootGroupKey() string {
	return RootGroupKey
}

// PolicyManager() returns the policy manager for considering config changes
func (np noopProposer) PolicyManager() policies.Manager {
	return np.policyManager
}

// NewBundle creates a new immutable bundle of configuration
func NewBundle(channelID string, config *cb.Config) (*Bundle, error) {
	mspConfigHandler := oldmspconfig.NewMSPConfigHandler()
	rootConfig := oldchannelconfig.NewRoot(mspConfigHandler)

	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(mspConfigHandler)
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	err := InitializeConfigValues(rootConfig, &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{RootGroupKey: config.ChannelGroup},
	})
	if err != nil {
		return nil, errors.Wrap(err, "initializing config values failed")
	}

	policyManager := policies.NewManagerImpl(RootGroupKey, policyProviderMap)
	err = InitializePolicyManager(policyManager, config.ChannelGroup)
	if err != nil {
		return nil, errors.Wrap(err, "initializing policymanager failed")
	}

	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID, nil, &cb.ConfigEnvelope{Config: config}, 0, 0)
	if err != nil {
		return nil, errors.Wrap(err, "creating envelope for configtx manager failed")
	}

	configtxManager, err := configtx.NewManagerImpl(env, noopProposer{
		policyManager: policyManager,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "initializing configtx manager failed")
	}

	return &Bundle{
		mspManager:      mspConfigHandler,
		policyManager:   policyManager,
		rootConfig:      rootConfig,
		configtxManager: configtxManager,
	}, nil
}
