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

	api "github.com/hyperledger/fabric/common/configvalues"
	"github.com/hyperledger/fabric/common/configvalues/channel/application"
	"github.com/hyperledger/fabric/common/configvalues/channel/orderer"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

var Schema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{
		application.GroupKey: application.Schema,
		orderer.GroupKey:     orderer.Schema,
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

// Channel config keys
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
	// SHA3256 is SHA3 with fixed size 256-bit hash
	SHA3256 = "SHA3256"

	// SHA256 is SHA2 with fixed size 256-bit hash
	SHA256 = "SHA256"
)

var logger = logging.MustGetLogger("configvalues/channel")

type ConfigReader interface {
	// HashingAlgorithm returns the default algorithm to be used when hashing
	// such as computing block hashes, and CreationPolicy digests
	HashingAlgorithm() func(input []byte) []byte

	// BlockDataHashingStructureWidth returns the width to use when constructing the
	// Merkle tree to compute the BlockData hash
	BlockDataHashingStructureWidth() uint32

	// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
	OrdererAddresses() []string
}

type values struct {
	hashingAlgorithm               func(input []byte) []byte
	blockDataHashingStructureWidth uint32
	ordererAddresses               []string
}

// SharedConfigImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type Config struct {
	pending *values
	current *values

	ordererConfig     *orderer.ManagerImpl
	applicationConfig *application.SharedConfigImpl
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewConfig(ordererConfig *orderer.ManagerImpl, applicationConfig *application.SharedConfigImpl) *Config {
	return &Config{
		current:           &values{},
		ordererConfig:     ordererConfig,
		applicationConfig: applicationConfig,
	}
}

// HashingAlgorithm returns a function pointer to the chain hashing algorihtm
func (c *Config) HashingAlgorithm() func(input []byte) []byte {
	return c.current.hashingAlgorithm
}

// BlockDataHashingStructure returns the width to use when forming the block data hashing structure
func (c *Config) BlockDataHashingStructureWidth() uint32 {
	return c.current.blockDataHashingStructureWidth
}

// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (c *Config) OrdererAddresses() []string {
	return c.current.ordererAddresses
}

// BeginValueProposals is used to start a new config proposal
func (c *Config) BeginValueProposals(groups []string) ([]api.ValueProposer, error) {
	handlers := make([]api.ValueProposer, len(groups))

	for i, group := range groups {
		switch group {
		case application.GroupKey:
			handlers[i] = c.applicationConfig
		case orderer.GroupKey:
			handlers[i] = c.ordererConfig
		default:
			return nil, fmt.Errorf("Disallowed channel group: %s", group)
		}
	}

	if c.pending != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}

	c.pending = &values{}
	return handlers, nil
}

// RollbackProposals is used to abandon a new config proposal
func (c *Config) RollbackProposals() {
	c.pending = nil
}

// CommitProposals is used to commit a new config proposal
func (c *Config) CommitProposals() {
	if c.pending == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	c.current = c.pending
	c.pending = nil
}

// PreCommit returns nil
func (c *Config) PreCommit() error { return nil }

// ProposeValue is used to add new config to the config proposal
func (c *Config) ProposeValue(key string, configValue *cb.ConfigValue) error {
	switch key {
	case HashingAlgorithmKey:
		hashingAlgorithm := &cb.HashingAlgorithm{}
		if err := proto.Unmarshal(configValue.Value, hashingAlgorithm); err != nil {
			return fmt.Errorf("Unmarshaling error for HashingAlgorithm: %s", err)
		}
		switch hashingAlgorithm.Name {
		case SHA256:
			c.pending.hashingAlgorithm = util.ComputeSHA256
		case SHA3256:
			c.pending.hashingAlgorithm = util.ComputeSHA3256
		default:
			return fmt.Errorf("Unknown hashing algorithm type: %s", hashingAlgorithm.Name)
		}
	case BlockDataHashingStructureKey:
		blockDataHashingStructure := &cb.BlockDataHashingStructure{}
		if err := proto.Unmarshal(configValue.Value, blockDataHashingStructure); err != nil {
			return fmt.Errorf("Unmarshaling error for BlockDataHashingStructure: %s", err)
		}

		if blockDataHashingStructure.Width != math.MaxUint32 {
			return fmt.Errorf("BlockDataHashStructure width only supported at MaxUint32 in this version")
		}

		c.pending.blockDataHashingStructureWidth = blockDataHashingStructure.Width
	case OrdererAddressesKey:
		ordererAddresses := &cb.OrdererAddresses{}
		if err := proto.Unmarshal(configValue.Value, ordererAddresses); err != nil {
			return fmt.Errorf("Unmarshaling error for HashingAlgorithm: %s", err)
		}
		c.pending.ordererAddresses = ordererAddresses.Addresses
	default:
		logger.Warningf("Uknown Chain config item with key %s", key)
	}
	return nil
}
