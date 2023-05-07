/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
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

func (ms *mockSystemChannelFilterSupport) Signer() identity.SignerSerializer {
	return nil
}

func (ms *mockSystemChannelFilterSupport) ChannelID() string {
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
		require.Equal(t, class, ConfigUpdateMsg)
	})
	t.Run("OrdererTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ORDERER_TRANSACTION)})
		require.Equal(t, class, UnsupportedMsg)
	})
	t.Run("ConfigTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)})
		require.Equal(t, class, ConfigMsg)
	})
	t.Run("EndorserTx", func(t *testing.T) {
		class := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ENDORSER_TRANSACTION)})
		require.Equal(t, class, NormalMsg)
	})
}

func TestProcessNormalMsg(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:      7,
			OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessNormalMsg(nil)
		require.Equal(t, cs, ms.SequenceVal)
		require.Nil(t, err)
	})
	t.Run("Maintenance", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:      7,
			OrdererConfigVal: newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessNormalMsg(nil)
		require.EqualError(t, err, "normal transactions are rejected: maintenance mode")
	})
}

func TestConfigUpdateMsg(t *testing.T) {
	t.Run("BadUpdate", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			ProposeConfigUpdateErr: fmt.Errorf("An error"),
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		config, cs, err := NewStandardChannel(ms, NewRuleSet(nil), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{})
		require.Nil(t, config)
		require.Equal(t, uint64(0), cs)
		require.EqualError(t, err, "error applying config update to existing channel 'foo': An error")
	})
	t.Run("BadMsg", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			ProposeConfigUpdateErr: fmt.Errorf("An error"),
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		config, cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{EmptyRejectRule}), cryptoProvider).ProcessConfigUpdateMsg(&cb.Envelope{})
		require.Nil(t, config)
		require.Equal(t, uint64(0), cs)
		require.NotNil(t, err)
	})
	t.Run("SignedEnvelopeFailure", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			OrdererConfigVal: &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		config, cs, err := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigUpdateMsg(nil)
		require.Nil(t, config)
		require.Equal(t, uint64(0), cs)
		require.NotNil(t, err)
		require.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("Success", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		stdChan := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider)
		stdChan.maintenanceFilter = AcceptRule
		config, cs, err := stdChan.ProcessConfigUpdateMsg(nil)
		require.NotNil(t, config)
		require.Equal(t, cs, ms.SequenceVal)
		require.Nil(t, err)
	})
}

func TestProcessConfigMsg(t *testing.T) {
	t.Run("WrongType", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       &mocks.OrdererConfig{},
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, _, err = NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider).ProcessConfigMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
					}),
				},
			}),
		})
		require.Error(t, err)
	})

	t.Run("Success", func(t *testing.T) {
		ms := &mockSystemChannelFilterSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			OrdererConfigVal:       newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL),
		}
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		stdChan := NewStandardChannel(ms, NewRuleSet([]Rule{AcceptRule}), cryptoProvider)
		stdChan.maintenanceFilter = AcceptRule
		config, cs, err := stdChan.ProcessConfigMsg(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						ChannelId: testChannelID,
						Type:      int32(cb.HeaderType_CONFIG),
					}),
				},
			}),
		})
		require.NotNil(t, config)
		require.Equal(t, cs, ms.SequenceVal)
		require.Nil(t, err)
		hdr, err := protoutil.ChannelHeader(config)
		require.NoError(t, err)
		require.Equal(
			t,
			int32(cb.HeaderType_CONFIG),
			hdr.Type,
			"Expect type of returned envelope to be %d, but got %d", cb.HeaderType_CONFIG, hdr.Type)
	})
}
