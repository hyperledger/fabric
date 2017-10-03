/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"math"

	"github.com/hyperledger/fabric/bccsp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

const defaultHashingAlgorithm = bccsp.SHA256

func configGroup(key string, value []byte) *cb.ConfigGroup {
	result := cb.NewConfigGroup()
	result.Values[key] = &cb.ConfigValue{
		Value: value,
	}
	return result
}

// TemplateConsortiumName creates a ConfigGroup representing the ConsortiumName
func TemplateConsortium(name string) *cb.ConfigGroup {
	return configGroup(ConsortiumKey, utils.MarshalOrPanic(&cb.Consortium{Name: name}))
}

// TemplateHashingAlgorithm creates a ConfigGroup representing the HashingAlgorithm
func TemplateHashingAlgorithm(name string) *cb.ConfigGroup {
	return configGroup(HashingAlgorithmKey, utils.MarshalOrPanic(&cb.HashingAlgorithm{Name: name}))
}

// DefaultHashingAlgorithm creates a headerless config item for the default hashing algorithm
func DefaultHashingAlgorithm() *cb.ConfigGroup {
	return TemplateHashingAlgorithm(defaultHashingAlgorithm)
}

const defaultBlockDataHashingStructureWidth = math.MaxUint32

// TemplateBlockDataHashingStructure creates a headerless config item representing the block data hashing structure
func TemplateBlockDataHashingStructure(width uint32) *cb.ConfigGroup {
	return configGroup(BlockDataHashingStructureKey, utils.MarshalOrPanic(&cb.BlockDataHashingStructure{Width: width}))
}

// DefaultBlockDatahashingStructure creates a headerless config item for the default block data hashing structure
func DefaultBlockDataHashingStructure() *cb.ConfigGroup {
	return TemplateBlockDataHashingStructure(defaultBlockDataHashingStructureWidth)
}

var defaultOrdererAddresses = []string{"127.0.0.1:7050"}

// TemplateOrdererAddressess creates a headerless config item representing the orderer addresses
func TemplateOrdererAddresses(addresses []string) *cb.ConfigGroup {
	return configGroup(OrdererAddressesKey, utils.MarshalOrPanic(&cb.OrdererAddresses{Addresses: addresses}))
}

// DefaultOrdererAddresses creates a headerless config item for the default orderer addresses
func DefaultOrdererAddresses() *cb.ConfigGroup {
	return TemplateOrdererAddresses(defaultOrdererAddresses)
}

func capabilitiesFromBoolMap(capabilities map[string]bool) *cb.Capabilities {
	value := &cb.Capabilities{
		Capabilities: make(map[string]*cb.Capability),
	}
	for capability, required := range capabilities {
		if !required {
			continue
		}
		value.Capabilities[capability] = &cb.Capability{}
	}
	return value
}

// TemplateChannelCapabilities creates a config value representing the channel capabilities
func TemplateChannelCapabilities(capabilities map[string]bool) *cb.ConfigGroup {
	return configGroup(CapabilitiesKey, utils.MarshalOrPanic(capabilitiesFromBoolMap(capabilities)))
}
