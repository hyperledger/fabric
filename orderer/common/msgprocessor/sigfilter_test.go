/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.ActivateSpec("orderer.common.msgprocessor=DEBUG")
}

func makeEnvelope() *cb.Envelope {
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
		}),
	}
}

func TestAccept(t *testing.T) {
	mpm := &mockchannelconfig.Resources{
		PolicyManagerVal: &mockpolicies.Manager{Policy: &mockpolicies.Policy{}},
		OrdererConfigVal: &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL},
	}
	assert.Nil(t, NewSigFilter("foo", "bar", mpm).Apply(makeEnvelope()), "Valid envelope and good policy")
}

func TestMissingPolicy(t *testing.T) {
	mpm := &mockchannelconfig.Resources{
		PolicyManagerVal: &mockpolicies.Manager{},
		OrdererConfigVal: &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL},
	}
	err := NewSigFilter("foo", "bar", mpm).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.Regexp(t, "could not find policy", err.Error())
}

func TestEmptyPayload(t *testing.T) {
	mpm := &mockchannelconfig.Resources{
		PolicyManagerVal: &mockpolicies.Manager{Policy: &mockpolicies.Policy{}},
		OrdererConfigVal: &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL},
	}
	err := NewSigFilter("foo", "bar", mpm).Apply(&cb.Envelope{})
	assert.Error(t, err)
	assert.Regexp(t, "could not convert message to signedData", err.Error())
}

func TestErrorOnPolicy(t *testing.T) {
	mpm := &mockchannelconfig.Resources{
		PolicyManagerVal: &mockpolicies.Manager{Policy: &mockpolicies.Policy{Err: fmt.Errorf("Error")}},
		OrdererConfigVal: &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL},
	}
	err := NewSigFilter("foo", "bar", mpm).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.Equal(t, ErrPermissionDenied, errors.Cause(err))
}

func TestMaintenance(t *testing.T) {
	mpm := &mockchannelconfig.Resources{
		PolicyManagerVal: &mockpolicies.Manager{
			Policy: &mockpolicies.Policy{},
			PolicyMap: map[string]policies.Policy{
				"bar":                          &mockpolicies.Policy{},
				policies.ChannelOrdererWriters: &mockpolicies.Policy{Err: fmt.Errorf("Error")},
			},
		},
		OrdererConfigVal: &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_MAINTENANCE},
	}

	err := NewSigFilter("foo", policies.ChannelOrdererWriters, mpm).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: permission denied")
	err = NewSigFilter("bar", policies.ChannelOrdererWriters, mpm).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: permission denied")

	mpm.OrdererConfigVal = &mockchannelconfig.Orderer{ConsensusTypeStateVal: orderer.ConsensusType_STATE_NORMAL}
	err = NewSigFilter("foo", policies.ChannelOrdererWriters, mpm).Apply(makeEnvelope())
	assert.NoError(t, err)
	err = NewSigFilter("bar", policies.ChannelOrdererWriters, mpm).Apply(makeEnvelope())
	assert.NoError(t, err)
}
