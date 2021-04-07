/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/metadata_validator.go --fake-name MetadataValidator . metadataValidator

type metadataValidator interface {
	MetadataValidator
}

//go:generate counterfeiter -o mocks/configtx_validator.go --fake-name ConfigTXValidator . configtxValidator

type configtxValidator interface {
	configtx.Validator
}

//go:generate counterfeiter -o mocks/channel_config.go --fake-name ChannelConfig . channelConfig

type channelConfig interface {
	channelconfig.Channel
}

//go:generate counterfeiter -o mocks/channel_capabilities.go --fake-name ChannelCapabilities . channelCapabilities

type channelCapabilities interface {
	channelconfig.ChannelCapabilities
}

var validConfig *cb.Config

func init() {
	cg, err := encoder.NewChannelGroup(genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir()))
	if err != nil {
		panic(err)
	}
	validConfig = &cb.Config{
		Sequence:     1,
		ChannelGroup: cg,
	}
}

func mockCrypto() *mocks.SignerSerializer {
	return &mocks.SignerSerializer{}
}

func makeConfigTxFromConfigUpdateTx(configUpdateTx *cb.Envelope) *cb.Envelope {
	confUpdate := configtx.UnmarshalConfigUpdateOrPanic(
		configtx.UnmarshalConfigUpdateEnvelopeOrPanic(
			protoutil.UnmarshalPayloadOrPanic(configUpdateTx.Payload).Data,
		).ConfigUpdate,
	)
	res, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, confUpdate.ChannelId, nil, &cb.ConfigEnvelope{
		Config:     validConfig,
		LastUpdate: configUpdateTx,
	}, 0, 0)
	if err != nil {
		panic(err)
	}
	return res
}

func wrapConfigTx(env *cb.Envelope) *cb.Envelope {
	result, err := protoutil.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, "foo", mockCrypto(), env, msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	return result
}

type mockSupport struct {
	msc *mocks.OrdererConfig
}

func newMockSupport() *mockSupport {
	mockOrdererConfig := &mocks.OrdererConfig{}
	mockOrdererConfig.ConsensusMetadataReturns([]byte("old consensus metadata"))
	return &mockSupport{
		msc: mockOrdererConfig,
	}
}

func (ms *mockSupport) OrdererConfig() (channelconfig.Orderer, bool) {
	return ms.msc, true
}

type mockChainCreator struct {
	ms                  *mockSupport
	newChains           []*cb.Envelope
	NewChannelConfigErr error
}

func newMockChainCreator() *mockChainCreator {
	mcc := &mockChainCreator{
		ms: newMockSupport(),
	}
	return mcc
}

func (mcc *mockChainCreator) ChannelsCount() int {
	return len(mcc.newChains)
}

func (mcc *mockChainCreator) CreateBundle(channelID string, config *cb.Config) (channelconfig.Resources, error) {
	mockResources := &mocks.Resources{}

	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ChannelIDReturns(channelID)
	mockResources.ConfigtxValidatorReturns(mockValidator)

	mockOrdererConfig := &mocks.OrdererConfig{}
	mockOrdererConfig.ConsensusMetadataReturns([]byte("new consensus metadata"))
	mockOrdererConfig.CapabilitiesReturns(&mocks.OrdererCapabilities{})
	mockResources.OrdererConfigReturns(mockOrdererConfig, true)

	mockChannelConfig := &mocks.ChannelConfig{}
	mockChannelConfig.CapabilitiesReturns(&mocks.ChannelCapabilities{})
	mockResources.ChannelConfigReturns(mockChannelConfig)

	return mockResources, nil
}

func (mcc *mockChainCreator) NewChannelConfig(envConfigUpdate *cb.Envelope) (channelconfig.Resources, error) {
	if mcc.NewChannelConfigErr != nil {
		return nil, mcc.NewChannelConfigErr
	}

	mockResources := &mocks.Resources{}
	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ProposeConfigUpdateReturns(
		&cb.ConfigEnvelope{
			Config:     validConfig,
			LastUpdate: envConfigUpdate,
		},
		nil,
	)
	mockResources.ConfigtxValidatorReturns(mockValidator)

	return mockResources, nil
}

func TestGoodProposal(t *testing.T) {
	newChainID := "new-chain-id"

	mcc := newMockChainCreator()
	mv := &mocks.MetadataValidator{}

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
	require.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	require.NoError(t, NewSystemChannelFilter(mcc.ms, mcc, mv).Apply(wrapped), "Did not accept valid transaction")
}

