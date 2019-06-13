/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

var acceptAllPolicy *cb.Policy
var rejectAllPolicy *cb.Policy

func init() {
	acceptAllPolicy = makePolicySource(true)
	rejectAllPolicy = makePolicySource(false)
}

// The proto utils has become a dumping ground of cyclic imports, it's easier to define this locally
func marshalOrPanic(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(fmt.Errorf("Error marshaling messages: %s, %s", msg, err))
	}
	return data
}

func makePolicySource(policyResult bool) *cb.Policy {
	var policyData *cb.SignaturePolicyEnvelope
	if policyResult {
		policyData = AcceptAllPolicy
	} else {
		policyData = RejectAllPolicy
	}
	return &cb.Policy{
		Type:  int32(cb.Policy_SIGNATURE),
		Value: marshalOrPanic(policyData),
	}
}

func providerMap() map[int32]policies.Provider {
	r := make(map[int32]policies.Provider)
	r[int32(cb.Policy_SIGNATURE)] = NewPolicyProvider(&mockDeserializer{})
	return r
}

func TestAccept(t *testing.T) {
	policyID := "policyID"
	m, err := policies.NewManagerImpl("test", providerMap(), &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			policyID: {Policy: acceptAllPolicy},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, m)

	policy, ok := m.GetPolicy(policyID)
	assert.True(t, ok, "Should have found policy which was just added, but did not")
	err = policy.Evaluate([]*cb.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	assert.NoError(t, err, "Should not have errored evaluating an acceptAll policy")
}

func TestReject(t *testing.T) {
	policyID := "policyID"
	m, err := policies.NewManagerImpl("test", providerMap(), &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			policyID: {Policy: rejectAllPolicy},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, m)
	policy, ok := m.GetPolicy(policyID)
	assert.True(t, ok, "Should have found policy which was just added, but did not")
	err = policy.Evaluate([]*cb.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	assert.Error(t, err, "Should have errored evaluating an rejectAll policy")
}

func TestRejectOnUnknown(t *testing.T) {
	m, err := policies.NewManagerImpl("test", providerMap(), &cb.ConfigGroup{})
	assert.NoError(t, err)
	assert.NotNil(t, m)
	policy, ok := m.GetPolicy("FakePolicyID")
	assert.False(t, ok, "Should not have found policy which was never added, but did")
	err = policy.Evaluate([]*cb.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	assert.Error(t, err, "Should have errored evaluating the default policy")
}

func TestNewPolicyErrorCase(t *testing.T) {
	provider := NewPolicyProvider(nil)

	pol1, msg1, err1 := provider.NewPolicy([]byte{0})
	assert.Nil(t, pol1)
	assert.Nil(t, msg1)
	assert.EqualError(t, err1, "Error unmarshaling to SignaturePolicy: proto: common.SignaturePolicyEnvelope: illegal tag 0 (wire type 0)")

	sigPolicy2 := &cb.SignaturePolicyEnvelope{Version: -1}
	data2 := marshalOrPanic(sigPolicy2)
	pol2, msg2, err2 := provider.NewPolicy(data2)
	assert.Nil(t, pol2)
	assert.Nil(t, msg2)
	assert.EqualError(t, err2, "This evaluator only understands messages of version 0, but version was -1")

	pol3, msg3, err3 := provider.NewPolicy([]byte{})
	assert.Nil(t, pol3)
	assert.Nil(t, msg3)
	assert.EqualError(t, err3, "Empty policy element")

	var pol4 *policy = nil
	err4 := pol4.Evaluate([]*cb.SignedData{})
	assert.EqualError(t, err4, "No such policy")
}

func TestEnvelopeBasedPolicyProvider(t *testing.T) {
	pp := &EnvelopeBasedPolicyProvider{Deserializer: &mockDeserializer{}}
	p, err := pp.NewPolicy(nil)
	assert.Nil(t, p)
	assert.Error(t, err, "invalid arguments")

	p, err = pp.NewPolicy(&cb.SignaturePolicyEnvelope{})
	assert.Nil(t, p)
	assert.Error(t, err, "Empty policy element")

	p, err = pp.NewPolicy(SignedByMspPeer("primus inter pares"))
	assert.NotNil(t, p)
	assert.NoError(t, err)
}

func TestVerifyFirstPanics(t *testing.T) {
	d := &deserializeAndVerify{}
	assert.Panics(t, func() { d.Verify() })
}
