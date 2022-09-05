/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// The tests in this file are all relatively pointless, as all of this function is exercised
// in the normal startup path and things will break horribly if they are broken.
// There's additionally really nothing to test without simply re-implementing the function
// in the test, which also provides no value.  But, not including these produces an artificially
// low code coverage count, so here they are.

func basicTest(t *testing.T, sv *StandardConfigValue) {
	require.NotNil(t, sv)
	require.NotEmpty(t, sv.Key())
	require.NotNil(t, sv.Value())
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
		ChannelGroup: protoutil.NewConfigGroup(),
	}

	// construct the config for top group
	config.ChannelGroup.Version = 0
	config.ChannelGroup.ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Values[BlockDataHashingStructureKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	topCapabilities := make(map[string]bool)
	topCapabilities[capabilities.ChannelV1_1] = true
	config.ChannelGroup.Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[ConsortiumKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.Consortium{
			Name: "testConsortium",
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[HashingAlgorithmKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[OrdererAddressesKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.OrdererAddresses{
			Addresses: []string{"orderer.example.com"},
		}),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Application group
	config.ChannelGroup.Groups[ApplicationGroupKey] = protoutil.NewConfigGroup()
	config.ChannelGroup.Groups[ApplicationGroupKey].Version = 0
	config.ChannelGroup.Groups[ApplicationGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	appCapabilities := make(map[string]bool)
	appCapabilities[capabilities.ApplicationV1_1] = true
	config.ChannelGroup.Groups[ApplicationGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(appCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Orderer group
	config.ChannelGroup.Groups[OrdererGroupKey] = protoutil.NewConfigGroup()
	config.ChannelGroup.Groups[OrdererGroupKey].Version = 0
	config.ChannelGroup.Groups[OrdererGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchSizeKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.BatchSize{
				MaxMessageCount:   65535,
				AbsoluteMaxBytes:  1024000000,
				PreferredMaxBytes: 1024000000,
			}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchTimeoutKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.BatchTimeout{
				Timeout: "2s",
			}),
		ModPolicy: AdminsPolicyKey,
	}
	ordererCapabilities := make(map[string]bool)
	ordererCapabilities[capabilities.OrdererV1_1] = true
	config.ChannelGroup.Groups[OrdererGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(ordererCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[ConsensusTypeKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.ConsensusType{
				Type: "solo",
			}),
		ModPolicy: AdminsPolicyKey,
	}

	env := &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: "testChain",
					Type:      int32(cb.HeaderType_CONFIG),
				}),
			},
			Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: config,
			}),
		}),
	}
	configBlock := &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{[]byte(protoutil.MarshalOrPanic(env))},
		},
	}
	return configBlock
}

// createCfgBlockWithUnSupportedCapabilities will create a config block that contains mismatched capabilities and should be rejected by the peer
func createCfgBlockWithUnsupportedCapabilities(t *testing.T) *cb.Block {
	// Create a config
	config := &cb.Config{
		Sequence:     0,
		ChannelGroup: protoutil.NewConfigGroup(),
	}

	// construct the config for top group
	config.ChannelGroup.Version = 0
	config.ChannelGroup.ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Values[BlockDataHashingStructureKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	topCapabilities := make(map[string]bool)
	topCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[ConsortiumKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.Consortium{
			Name: "testConsortium",
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[HashingAlgorithmKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Values[OrdererAddressesKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(&cb.OrdererAddresses{
			Addresses: []string{"orderer.example.com"},
		}),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Application group
	config.ChannelGroup.Groups[ApplicationGroupKey] = protoutil.NewConfigGroup()
	config.ChannelGroup.Groups[ApplicationGroupKey].Version = 0
	config.ChannelGroup.Groups[ApplicationGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[ApplicationGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	appCapabilities := make(map[string]bool)
	appCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Groups[ApplicationGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(appCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}

	// construct the config for Orderer group
	config.ChannelGroup.Groups[OrdererGroupKey] = protoutil.NewConfigGroup()
	config.ChannelGroup.Groups[OrdererGroupKey].Version = 0
	config.ChannelGroup.Groups[OrdererGroupKey].ModPolicy = AdminsPolicyKey
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[ReadersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[WritersPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Policies[AdminsPolicyKey] = &cb.ConfigPolicy{}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchSizeKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.BatchSize{
				MaxMessageCount:   65535,
				AbsoluteMaxBytes:  1024000000,
				PreferredMaxBytes: 1024000000,
			}),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[BatchTimeoutKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.BatchTimeout{
				Timeout: "2s",
			}),
		ModPolicy: AdminsPolicyKey,
	}
	ordererCapabilities := make(map[string]bool)
	ordererCapabilities["INCOMPATIBLE_CAPABILITIES"] = true
	config.ChannelGroup.Groups[OrdererGroupKey].Values[CapabilitiesKey] = &cb.ConfigValue{
		Value:     protoutil.MarshalOrPanic(CapabilitiesValue(ordererCapabilities).Value()),
		ModPolicy: AdminsPolicyKey,
	}
	config.ChannelGroup.Groups[OrdererGroupKey].Values[ConsensusTypeKey] = &cb.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&ab.ConsensusType{
				Type: "solo",
			}),
		ModPolicy: AdminsPolicyKey,
	}

	env := &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: "testChain",
					Type:      int32(cb.HeaderType_CONFIG),
				}),
			},
			Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: config,
			}),
		}),
	}
	configBlock := &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{[]byte(protoutil.MarshalOrPanic(env))},
		},
	}
	return configBlock
}

