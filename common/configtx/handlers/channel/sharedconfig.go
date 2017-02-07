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

package channel

import (
	"fmt"
	"math"

	"github.com/hyperledger/fabric/common/configtx/handlers/application"
	"github.com/hyperledger/fabric/common/configtx/handlers/orderer"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

const (
	// ApplicationGroupKey is the group name for the application config
	ApplicationGroupKey = "Application"

	// OrdererGroupKey is the group name for the orderer config
	OrdererGroupKey = "Orderer"
)

var Schema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{
		ApplicationGroupKey: application.Schema,
		OrdererGroupKey:     orderer.Schema,
	},
	Values: map[string]*cb.ConfigValueSchema{
		HashingAlgorithmKey:          nil,
		BlockDataHashingStructureKey: nil,
		OrdererAddressesKey:          nil,
	},
	Policies: map[string]*cb.ConfigPolicySchema{
	// TODO, set appropriately once hierarchical policies are implemented
	},
}

// Chain config keys
const (
	// HashingAlgorithmKey is the cb.ConfigItem type key name for the HashingAlgorithm message
	HashingAlgorithmKey = "HashingAlgorithm"

	// BlockDataHashingStructureKey is the cb.ConfigItem type key name for the BlockDataHashingStructure message
	BlockDataHashingStructureKey = "BlockDataHashingStructure"

	// OrdererAddressesKey is the cb.ConfigItem type key name for the OrdererAddresses message
	OrdererAddressesKey = "OrdererAddresses"
)

// Hashing algorithm types
const (
	// SHAKE256 is the algorithm type for the sha3 shake256 hashing algorithm with 512 bits of output
	SHA3Shake256 = "SHAKE256"
)

var logger = logging.MustGetLogger("configtx/handlers/chainconfig")

type chainConfig struct {
	hashingAlgorithm               func(input []byte) []byte
	blockDataHashingStructureWidth uint32
	ordererAddresses               []string
}

// SharedConfigImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type SharedConfigImpl struct {
	pendingConfig *chainConfig
	config        *chainConfig
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewSharedConfigImpl() *SharedConfigImpl {
	return &SharedConfigImpl{
		config: &chainConfig{},
	}
}

// HashingAlgorithm returns a function pointer to the chain hashing algorihtm
func (pm *SharedConfigImpl) HashingAlgorithm() func(input []byte) []byte {
	return pm.config.hashingAlgorithm
}

// BlockDataHashingStructure returns the width to use when forming the block data hashing structure
func (pm *SharedConfigImpl) BlockDataHashingStructureWidth() uint32 {
	return pm.config.blockDataHashingStructureWidth
}

// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (pm *SharedConfigImpl) OrdererAddresses() []string {
	return pm.config.ordererAddresses
}

// BeginConfig is used to start a new config proposal
func (pm *SharedConfigImpl) BeginConfig() {
	if pm.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	pm.pendingConfig = &chainConfig{}
}

// RollbackConfig is used to abandon a new config proposal
func (pm *SharedConfigImpl) RollbackConfig() {
	pm.pendingConfig = nil
}

// CommitConfig is used to commit a new config proposal
func (pm *SharedConfigImpl) CommitConfig() {
	if pm.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	pm.config = pm.pendingConfig
	pm.pendingConfig = nil
}

// ProposeConfig is used to add new config to the config proposal
func (pm *SharedConfigImpl) ProposeConfig(configItem *cb.ConfigItem) error {
	if configItem.Type != cb.ConfigItem_CHAIN {
		return fmt.Errorf("Expected type of ConfigItem_Chain, got %v", configItem.Type)
	}

	switch configItem.Key {
	case HashingAlgorithmKey:
		hashingAlgorithm := &cb.HashingAlgorithm{}
		if err := proto.Unmarshal(configItem.Value, hashingAlgorithm); err != nil {
			return fmt.Errorf("Unmarshaling error for HashingAlgorithm: %s", err)
		}
		switch hashingAlgorithm.Name {
		case SHA3Shake256:
			pm.pendingConfig.hashingAlgorithm = util.ComputeCryptoHash
		default:
			return fmt.Errorf("Unknown hashing algorithm type: %s", hashingAlgorithm.Name)
		}
	case BlockDataHashingStructureKey:
		blockDataHashingStructure := &cb.BlockDataHashingStructure{}
		if err := proto.Unmarshal(configItem.Value, blockDataHashingStructure); err != nil {
			return fmt.Errorf("Unmarshaling error for BlockDataHashingStructure: %s", err)
		}

		if blockDataHashingStructure.Width != math.MaxUint32 {
			return fmt.Errorf("BlockDataHashStructure width only supported at MaxUint32 in this version")
		}

		pm.pendingConfig.blockDataHashingStructureWidth = blockDataHashingStructure.Width
	case OrdererAddressesKey:
		ordererAddresses := &cb.OrdererAddresses{}
		if err := proto.Unmarshal(configItem.Value, ordererAddresses); err != nil {
			return fmt.Errorf("Unmarshaling error for HashingAlgorithm: %s", err)
		}
		pm.pendingConfig.ordererAddresses = ordererAddresses.Addresses
	default:
		logger.Warningf("Uknown Chain config item with key %s", configItem.Key)
	}
	return nil
}
