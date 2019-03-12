/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

var validConfig *cb.Config

func init() {
	cg, err := encoder.NewChannelGroup(configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile))
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
	msc *mockconfig.Orderer
}

func newMockSupport() *mockSupport {
	return &mockSupport{
		msc: &mockconfig.Orderer{},
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
	return &mockconfig.Resources{
		ConfigtxValidatorVal: &mockconfigtx.Validator{
			ChainIDVal: channelID,
		},
		OrdererConfigVal: &mockconfig.Orderer{
			CapabilitiesVal: &mockconfig.OrdererCapabilities{},
		},
		ChannelConfigVal: &mockconfig.Channel{
			CapabilitiesVal: &mockconfig.ChannelCapabilities{},
		},
	}, nil
}

func (mcc *mockChainCreator) NewChannelConfig(envConfigUpdate *cb.Envelope) (channelconfig.Resources, error) {
	if mcc.NewChannelConfigErr != nil {
		return nil, mcc.NewChannelConfigErr
	}
	return &mockconfig.Resources{
		ConfigtxValidatorVal: &mockconfigtx.Validator{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{
				Config:     validConfig,
				LastUpdate: envConfigUpdate,
			},
		},
	}, nil
}

func TestGoodProposal(t *testing.T) {
	newChainID := "new-chain-id"

	mcc := newMockChainCreator()

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
	assert.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	assert.NoError(t, NewSystemChannelFilter(mcc.ms, mcc).Apply(wrapped), "Did not accept valid transaction")
}

func TestProposalRejectedByConfig(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.NewChannelConfigErr = fmt.Errorf("desired err text")

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
	assert.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc).Apply(wrapped)

	assert.NotNil(t, err, "Did not accept valid transaction")
	assert.Regexp(t, mcc.NewChannelConfigErr.Error(), err)
	assert.Len(t, mcc.newChains, 0, "Proposal should not have created a new chain")
}

func TestNumChainsExceeded(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.MaxChannelsCountVal = 1
	mcc.newChains = make([]*cb.Envelope, 2)

	configUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, nil, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
	assert.Nil(t, err, "Error constructing configtx")
	ingressTx := makeConfigTxFromConfigUpdateTx(configUpdate)

	wrapped := wrapConfigTx(ingressTx)

	err = NewSystemChannelFilter(mcc.ms, mcc).Apply(wrapped)

	assert.NotNil(t, err, "Transaction had created too many channels")
	assert.Regexp(t, "exceed maximimum number", err)
}

func TestBadProposal(t *testing.T) {
	mcc := newMockChainCreator()
	sysFilter := NewSystemChannelFilter(mcc.ms, mcc)
	t.Run("BadPayload", func(t *testing.T) {
		err := sysFilter.Apply(&cb.Envelope{Payload: []byte("garbage payload")})
		assert.Regexp(t, "bad payload", err)
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
			"payload data error unmarshaling to envelope",
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
			"error unmarshaling wrapped configtx envelope payload",
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
			"error unmarshaling wrapped configtx envelope channel header",
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
			assert.NotNil(t, err)
			assert.Regexp(t, tc.regexp, err.Error())
		})
	}
}
