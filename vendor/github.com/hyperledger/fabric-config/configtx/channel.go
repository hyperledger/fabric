/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"errors"
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go/common"
)

// ChannelGroup encapsulates the parts of the config that control channels.
// This type implements retrieval of the various channel config values.
type ChannelGroup struct {
	channelGroup *cb.ConfigGroup
}

// Channel returns the channel group from the updated config.
func (c *ConfigTx) Channel() *ChannelGroup {
	return &ChannelGroup{channelGroup: c.updated.ChannelGroup}
}

// Configuration returns a channel configuration value from a config transaction.
func (c *ChannelGroup) Configuration() (Channel, error) {
	var (
		config Channel
		err    error
	)

	if _, ok := c.channelGroup.Values[ConsortiumKey]; ok {
		consortiumProto := &cb.Consortium{}
		err := unmarshalConfigValueAtKey(c.channelGroup, ConsortiumKey, consortiumProto)
		if err != nil {
			return Channel{}, err
		}
		config.Consortium = consortiumProto.Name
	}

	if applicationGroup, ok := c.channelGroup.Groups[ApplicationGroupKey]; ok {
		a := &ApplicationGroup{applicationGroup: applicationGroup}
		config.Application, err = a.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if ordererGroup, ok := c.channelGroup.Groups[OrdererGroupKey]; ok {
		o := &OrdererGroup{ordererGroup: ordererGroup, channelGroup: c.channelGroup}
		config.Orderer, err = o.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if consortiumsGroup, ok := c.channelGroup.Groups[ConsortiumsGroupKey]; ok {
		c := &ConsortiumsGroup{consortiumsGroup: consortiumsGroup}
		config.Consortiums, err = c.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if _, ok := c.channelGroup.Values[CapabilitiesKey]; ok {
		config.Capabilities, err = c.Capabilities()
		if err != nil {
			return Channel{}, err
		}
	}

	config.Policies, err = c.Policies()
	if err != nil {
		return Channel{}, err
	}

	return config, nil
}

// Policies returns a map of policies for channel configuration.
func (c *ChannelGroup) Policies() (map[string]Policy, error) {
	return getPolicies(c.channelGroup.Policies)
}

// SetModPolicy sets the specified modification policy for the channel group.
func (c *ChannelGroup) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	c.channelGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the channel group's config policy map.
// If the policy already exists in current configuration, its value will be overwritten.
func (c *ChannelGroup) SetPolicy(policyName string, policy Policy) error {
	return setPolicy(c.channelGroup, policyName, policy)
}

// SetPolicies sets the specified policies in the channel group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (c *ChannelGroup) SetPolicies(policies map[string]Policy) error {
	return setPolicies(c.channelGroup, policies)
}

// RemovePolicy removes an existing channel level policy.
func (c *ChannelGroup) RemovePolicy(policyName string) error {
	policies, err := c.Policies()
	if err != nil {
		return err
	}

	removePolicy(c.channelGroup, policyName, policies)
	return nil
}

// Capabilities returns a map of enabled channel capabilities
// from a config transaction's updated config.
func (c *ChannelGroup) Capabilities() ([]string, error) {
	capabilities, err := getCapabilities(c.channelGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving channel capabilities: %v", err)
	}

	return capabilities, nil
}

// AddCapability adds capability to the provided channel config.
// If the provided capability already exists in current configuration, this action
// will be a no-op.
func (c *ChannelGroup) AddCapability(capability string) error {
	capabilities, err := c.Capabilities()
	if err != nil {
		return err
	}

	err = addCapability(c.channelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveCapability removes capability to the provided channel config.
func (c *ChannelGroup) RemoveCapability(capability string) error {
	capabilities, err := c.Capabilities()
	if err != nil {
		return err
	}

	err = removeCapability(c.channelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveLegacyOrdererAddresses removes the deprecated top level orderer addresses config key and value
// from the channel config.
// In fabric 1.4, top level orderer addresses were migrated to the org level orderer endpoints
// While top-level orderer addresses are still supported, the organization value is preferred.
func (c *ChannelGroup) RemoveLegacyOrdererAddresses() {
	delete(c.channelGroup.Values, OrdererAddressesKey)
}
