/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/smartbft"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func newMockOrdererConfig(migration bool, state orderer.ConsensusType_State) *mocks.OrdererConfig {
	mockOrderer := &mocks.OrdererConfig{}
	mockCapabilities := &mocks.OrdererCapabilities{}
	mockCapabilities.ConsensusTypeMigrationReturns(migration)
	mockOrderer.CapabilitiesReturns(mockCapabilities)
	mockOrderer.ConsensusTypeReturns("etcdraft")
	mockOrderer.ConsensusStateReturns(state)
	mockOrderer.ConsensusMetadataReturns(protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{
		Consenters: []*etcdraft.Consenter{
			{Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}},
			{Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}},
		},
	}))
	return mockOrderer
}

func TestMaintenanceNoConfig(t *testing.T) {
	ms := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mocks.OrdererConfig{},
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(ms, cryptoProvider)
	require.NotNil(t, mf)
	ms.OrdererConfigVal = nil
	require.Panics(t, func() { _ = mf.Apply(&common.Envelope{}) }, "No orderer config")
}

func TestMaintenanceDisabled(t *testing.T) {
	msInactive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(false, orderer.ConsensusType_STATE_NORMAL),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msInactive, cryptoProvider)
	require.NotNil(t, mf)
	raftMetadata := protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{Consenters: []*etcdraft.Consenter{{Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}})
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: raftMetadata, state: orderer.ConsensusType_STATE_NORMAL}

	t.Run("Good", func(t *testing.T) {
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, current, 3, 0)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Block entry to maintenance", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: next config attempted to change ConsensusType.State to STATE_MAINTENANCE, but capability is disabled")
	})

	t.Run("Block type change", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: next config attempted to change ConsensusType.Type from etcdraft to BFT, but capability is disabled")
	})
}

func TestMaintenanceParseEnvelope(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	badBytes := []byte{1, 2, 3, 4}

	type testCase struct {
		name     string
		envelope *common.Envelope
		errMsg   string
	}

	testCases := []testCase{
		{
			name:     "Empty Envelope",
			envelope: &common.Envelope{},
			errMsg:   "envelope unmarshalling failed: envelope must have a Header",
		},
		{
			name:     "Bad payload",
			envelope: &common.Envelope{Payload: badBytes},
			errMsg:   "envelope unmarshalling failed: error unmarshalling Payload",
		},
		{
			name: "Bad ChannelHeader",
			envelope: &common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: badBytes,
					},
				}),
			},
			errMsg: "envelope unmarshalling failed: error unmarshalling ChannelHeader",
		},
		{
			name: "Bad ChannelHeader Type",
			envelope: &common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChain",
							Type:      int32(common.HeaderType_ORDERER_TRANSACTION), // Expect CONFIG
						}),
					},
				}),
			},
			errMsg: "envelope unmarshalling failed: invalid type",
		},
		{
			name: "Bad Data",
			envelope: &common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChain",
							Type:      int32(common.HeaderType_CONFIG),
						}),
					},
					Data: badBytes,
				}),
			},
			errMsg: "envelope unmarshalling failed: error unmarshalling message for type CONFIG",
		},
	}

	for _, tCase := range testCases {
		t.Run(tCase.name, func(t *testing.T) {
			err := mf.Apply(tCase.envelope)
			require.Error(t, err)
			require.Contains(t, err.Error(), tCase.errMsg)
		})
	}
}

func TestMaintenanceInspectEntry(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	bogusMetadata := []byte{1, 2, 3, 4}
	validRaftMetadata := protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{Consenters: []*etcdraft.Consenter{{Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}})
	bftMetadata := protoutil.MarshalOrPanic(createValidBFTMetadata())
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: validRaftMetadata, state: orderer.ConsensusType_STATE_NORMAL}

	t.Run("Good", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validRaftMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from etcdraft to BFT, but ConsensusType.State is changing from STATE_NORMAL to STATE_MAINTENANCE")
	})

	t.Run("Bad: concurrent change to metadata & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: bftMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Metadata, but ConsensusType.State is changing from STATE_NORMAL to STATE_MAINTENANCE")
	})

	t.Run("Bad: concurrent change to state & orderer value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validRaftMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 1)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: config update contains changes to groups within the Orderer group")
	})

	t.Run("Bad: change consensus type not in maintenance", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change consensus type from etcdraft to BFT, but current config ConsensusType.State is not in maintenance mode")
	})
}

