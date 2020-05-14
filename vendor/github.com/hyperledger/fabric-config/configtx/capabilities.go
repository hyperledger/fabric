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
		return nil, fmt.Errorf("unmarshaling capabilities: %v", err)
	}

	capabilities := []string{}

	for capability := range capabilitiesProto.Capabilities {
		capabilities = append(capabilities, capability)
	}

	return capabilities, nil
}