func TestValidateCapabilities(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	// Test config block with valid capabilities requirement
	cfgBlock := createCfgBlockWithSupportedCapabilities(t)
	err = ValidateCapabilities(cfgBlock, cryptoProvider)
	require.NoError(t, err)

	// Test config block with invalid capabilities requirement
	cfgBlock = createCfgBlockWithUnsupportedCapabilities(t)
	err = ValidateCapabilities(cfgBlock, cryptoProvider)
	require.EqualError(t, err, "Channel capability INCOMPATIBLE_CAPABILITIES is required but not supported")
}

func TestExtractMSPIDsForApplicationOrgs(t *testing.T) {
	// load test_configblock.json that contains the application group
	// and other properties needed to build channel config and extract MSPIDs
	blockData, err := ioutil.ReadFile("testdata/test_configblock.json")
	require.NoError(t, err)
	block := &cb.Block{}
	protolator.DeepUnmarshalJSON(bytes.NewBuffer(blockData), block)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mspids, err := ExtractMSPIDsForApplicationOrgs(block, cryptoProvider)
	require.NoError(t, err)
	require.ElementsMatch(t, mspids, []string{"Org1MSP", "Org2MSP"})
}

func TestMarshalEtcdRaftMetadata(t *testing.T) {
	md := &etcdraft.ConfigMetadata{
		Consenters: []*etcdraft.Consenter{
			{
				Host:          "node-1.example.com",
				Port:          7050,
				ClientTlsCert: []byte("testdata/tls-client-1.pem"),
				ServerTlsCert: []byte("testdata/tls-server-1.pem"),
			},
			{
				Host:          "node-2.example.com",
				Port:          7050,
				ClientTlsCert: []byte("testdata/tls-client-2.pem"),
				ServerTlsCert: []byte("testdata/tls-server-2.pem"),
			},
			{
				Host:          "node-3.example.com",
				Port:          7050,
				ClientTlsCert: []byte("testdata/tls-client-3.pem"),
				ServerTlsCert: []byte("testdata/tls-server-3.pem"),
			},
		},
	}
	packed, err := MarshalEtcdRaftMetadata(md)
	require.Nil(t, err, "marshalling should succeed")
	require.NotNil(t, packed)

	packed, err = MarshalEtcdRaftMetadata(md)
	require.Nil(t, err, "marshalling should succeed a second time because we did not mutate ourselves")
	require.NotNil(t, packed)

	unpacked := &etcdraft.ConfigMetadata{}
	require.Nil(t, proto.Unmarshal(packed, unpacked), "unmarshalling should succeed")

	var outputCerts, inputCerts [3][]byte
	for i := range unpacked.GetConsenters() {
		outputCerts[i] = []byte(unpacked.GetConsenters()[i].GetClientTlsCert())
		inputCerts[i], _ = ioutil.ReadFile(fmt.Sprintf("testdata/tls-client-%d.pem", i+1))

	}

	for i := 0; i < len(inputCerts)-1; i++ {
		require.NotEqual(t, outputCerts[i+1], outputCerts[i], "expected extracted certs to differ from each other")
	}
}