func TestProposalRejectedByConfig(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.NewChannelConfigErr = fmt.Errorf("desired err text")
	mv := &mocks.MetadataValidator{}

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
	require.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc, mv).Apply(wrapped)

	require.NotNil(t, err, "Did not accept valid transaction")
	require.Regexp(t, mcc.NewChannelConfigErr.Error(), err)
	require.Len(t, mcc.newChains, 0, "Proposal should not have created a new chain")
}

func TestNumChainsExceeded(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.MaxChannelsCountReturns(1)
	mcc.newChains = make([]*cb.Envelope, 2)
	mv := &mocks.MetadataValidator{}

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
	require.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc, mv).Apply(wrapped)

	require.NotNil(t, err, "Transaction had created too many channels")
	require.Regexp(t, "exceed maximimum number", err)
}

func TestMaintenanceMode(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ConsensusStateReturns(orderer.ConsensusType_STATE_MAINTENANCE)
	mv := &mocks.MetadataValidator{}

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
	require.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc, mv).Apply(wrapped)
	require.EqualError(t, err, "channel creation is not permitted: maintenance mode")
}

func TestBadProposal(t *testing.T) {
	mcc := newMockChainCreator()
	mv := &mocks.MetadataValidator{}
	sysFilter := NewSystemChannelFilter(mcc.ms, mcc, mv)
	t.Run("BadPayload", func(t *testing.T) {
		err := sysFilter.Apply(&cb.Envelope{Payload: []byte("garbage payload")})
		require.Regexp(t, "bad payload", err)
	})

	for _, tc := range []struct {
		name    string
		payload *cb.Payload
		regexp  string
	}{
		{
			"MissingPayloadHeader",
			&cb.Payload{},
			"missing payload header",
		},
		{
			"BadChannelHeader",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: []byte("bad channel header"),
				},
			},
			"bad channel header",
		},
		{
			"BadConfigTx",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: []byte("bad configTx"),
			},
			"payload data error unmarshalling to envelope",
		},
		{
			"BadConfigTxPayload",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: []byte("bad payload"),
					},
				),
			},
			"error unmarshalling wrapped configtx envelope payload",
		},
		{
			"MissingConfigTxChannelHeader",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(
							&cb.Payload{},
						),
					},
				),
			},
			"wrapped configtx envelope missing header",
		},
		{
			"BadConfigTxChannelHeader",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: []byte("bad channel header"),
								},
							},
						),
					},
				),
			},
			"error unmarshalling wrapped configtx envelope channel header",
		},
		{
			"BadConfigTxChannelHeaderType",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: protoutil.MarshalOrPanic(
										&cb.ChannelHeader{
											Type: 0xBad,
										},
									),
								},
							},
						),
					},
				),
			},
			"wrapped configtx envelope not a config transaction",
		},
		{
			"BadConfigEnvelope",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: protoutil.MarshalOrPanic(
										&cb.ChannelHeader{
											Type: int32(cb.HeaderType_CONFIG),
										},
									),
								},
								Data: []byte("bad config update"),
							},
						),
					},
				),
			},
			"error unmarshalling wrapped configtx config envelope from payload",
		},
		{
			"MissingConfigEnvelopeLastUpdate",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: protoutil.MarshalOrPanic(
					&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: protoutil.MarshalOrPanic(
										&cb.ChannelHeader{
											Type: int32(cb.HeaderType_CONFIG),
										},
									),
								},
								Data: protoutil.MarshalOrPanic(
									&cb.ConfigEnvelope{},
								),
							},
						),
					},
				),
			},
			"updated config does not include a config update",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := sysFilter.Apply(&cb.Envelope{Payload: protoutil.MarshalOrPanic(tc.payload)})
			require.NotNil(t, err)
			require.Regexp(t, tc.regexp, err.Error())
		})
	}
}

func TestFailedMetadataValidation(t *testing.T) {
	newChainID := "NewChainID"

	errorString := "bananas"
	mv := &mocks.MetadataValidator{}
	mv.ValidateConsensusMetadataReturns(errors.New(errorString))
	mcc := newMockChainCreator()

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
	require.NoError(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc, mv).Apply(wrapped)

	// validate that the filter application returns error
	require.EqualError(t, err, "consensus metadata update for channel creation is invalid: bananas", "Transaction metadata validation fails")

	// validate arguments to ValidateConsensusMetadata
	require.Equal(t, 1, mv.ValidateConsensusMetadataCallCount())
	om, nm, nc := mv.ValidateConsensusMetadataArgsForCall(0)
	require.True(t, nc)
	require.Equal(t, []byte("old consensus metadata"), om.ConsensusMetadata())
	require.Equal(t, []byte("new consensus metadata"), nm.ConsensusMetadata())
}
