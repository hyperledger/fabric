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
	"fmt"
	"math"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Channel config keys
const (
	// ConsortiumKey is the key for the cb.ConfigValue for the Consortium message
	ConsortiumKey = "Consortium"

	// HashingAlgorithmKey is the cb.ConfigItem type key name for the HashingAlgorithm message
	HashingAlgorithmKey = "HashingAlgorithm"

	// BlockDataHashingStructureKey is the cb.ConfigItem type key name for the BlockDataHashingStructure message
	BlockDataHashingStructureKey = "BlockDataHashingStructure"

	// OrdererAddressesKey is the cb.ConfigItem type key name for the OrdererAddresses message
	OrdererAddressesKey = "OrdererAddresses"

	// GroupKey is the name of the channel group
	ChannelGroupKey = "Channel"
)

// ChannelValues gives read only access to the channel configuration
type ChannelValues interface {
	// HashingAlgorithm returns the default algorithm to be used when hashing
	// such as computing block hashes, and CreationPolicy digests
	HashingAlgorithm() func(input []byte) []byte

	// BlockDataHashingStructureWidth returns the width to use when constructing the
	// Merkle tree to compute the BlockData hash
	BlockDataHashingStructureWidth() uint32

	// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
	OrdererAddresses() []string
}

// ChannelProtos is where the proposed configuration is unmarshaled into
type ChannelProtos struct {
	HashingAlgorithm          *cb.HashingAlgorithm
	BlockDataHashingStructure *cb.BlockDataHashingStructure
	OrdererAddresses          *cb.OrdererAddresses
	Consortium                *cb.Consortium
}

type channelConfigSetter struct {
	target **ChannelConfig
	*ChannelConfig
}

func (ccs *channelConfigSetter) Commit() {
	*(ccs.target) = ccs.ChannelConfig
}

// ChannelGroup
type ChannelGroup struct {
	*ChannelConfig
	*Proposer
	mspConfigHandler *msp.MSPConfigHandler
}

func NewChannelGroup(mspConfigHandler *msp.MSPConfigHandler) *ChannelGroup {
	cg := &ChannelGroup{
		ChannelConfig:    NewChannelConfig(),
		mspConfigHandler: mspConfigHandler,
	}
	cg.Proposer = NewProposer(cg)
	return cg
}

// Allocate creates new config resources for a pending config update
func (cg *ChannelGroup) Allocate() Values {
	return &channelConfigSetter{
		ChannelConfig: NewChannelConfig(),
		target:        &cg.ChannelConfig,
	}
}

// OrdererConfig returns the orderer config associated with this channel
func (cg *ChannelGroup) OrdererConfig() *OrdererGroup {
	return cg.ChannelConfig.ordererConfig
}

// ApplicationConfig returns the application config associated with this channel
func (cg *ChannelGroup) ApplicationConfig() *ApplicationGroup {
	return cg.ChannelConfig.appConfig
}

// ConsortiumsConfig returns the consortium config associated with this channel if it exists
func (cg *ChannelGroup) ConsortiumsConfig() *ConsortiumsGroup {
	return cg.ChannelConfig.consortiumsConfig
}

// NewGroup instantiates either a new application or orderer config
func (cg *ChannelGroup) NewGroup(group string) (ValueProposer, error) {
	switch group {
	case ApplicationGroupKey:
		return NewApplicationGroup(cg.mspConfigHandler), nil
	case OrdererGroupKey:
		return NewOrdererGroup(cg.mspConfigHandler), nil
	case ConsortiumsGroupKey:
		return NewConsortiumsGroup(cg.mspConfigHandler), nil
	default:
		return nil, fmt.Errorf("Disallowed channel group: %s", group)
	}
}

// ChannelConfig stores the channel configuration
type ChannelConfig struct {
	*standardValues
	protos *ChannelProtos

	hashingAlgorithm func(input []byte) []byte

	appConfig         *ApplicationGroup
	ordererConfig     *OrdererGroup
	consortiumsConfig *ConsortiumsGroup
}

// NewChannelConfig creates a new ChannelConfig
func NewChannelConfig() *ChannelConfig {
	cc := &ChannelConfig{
		protos: &ChannelProtos{},
	}

	var err error
	cc.standardValues, err = NewStandardValues(cc.protos)
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}
	return cc
}

// HashingAlgorithm returns a function pointer to the chain hashing algorihtm
func (cc *ChannelConfig) HashingAlgorithm() func(input []byte) []byte {
	return cc.hashingAlgorithm
}

// BlockDataHashingStructure returns the width to use when forming the block data hashing structure
func (cc *ChannelConfig) BlockDataHashingStructureWidth() uint32 {
	return cc.protos.BlockDataHashingStructure.Width
}

// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (cc *ChannelConfig) OrdererAddresses() []string {
	return cc.protos.OrdererAddresses.Addresses
}

// ConsortiumName returns the name of the consortium this channel was created under
func (cc *ChannelConfig) ConsortiumName() string {
	return cc.protos.Consortium.Name
}

// Validate inspects the generated configuration protos, ensures that the values are correct, and
// sets the ChannelConfig fields that may be referenced after Commit
func (cc *ChannelConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	for _, validator := range []func() error{
		cc.validateHashingAlgorithm,
		cc.validateBlockDataHashingStructure,
		cc.validateOrdererAddresses,
	} {
		if err := validator(); err != nil {
			return err
		}
	}

	var ok bool
	for key, value := range groups {
		switch key {
		case ApplicationGroupKey:
			cc.appConfig, ok = value.(*ApplicationGroup)
			if !ok {
				return fmt.Errorf("Application group was not Application config")
			}
		case OrdererGroupKey:
			cc.ordererConfig, ok = value.(*OrdererGroup)
			if !ok {
				return fmt.Errorf("Orderer group was not Orderer config")
			}
		case ConsortiumsGroupKey:
			cc.consortiumsConfig, ok = value.(*ConsortiumsGroup)
			if !ok {
				return fmt.Errorf("Consortiums group was no Consortium config")
			}
		default:
			return fmt.Errorf("Disallowed channel group: %s", key)
		}
	}

	return nil
}

func (cc *ChannelConfig) validateHashingAlgorithm() error {
	switch cc.protos.HashingAlgorithm.Name {
	case bccsp.SHA256:
		cc.hashingAlgorithm = util.ComputeSHA256
	case bccsp.SHA3_256:
		cc.hashingAlgorithm = util.ComputeSHA3256
	default:
		return fmt.Errorf("Unknown hashing algorithm type: %s", cc.protos.HashingAlgorithm.Name)
	}

	return nil
}

func (cc *ChannelConfig) validateBlockDataHashingStructure() error {
	if cc.protos.BlockDataHashingStructure.Width != math.MaxUint32 {
		return fmt.Errorf("BlockDataHashStructure width only supported at MaxUint32 in this version")
	}
	return nil
}

func (cc *ChannelConfig) validateOrdererAddresses() error {
	if len(cc.protos.OrdererAddresses.Addresses) == 0 {
		return fmt.Errorf("Must set some OrdererAddresses")
	}
	return nil
}
