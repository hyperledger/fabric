/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSystemChannelSupport struct {
	NewChannelConfigVal *mockconfigtx.Validator
	NewChannelConfigErr error
}

func (mscs *mockSystemChannelSupport) NewChannelConfig(env *cb.Envelope) (channelconfig.Resources, error) {
	return &mockchannelconfig.Resources{
		ConfigtxValidatorVal: mscs.NewChannelConfigVal,
	}, mscs.NewChannelConfigErr
}

func TestProcessSystemChannelNormalMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mockconfig.Orderer{},
		}
		_, err := NewSystemChannel(ms, mscs, nil).ProcessNormalMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "header not set", err.Error())
	})
	t.Run("Mismatched channel ID", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mockconfig.Orderer{},
		}
		_, err := NewSystemChannel(ms, mscs, nil).ProcessNormalMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + ".different",
					}),
				},
			}),
		})
		assert.Equal(t, ErrChannelDoesNotExist, err)
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal: 7,
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal: &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			},
		}
		cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessNormalMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		assert.Nil(t, err)
		assert.Equal(t, ms.SequenceVal, cs)

	})
}

func TestSystemChannelConfigUpdateMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "header not set", err.Error())
	})
	t.Run("NormalUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal: &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			},
		}
		sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}))
		sysChan.maintenanceFilter = AcceptRule
		config, cs, err := sysChan.ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		assert.NotNil(t, config)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
	})
	t.Run("BadNewChannelConfig", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigErr: fmt.Errorf("An error"),
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, mscs.NewChannelConfigErr, err)
	})
	t.Run("BadProposedUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{
				ProposeConfigUpdateError: fmt.Errorf("An error"),
			},
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.EqualError(t, err, "error validating channel creation transaction for new channel 'foodifferent', could not succesfully apply update to template configuration: An error")
	})
	t.Run("BadSignEnvelope", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{},
		}
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("BadByFilter", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{RejectRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, RejectRule.Apply(nil), err)
	})
	t.Run("RejectByMaintenance", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}))
		sysChan.maintenanceFilter = RejectRule
		_, _, err := sysChan.ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
					}),
				},
			}),
		})
		assert.Equal(t, RejectRule.Apply(nil), errors.Cause(err))
	})
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		config, cs, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, cs, ms.SequenceVal)
		assert.NotNil(t, config)
		assert.Nil(t, err)
	})
}

func TestSystemChannelConfigMsg(t *testing.T) {
	t.Run("ConfigMsg", func(t *testing.T) {
		t.Run("BadPayloadData", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: &mockconfigtx.Validator{
					ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				},
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mockconfig.Orderer{},
			}
			_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_CONFIG),
						}),
					},
					Data: []byte("hello"),
				}),
			})
			assert.Error(t, err)
		})

		t.Run("Good", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: &mockconfigtx.Validator{
					ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				},
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal: &mockconfig.Orderer{
					CapabilitiesVal: &mockconfig.OrdererCapabilities{},
				},
			}
			sysChan := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule}))
			sysChan.maintenanceFilter = AcceptRule
			config, seq, err := sysChan.ProcessConfigMsg(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_CONFIG),
						}),
					},
				}),
			})
			assert.Equal(t, seq, ms.SequenceVal)
			assert.NotNil(t, config)
			assert.Nil(t, err)
			hdr, err := utils.ChannelHeader(config)
			require.NoError(t, err)
			assert.Equal(
				t,
				int32(cb.HeaderType_CONFIG),
				hdr.Type,
				"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_CONFIG, hdr.Type)
		})
	})

	t.Run("OrdererTxMsg", func(t *testing.T) {
		t.Run("BadPayloadData", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: &mockconfigtx.Validator{
					ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				},
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mockconfig.Orderer{},
			}
			_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: []byte("hello"),
				}),
			})
			assert.Error(t, err)
		})

		t.Run("WrongEnvelopeType", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: &mockconfigtx.Validator{
					ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				},
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mockconfig.Orderer{},
			}
			_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: utils.MarshalOrPanic(&cb.Envelope{
						Payload: utils.MarshalOrPanic(&cb.Payload{
							Header: &cb.Header{
								ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
									ChannelId: testChannelID,
									Type:      int32(cb.HeaderType_MESSAGE),
								}),
							},
						}),
					}),
				}),
			})
			assert.Error(t, err)
		})

		t.Run("GoodConfigMsg", func(t *testing.T) {
			mscs := &mockSystemChannelSupport{
				NewChannelConfigVal: &mockconfigtx.Validator{
					ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				},
			}
			ms := &mockSystemChannelFilterSupport{
				SequenceVal:            7,
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
				OrdererConfigVal:       &mockconfig.Orderer{},
			}
			config, seq, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
				Payload: utils.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
							ChannelId: testChannelID,
							Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: utils.MarshalOrPanic(&cb.Envelope{
						Payload: utils.MarshalOrPanic(&cb.Payload{
							Header: &cb.Header{
								ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
									ChannelId: testChannelID,
									Type:      int32(cb.HeaderType_CONFIG),
								}),
							},
							Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{
								LastUpdate: &cb.Envelope{
									Payload: utils.MarshalOrPanic(&cb.Payload{
										Header: &cb.Header{
											ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
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
			assert.Equal(t, seq, ms.SequenceVal)
			assert.NotNil(t, config)
			assert.Nil(t, err)
			hdr, err := utils.ChannelHeader(config)
			assert.Equal(
				t,
				int32(cb.HeaderType_ORDERER_TRANSACTION),
				hdr.Type,
				"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_ORDERER_TRANSACTION, hdr.Type)
		})
	})

	t.Run("OtherMsgType", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Validator{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewSystemChannel(ms, mscs, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_MESSAGE),
					}),
				},
			}),
		})
		assert.Error(t, err)
	})
}

