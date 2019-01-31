/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/capabilities"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

// The tests in this file are all relatively pointless, as all of this function is exercised
// in the normal startup path and things will break horribly if they are broken.
// There's additionally really nothing to test without simply re-implementing the function
// in the test, which also provides no value.  But, not including these produces an artificially
// low code coverage count, so here they are.

func basicTest(t *testing.T, sv *StandardConfigValue) {
	assert.NotNil(t, sv)
	assert.NotEmpty(t, sv.Key())
	assert.NotNil(t, sv.Value())
}

func TestUtilsBasic(t *testing.T) {
	basicTest(t, ConsortiumValue("foo"))
	basicTest(t, HashingAlgorithmValue())
	basicTest(t, BlockDataHashingStructureValue())
	basicTest(t, OrdererAddressesValue([]string{"foo:1", "bar:2"}))
	basicTest(t, ConsensusTypeValue("foo", []byte("bar")))
	basicTest(t, BatchSizeValue(1, 2, 3))
	basicTest(t, BatchTimeoutValue("1s"))
	basicTest(t, ChannelRestrictionsValue(7))
	basicTest(t, KafkaBrokersValue([]string{"foo:1", "bar:2"}))
	basicTest(t, MSPValue(&mspprotos.MSPConfig{}))
	basicTest(t, CapabilitiesValue(map[string]bool{"foo": true, "bar": false}))
	basicTest(t, AnchorPeersValue([]*pb.AnchorPeer{{}, {}}))
	basicTest(t, ChannelCreationPolicyValue(&cb.Policy{}))
	basicTest(t, ACLValues(map[string]string{"foo": "fooval", "bar": "barval"}))
}

// createCfgBlockWithSupportedCapabilities will create a config block that contains valid capabilities and should be accepted by the peer
func createCfgBlockWithSupportedCapabilities(t *testing.T) *cb.Block {
	// Create a config
	config := &cb.Config{
		Sequence:     0,
		ChannelGroup: cb.NewConfigGroup(),
	}

	// construct the config for top group
	config.ChannelGroup.Version = 0
	config.ChannelGroup.ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Values[BlockDataHashingStructureKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	topCapabilities := make(map[string]bool)
	topCapabilities[capabilities.ChannelV1_1] = true
	config.ChannelGroup.Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[ConsortiumKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.Consortium{
			Name: "testConsortium",
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[HashingAlgorithmKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[OrdererAddressesKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.OrdererAddresses{
			Addresses: []string{"orderer.example.com"},
		}),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Application group
	config.ChannelGroup.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	config.ChannelGroup.Groups[ApplicationGroupKey].Version = 0
	config.ChannelGroup.Groups[ApplicationGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	appCapabilities := make(map[string]bool)
	appCapabilities[capabilities.ApplicationV1_1] = true
	config.ChannelGroup.Groups[ApplicationGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(appCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Orderer group
	config.ChannelGroup.Groups[OrdererGroupKey] = cb.NewConfigGroup()
	config.ChannelGroup.Groups[OrdererGroupKey].Version = 0
	config.ChannelGroup.Groups[OrdererGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchSizeKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.BatchSize{
				MaxMessageCount:   65535,
				AbsoluteMaxBytes:  1024000000,
				PreferredMaxBytes: 1024000000,
			}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchTimeoutKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.BatchTimeout{
				Timeout: "2s",
			}),
		ModPolicy: AdminsPolicyKey,
	}
	ordererCapabilities := make(map[string]bool)
	ordererCapabilities[capabilities.OrdererV1_1] = true
	config.ChannelGroup.Groups[OrdererGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(ordererCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[ConsensusTypeKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.ConsensusType{
				Type: "solo",
			}),
		ModPolicy: AdminsPolicyKey,
	}

	env := &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: "testChain",
					Type:      int32(cb.HeaderType_CONFIG),
				}),
			},
			Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: config,
			}),
		}),
	}
	configBlock := &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{[]byte(utils.MarshalOrPanic(env))},
		},
	}
	return configBlock
}

// createCfgBlockWithUnSupportedCapabilities will create a config block that contains mismatched capabilities and should be rejected by the peer
func createCfgBlockWithUnsupportedCapabilities(t *testing.T) *cb.Block {
	// Create a config
	config := &cb.Config{
		Sequence:     0,
		ChannelGroup: cb.NewConfigGroup(),
	}

	// construct the config for top group
	config.ChannelGroup.Version = 0
	config.ChannelGroup.ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Values[BlockDataHashingStructureKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	topCapabilities := make(map[string]bool)
	topCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[ConsortiumKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.Consortium{
			Name: "testConsortium",
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[HashingAlgorithmKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[OrdererAddressesKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(&cb.OrdererAddresses{
			Addresses: []string{"orderer.example.com"},
		}),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Application group
	config.ChannelGroup.Groups[ApplicationGroupKey] = cb.NewConfigGroup()
	config.ChannelGroup.Groups[ApplicationGroupKey].Version = 0
	config.ChannelGroup.Groups[ApplicationGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	appCapabilities := make(map[string]bool)
	appCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Groups[ApplicationGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(appCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Orderer group
	config.ChannelGroup.Groups[OrdererGroupKey] = cb.NewConfigGroup()
	config.ChannelGroup.Groups[OrdererGroupKey].Version = 0
	config.ChannelGroup.Groups[OrdererGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchSizeKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.BatchSize{
				MaxMessageCount:   65535,
				AbsoluteMaxBytes:  1024000000,
				PreferredMaxBytes: 1024000000,
			}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchTimeoutKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.BatchTimeout{
				Timeout: "2s",
			}),
		ModPolicy: AdminsPolicyKey,
	}
	ordererCapabilities := make(map[string]bool)
	ordererCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Groups[OrdererGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     utils.MarshalOrPanic(CapabilitiesValue(ordererCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[ConsensusTypeKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(
			&ab.ConsensusType{
				Type: "solo",
			}),
		ModPolicy: AdminsPolicyKey,
	}

	env := &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: "testChain",
					Type:      int32(cb.HeaderType_CONFIG),
				}),
			},
			Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: config,
			}),
		}),
	}
	configBlock := &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{[]byte(utils.MarshalOrPanic(env))},
		},
	}
	return configBlock
}

func TestValidateCapabilities(t *testing.T) {

	// Test config block with valid capabilities requirement
	cfgBlock := createCfgBlockWithSupportedCapabilities(t)
	assert.Nil(t, ValidateCapabilities(cfgBlock), "Should return Nil with matched capabilities checking")

	// Test config block with invalid capabilities requirement
	cfgBlock = createCfgBlockWithUnsupportedCapabilities(t)
	assert.NotNil(t, ValidateCapabilities(cfgBlock), "Should return Error with mismatched capabilities checking")

}