func TestMaintenanceInspectChange(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	bogusMetadata := []byte{1, 2, 3, 4}
	validBFTMetadata := protoutil.MarshalOrPanic(createValidBFTMetadata())
	raftMetadata := protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{Consenters: []*etcdraft.Consenter{{Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}})
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: raftMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}

	t.Run("Good type change with valid BFT metadata and suitable consenter mapping", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validBFTMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Good exit, no change", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: raftMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Bad: good type change with invalid BFT metadata", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.Error(t, err)
		require.EqualError(t, err,
			"config transaction inspection failed: invalid BFT metadata configuration")
	})

	t.Run("Bad: BFT metadata cannot be unmarshalled", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.Error(t, err)
		require.Contains(t, err.Error(),
			"config transaction inspection failed: failed to unmarshal BFT metadata configuration")
	})

	t.Run("Bad: good type change with valid BFT metadata but missing consenters mapping", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validBFTMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.Error(t, err)
		require.EqualError(t, err,
			"config transaction inspection failed: invalid BFT consenter mapping configuration: Invalid new config: bft consenters are missing")
	})

	t.Run("Bad: good type change with valid BFT metadata but corrupt consenters mapping", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validBFTMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 2)
		err := mf.Apply(configTx)
		require.Error(t, err)

		t1 := strings.ReplaceAll(err.Error(), " ", "")
		t2 := strings.ReplaceAll("config transaction inspection failed: invalid BFT consenter mapping configuration: No suitable BFT consenter for Raft consenter: host:\"127.0.0.1\" port:4000 client_tls_cert:\"\\x01\\x02\\x03\" server_tls_cert:\"\\x04\\x05\\x06\"", " ", "")
		require.Equal(t, t1, t2)
	})

	t.Run("Bad: good type change with valid BFT metadata but missing consenters", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validBFTMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 3)
		err := mf.Apply(configTx)
		require.Error(t, err)
		require.EqualError(t, err,
			"config transaction inspection failed: invalid BFT consenter mapping configuration: Invalid new config: the number of bft consenters: 1 is not equal to the number of raft consenters: 2")
	})

	t.Run("Bad: unsupported consensus type", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "unsupported", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change consensus type from etcdraft to unsupported, transition not supported")
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validBFTMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from etcdraft to BFT, but ConsensusType.State is changing from STATE_MAINTENANCE to STATE_NORMAL")
	})
}

func TestMaintenanceInspectExit(t *testing.T) {
	validMetadata := protoutil.MarshalOrPanic(createValidBFTMetadata())
	mockOrderer := newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE)
	mockOrderer.ConsensusTypeReturns("BFT")
	mockOrderer.ConsensusMetadataReturns(validMetadata)

	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: mockOrderer,
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}

	t.Run("Good exit", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from BFT to etcdraft, but ConsensusType.State is changing from STATE_MAINTENANCE to STATE_NORMAL")
	})

	t.Run("Bad: change consensus type from BFT to Raft", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.EqualError(t, err,
			"config transaction inspection failed: attempted to change consensus type from BFT to etcdraft, transition not supported")
	})

	t.Run("Bad: exit with extra group", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 1, 1)
		err := mf.Apply(configTx)
		require.EqualError(t, err, "config transaction inspection failed: config update contains changes to more than one group")
	})

	t.Run("Bad: exit with extra value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 2, 1)
		err := mf.Apply(configTx)
		require.EqualError(t, err, "config transaction inspection failed: config update contains changes to values in group Channel")
	})

	t.Run("Bad: exit with extra orderer value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 3, 0)
		err := mf.Apply(configTx)
		require.EqualError(t, err, "config transaction inspection failed: config update contain more then just the ConsensusType value in the Orderer group")
	})

	t.Run("Bad: exit with extra orderer value required for BFT", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 1)
		err := mf.Apply(configTx)
		require.EqualError(t, err, "config transaction inspection failed: config update contains changes to groups within the Orderer group")
	})
}

func TestMaintenanceExtra(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	raftMetadata := protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{Consenters: []*etcdraft.Consenter{{Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}})
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: raftMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
	validMetadata := protoutil.MarshalOrPanic(createValidBFTMetadata())

	t.Run("Good: with extra group", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 1, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Good: with extra value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 2, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Good: with extra orderer value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 3, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})

	t.Run("Good: with extra orderer value required for BFT", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "BFT", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 0, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	})
}

func TestMaintenanceMissingConsensusType(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE),
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mf := NewMaintenanceFilter(msActive, cryptoProvider)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: nil, state: orderer.ConsensusType_STATE_MAINTENANCE}
	for i := 1; i < 4; i++ {
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, current, i, 1)
		err := mf.Apply(configTx)
		require.NoError(t, err)
	}
}

type consensusTypeInfo struct {
	ordererType string
	metadata    []byte
	state       orderer.ConsensusType_State
}

func makeConfigEnvelope(t *testing.T, current, next consensusTypeInfo) *common.Envelope {
	original := makeBaseConfig(t)
	updated := proto.Clone(original).(*common.Config)

	original.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     current.ordererType,
				Metadata: current.metadata,
				State:    current.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     next.ordererType,
				Metadata: next.metadata,
				State:    next.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	configTx := makeConfigTx(original, updated, t)

	return configTx
}

