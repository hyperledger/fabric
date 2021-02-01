/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mockSystemChannelSupport struct {
	NewChannelConfigVal *mocks.ConfigTXValidator
	NewChannelConfigErr error
}

func newMockConfigTXValidator(err error) *mocks.ConfigTXValidator {
	mockValidator := &mocks.ConfigTXValidator{}
	mockValidator.ProposeConfigUpdateReturns(&cb.ConfigEnvelope{}, err)
	return mockValidator
}

func (mscs *mockSystemChannelSupport) NewChannelConfig(env *cb.Envelope) (channelconfig.Resources, error) {
	mockResources := &mocks.Resources{}
	mockResources.ConfigtxValidatorReturns(mscs.NewChannelConfigVal)
	return mockResources, mscs.NewChannelConfigErr
}

func TestProcessSystemChannelNormalMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = NewSystemChannel(ms, mscs, nil, cryptoProvider).ProcessNormalMsg(&cb.Envelope{})
		require.NotNil(t, err)
		require.Regexp(t, "header not set", err.Error())
	})
	t.Run("Mismatched channel ID", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = NewSystemChannel(ms, mscs, nil, cryptoProvider).ProcessNormalMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + ".different",
					}),
				},
			}),
		})
		require.Equal(t, ErrChannelDoesNotExist, err)
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:      7,
			OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessNormalMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		require.Nil(t, err)
		require.Equal(t, ms.SequenceVal, cs)
	})
}

func TestSystemChannelConfigUpdateMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{})
		require.NotNil(t, err)
		require.Regexp(t, "header not set", err.Error())
	})
	t.Run("NormalUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider)
		sysChan.maintenanceFilter = AcceptRule
		config, cs, err := sysChan.ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		require.NotNil(t, config)
		require.Equal(t, cs, ms.SequenceVal)
		require.Nil(t, err)
	})
	t.Run("BadNewChannelConfig", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigErr: fmt.Errorf("An error"),
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		require.Equal(t, mscs.NewChannelConfigErr, err)
	})
	t.Run("BadProposedUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: newMockConfigTXValidator(fmt.Errorf("An error")),
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		require.EqualError(t, err, "error validating channel creation transaction for new channel 'foodifferent', could not successfully apply update to template configuration: An error")
	})
	t.Run("BadSignEnvelope", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mocks.ConfigTXValidator{},
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		require.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("BadByFilter", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: newMockConfigTXValidator(nil),
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{RejectRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		require.Equal(t, RejectRule.Apply(nil), err)
	})
	t.Run("RejectByMaintenance", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: newMockConfigTXValidator(nil),
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider)
		sysChan.maintenanceFilter = RejectRule
		_, _, err = sysChan.ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		require.Equal(t, RejectRule.Apply(nil), errors.Cause(err))
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: newMockConfigTXValidator(nil),
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		config, cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		require.Equal(t, cs, ms.SequenceVal)
		require.NotNil(t, config)
		require.Nil(t, err)
	})
}

