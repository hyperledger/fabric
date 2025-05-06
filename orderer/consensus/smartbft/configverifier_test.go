/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/proto"
)

func TestValidateConfig(t *testing.T) {
	configBlockEnvelope := makeConfigTx("mychannel")
	configBlockEnvelopePayload := protoutil.UnmarshalPayloadOrPanic(configBlockEnvelope.Payload)
	configEnvelope := &common.ConfigEnvelope{}
	err := proto.Unmarshal(configBlockEnvelopePayload.Data, configEnvelope)
	assert.NoError(t, err)

	lateConfigEnvelope := proto.Clone(configEnvelope).(*common.ConfigEnvelope)
	lateConfigEnvelope.Config.Sequence--

	for _, testCase := range []struct {
		name                       string
		envelope                   *common.Envelope
		mutateEnvelope             func(envelope *common.Envelope)
		applyFiltersReturns        error
		proposeConfigUpdateReturns *common.ConfigEnvelope
		proposeConfigUpdaterr      error
		expectedError              string
	}{
		{
			name:                       "green path - config block",
			envelope:                   configBlockEnvelope,
			proposeConfigUpdateReturns: configEnvelope,
			mutateEnvelope:             func(_ *common.Envelope) {},
		},
		{
			name:     "invalid envelope",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = []byte{1, 2, 3}
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error unmarshalling Payload: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "empty header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = nil
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "no header was set",
		},
		{
			name:     "no channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{}})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "no channel header was set",
		},
		{
			name:     "invalid channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{
					ChannelHeader: []byte{1, 2, 3},
				}})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError: "channel header unmarshalling error: error unmarshalling ChannelHeader: " +
				"proto: cannot parse invalid wire-format data",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: configBlockEnvelopePayload.Header.ChannelHeader,
					},
					Data: []byte{1, 2, 3},
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "data unmarshalling error: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data:   []byte{1, 2, 3},
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "data unmarshalling error: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data:   protoutil.MarshalOrPanic(&common.ConfigEnvelope{}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "invalid config envelope",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data:   protoutil.MarshalOrPanic(&common.ConfigEnvelope{}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "invalid config envelope",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						LastUpdate: &common.Envelope{
							Payload: protoutil.MarshalOrPanic(&common.Payload{
								Header: &common.Header{
									ChannelHeader: []byte{1, 2, 3},
								},
							}),
						},
						Config: &common.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error extracting channel ID from config update",
		},
		{
			name:     "invalid inner payload",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						LastUpdate: &common.Envelope{
							Payload: []byte{1, 2, 3},
						},
						Config: &common.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error unmarshalling Payload: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "invalid inner payload header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						LastUpdate: &common.Envelope{
							Payload: protoutil.MarshalOrPanic(&common.Payload{}),
						},
						Config: &common.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "inner header is nil",
		},
		{
			name:     "invalid inner payload channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *common.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&common.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						LastUpdate: &common.Envelope{
							Payload: protoutil.MarshalOrPanic(&common.Payload{
								Header: &common.Header{},
							}),
						},
						Config: &common.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "inner channelheader is nil",
		},
		{
			name:                       "unauthorized path",
			envelope:                   configBlockEnvelope,
			proposeConfigUpdateReturns: configEnvelope,
			mutateEnvelope:             func(_ *common.Envelope) {},
			applyFiltersReturns:        errors.New("unauthorized"),
			expectedError:              "unauthorized",
		},
		{
			name:                       "not up to date config update",
			envelope:                   configBlockEnvelope,
			proposeConfigUpdateReturns: lateConfigEnvelope,
			mutateEnvelope:             func(_ *common.Envelope) {},
			expectedError:              "pending config does not match calculated expected config",
		},
		{
			name:                  "propose config update fails",
			envelope:              configBlockEnvelope,
			proposeConfigUpdaterr: errors.New("error proposing config update"),
			mutateEnvelope:        func(_ *common.Envelope) {},
			expectedError:         "error proposing config update",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			cup := &mocks.ConfigUpdateProposer{}
			f := &mocks.Filters{}
			b := &mocks.Bundle{}
			ctv := &mocks.ConfigTxValidator{}
			cbv := &smartbft.ConfigBlockValidator{
				Logger:               flogging.MustGetLogger("test"),
				ConfigUpdateProposer: cup,
				Filters:              f,
				ValidatingChannel:    "mychannel",
			}
			env := proto.Clone(testCase.envelope).(*common.Envelope)
			testCase.mutateEnvelope(env)
			f.On("ApplyFilters", mock.Anything, env).Return(testCase.applyFiltersReturns)
			cup.On("ProposeConfigUpdate", mock.Anything, mock.Anything).
				Return(testCase.proposeConfigUpdateReturns, testCase.proposeConfigUpdaterr)
			b.On("ConfigtxValidator").Return(ctv)
			ctv.On("ProposeConfigUpdate", mock.Anything, mock.Anything).
				Return(testCase.proposeConfigUpdateReturns, testCase.proposeConfigUpdaterr)
			err = cbv.ValidateConfig(env)
			if testCase.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, testCase.expectedError, strings.ReplaceAll(err.Error(), "\u00a0", " "))
			}
		})
	}
}

func makeConfigTx(chainID string) *common.Envelope {
	gConf := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	gConf.Orderer.Capabilities = map[string]bool{
		capabilities.OrdererV2_0: true,
	}
	for _, cm := range gConf.Orderer.ConsenterMapping {
		cm.ClientTLSCert = ""
		cm.ServerTLSCert = ""
		cm.Identity = ""
	}

	channelGroup, err := encoder.NewChannelGroup(gConf)
	if err != nil {
		panic(err)
	}

	return makeConfigTxFromConfigUpdateEnvelope(chainID, &common.ConfigUpdateEnvelope{
		ConfigUpdate: protoutil.MarshalOrPanic(&common.ConfigUpdate{
			WriteSet: channelGroup,
		}),
	})
}

func makeConfigTxFromConfigUpdateEnvelope(chainID string, configUpdateEnv *common.ConfigUpdateEnvelope) *common.Envelope {
	signer := &mocks.SignerSerializer{}
	signer.On("Serialize").Return([]byte{}, nil)
	signer.On("Sign", mock.Anything).Return([]byte{}, nil)

	configUpdateTx, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, chainID, signer, configUpdateEnv, 0, 0)
	if err != nil {
		panic(err)
	}
	configTx, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG, chainID, signer, &common.ConfigEnvelope{
		Config:     &common.Config{Sequence: 1, ChannelGroup: configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate).WriteSet},
		LastUpdate: configUpdateTx,
	},
		0, 0)
	if err != nil {
		panic(err)
	}
	return configTx
}