func makeConfigEnvelopeWithExtraStuff(t *testing.T, current, next consensusTypeInfo, extra int, addBFTConsenterMapping int) *common.Envelope {
	original := makeBaseConfig(t)
	updated := proto.Clone(original).(*common.Config)

	original.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     current.ordererType,
				Metadata: current.metadata,
				State:    current.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: protoutil.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     next.ordererType,
				Metadata: next.metadata,
				State:    next.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	switch extra {
	case 1:
		updated.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Policies = nil
	case 2:
		updated.ChannelGroup.Values[channelconfig.ConsortiumKey] = &common.ConfigValue{
			Value:     protoutil.MarshalOrPanic(&common.Consortium{}),
			ModPolicy: channelconfig.AdminsPolicyKey,
			Version:   1,
		}
	case 3:
		updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.BatchSizeKey] = &common.ConfigValue{
			Value: protoutil.MarshalOrPanic(
				&orderer.BatchSize{
					AbsoluteMaxBytes:  10241024,
					MaxMessageCount:   1024,
					PreferredMaxBytes: 10241024,
				}),
			ModPolicy: channelconfig.AdminsPolicyKey,
		}
	}

	switch addBFTConsenterMapping {
	case 1:
		updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey] = &common.ConfigValue{
			Value:     protoutil.MarshalOrPanic(&common.Orderers{ConsenterMapping: []*common.Consenter{{Id: 1, Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Id: 2, Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}}),
			ModPolicy: channelconfig.AdminsPolicyKey,
		}
	case 2:
		updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey] = &common.ConfigValue{
			Value:     protoutil.MarshalOrPanic(&common.Orderers{ConsenterMapping: []*common.Consenter{{Id: 1, Host: "127.0.0.1", Port: 4005, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}, {Id: 2, Host: "127.0.0.1", Port: 4001, ClientTlsCert: []byte{7, 8, 9}, ServerTlsCert: []byte{1, 2, 3}}}}),
			ModPolicy: channelconfig.AdminsPolicyKey,
		}
	case 3:
		updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey] = &common.ConfigValue{
			Value:     protoutil.MarshalOrPanic(&common.Orderers{ConsenterMapping: []*common.Consenter{{Id: 1, Host: "127.0.0.1", Port: 4000, ClientTlsCert: []byte{1, 2, 3}, ServerTlsCert: []byte{4, 5, 6}}}}),
			ModPolicy: channelconfig.AdminsPolicyKey,
		}
	}

	configTx := makeConfigTx(original, updated, t)

	return configTx
}

func makeConfigTx(original, updated *common.Config, t *testing.T) *common.Envelope {
	configUpdate, err := update.Compute(original, updated)
	require.NoError(t, err)
	configUpdateEnv := &common.ConfigUpdateEnvelope{
		ConfigUpdate: protoutil.MarshalOrPanic(configUpdate),
	}
	configUpdateTx, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, testChannelID, &mocks.SignerSerializer{}, configUpdateEnv, 0, 0)
	require.NoError(t, err)
	configTx, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG,
		testChannelID,
		&mocks.SignerSerializer{},
		&common.ConfigEnvelope{
			Config:     updated,
			LastUpdate: configUpdateTx,
		},
		0,
		0)
	require.NoError(t, err)
	return configTx
}

func makeBaseConfig(t *testing.T) *common.Config {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)

	gConf := genesisconfig.Load(genesisconfig.SampleAppChannelEtcdRaftProfile, configtest.GetDevConfigDir())
	generateCertificates(t, gConf, tlsCA, certDir)

	gConf.Orderer.Capabilities = map[string]bool{
		capabilities.ChannelV3_0: true,
	}

	gConf.Orderer.OrdererType = "etcdraft"
	channelGroup, err := encoder.NewChannelGroup(gConf)
	require.NoError(t, err)
	original := &common.Config{
		ChannelGroup: channelGroup,
	}
	return original
}

func generateCertificates(t *testing.T, confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		require.NoError(t, err)

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}
}

func createValidBFTMetadata() *smartbft.Options {
	bftMetadata := &smartbft.Options{
		RequestBatchMaxCount:      types.DefaultConfig.RequestBatchMaxCount,
		RequestBatchMaxBytes:      types.DefaultConfig.RequestBatchMaxBytes,
		RequestBatchMaxInterval:   types.DefaultConfig.RequestBatchMaxInterval.String(),
		IncomingMessageBufferSize: types.DefaultConfig.IncomingMessageBufferSize,
		RequestPoolSize:           types.DefaultConfig.RequestPoolSize,
		RequestForwardTimeout:     types.DefaultConfig.RequestForwardTimeout.String(),
		RequestComplainTimeout:    types.DefaultConfig.RequestComplainTimeout.String(),
		RequestAutoRemoveTimeout:  types.DefaultConfig.RequestAutoRemoveTimeout.String(),
		ViewChangeResendInterval:  types.DefaultConfig.ViewChangeResendInterval.String(),
		ViewChangeTimeout:         types.DefaultConfig.ViewChangeTimeout.String(),
		LeaderHeartbeatTimeout:    types.DefaultConfig.LeaderHeartbeatTimeout.String(),
		LeaderHeartbeatCount:      types.DefaultConfig.LeaderHeartbeatCount,
		CollectTimeout:            types.DefaultConfig.CollectTimeout.String(),
		SyncOnStart:               types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:         types.DefaultConfig.SpeedUpViewChange,
	}
	return bftMetadata
}
