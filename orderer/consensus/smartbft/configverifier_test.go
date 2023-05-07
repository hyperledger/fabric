/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestValidateConfig(t *testing.T) {
	configBlockEnvelope := makeConfigTx("mychannel", 1)
	configBlockEnvelopePayload := protoutil.UnmarshalPayloadOrPanic(configBlockEnvelope.Payload)
	configEnvelope := &cb.ConfigEnvelope{}
	err := proto.Unmarshal(configBlockEnvelopePayload.Data, configEnvelope)
	assert.NoError(t, err)

	lateConfigEnvelope := proto.Clone(configEnvelope).(*cb.ConfigEnvelope)
	lateConfigEnvelope.Config.Sequence--

	for _, testCase := range []struct {
		name                       string
		envelope                   *cb.Envelope
		mutateEnvelope             func(envelope *cb.Envelope)
		applyFiltersReturns        error
		proposeConfigUpdateReturns *cb.ConfigEnvelope
		proposeConfigUpdaterr      error
		expectedError              string
	}{
		{
			name:                       "green path - config block",
			envelope:                   configBlockEnvelope,
			proposeConfigUpdateReturns: configEnvelope,
			mutateEnvelope:             func(_ *cb.Envelope) {},
		},
		{
			name:     "invalid envelope",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = []byte{1, 2, 3}
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error unmarshalling Payload: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "empty header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = nil
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "no header was set",
		},
		{
			name:     "no channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{Header: &cb.Header{}})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "no channel header was set",
		},
		{
			name:     "invalid channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{Header: &cb.Header{
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
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
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
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
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
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data:   protoutil.MarshalOrPanic(&cb.ConfigEnvelope{}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "invalid config envelope",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data:   protoutil.MarshalOrPanic(&cb.ConfigEnvelope{}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "invalid config envelope",
		},
		{
			name:     "invalid payload data",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						LastUpdate: &cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: []byte{1, 2, 3},
								},
							}),
						},
						Config: &cb.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error extracting channel ID from config update",
		},
		{
			name:     "invalid inner payload",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						LastUpdate: &cb.Envelope{
							Payload: []byte{1, 2, 3},
						},
						Config: &cb.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "error unmarshalling Payload: proto: cannot parse invalid wire-format data",
		},
		{
			name:     "invalid inner payload header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						LastUpdate: &cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{}),
						},
						Config: &cb.Config{},
					}),
				})
			},
			proposeConfigUpdateReturns: configEnvelope,
			expectedError:              "inner header is nil",
		},
		{
			name:     "invalid inner payload channel header",
			envelope: configBlockEnvelope,
			mutateEnvelope: func(env *cb.Envelope) {
				env.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: configBlockEnvelopePayload.Header,
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						LastUpdate: &cb.Envelope{
							Payload: protoutil.MarshalOrPanic(&cb.Payload{
								Header: &cb.Header{},
							}),
						},
						Config: &cb.Config{},
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
			mutateEnvelope:             func(_ *cb.Envelope) {},
			applyFiltersReturns:        errors.New("unauthorized"),
			expectedError:              "unauthorized",
		},
		{
			name:                       "not up to date config update",
			envelope:                   configBlockEnvelope,
			proposeConfigUpdateReturns: lateConfigEnvelope,
			mutateEnvelope:             func(_ *cb.Envelope) {},
			expectedError:              "pending config does not match calculated expected config",
		},
		{
			name:                  "propose config update fails",
			envelope:              configBlockEnvelope,
			proposeConfigUpdaterr: errors.New("error proposing config update"),
			mutateEnvelope:        func(_ *cb.Envelope) {},
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
			env := proto.Clone(testCase.envelope).(*cb.Envelope)
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

func makeConfigTx(chainID string, i int) *cb.Envelope {
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

	return makeConfigTxFromConfigUpdateEnvelope(chainID, &cb.ConfigUpdateEnvelope{
		ConfigUpdate: protoutil.MarshalOrPanic(&cb.ConfigUpdate{
			WriteSet: channelGroup,
		}),
	})
}

func makeConfigTxFromConfigUpdateEnvelope(chainID string, configUpdateEnv *cb.ConfigUpdateEnvelope) *cb.Envelope {
	signer := &mocks.SignerSerializer{}
	signer.On("Serialize").Return([]byte{}, nil)
	signer.On("Sign", mock.Anything).Return([]byte{}, nil)

	configUpdateTx, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, chainID, signer, configUpdateEnv, 0, 0)
	if err != nil {
		panic(err)
	}
	configTx, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, chainID, signer, &cb.ConfigEnvelope{
		Config:     &cb.Config{Sequence: 1, ChannelGroup: configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate).WriteSet},
		LastUpdate: configUpdateTx,
	},
		0, 0)
	if err != nil {
		panic(err)
	}
	return configTx
}
