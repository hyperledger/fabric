/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"testing"

	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/configtxlator/update"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaintenanceNoConfig(t *testing.T) {
	ms := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "solo",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL,
		},
	}
	mf := NewMaintenanceFilter(ms)
	require.NotNil(t, mf)
	ms.OrdererConfigVal = nil
	assert.Panics(t, func() { _ = mf.Apply(&common.Envelope{}) }, "No orderer config")
}

func TestMaintenanceDisabled(t *testing.T) {
	msInactive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: false},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL,
		},
	}
	mf := NewMaintenanceFilter(msInactive)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}

	t.Run("Good", func(t *testing.T) {
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, current, 3)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Block entry to maintenance", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: next config attempted to change ConsensusType.State to STATE_MAINTENANCE, but capability is disabled")
	})

	t.Run("Block type change", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "solo", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: next config attempted to change ConsensusType.Type from kafka to solo, but capability is disabled")
	})
}

func TestMaintenanceParseEvelope(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL,
		},
	}
	mf := NewMaintenanceFilter(msActive)
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
			errMsg:   "envelope unmarshalling failed: error unmarshaling Payload",
		},
		{
			name: "Bad ChannelHeader",
			envelope: &common.Envelope{
				Payload: utils.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: badBytes,
					},
				}),
			},
			errMsg: "envelope unmarshalling failed: error unmarshaling ChannelHeader",
		},
		{
			name: "Bad ChannelHeader Type",
			envelope: &common.Envelope{
				Payload: utils.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
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
				Payload: utils.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChain",
							Type:      int32(common.HeaderType_CONFIG),
						}),
					},
					Data: badBytes,
				}),
			},
			errMsg: "envelope unmarshalling failed: error unmarshaling message for type CONFIG",
		},
	}

	for _, tCase := range testCases {
		t.Run(tCase.name, func(t *testing.T) {
			err := mf.Apply(tCase.envelope)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tCase.errMsg)
		})
	}
}

func TestMaintenanceInspectEntry(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL,
		},
	}
	mf := NewMaintenanceFilter(msActive)
	require.NotNil(t, mf)
	bogusMetadata := []byte{1, 2, 3, 4}
	current := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}

	t.Run("Good", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from kafka to etcdraft, but ConsensusType.State is changing from STATE_NORMAL to STATE_MAINTENANCE")
	})

	t.Run("Bad: change consensus type not in maintenance", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: attempted to change consensus type from kafka to etcdraft, but current config ConsensusType.State is not in maintenance mode")
	})
}

func TestMaintenanceInspectChange(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE,
		},
	}
	mf := NewMaintenanceFilter(msActive)
	require.NotNil(t, mf)
	bogusMetadata := []byte{1, 2, 3, 4}
	validMetadata := utils.MarshalOrPanic(&etcdraft.ConfigMetadata{})
	current := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_MAINTENANCE}

	t.Run("Good type change", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Good exit, no change", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Bad: unsupported consensus type", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "unsupported", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: attempted to change consensus type from kafka to unsupported, transition not supported")
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from kafka to etcdraft, but ConsensusType.State is changing from STATE_MAINTENANCE to STATE_NORMAL")
	})

	t.Run("Bad: etcdraft metadata", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: bogusMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			"config transaction inspection failed: failed to unmarshal etcdraft metadata configuration")
	})
}

func TestMaintenanceInspectExit(t *testing.T) {
	validMetadata := utils.MarshalOrPanic(&etcdraft.ConfigMetadata{})
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "etcdraft",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE,
			ConsensusMetadataVal:  validMetadata,
		},
	}
	mf := NewMaintenanceFilter(msActive)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}

	t.Run("Good exit", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Bad: concurrent change to consensus type & state", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "kafka", metadata: []byte{}, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelope(t, current, next)
		err := mf.Apply(configTx)
		assert.EqualError(t, err,
			"config transaction inspection failed: attempted to change ConsensusType.Type from etcdraft to kafka, but ConsensusType.State is changing from STATE_MAINTENANCE to STATE_NORMAL")
	})

	t.Run("Bad: exit with extra group", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 1)
		err := mf.Apply(configTx)
		assert.EqualError(t, err, "config transaction inspection failed: config update contains changes to more than one group")
	})

	t.Run("Bad: exit with extra value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 2)
		err := mf.Apply(configTx)
		assert.EqualError(t, err, "config transaction inspection failed: config update contains changes to values in group Channel")
	})

	t.Run("Bad: exit with extra orderer value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_NORMAL}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 3)
		err := mf.Apply(configTx)
		assert.EqualError(t, err, "config transaction inspection failed: config update contain more then just the ConsensusType value in the Orderer group")
	})
}

