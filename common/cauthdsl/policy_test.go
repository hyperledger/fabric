/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var (
	acceptAllPolicy *cb.Policy
	rejectAllPolicy *cb.Policy
)

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
		policyData = policydsl.AcceptAllPolicy
	} else {
		policyData = policydsl.RejectAllPolicy
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
	require.NoError(t, err)
	require.NotNil(t, m)

	policy, ok := m.GetPolicy(policyID)
	require.True(t, ok, "Should have found policy which was just added, but did not")
	err = policy.EvaluateSignedData([]*protoutil.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	require.NoError(t, err, "Should not have errored evaluating an acceptAll policy")
}

func TestReject(t *testing.T) {
	policyID := "policyID"
	m, err := policies.NewManagerImpl("test", providerMap(), &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			policyID: {Policy: rejectAllPolicy},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, m)
	policy, ok := m.GetPolicy(policyID)
	require.True(t, ok, "Should have found policy which was just added, but did not")
	err = policy.EvaluateSignedData([]*protoutil.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	require.Error(t, err, "Should have errored evaluating an rejectAll policy")
}

func TestRejectOnUnknown(t *testing.T) {
	m, err := policies.NewManagerImpl("test", providerMap(), &cb.ConfigGroup{})
	require.NoError(t, err)
	require.NotNil(t, m)
	policy, ok := m.GetPolicy("FakePolicyID")
	require.False(t, ok, "Should not have found policy which was never added, but did")
	err = policy.EvaluateSignedData([]*protoutil.SignedData{{Identity: []byte("identity"), Data: []byte("data"), Signature: []byte("sig")}})
	require.Error(t, err, "Should have errored evaluating the default policy")
}

func TestNewPolicyErrorCase(t *testing.T) {
	provider := NewPolicyProvider(nil)

	pol1, msg1, err1 := provider.NewPolicy([]byte{0})
	require.Nil(t, pol1)
	require.Nil(t, msg1)
	require.ErrorContains(t, err1, "Error unmarshalling to SignaturePolicy")

	sigPolicy2 := &cb.SignaturePolicyEnvelope{Version: -1}
	data2 := marshalOrPanic(sigPolicy2)
	pol2, msg2, err2 := provider.NewPolicy(data2)
	require.Nil(t, pol2)
	require.Nil(t, msg2)
	require.EqualError(t, err2, "This evaluator only understands messages of version 0, but version was -1")

	pol3, msg3, err3 := provider.NewPolicy([]byte{})
	require.Nil(t, pol3)
	require.Nil(t, msg3)
	require.EqualError(t, err3, "Empty policy element")

	var pol4 *policy
	err4 := pol4.EvaluateSignedData([]*protoutil.SignedData{})
	require.EqualError(t, err4, "no such policy")
}

func TestEnvelopeBasedPolicyProvider(t *testing.T) {
	pp := &EnvelopeBasedPolicyProvider{Deserializer: &mockDeserializer{}}
	p, err := pp.NewPolicy(nil)
	require.Nil(t, p)
	require.Error(t, err, "invalid arguments")

	p, err = pp.NewPolicy(&cb.SignaturePolicyEnvelope{})
	require.Nil(t, p)
	require.Error(t, err, "Empty policy element")

	p, err = pp.NewPolicy(policydsl.SignedByMspPeer("primus inter pares"))
	require.NotNil(t, p)
	require.NoError(t, err)
}

func TestConverter(t *testing.T) {
	p := policy{}

	cp, err := p.Convert()
	require.Nil(t, cp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil policy field")

	p.signaturePolicyEnvelope = policydsl.RejectAllPolicy

	cp, err = p.Convert()
	require.NotNil(t, cp)
	require.NoError(t, err)
	require.Equal(t, cp, policydsl.RejectAllPolicy)
}
