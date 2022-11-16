/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig(t *testing.T) {
	// Config block
	configBlockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	configBlock := &cb.Block{}
	require.NoError(t, proto.Unmarshal(configBlockBytes, configBlock))

	configBlockEnvelope := protoutil.UnmarshalEnvelopeOrPanic(configBlock.Data.Data[0])
	configBlockEnvelopePayload := protoutil.UnmarshalPayloadOrPanic(configBlockEnvelope.Payload)
	configEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(configBlockEnvelopePayload.Data, configEnvelope)
	assert.NoError(t, err)

	lateConfigEnvelope := proto.Clone(configEnvelope).(*cb.ConfigEnvelope)
	lateConfigEnvelope.Config.Sequence--

	// New channel block
	newChannelBlockBytes, err := ioutil.ReadFile("testdata/orderertxn.block")
	require.NoError(t, err)

	newChannelBlock := &cb.Block{}
	require.NoError(t, proto.Unmarshal(newChannelBlockBytes, newChannelBlock))

	newChannelBlockEnvelope := protoutil.UnmarshalEnvelopeOrPanic(newChannelBlock.Data.Data[0])
	newChannelBlockPayload := protoutil.UnmarshalPayloadOrPanic(newChannelBlockEnvelope.Payload)
	newChannelBlockYetAnotherEnvelope := protoutil.UnmarshalEnvelopeOrPanic(newChannelBlockPayload.Data)
	newChannelBlockYetAnotherPayload := protoutil.UnmarshalPayloadOrPanic(newChannelBlockYetAnotherEnvelope.Payload)
	newChannelEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(newChannelBlockYetAnotherPayload.Data, newChannelEnvelope)
	assert.NoError(t, err)

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
		{
			name:                       "green path - new channel block",
			envelope:                   newChannelBlockEnvelope,
			proposeConfigUpdateReturns: newChannelEnvelope,
			mutateEnvelope:             func(_ *cb.Envelope) {},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			cup := &mocks.ConfigUpdateProposer{}
			f := &mocks.Filters{}
			b := &mocks.Bundle{}
			cct := &mocks.ChannelConfigTemplator{}
			ctv := &mocks.ConfigTxValidator{}
			cbv := &smartbft.ConfigBlockValidator{
				Logger:                 flogging.MustGetLogger("test"),
				ConfigUpdateProposer:   cup,
				Filters:                f,
				ChannelConfigTemplator: cct,
				ValidatingChannel:      "mychannel",
			}
			env := proto.Clone(testCase.envelope).(*cb.Envelope)
			testCase.mutateEnvelope(env)
			f.On("ApplyFilters", mock.Anything, env).Return(testCase.applyFiltersReturns)
			cup.On("ProposeConfigUpdate", mock.Anything, mock.Anything).
				Return(testCase.proposeConfigUpdateReturns, testCase.proposeConfigUpdaterr)
			cct.On("NewChannelConfig", mock.Anything).Return(b, nil)
			b.On("ConfigtxValidator").Return(ctv)
			ctv.On("ProposeConfigUpdate", mock.Anything, mock.Anything).
				Return(testCase.proposeConfigUpdateReturns, testCase.proposeConfigUpdaterr)
			err = cbv.ValidateConfig(env)
			if testCase.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, testCase.expectedError)
			}
		})
	}
}
