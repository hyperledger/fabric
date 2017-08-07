/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

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
