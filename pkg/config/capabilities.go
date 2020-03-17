/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
)

// GetChannelCapabilities returns a map of enabled channel capabilities
// from a config transaction.
func (c *ConfigTx) GetChannelCapabilities() (map[string]bool, error) {
	capabilities, err := getCapabilities(c.base.ChannelGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving channel capabilities: %v", err)
	}

	return capabilities, nil
}

// GetOrdererCapabilities returns a map of enabled orderer capabilities
// from a config transaction.
func (c *ConfigTx) GetOrdererCapabilities() (map[string]bool, error) {
	orderer, ok := c.base.ChannelGroup.Groups[OrdererGroupKey]
	if !ok {
		return nil, errors.New("orderer missing from config")
	}

	capabilities, err := getCapabilities(orderer)
	if err != nil {
		return nil, fmt.Errorf("retrieving orderer capabilities: %v", err)
	}

	return capabilities, nil
}

// GetApplicationCapabilities returns a map of enabled application capabilities
// from a config transaction.
func (c *ConfigTx) GetApplicationCapabilities() (map[string]bool, error) {
	application, ok := c.base.ChannelGroup.Groups[ApplicationGroupKey]
	if !ok {
		return nil, errors.New("application missing from config")
	}

	capabilities, err := getCapabilities(application)
	if err != nil {
		return nil, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	return capabilities, nil
}

func getCapabilities(configGroup *cb.ConfigGroup) (map[string]bool, error) {
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

	capabilities := map[string]bool{}

	for capability := range capabilitiesProto.Capabilities {
		capabilities[capability] = true
	}

	return capabilities, nil
}
