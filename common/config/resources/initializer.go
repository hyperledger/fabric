/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	newchannelconfig "github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

const RootGroupKey = "Resources"

type Bundle struct {
	vpr *valueProposerRoot
	cm  configtxapi.Manager
	pm  policies.Manager
	rpm policies.Manager
}

// New creates a new resources config bundle
// TODO, change interface to take config and not an envelope
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

	payload, err := utils.UnmarshalPayload(envConfig.Payload)
	if err != nil {
		return nil, err
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, err
	}

	if configEnvelope.Config == nil || configEnvelope.Config.ChannelGroup == nil {
		return nil, fmt.Errorf("config is nil")
	}

	resourcesPolicyManager, err := policies.NewManagerImpl(RootGroupKey, policyProviderMap, configEnvelope.Config.ChannelGroup)
	if err != nil {
		return nil, err
	}

	b := &Bundle{
		vpr: newValueProposerRoot(),
		rpm: resourcesPolicyManager,
		pm: &policyRouter{
			channelPolicyManager:   channelPolicyManager,
			resourcesPolicyManager: resourcesPolicyManager,
		},
	}

	b.cm, err = configtx.NewManagerImpl(envConfig, b)
	if err != nil {
		return nil, err
	}

	err = newchannelconfig.InitializeConfigValues(b.vpr, &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			RootGroupKey: configEnvelope.Config.ChannelGroup,
		},
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Bundle) RootGroupKey() string {
	return RootGroupKey
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
