/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

const testChannelID = "foo"

type mockSupport struct {
	ProposeConfigUpdateVal *cb.ConfigEnvelope
	ProposeConfigUpdateErr error
	SequenceVal            uint64
}

func (ms *mockSupport) ProposeConfigUpdate(env *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return ms.ProposeConfigUpdateVal, ms.ProposeConfigUpdateErr
}

func (ms *mockSupport) Sequence() uint64 {
	return ms.SequenceVal
}

func (ms *mockSupport) Signer() crypto.LocalSigner {
	return nil
}

func (ms *mockSupport) ChainID() string {
	return testChannelID
}

func TestClassifyMsg(t *testing.T) {
	t.Run("ConfigUpdate", func(t *testing.T) {
		class, err := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG_UPDATE)})
		assert.Equal(t, class, ConfigUpdateMsg)
		assert.Nil(t, err)
	})
	t.Run("OrdererTx", func(t *testing.T) {
		class, err := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ORDERER_TRANSACTION)})
		assert.Equal(t, class, ConfigUpdateMsg)
		assert.Nil(t, err)
	})
	t.Run("ConfigTx", func(t *testing.T) {
		class, err := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)})
		assert.Equal(t, class, ConfigUpdateMsg)
		assert.Nil(t, err)
	})
	t.Run("EndorserTx", func(t *testing.T) {
		class, err := (&StandardChannel{}).ClassifyMsg(&cb.ChannelHeader{Type: int32(cb.HeaderType_ENDORSER_TRANSACTION)})
		assert.Equal(t, class, NormalMsg)
		assert.Nil(t, err)
	})
}

func TestProcessNormalMsg(t *testing.T) {
	ms := &mockSupport{
		SequenceVal: 7,
	}
	cs, err := NewStandardChannel(ms, filter.NewRuleSet([]filter.Rule{filter.AcceptRule})).ProcessNormalMsg(nil)
	assert.Equal(t, cs, ms.SequenceVal)
	assert.Nil(t, err)
}

func TestConfigUpdateMsg(t *testing.T) {
	t.Run("BadMsg", func(t *testing.T) {
		ms := &mockSupport{
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
			ProposeConfigUpdateErr: fmt.Errorf("An error"),
		}
		config, cs, err := NewStandardChannel(ms, filter.NewRuleSet([]filter.Rule{filter.EmptyRejectRule})).ProcessConfigUpdateMsg(&cb.Envelope{})
		assert.Nil(t, config)
		assert.Equal(t, uint64(0), cs)
		assert.NotNil(t, err)
	})
	t.Run("SignedEnvelopeFailure", func(t *testing.T) {
		ms := &mockSupport{}
		config, cs, err := NewStandardChannel(ms, filter.NewRuleSet([]filter.Rule{filter.AcceptRule})).ProcessConfigUpdateMsg(nil)
		assert.Nil(t, config)
		assert.Equal(t, uint64(0), cs)
		assert.NotNil(t, err)
		assert.Regexp(t, "Marshal called with nil", err)
	})
	t.Run("Success", func(t *testing.T) {
		ms := &mockSupport{
			SequenceVal:            7,
			ProposeConfigUpdateVal: &cb.ConfigEnvelope{},
		}
		config, cs, err := NewStandardChannel(ms, filter.NewRuleSet([]filter.Rule{filter.AcceptRule})).ProcessConfigUpdateMsg(nil)
		assert.NotNil(t, config)
		assert.Equal(t, cs, ms.SequenceVal)
		assert.Nil(t, err)
	})
}
