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
	"github.com/hyperledger/fabric/common/crypto"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockcrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

func mockCrypto() crypto.LocalSigner {
	return mockcrypto.FakeLocalSigner
}

func makeConfigTxFromConfigUpdateEnvelope(chainID string, configUpdateEnv *cb.ConfigUpdateEnvelope) *cb.Envelope {
	configUpdateTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, chainID, mockCrypto(), configUpdateEnv, msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	configTx, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, chainID, mockCrypto(), &cb.ConfigEnvelope{
		Config:     &cb.Config{Sequence: 1, ChannelGroup: configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate).WriteSet},
		LastUpdate: configUpdateTx},
		msgVersion, epoch)
	if err != nil {
		panic(err)
	}
	return configTx
}

func wrapConfigTx(env *cb.Envelope) *cb.Envelope {
	result, err := utils.CreateSignedEnvelope(cb.HeaderType_ORDERER_TRANSACTION, "foo", mockCrypto(), env, msgVersion, epoch)
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

func (mcc *mockChainCreator) newChain(configTx *cb.Envelope) {
	mcc.newChains = append(mcc.newChains, configTx)
}

func (mcc *mockChainCreator) ChannelsCount() int {
	return len(mcc.newChains)
}

func (mcc *mockChainCreator) CreateBundle(channelID string, config *cb.Config) (channelconfig.Resources, error) {
	return &mockconfig.Resources{
		ConfigtxManagerVal: &mockconfigtx.Manager{
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
	confUpdate := configtx.UnmarshalConfigUpdateOrPanic(configtx.UnmarshalConfigUpdateEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(envConfigUpdate.Payload).Data).ConfigUpdate)
	return &mockconfig.Resources{
		ConfigtxManagerVal: &mockconfigtx.Manager{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{
				Config:     &cb.Config{Sequence: 1, ChannelGroup: confUpdate.WriteSet},
				LastUpdate: envConfigUpdate,
			},
		},
	}, nil
}

func TestGoodProposal(t *testing.T) {
	newChainID := "new-chain-id"

	mcc := newMockChainCreator()

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			channelconfig.DefaultHashingAlgorithm(),
			channelconfig.DefaultBlockDataHashingStructure(),
			channelconfig.TemplateOrdererAddresses([]string{"foo"}),
		),
		channelconfig.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	assert.Nil(t, err, "Error constructing configtx")

	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	assert.NoError(t, NewSystemChannelFilter(mcc.ms, mcc).Apply(wrapped), "Did not accept valid transaction")
}

func TestProposalRejectedByConfig(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.NewChannelConfigErr = fmt.Errorf("desired err text")

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			channelconfig.DefaultHashingAlgorithm(),
			channelconfig.DefaultBlockDataHashingStructure(),
			channelconfig.TemplateOrdererAddresses([]string{"foo"}),
		),
		channelconfig.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
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

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			channelconfig.DefaultHashingAlgorithm(),
			channelconfig.DefaultBlockDataHashingStructure(),
			channelconfig.TemplateOrdererAddresses([]string{"foo"}),
		),
		channelconfig.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
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
					ChannelHeader: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
					&cb.Envelope{
						Payload: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
					&cb.Envelope{
						Payload: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
					&cb.Envelope{
						Payload: utils.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
					&cb.Envelope{
						Payload: utils.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: utils.MarshalOrPanic(
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
					ChannelHeader: utils.MarshalOrPanic(
						&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ORDERER_TRANSACTION),
						},
					),
				},
				Data: utils.MarshalOrPanic(
					&cb.Envelope{
						Payload: utils.MarshalOrPanic(
							&cb.Payload{
								Header: &cb.Header{
									ChannelHeader: utils.MarshalOrPanic(
										&cb.ChannelHeader{
											Type: int32(cb.HeaderType_CONFIG),
										},
									),
								},
								Data: utils.MarshalOrPanic(
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
			err := sysFilter.Apply(&cb.Envelope{Payload: utils.MarshalOrPanic(tc.payload)})
			assert.NotNil(t, err)
			assert.Regexp(t, tc.regexp, err.Error())
		})
	}
}
