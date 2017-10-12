/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// RootGroupKey is the namespace in the config tree for this set of config
const RootGroupKey = "Resources"

var logger = flogging.MustGetLogger("common/config/resource")

// Bundle stores an immutable group of resources configuration
type Bundle struct {
	rg  *resourceGroup
	cm  configtxapi.Manager
	pm  policies.Manager
	rpm policies.Manager
}

// New creates a new resources config bundle
// TODO, change interface to take config and not an envelope
// TODO, add an atomic BundleSource
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

	resourceGroup, err := newResourceGroup(configEnvelope.Config.ChannelGroup)
	if err != nil {
		return nil, err
	}

	b := &Bundle{
		rpm: resourcesPolicyManager,
		rg:  resourceGroup,
		pm: &policyRouter{
			channelPolicyManager:   channelPolicyManager,
			resourcesPolicyManager: resourcesPolicyManager,
		},
	}

	b.cm, err = configtx.NewManagerImpl(envConfig, b)
	if err != nil {
		return nil, err
	}

	return b, nil
}

// RootGroupKey returns the name of the key for the root group (the namespace for this config).
func (b *Bundle) RootGroupKey() string {
	return RootGroupKey
}

// ConfigtxManager returns a reference to a configtx.Manager which can process updates to this config.
func (b *Bundle) ConfigtxManager() configtxapi.Manager {
	return b.cm
}

// PolicyManager returns a policy manager which can resolve names both in the /Channel and /Resources namespaces.
func (b *Bundle) PolicyManager() policies.Manager {
	return b.pm
}

// APIPolicyMapper returns a way to map API names to policies governing their invocation.
func (b *Bundle) APIPolicyMapper() PolicyMapper {
	return b.rg.apisGroup
}

// ChaincodeRegistery returns a way to query for chaincodes defined in this channel.
func (b *Bundle) ChaincodeRegistry() ChaincodeRegistry {
	return b.rg.chaincodesGroup
}
