/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

const testChannelID = "foo"

type mockSystemChannelFilterSupport struct {
	ProposeConfigUpdateVal *cb.ConfigEnvelope
	ProposeConfigUpdateErr error
	SequenceVal            uint64
	OrdererConfigVal       channelconfig.Orderer
}

func (ms *mockSystemChannelFilterSupport) ProposeConfigUpdate(env *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return ms.ProposeConfigUpdateVal, ms.ProposeConfigUpdateErr
}

func (ms *mockSystemChannelFilterSupport) Sequence() uint64 {
	return ms.SequenceVal
}

func (ms *mockSystemChannelFilterSupport) Signer() crypto.LocalSigner {
	return nil
}

func (ms *mockSystemChannelFilterSupport) ChainID() string {
	return testChannelID
}

func (ms *mockSystemChannelFilterSupport) OrdererConfig() (channelconfig.Orderer, bool) {
	if ms.OrdererConfigVal == nil {
		return nil, false
	}

	return ms.OrdererConfigVal, true
}

func TestClassifyMsg(t *testing.T) {
	t.Run("ConfigUpdate", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG_UPDATE)})
		assert.Equal(t, class, ConfigUpdateMsg)
	})
	t.Run("OrdererTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ORDERER_TRANSACTION)})
		assert.Equal(t, class, ConfigMsg)
	})
	t.Run("ConfigTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)})
		assert.Equal(t, class, ConfigMsg)
	})
	t.Run("EndorserTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ENDORSER_TRANSACTION)})
		assert.Equal(t, class, NormalMsg)
	})
}

func TestProcessNormalMsg(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal: 7,
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
				ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL,
			},
		}
		cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule})).ProcessNormalMsg(nil)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
	})
	t.Run("Maintenance", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal: 7,
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal:       &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
				ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE,
			},
		}
		_, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule})).ProcessNormalMsg(nil)
		assert.EqualError(t, err, "normal transactions are rejected: maintenance mode")
	})
}

func TestConfigUpdateMsg(t *testing.T) {
	t.Run("BadUpdate", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			ProposeConfigUpdateErr: fmt.Errorf("An error"),
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		config, cs, err := NewStandardChannel(ms, NewRuleSet(nil)).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.Nil(t, config)
		assert.Equal(t, uint64(0), cs)
		assert.EqualError(t, err, "error applying config update to existing channel 'foo': An error")
	})
	t.Run("BadMsg", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			ProposeConfigUpdateErr: fmt.Errorf("An error"),
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		config, cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{EmptyRejectRule})).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.Nil(t, config)
		assert.Equal(t, uint64(0), cs)
		assert.NotNil(t, err)
	})
	t.Run("SignedEnvelopeFailure", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mockconfig.Orderer{},
		}
		config, cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule})).ProcessConfigUpdateMsg(nil)
		assert.Nil(t, config)
		assert.Equal(t, uint64(0), cs)
		assert.NotNil(t, err)
		assert.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("Success", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal: &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			},
		}
		stdChan := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}))
		stdChan.maintenanceFilter = AcceptRule
		config, cs, err := stdChan.ProcessConfigUpdateMsg(nil)
		assert.NotNil(t, config)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
	})
}

func TestProcessConfigMsg(t *testing.T) {
	t.Run("WrongType", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mockconfig.Orderer{},
		}
		_, _, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule})).ProcessConfigMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
					}),
				},
			}),
		})
		assert.Error(t, err)
	})

	t.Run("Success", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal: &mockconfig.Orderer{
				CapabilitiesVal: &mockconfig.OrdererCapabilities{ConsensusTypeMigrationVal: true},
			},
		}
		stdChan := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}))
		stdChan.maintenanceFilter = AcceptRule
		config, cs, err := stdChan.ProcessConfigMsg(&cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_CONFIG),
					}),
				},
			}),
		})
		assert.NotNil(t, config)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
		hdr, err := utils.ChannelHeader(config)
		assert.Equal(
			t,
			int32(cb.HeaderType_CONFIG),
			hdr.Type,
			"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_CONFIG, hdr.Type)
	})
}