func TestSystemChannelConfigMsg(t *testing.T) {
	t.Run("ConfigMsg", func(t *testing.T) {
		t.Run("BadPayloadData", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: newMockConfigTXValidator(nil),
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mocks.OrdererConfig{},
			}
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)
			_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_CONFIG),
						}),
					},
					Data: []byte("hello"),
				}),
			})
			require.Error(t, err)
		})

		t.Run("Good", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: newMockConfigTXValidator(nil),
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       newMockOrdererConfig(false, orderer.ConsensusType_STATE_NORMAL),
			}
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)
			sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider)
			sysChan.maintenanceFilter = AcceptRule
			config, seq, err := sysChan.ProcessConfigMsg(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_CONFIG),
						}),
					},
				}),
			})
			require.Equal(t, seq, ms.SequenceVal)
			require.NotNil(t, config)
			require.Nil(t, err)
			hdr, err := protoutil.ChannelHeader(config)
			require.NoError(t, err)
			require.Equal(
				t,
				int32(cb.HeaderType_CONFIG),
				hdr.Type,
				"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_CONFIG, hdr.Type)
		})
	})

	t.Run("OrdererTxMsg", func(t *testing.T) {
		t.Run("BadPayloadData", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: newMockConfigTXValidator(nil),
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mocks.OrdererConfig{},
			}
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)
			_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: []byte("hello"),
				}),
			})
			require.Error(t, err)
		})

		t.Run("WrongEnvelopeType", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: newMockConfigTXValidator(nil),
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mocks.OrdererConfig{},
			}
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)
			_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: protoutil.MarshalOrPanic(&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(&cb.Payload{
							Header: &cb.Header{
								ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
									ChannelId: testChannelID,
									Type:      int32(cb.HeaderType_MESSAGE),
								}),
							},
						}),
					}),
				}),
			})
			require.Error(t, err)
		})

		t.Run("GoodConfigMsg", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: newMockConfigTXValidator(nil),
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mocks.OrdererConfig{},
			}
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)
			config, seq, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: protoutil.MarshalOrPanic(&cb.Envelope{
						Payload: protoutil.MarshalOrPanic(&cb.Payload{
							Header: &cb.Header{
								ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
									ChannelId: testChannelID,
									Type:      int32(cb.HeaderType_CONFIG),
								}),
							},
							Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
								LastUpdate: &cb.Envelope{
									Payload: protoutil.MarshalOrPanic(&cb.Payload{
										Header: &cb.Header{
											ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
												ChannelId: testChannelID + "different",
												Type:      int32(cb.HeaderType_CONFIG_UPDATE),
											}),
										},
									}),
								},
							}),
						}),
					}),
				}),
			})
			require.Equal(t, seq, ms.SequenceVal)
			require.NotNil(t, config)
			require.Nil(t, err)
			hdr, err := protoutil.ChannelHeader(config)
			require.NoError(t, err)
			require.Equal(
				t,
				int32(cb.HeaderType_ORDERER_TRANSACTION),
				hdr.Type,
				"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_ORDERER_TRANSACTION, hdr.Type)
		})
	})

	t.Run("OtherMsgType", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: newMockConfigTXValidator(nil),
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_MESSAGE),
					}),
				},
			}),
		})
		require.Error(t, err)
	})
}

type mockDefaultTemplatorSupport struct {
	channelconfig.Resources
}

func (mdts *mockDefaultTemplatorSupport) Signer() identity.SignerSerializer {
	return nil
}

