/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/pkg/errors"
)

// RootGroupKey is the namespace in the config tree for this set of config
const RootGroupKey = "Resources"

var logger = flogging.MustGetLogger("common/config/resource")

// Bundle stores an immutable group of resources configuration
type Bundle struct {
	channelID string
	resConf   *cb.Config
	chanConf  channelconfig.Resources
	rg        *resourceGroup
	cm        configtx.Validator
	pm        *policyRouter
}

// NewBundleFromEnvelope creates a new resources config bundle.
// TODO, this method should probably be removed, as the resourcesconfig is never in an Envelope naturally.
// TODO, add an atomic BundleSource
func NewBundleFromEnvelope(envConfig *cb.Envelope, chanConf channelconfig.Resources) (*Bundle, error) {
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

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal channel header")
	}

	return NewBundle(chdr.ChannelId, configEnvelope.Config, chanConf)
}

// NewBundle creates a resources config bundle which implements the Resources interface.
func NewBundle(channelID string, config *cb.Config, chanConf channelconfig.Resources) (*Bundle, error) {
	resourceGroup, err := newResourceGroup(config.ChannelGroup)
	if err != nil {
		return nil, err
	}

	b := &Bundle{
		channelID: channelID,
		rg:        resourceGroup,
		resConf:   config,
	}

	return b.NewFromChannelConfig(chanConf)
}

// RootGroupKey returns the name of the key for the root group (the namespace for this config).
func (b *Bundle) RootGroupKey() string {
	return RootGroupKey
}

// ConfigtxValidator returns a reference to a configtx.Validator which can process updates to this config.
func (b *Bundle) ConfigtxValidator() configtx.Validator {
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

// ChannelConfig returns the channel config which this resources config depends on.
// Note, consumers of the resourcesconfig should almost never refer to the PolicyManager
// within the channel config, and should instead refer to the PolicyManager exposed by
// this Bundle.
func (b *Bundle) ChannelConfig() channelconfig.Resources {
	return b.chanConf
}

// ValidateNew is currently a no-op.  The idea is that there may be additional checking which needs to be done
// between an old resource configuration and a new one.  In the channel resource configuration case, we add some
// checks to make sure that the MSP IDs have not changed, and that the consensus type has not changed.  There is
// currently no such check required in the peer resources, but, in the interest of following the pattern established
// by the channel configuration, it is included here.
func (b *Bundle) ValidateNew(resources Resources) error {
	return nil
}

// NewFromChannelConfig builds a new Bundle, based on this Bundle, but with a different underlying
// channel config.  This is usually invoked when a channel config update is processed.
func (b *Bundle) NewFromChannelConfig(chanConf channelconfig.Resources) (*Bundle, error) {
	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(chanConf.MSPManager())
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	resourcesPolicyManager, err := policies.NewManagerImpl(RootGroupKey, policyProviderMap, b.resConf.ChannelGroup)

	if err != nil {
		return nil, err
	}

	result := &Bundle{
		channelID: b.channelID,
		chanConf:  chanConf,
		pm: &policyRouter{
			channelPolicyManager:   chanConf.PolicyManager(),
			resourcesPolicyManager: resourcesPolicyManager,
		},
		rg:      b.rg,
		resConf: b.resConf,
	}

	result.cm, err = configtx.NewValidatorImpl(b.channelID, result.resConf, RootGroupKey, result.pm)
	if err != nil {
		return nil, err
	}

	return result, nil
}
