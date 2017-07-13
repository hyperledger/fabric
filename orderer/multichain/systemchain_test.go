/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multichain

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"

	"github.com/stretchr/testify/assert"
)

type mockSupport struct {
	msc *mockconfig.Orderer
}

func newMockSupport() *mockSupport {
	return &mockSupport{
		msc: &mockconfig.Orderer{},
	}
}

func (ms *mockSupport) SharedConfig() config.Orderer {
	return ms.msc
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

func (mcc *mockChainCreator) channelsCount() int {
	return len(mcc.newChains)
}

func (mcc *mockChainCreator) NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error) {
	if mcc.NewChannelConfigErr != nil {
		return nil, mcc.NewChannelConfigErr
	}
	confUpdate := configtx.UnmarshalConfigUpdateOrPanic(configtx.UnmarshalConfigUpdateEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(envConfigUpdate.Payload).Data).ConfigUpdate)
	return &mockconfigtx.Manager{
		ConfigEnvelopeVal: &cb.ConfigEnvelope{
			Config:     &cb.Config{Sequence: 1, ChannelGroup: confUpdate.WriteSet},
			LastUpdate: envConfigUpdate,
		},
	}, nil
}

func TestGoodProposal(t *testing.T) {
	newChainID := "new-chain-id"

	mcc := newMockChainCreator()

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	assert.Nil(t, err, "Error constructing configtx")

	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, committer := sysFilter.Apply(wrapped)

	assert.EqualValues(t, action, filter.Accept, "Did not accept valid transaction")
	assert.True(t, committer.Isolated(), "Channel creation belong in its own block")

	committer.Commit()
	assert.Len(t, mcc.newChains, 1, "Proposal should only have created 1 new chain")

	assert.Equal(t, ingressTx, mcc.newChains[0], "New chain should have been created with ingressTx")
}

func TestProposalRejectedByConfig(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.NewChannelConfigErr = fmt.Errorf("Error creating channel")

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, action, filter.Reject, "Did not accept valid transaction")
	assert.Len(t, mcc.newChains, 0, "Proposal should not have created a new chain")
}

func TestNumChainsExceeded(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.MaxChannelsCountVal = 1
	mcc.newChains = make([]*cb.Envelope, 2)

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, filter.Reject, action, "Transaction had created too many channels")
}

func TestBadProposal(t *testing.T) {
	mcc := newMockChainCreator()
	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	// logging.SetLevel(logging.DEBUG, "orderer/multichain")
	t.Run("BadPayload", func(t *testing.T) {
		action, committer := sysFilter.Apply(&cb.Envelope{Payload: []byte("bad payload")})
		assert.EqualValues(t, action, filter.Forward, "Should of skipped invalid tx")
		assert.Nil(t, committer)
	})

	// set logger to logger with a backend that writes to a byte buffer
	var buffer bytes.Buffer
	logger.SetBackend(logging.AddModuleLevel(logging.NewLogBackend(&buffer, "", 0)))
	// reset the logger after test
	defer func() {
		logger = logging.MustGetLogger("orderer/multichain")
	}()

	for _, tc := range []struct {
		name    string
		payload *cb.Payload
		action  filter.Action
		regexp  string
	}{
		{
			"MissingPayloadHeader",
			&cb.Payload{},
			filter.Forward,
			"",
		},
		{
			"BadChannelHeader",
			&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: []byte("bad channel header"),
				},
			},
			filter.Forward,
			"",
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
			filter.Reject,
			"",
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
			filter.Reject,
			"Error unmarshaling envelope payload",
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
			filter.Reject,
			"Not a config transaction",
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
			filter.Reject,
			"Error unmarshaling channel header",
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
			filter.Reject,
			"Not a config transaction",
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
			filter.Reject,
			"Error unmarshalling config envelope from payload",
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
			filter.Reject,
			"Must include a config update",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer.Reset()
			action, committer := sysFilter.Apply(&cb.Envelope{Payload: utils.MarshalOrPanic(tc.payload)})
			assert.EqualValues(t, tc.action, action, "Expected tx to be %sed, but instead the tx will be %sed.", filterActionToString(tc.action), filterActionToString(action))
			assert.Nil(t, committer)
			assert.Regexp(t, tc.regexp, buffer.String())
		})
	}
}

func filterActionToString(action filter.Action) string {
	switch action {
	case filter.Accept:
		return "accept"
	case filter.Forward:
		return "forward"
	case filter.Reject:
		return "reject"
	default:
		return ""
	}
}
