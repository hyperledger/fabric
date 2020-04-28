/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go/common"
)

type ChannelGroup struct {
	channelGroup *cb.ConfigGroup
}

type UpdatedChannelGroup struct {
	*ChannelGroup
}

func (c *Config) Channel() *ChannelGroup {
	return &ChannelGroup{channelGroup: c.ChannelGroup}
}

func (u *UpdatedConfig) Channel() *UpdatedChannelGroup {
	return &UpdatedChannelGroup{ChannelGroup: &ChannelGroup{channelGroup: u.ChannelGroup}}
}

// Configuration returns a channel configuration value from a config transaction.
func (c *ChannelGroup) Configuration() (Channel, error) {
	var (
		err          error
		consortium   string
		application  Application
		orderer      Orderer
		consortiums  []Consortium
		capabilities []string
	)

	if _, ok := c.channelGroup.Values[ConsortiumKey]; ok {
		consortiumProto := &cb.Consortium{}
		err := unmarshalConfigValueAtKey(c.channelGroup, ConsortiumKey, consortiumProto)
		if err != nil {
			return Channel{}, err
		}
		consortium = consortiumProto.Name
	}

	if applicationGroup, ok := c.channelGroup.Groups[ApplicationGroupKey]; ok {
		a := &ApplicationGroup{applicationGroup: applicationGroup}
		application, err = a.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if ordererGroup, ok := c.channelGroup.Groups[OrdererGroupKey]; ok {
		o := &OrdererGroup{ordererGroup: ordererGroup, channelGroup: c.channelGroup}
		orderer, err = o.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if consortiumsGroup, ok := c.channelGroup.Groups[ConsortiumsGroupKey]; ok {
		c := &ConsortiumsGroup{consortiumsGroup: consortiumsGroup}
		consortiums, err = c.Configuration()
		if err != nil {
			return Channel{}, err
		}
	}

	if _, ok := c.channelGroup.Values[CapabilitiesKey]; ok {
		capabilities, err = c.Capabilities()
		if err != nil {
			return Channel{}, err
		}
	}

	policies, err := c.Policies()
	if err != nil {
		return Channel{}, err
	}

	return Channel{
		Consortium:   consortium,
		Application:  application,
		Orderer:      orderer,
		Consortiums:  consortiums,
		Capabilities: capabilities,
		Policies:     policies,
	}, nil
}

// Configuration returns a channel configuration value from a config transaction.
func (u *UpdatedChannelGroup) Configuration() (Channel, error) {
	return u.ChannelGroup.Configuration()
}

// Policies returns a map of policies for channel configuration.
func (c *ChannelGroup) Policies() (map[string]Policy, error) {
	return getPolicies(c.channelGroup.Policies)
}

// Policies returns a map of policies for channel configuration.
func (u *UpdatedChannelGroup) Policies() (map[string]Policy, error) {
	return u.ChannelGroup.Policies()
}

// SetPolicy sets the specified policy in the channel group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (u *UpdatedChannelGroup) SetPolicy(modPolicy, policyName string, policy Policy) error {
	return setPolicy(u.channelGroup, modPolicy, policyName, policy)
}

// RemovePolicy removes an existing channel level policy.
func (u *UpdatedChannelGroup) RemovePolicy(policyName string) error {
	policies, err := u.Policies()
	if err != nil {
		return err
	}

	removePolicy(u.channelGroup, policyName, policies)
	return nil
}

// Capabilities returns a map of enabled channel capabilities
// from a config transaction's original config.
func (c *ChannelGroup) Capabilities() ([]string, error) {
	capabilities, err := getCapabilities(c.channelGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving channel capabilities: %v", err)
	}

	return capabilities, nil
}

// Capabilities returns a map of enabled channel capabilities
// from a config transaction's updated config..
func (u *UpdatedChannelGroup) Capabilities() ([]string, error) {
	return u.ChannelGroup.Capabilities()
}

// AddCapability adds capability to the provided channel config.
// If the provided capability already exist in current configuration, this action
// will be a no-op.
func (u *UpdatedChannelGroup) AddCapability(capability string) error {
	capabilities, err := u.Capabilities()
	if err != nil {
		return err
	}

	err = addCapability(u.channelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveCapability removes capability to the provided channel config.
func (u *UpdatedChannelGroup) RemoveCapability(capability string) error {
	capabilities, err := u.Capabilities()
	if err != nil {
		return err
	}

	err = removeCapability(u.channelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}