type mockDefaultTemplatorSupport struct {
	channelconfig.Resources
}

func (mdts *mockDefaultTemplatorSupport) Signer() crypto.LocalSigner {
	return nil
}

func TestNewChannelConfig(t *testing.T) {
	channelID := "foo"
	gConf := configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile)
	gConf.Orderer.Capabilities = map[string]bool{
		capabilities.OrdererV1_4_2: true,
	}
	channelGroup, err := encoder.NewChannelGroup(gConf)
	assert.NoError(t, err)
	ctxm, err := channelconfig.NewBundle(channelID, &cb.Config{ChannelGroup: channelGroup})

	originalCG := proto.Clone(ctxm.ConfigtxValidator().ConfigProto().ChannelGroup).(*cb.ConfigGroup)

	templator := NewDefaultTemplator(&mockDefaultTemplatorSupport{
		Resources: ctxm,
	})

	t.Run("BadPayload", func(t *testing.T) {
		_, err := templator.NewChannelConfig(&cb.Envelope{Payload: []byte("bad payload")})
		assert.Error(t, err, "Should not be able to create new channel config from bad payload.")
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("bad config update envelope data"),
				}),
			},
			"^Failing initial channel config creation because of config update unmarshaling error:",
		},
		{
			"MismatchedChannelID",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{},
					),
				}),
			},
			"^Config update has an empty writeset$",
		},
		{
			"WriteSetNoGroups",
			&cb.Payload{
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
						&cb.ConfigUpdate{
							WriteSet: &cb.ConfigGroup{
								Groups: map[string]*cb.ConfigGroup{
									channelconfig.ApplicationGroupKey: {
										Version: 1,
									},
								},
								Values: map[string]*cb.ConfigValue{
									channelconfig.ConsortiumKey: {
										Value: utils.MarshalOrPanic(
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
				Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(utils.MakeChannelHeader(cb.HeaderType_CONFIG_UPDATE, 0, "", epoch))},
				Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
					ConfigUpdate: utils.MarshalOrPanic(
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
										Value: utils.MarshalOrPanic(
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
			"Attempted to include a member which is not in the consortium",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := templator.NewChannelConfig(&cb.Envelope{Payload: utils.MarshalOrPanic(tc.payload)})
			if assert.Error(t, err) {
				assert.Regexp(t, tc.regex, err.Error())
			}
		})
	}

	// Successful
	t.Run("Success", func(t *testing.T) {
		createTx, err := encoder.MakeChannelCreationTransaction("foo", nil, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
		assert.Nil(t, err)
		res, err := templator.NewChannelConfig(createTx)
		assert.Nil(t, err)
		assert.NotEmpty(t, res.ConfigtxValidator().ConfigProto().ChannelGroup.ModPolicy)
		assert.True(t, proto.Equal(originalCG, ctxm.ConfigtxValidator().ConfigProto().ChannelGroup), "Underlying system channel config proto was mutated")
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

	assert.True(t, proto.Equal(expected, data))
}
