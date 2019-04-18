/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"
	"math"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
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

	// CapabilitiesKey is the name of the key which refers to capabilities, it appears at the channel,
	// application, and orderer levels and this constant is used for all three.
	CapabilitiesKey = "Capabilities"
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
	Capabilities              *cb.Capabilities
}

// ChannelConfig stores the channel configuration
type ChannelConfig struct {
	protos *ChannelProtos

	hashingAlgorithm func(input []byte) []byte

	mspManager msp.MSPManager

	appConfig         *ApplicationConfig
	ordererConfig     *OrdererConfig
	consortiumsConfig *ConsortiumsConfig
}

// NewChannelConfig creates a new ChannelConfig
func NewChannelConfig(channelGroup *cb.ConfigGroup) (*ChannelConfig, error) {
	cc := &ChannelConfig{
		protos: &ChannelProtos{},
	}

	if err := DeserializeProtoValuesFromGroup(channelGroup, cc.protos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	capabilities := cc.Capabilities()

	if err := cc.Validate(capabilities); err != nil {
		return nil, err
	}

	mspConfigHandler := NewMSPConfigHandler(capabilities.MSPVersion())

	var err error
	for groupName, group := range channelGroup.Groups {
		switch groupName {
		case ApplicationGroupKey:
			cc.appConfig, err = NewApplicationConfig(group, mspConfigHandler)
		case OrdererGroupKey:
			cc.ordererConfig, err = NewOrdererConfig(group, mspConfigHandler, capabilities)
		case ConsortiumsGroupKey:
			cc.consortiumsConfig, err = NewConsortiumsConfig(group, mspConfigHandler)
		default:
			return nil, fmt.Errorf("Disallowed channel group: %s", group)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "could not create channel %s sub-group config", groupName)
		}
	}

	if cc.mspManager, err = mspConfigHandler.CreateMSPManager(); err != nil {
		return nil, err
	}

	return cc, nil
}

// MSPManager returns the MSP manager for this config
func (cc *ChannelConfig) MSPManager() msp.MSPManager {
	return cc.mspManager
}

// OrdererConfig returns the orderer config associated with this channel
func (cc *ChannelConfig) OrdererConfig() *OrdererConfig {
	return cc.ordererConfig
}

// ApplicationConfig returns the application config associated with this channel
func (cc *ChannelConfig) ApplicationConfig() *ApplicationConfig {
	return cc.appConfig
}

// ConsortiumsConfig returns the consortium config associated with this channel if it exists
func (cc *ChannelConfig) ConsortiumsConfig() *ConsortiumsConfig {
	return cc.consortiumsConfig
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

// Capabilities returns information about the available capabilities for this channel
func (cc *ChannelConfig) Capabilities() ChannelCapabilities {
	return capabilities.NewChannelProvider(cc.protos.Capabilities.Capabilities)
}

// Validate inspects the generated configuration protos and ensures that the values are correct
func (cc *ChannelConfig) Validate(channelCapabilities ChannelCapabilities) error {
	for _, validator := range []func() error{
		cc.validateHashingAlgorithm,
		cc.validateBlockDataHashingStructure,
	} {
		if err := validator(); err != nil {
			return err
		}
	}

	if !channelCapabilities.OrgSpecificOrdererEndpoints() {
		return cc.validateOrdererAddresses()
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
