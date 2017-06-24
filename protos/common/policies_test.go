/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	common1 "github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestPoliciesEnums(t *testing.T) {
	var policy Policy_PolicyType
	policy = 0
	assert.Equal(t, "UNKNOWN", policy.String())
	policy = 1
	assert.Equal(t, "SIGNATURE", policy.String())
	policy = 2
	assert.Equal(t, "MSP", policy.String())
	policy = 3
	assert.Equal(t, "IMPLICIT_META", policy.String())

	_, _ = policy.EnumDescriptor()

	var meta ImplicitMetaPolicy_Rule
	meta = 0
	assert.Equal(t, "ANY", meta.String())
	meta = 1
	assert.Equal(t, "ALL", meta.String())
	meta = 2
	assert.Equal(t, "MAJORITY", meta.String())

	_, _ = meta.EnumDescriptor()
}

func TestPoliciesStructs(t *testing.T) {
	var policy *Policy
	assert.Equal(t, int32(0), policy.GetType())
	assert.Nil(t, policy.GetValue())
	policy = &Policy{
		Value: []byte("policy"),
		Type:  int32(1),
	}
	assert.Equal(t, int32(1), policy.GetType())
	assert.NotNil(t, policy.GetValue())
	policy.Reset()
	assert.Nil(t, policy.GetValue())
	_, _ = policy.Descriptor()
	_ = policy.String()
	policy.ProtoMessage()

	var env *SignaturePolicyEnvelope
	env = nil
	assert.Equal(t, int32(0), env.GetVersion())
	assert.Nil(t, env.GetIdentities())
	assert.Nil(t, env.GetRule())
	env = &SignaturePolicyEnvelope{
		Rule:       &SignaturePolicy{},
		Identities: []*common1.MSPPrincipal{&common1.MSPPrincipal{}},
		Version:    int32(1),
	}
	assert.Equal(t, int32(1), env.GetVersion())
	assert.NotNil(t, env.GetIdentities())
	assert.NotNil(t, env.GetRule())
	env.Reset()
	assert.Nil(t, env.GetIdentities())
	assert.Nil(t, env.GetRule())
	_, _ = env.Descriptor()
	_ = env.String()
	env.ProtoMessage()

	var sigpolicy *SignaturePolicy
	sigpolicy = nil
	assert.Nil(t, sigpolicy.GetType())
	assert.Nil(t, sigpolicy.GetNOutOf())
	assert.Equal(t, int32(0), sigpolicy.GetSignedBy())
	sigpolicy = &SignaturePolicy{
		Type: &SignaturePolicy_SignedBy{
			SignedBy: 2,
		},
	}
	assert.Equal(t, int32(2), sigpolicy.GetSignedBy())
	bytes, err := proto.Marshal(sigpolicy)
	assert.NoError(t, err, "Error marshaling SignedBy policy")
	_ = proto.Size(sigpolicy)
	err = proto.Unmarshal(bytes, &SignaturePolicy{})
	assert.NoError(t, err, "Error unmarshaling SignedBy policy")
	sigpolicy = &SignaturePolicy{
		Type: &SignaturePolicy_NOutOf_{
			NOutOf: &SignaturePolicy_NOutOf{},
		},
	}
	assert.NotNil(t, sigpolicy.GetNOutOf())
	bytes, err = proto.Marshal(sigpolicy)
	assert.NoError(t, err, "Error marshaling NOutOf policy")
	_ = proto.Size(sigpolicy)
	err = proto.Unmarshal(bytes, &SignaturePolicy{})
	assert.NoError(t, err, "Error unmarshaling NoutOf policy")
	_, _ = sigpolicy.Descriptor()
	_ = sigpolicy.String()
	sigpolicy.ProtoMessage()

	var n *SignaturePolicy_NOutOf
	n = nil
	assert.Equal(t, int32(0), n.GetN())
	assert.Nil(t, n.GetRules())
	n = &SignaturePolicy_NOutOf{
		Rules: []*SignaturePolicy{&SignaturePolicy{}},
		N:     int32(1),
	}
	assert.Equal(t, int32(1), n.GetN())
	assert.NotNil(t, n.GetRules())
	n.Reset()
	assert.Nil(t, n.GetRules())
	_, _ = n.Descriptor()
	_ = n.String()
	n.ProtoMessage()

	var impolicy *ImplicitMetaPolicy
	impolicy = nil
	assert.Equal(t, "", impolicy.GetSubPolicy())
	assert.Equal(t, ImplicitMetaPolicy_ANY, impolicy.GetRule())
	impolicy = &ImplicitMetaPolicy{
		SubPolicy: "subpolicy",
		Rule:      ImplicitMetaPolicy_MAJORITY,
	}
	assert.Equal(t, "subpolicy", impolicy.GetSubPolicy())
	assert.Equal(t, ImplicitMetaPolicy_MAJORITY, impolicy.GetRule())
	impolicy.Reset()
	assert.Equal(t, "", impolicy.GetSubPolicy())
	_, _ = impolicy.Descriptor()
	_ = impolicy.String()
	impolicy.ProtoMessage()

}