func TestMaintenanceExtra(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE,
		},
	}
	mf := NewMaintenanceFilter(msActive)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "kafka", metadata: nil, state: orderer.ConsensusType_STATE_MAINTENANCE}
	validMetadata := utils.MarshalOrPanic(&etcdraft.ConfigMetadata{})

	t.Run("Good: with extra group", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 1)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Good: with extra value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 2)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})

	t.Run("Good: with extra orderer value", func(t *testing.T) {
		next := consensusTypeInfo{ordererType: "etcdraft", metadata: validMetadata, state: orderer.ConsensusType_STATE_MAINTENANCE}
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, next, 3)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	})
}

func TestMaintenanceMissingConsensusType(t *testing.T) {
	msActive := &mockSystemChannelFilterSupport{
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			ConsensusTypeVal:      "kafka",
			ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE,
		},
	}
	mf := NewMaintenanceFilter(msActive)
	require.NotNil(t, mf)
	current := consensusTypeInfo{ordererType: "kafka", metadata: nil, state: orderer.ConsensusType_STATE_MAINTENANCE}
	for i := 1; i < 4; i++ {
		configTx := makeConfigEnvelopeWithExtraStuff(t, current, current, i)
		err := mf.Apply(configTx)
		assert.NoError(t, err)
	}
}

type consensusTypeInfo struct {
	ordererType string
	metadata    []byte
	state       orderer.ConsensusType_State
}

func makeConfigEnvelope(t *testing.T, current, next consensusTypeInfo) *common.Envelope {
	original := makeBaseConfig(t)
	updated := makeBaseConfig(t)

	original.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: utils.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     current.ordererType,
				Metadata: current.metadata,
				State:    current.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: utils.MarshalOrPanic(
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

func makeConfigEnvelopeWithExtraStuff(t *testing.T, current, next consensusTypeInfo, extra int) *common.Envelope {
	original := makeBaseConfig(t)
	updated := makeBaseConfig(t)

	original.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: utils.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     current.ordererType,
				Metadata: current.metadata,
				State:    current.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey] = &common.ConfigValue{
		Value: utils.MarshalOrPanic(
			&orderer.ConsensusType{
				Type:     next.ordererType,
				Metadata: next.metadata,
				State:    next.state,
			}),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	switch extra {
	case 1:
		updated.ChannelGroup.Groups[channelconfig.ConsortiumsGroupKey] = &common.ConfigGroup{}
	case 2:
		updated.ChannelGroup.Values[channelconfig.ConsortiumKey] = &common.ConfigValue{
			Value:     utils.MarshalOrPanic(&common.Consortium{}),
			ModPolicy: channelconfig.AdminsPolicyKey,
			Version:   1,
		}
	case 3:
		updated.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.BatchSizeKey] = &common.ConfigValue{
			Value: utils.MarshalOrPanic(
				&orderer.BatchSize{
					AbsoluteMaxBytes:  10241024,
					MaxMessageCount:   1024,
					PreferredMaxBytes: 10241024,
				}),
			ModPolicy: channelconfig.AdminsPolicyKey,
		}
	default:
		return nil
	}

	configTx := makeConfigTx(original, updated, t)

	return configTx
}

func makeConfigTx(original, updated *common.Config, t *testing.T) *common.Envelope {
	configUpdate, err := update.Compute(original, updated)
	require.NoError(t, err)
	configUpdateEnv := &common.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configUpdate),
	}
	configUpdateTx, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, testChannelID, mockCrypto(), configUpdateEnv, 0, 0)
	require.NoError(t, err)
	configTx, err := utils.CreateSignedEnvelope(
		common.HeaderType_CONFIG,
		testChannelID,
		mockCrypto(),
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
	gConf := configtxgentest.Load(localconfig.SampleInsecureSoloProfile)
	gConf.Orderer.Capabilities = map[string]bool{
		capabilities.OrdererV1_4_2: true,
	}
	gConf.Orderer.OrdererType = "kafka"
	channelGroup, err := encoder.NewChannelGroup(gConf)
	require.NoError(t, err)
	original := &common.Config{
		ChannelGroup: channelGroup,
	}
	return original
}