func TestNewChannelConfig(t *testing.T) {
	channelID := "foo"
	gConf := genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile, configtest.GetDevConfigDir())
	gConf.Orderer.Capabilities = map[string]bool{
		capabilities.OrdererV2_0: true,
	}
	channelGroup, err := encoder.NewChannelGroup(gConf)
	require.NoError(t, err)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	ctxm, err := channelconfig.NewBundle(channelID, &cb.Config{ChannelGroup: channelGroup}, cryptoProvider)
	require.NoError(t, err)

	originalCG := proto.Clone(ctxm.ConfigtxValidator().ConfigProto().ChannelGroup).(*cb.ConfigGroup)

	templator := NewDefaultTemplator(&mockDefaultTemplatorSupport{
		Resources: ctxm,
	}, cryptoProvider)

	t.Run("BadPayload", func(t *testing.T) {
		_, err := templator.NewChannelConfig(&cb.Envelope{Payload: []byte("bad payload")})
		require.Error(t, err, "Should not be able to create new channel config from bad payload.")
	})

	for _, tc := range []struct {
		name    string
		payload *cb.Payload
		regex   string
	}{
		{
			"BadPayloadData",
			&cb.Payload{
				Data: []byte("bad payload data"),
			},
			"^Failing initial channel config creation because of config update envelope unmarshaling error:",
		},
		{
			"BadConfigUpdate",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("bad config update envelope data"),
				}),
			},
			"^Failing initial channel config creation because of config update unmarshaling error:",
		},
		{
			"MismatchedChannelID",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							ChannelId: "foo",
						},
					),
				}),
			},
			"mismatched channel IDs",
		},
		{
			"EmptyConfigUpdateWriteSet",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{},
					),
				}),
			},
			"^Config update has an empty writeset$",
		},
		{
			"WriteSetNoGroups",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"WriteSetNoApplicationGroup",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{},
							},
						},
					),
				}),
			},
			"^Config update has missing application group$",
		},
		{
			"BadWriteSetApplicationGroupVersion",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 100,
									},
								},
							},
						},
					),
				}),
			},
			"^Config update for channel creation does not set application group version to 1,",
		},
		{
			"MissingWriteSetConsortiumValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{},
							},
						},
					),
				}),
			},
			"^Consortium config value missing$",
		},
		{
			"BadWriteSetConsortiumValueValue",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: []byte("bad consortium value"),
									},
								},
							},
						},
					),
				}),
			},
			"^Error reading unmarshaling consortium name:",
		},
		{
			"UnknownConsortiumName",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: protoutil.MarshalOrPanic(
											&cb.Consortium{
												Name: "NotTheNameYouAreLookingFor",
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"^Unknown consortium name:",
		},
		{
			"Missing consortium members",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: protoutil.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Proposed configuration has no application group members, but consortium contains members",
		},
		{
			"Member not in consortium",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: protoutil.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
										Groups: map[string]*cb.ConfigGroup{
											"BadOrgName": {},
										},
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: protoutil.MarshalOrPanic(
											&cb.Consortium{
												Name: genesisconfig.SampleConsortiumName,
											},
										),
									},
								},
							},
						},
					),
				}),
			},
			"Attempted to include member BadOrgName which is not in the consortium",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := templator.NewChannelConfig(&cb.Envelope{Payload: protoutil.MarshalOrPanic(tc.payload)})
			require.Error(t, err)
			require.Regexp(t, tc.regex, err.Error())
		})
	}

	// Successful
	t.Run("Success", func(t *testing.T) {
		createTx, err := encoder.MakeChannelCreationTransaction("foo", nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()))
		require.Nil(t, err)
		res, err := templator.NewChannelConfig(createTx)
		require.Nil(t, err)
		require.NotEmpty(t, res.ConfigtxValidator().ConfigProto().ChannelGroup.ModPolicy)
		require.True(t, proto.Equal(originalCG, ctxm.ConfigtxValidator().ConfigProto().ChannelGroup), "Underlying system channel config proto was mutated")
	})

	// Successful new channel config type
	t.Run("SuccessWithNewCreateType", func(t *testing.T) {
		createTx, err := encoder.MakeChannelCreationTransactionWithSystemChannelContext(
			"foo",
			nil,
			genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir()),
			genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile, configtest.GetDevConfigDir()),
		)
		require.Nil(t, err)
		_, err = templator.NewChannelConfig(createTx)
		require.Nil(t, err)
	})
}

func TestZeroVersions(t *testing.T) {
	data := &cb.ConfigGroup{
		Version: 7,
		Groups: map[string]*cb.ConfigGroup{
			"foo": {
				Version: 6,
			},
			"bar": {
				Values: map[string]*cb.ConfigValue{
					"foo": {
						Version: 3,
					},
				},
				Policies: map[string]*cb.ConfigPolicy{
					"bar": {
						Version: 5,
					},
				},
			},
		},
		Values: map[string]*cb.ConfigValue{
			"foo": {
				Version: 3,
			},
			"bar": {
				Version: 9,
			},
		},
		Policies: map[string]*cb.ConfigPolicy{
			"foo": {
				Version: 4,
			},
			"bar": {
				Version: 5,
			},
		},
	}

	expected := &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			"foo": {},
			"bar": {
				Values: map[string]*cb.ConfigValue{
					"foo": {},
				},
				Policies: map[string]*cb.ConfigPolicy{
					"bar": {},
				},
			},
		},
		Values: map[string]*cb.ConfigValue{
			"foo": {},
			"bar": {},
		},
		Policies: map[string]*cb.ConfigPolicy{
			"foo": {},
			"bar": {},
		},
	}

	zeroVersions(data)

	require.True(t, proto.Equal(expected, data))
}
