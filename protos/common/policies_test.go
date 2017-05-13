/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
	policy := &Policy{
		Policy: []byte("policy"),
	}
	policy.Reset()
	assert.Nil(t, policy.Policy)
	_, _ = policy.Descriptor()
	_ = policy.String()
	policy.ProtoMessage()

	var env *SignaturePolicyEnvelope
	env = nil
	assert.Nil(t, env.GetIdentities())
	assert.Nil(t, env.GetPolicy())
	env = &SignaturePolicyEnvelope{
		Policy:     &SignaturePolicy{},
		Identities: []*common1.MSPPrincipal{&common1.MSPPrincipal{}},
	}
	assert.NotNil(t, env.GetIdentities())
	assert.NotNil(t, env.GetPolicy())
	env.Reset()
	assert.Nil(t, env.GetIdentities())
	assert.Nil(t, env.GetPolicy())
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
	assert.Nil(t, n.GetPolicies())
	n = &SignaturePolicy_NOutOf{
		Policies: []*SignaturePolicy{&SignaturePolicy{}},
	}
	assert.NotNil(t, n.GetPolicies())
	n.Reset()
	assert.Nil(t, n.GetPolicies())
	_, _ = n.Descriptor()
	_ = n.String()
	n.ProtoMessage()

	impolicy := &ImplicitMetaPolicy{
		SubPolicy: "subpolicy",
	}
	impolicy.Reset()
	assert.Equal(t, "", impolicy.SubPolicy)
	_, _ = impolicy.Descriptor()
	_ = impolicy.String()
	impolicy.ProtoMessage()

}
