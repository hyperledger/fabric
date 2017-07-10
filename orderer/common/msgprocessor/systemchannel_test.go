/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

type mockSystemChannelSupport struct {
	NewChannelConfigVal *mockconfigtx.Manager
	NewChannelConfigErr error
}

func (mscs *mockSystemChannelSupport) NewChannelConfig(env *cb.Envelope) (configtxapi.Manager, error) {
	return mscs.NewChannelConfigVal, mscs.NewChannelConfigErr
}

func TestProcessSystemChannelNormalMsg(t *testing.T) {
	t.Run("Missing header", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSupport{}
		_, err := NewSystemChannel(ms, mscs, nil).ProcessNormalMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "no header was set", err.Error())
	})
	t.Run("Mismatched channel ID", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSupport{}
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
		ms := &mockSupport{
			SequenceVal: 7,
		}
		cs, err := NewSystemChannel(ms, mscs, filter.NewRuleSet([]filter.Rule{filter.AcceptRule})).ProcessNormalMsg(&cb.Envelope{
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
		ms := &mockSupport{}
		_, _, err := NewSystemChannel(ms, mscs, nil).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.NotNil(t, err)
		assert.Regexp(t, "no header was set", err.Error())
	})
	t.Run("NormalUpdate", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{}
		ms := &mockSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		config, cs, err := NewSystemChannel(ms, mscs, filter.NewRuleSet([]filter.Rule{filter.AcceptRule})).ProcessConfigUpdateMsg(&cb.Envelope{
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
		ms := &mockSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, nil).ProcessConfigUpdateMsg(&cb.Envelope{
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
			NewChannelConfigVal: &mockconfigtx.Manager{
				ProposeConfigUpdateError: fmt.Errorf("An error"),
			},
		}
		ms := &mockSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, nil).ProcessConfigUpdateMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID + "different",
					}),
				},
			}),
		})
		assert.Equal(t, mscs.NewChannelConfigVal.ProposeConfigUpdateError, err)
	})
	t.Run("BadSignEnvelope", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{},
		}
		ms := &mockSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		_, _, err := NewSystemChannel(ms, mscs, nil).ProcessConfigUpdateMsg(&cb.Envelope{
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
	t.Run("Good", func(t *testing.T) {
		mscs := &mockSystemChannelSupport{
			NewChannelConfigVal: &mockconfigtx.Manager{
				ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			},
		}
		ms := &mockSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		config, cs, err := NewSystemChannel(ms, mscs, nil).ProcessConfigUpdateMsg(&cb.Envelope{
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
