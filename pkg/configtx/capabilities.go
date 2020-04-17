/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
)

// ChannelCapabilities returns a map of enabled channel capabilities
// from a config transaction.
func (c *ConfigTx) ChannelCapabilities() ([]string, error) {
	capabilities, err := getCapabilities(c.original.ChannelGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving channel capabilities: %v", err)
	}

	return capabilities, nil
}

// OrdererCapabilities returns a map of enabled orderer capabilities
// from a config transaction.
func (c *ConfigTx) OrdererCapabilities() ([]string, error) {
	orderer := c.original.ChannelGroup.Groups[OrdererGroupKey]

	capabilities, err := getCapabilities(orderer)
	if err != nil {
		return nil, fmt.Errorf("retrieving orderer capabilities: %v", err)
	}

	return capabilities, nil
}

// ApplicationCapabilities returns a map of enabled application capabilities
// from a config transaction.
func (c *ConfigTx) ApplicationCapabilities() ([]string, error) {
	application := c.original.ChannelGroup.Groups[ApplicationGroupKey]

	capabilities, err := getCapabilities(application)
	if err != nil {
		return nil, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	return capabilities, nil
}

// AddChannelCapability adds capability to the provided channel config.
// If the provided capability already exist in current configuration, this action
// will be a no-op.
func (c *ConfigTx) AddChannelCapability(capability string) error {
	capabilities, err := c.ChannelCapabilities()
	if err != nil {
		return err
	}

	err = addCapability(c.updated.ChannelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// AddOrdererCapability adds capability to the provided channel config.
// If the provided capability already exist in current configuration, this action
// will be a no-op.
func (c *ConfigTx) AddOrdererCapability(capability string) error {
	capabilities, err := c.OrdererCapabilities()
	if err != nil {
		return err
	}

	err = addCapability(c.updated.ChannelGroup.Groups[OrdererGroupKey], capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// AddApplicationCapability sets capability to the provided channel config.
// If the provided capability already exist in current configuration, this action
// will be a no-op.
func (c *ConfigTx) AddApplicationCapability(capability string) error {
	capabilities, err := c.ApplicationCapabilities()
	if err != nil {
		return err
	}

	err = addCapability(c.updated.ChannelGroup.Groups[ApplicationGroupKey], capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveChannelCapability removes capability to the provided channel config.
func (c *ConfigTx) RemoveChannelCapability(capability string) error {
	capabilities, err := c.ChannelCapabilities()
	if err != nil {
		return err
	}

	err = removeCapability(c.updated.ChannelGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveOrdererCapability removes capability to the provided channel config.
func (c *ConfigTx) RemoveOrdererCapability(capability string) error {
	capabilities, err := c.OrdererCapabilities()
	if err != nil {
		return err
	}

	err = removeCapability(c.updated.ChannelGroup.Groups[OrdererGroupKey], capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveApplicationCapability removes capability to the provided channel config.
func (c *ConfigTx) RemoveApplicationCapability(capability string) error {
	capabilities, err := c.ApplicationCapabilities()
	if err != nil {
		return err
	}

	err = removeCapability(c.updated.ChannelGroup.Groups[ApplicationGroupKey], capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// capabilitiesValue returns the config definition for a set of capabilities.
// It is a value for the /Channel/Orderer, Channel/Application/, and /Channel groups.
func capabilitiesValue(capabilities []string) *standardConfigValue {
	c := &cb.Capabilities{
		Capabilities: make(map[string]*cb.Capability),
	}

	for _, capability := range capabilities {
		c.Capabilities[capability] = &cb.Capability{}
	}

	return &standardConfigValue{
		key:   CapabilitiesKey,
		value: c,
	}
}

func addCapability(configGroup *cb.ConfigGroup, capabilities []string, modPolicy string, capability string) error {
	for _, c := range capabilities {
		if c == capability {
			// if capability already exist, do nothing.
			return nil
		}
	}
	capabilities = append(capabilities, capability)

	err := setValue(configGroup, capabilitiesValue(capabilities), modPolicy)
	if err != nil {
		return fmt.Errorf("adding capability: %v", err)
	}

	return nil
}

func removeCapability(configGroup *cb.ConfigGroup, capabilities []string, modPolicy string, capability string) error {
	var updatedCapabilities []string

	for _, c := range capabilities {
		if c != capability {
			updatedCapabilities = append(updatedCapabilities, c)
		}
	}

	if len(updatedCapabilities) == len(capabilities) {
		return errors.New("capability not set")
	}

	err := setValue(configGroup, capabilitiesValue(updatedCapabilities), modPolicy)
	if err != nil {
		return fmt.Errorf("removing capability: %v", err)
	}

	return nil
}

func getCapabilities(configGroup *cb.ConfigGroup) ([]string, error) {
	capabilitiesValue, ok := configGroup.Values[CapabilitiesKey]
	if !ok {
		// no capabilities defined/enabled
		return nil, nil
	}

	capabilitiesProto := &cb.Capabilities{}

	err := proto.Unmarshal(capabilitiesValue.Value, capabilitiesProto)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling capabilities: %v", err)
	}

	capabilities := []string{}

	for capability := range capabilitiesProto.Capabilities {
		capabilities = append(capabilities, capability)
	}

	return capabilities, nil
}
