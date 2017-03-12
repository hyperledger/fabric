/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package cauthdsl

import (
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/utils"
)

// AcceptAllPolicy always evaluates to true
var AcceptAllPolicy *cb.SignaturePolicyEnvelope

// MarshaledAcceptAllPolicy is the Marshaled version of AcceptAllPolicy
var MarshaledAcceptAllPolicy []byte

// RejectAllPolicy always evaluates to false
var RejectAllPolicy *cb.SignaturePolicyEnvelope

// MarshaledRejectAllPolicy is the Marshaled version of RejectAllPolicy
var MarshaledRejectAllPolicy []byte

func init() {
	var err error

	AcceptAllPolicy = Envelope(NOutOf(0, []*cb.SignaturePolicy{}), [][]byte{})
	MarshaledAcceptAllPolicy, err = proto.Marshal(AcceptAllPolicy)
	if err != nil {
		panic("Error marshaling trueEnvelope")
	}

	RejectAllPolicy = Envelope(NOutOf(1, []*cb.SignaturePolicy{}), [][]byte{})
	MarshaledRejectAllPolicy, err = proto.Marshal(RejectAllPolicy)
	if err != nil {
		panic("Error marshaling falseEnvelope")
	}
}

// Envelope builds an envelope message embedding a SignaturePolicy
func Envelope(policy *cb.SignaturePolicy, identities [][]byte) *cb.SignaturePolicyEnvelope {
	ids := make([]*cb.MSPPrincipal, len(identities))
	for i, _ := range ids {
		ids[i] = &cb.MSPPrincipal{PrincipalClassification: cb.MSPPrincipal_IDENTITY, Principal: identities[i]}
	}

	return &cb.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     policy,
		Identities: ids,
	}
}

// SignedBy creates a SignaturePolicy requiring a given signer's signature
func SignedBy(index int32) *cb.SignaturePolicy {
	return &cb.SignaturePolicy{
		Type: &cb.SignaturePolicy_SignedBy{
			SignedBy: index,
		},
	}
}

// SignedByMspMember creates a SignaturePolicyEnvelope
// requiring 1 signature from any member of the specified MSP
func SignedByMspMember(mspId string) *cb.SignaturePolicyEnvelope {
	// specify the principal: it's a member of the msp we just found
	principal := &cb.MSPPrincipal{
		PrincipalClassification: cb.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&cb.MSPRole{Role: cb.MSPRole_MEMBER, MspIdentifier: mspId})}

	// create the policy: it requires exactly 1 signature from the first (and only) principal
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     NOutOf(1, []*cb.SignaturePolicy{SignedBy(0)}),
		Identities: []*cb.MSPPrincipal{principal},
	}

	return p
}

// SignedByMspAdmin creates a SignaturePolicyEnvelope
// requiring 1 signature from any admin of the specified MSP
func SignedByMspAdmin(mspId string) *cb.SignaturePolicyEnvelope {
	// specify the principal: it's a member of the msp we just found
	principal := &cb.MSPPrincipal{
		PrincipalClassification: cb.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&cb.MSPRole{Role: cb.MSPRole_ADMIN, MspIdentifier: mspId})}

	// create the policy: it requires exactly 1 signature from the first (and only) principal
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     NOutOf(1, []*cb.SignaturePolicy{SignedBy(0)}),
		Identities: []*cb.MSPPrincipal{principal},
	}

	return p
}

// And is a convenience method which utilizes NOutOf to produce And equivalent behavior
func And(lhs, rhs *cb.SignaturePolicy) *cb.SignaturePolicy {
	return NOutOf(2, []*cb.SignaturePolicy{lhs, rhs})
}

// Or is a convenience method which utilizes NOutOf to produce Or equivalent behavior
func Or(lhs, rhs *cb.SignaturePolicy) *cb.SignaturePolicy {
	return NOutOf(1, []*cb.SignaturePolicy{lhs, rhs})
}

// NOutOf creates a policy which requires N out of the slice of policies to evaluate to true
func NOutOf(n int32, policies []*cb.SignaturePolicy) *cb.SignaturePolicy {
	return &cb.SignaturePolicy{
		Type: &cb.SignaturePolicy_NOutOf_{
			NOutOf: &cb.SignaturePolicy_NOutOf{
				N:        n,
				Policies: policies,
			},
		},
	}
}
