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

package chainconfig

import (
	"math"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

const defaultHashingAlgorithm = SHA3Shake256

// TemplateHashingAlgorithm creates a headerless configuration item representing the hashing algorithm
func TemplateHashingAlgorithm(name string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   HashingAlgorithmKey,
		Value: utils.MarshalOrPanic(&cb.HashingAlgorithm{Name: name}),
	}

}

// DefaultHashingAlgorithm creates a headerless configuration item for the default hashing algorithm
func DefaultHashingAlgorithm() *cb.ConfigurationItem {
	return TemplateHashingAlgorithm(defaultHashingAlgorithm)
}

const defaultBlockDataHashingStructureWidth = math.MaxUint32

// TemplateBlockDataHashingStructure creates a headerless configuration item representing the block data hashing structure
func TemplateBlockDataHashingStructure(width uint32) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   BlockDataHashingStructureKey,
		Value: utils.MarshalOrPanic(&cb.BlockDataHashingStructure{Width: width}),
	}
}

// DefaultBlockDatahashingStructure creates a headerless configuration item for the default block data hashing structure
func DefaultBlockDataHashingStructure() *cb.ConfigurationItem {
	return TemplateBlockDataHashingStructure(defaultBlockDataHashingStructureWidth)
}

var defaultOrdererAddresses = []string{"127.0.0.1:7050"}

// TemplateOrdererAddressess creates a headerless configuration item representing the orderer addresses
func TemplateOrdererAddresses(addresses []string) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   OrdererAddressesKey,
		Value: utils.MarshalOrPanic(&cb.OrdererAddresses{Addresses: addresses}),
	}
}

// DefaultOrdererAddresses creates a headerless configuration item for the default orderer addresses
func DefaultOrdererAddresses() *cb.ConfigurationItem {
	return TemplateOrdererAddresses(defaultOrdererAddresses)
}
