/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policydsl

import (
	"sort"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
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
	AcceptAllPolicy = Envelope(NOutOf(0, []*cb.SignaturePolicy{}), [][]byte{})
	MarshaledAcceptAllPolicy = protoMarshalOrPanic(AcceptAllPolicy)

	RejectAllPolicy = Envelope(NOutOf(1, []*cb.SignaturePolicy{}), [][]byte{})
	MarshaledRejectAllPolicy = protoMarshalOrPanic(RejectAllPolicy)
}

// Envelope builds an envelope message embedding a SignaturePolicy
func Envelope(policy *cb.SignaturePolicy, identities [][]byte) *cb.SignaturePolicyEnvelope {
	ids := make([]*mb.MSPPrincipal, len(identities))
	for i := range ids {
		ids[i] = &mb.MSPPrincipal{PrincipalClassification: mb.MSPPrincipal_IDENTITY, Principal: identities[i]}
	}

	return &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       policy,
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
	return signedByFabricEntity(mspId, mb.MSPRole_MEMBER)
}

// SignedByMspClient creates a SignaturePolicyEnvelope
// requiring 1 signature from any client of the specified MSP
func SignedByMspClient(mspId string) *cb.SignaturePolicyEnvelope {
	return signedByFabricEntity(mspId, mb.MSPRole_CLIENT)
}

// SignedByMspPeer creates a SignaturePolicyEnvelope
// requiring 1 signature from any peer of the specified MSP
func SignedByMspPeer(mspId string) *cb.SignaturePolicyEnvelope {
	return signedByFabricEntity(mspId, mb.MSPRole_PEER)
}

// SignedByFabricEntity creates a SignaturePolicyEnvelope
// requiring 1 signature from any fabric entity, having the passed role, of the specified MSP
func signedByFabricEntity(mspId string, role mb.MSPRole_MSPRoleType) *cb.SignaturePolicyEnvelope {
	// specify the principal: it's a member of the msp we just found
	principal := &mb.MSPPrincipal{
		PrincipalClassification: mb.MSPPrincipal_ROLE,
		Principal:               protoMarshalOrPanic(&mb.MSPRole{Role: role, MspIdentifier: mspId}),
	}

	// create the policy: it requires exactly 1 signature from the first (and only) principal
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0)}),
		Identities: []*mb.MSPPrincipal{principal},
	}

	return p
}

// SignedByMspAdmin creates a SignaturePolicyEnvelope
// requiring 1 signature from any admin of the specified MSP
func SignedByMspAdmin(mspId string) *cb.SignaturePolicyEnvelope {
	// specify the principal: it's a member of the msp we just found
	principal := &mb.MSPPrincipal{
		PrincipalClassification: mb.MSPPrincipal_ROLE,
		Principal:               protoMarshalOrPanic(&mb.MSPRole{Role: mb.MSPRole_ADMIN, MspIdentifier: mspId}),
	}

	// create the policy: it requires exactly 1 signature from the first (and only) principal
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(1, []*cb.SignaturePolicy{SignedBy(0)}),
		Identities: []*mb.MSPPrincipal{principal},
	}

	return p
}

// wrapper for generating "any of a given role" type policies
func signedByAnyOfGivenRole(role mb.MSPRole_MSPRoleType, ids []string) *cb.SignaturePolicyEnvelope {
	return SignedByNOutOfGivenRole(1, role, ids)
}

func SignedByNOutOfGivenRole(n int32, role mb.MSPRole_MSPRoleType, ids []string) *cb.SignaturePolicyEnvelope {
	// we create an array of principals, one principal
	// per application MSP defined on this chain
	sort.Strings(ids)
	principals := make([]*mb.MSPPrincipal, len(ids))
	sigspolicy := make([]*cb.SignaturePolicy, len(ids))

	for i, id := range ids {
		principals[i] = &mb.MSPPrincipal{
			PrincipalClassification: mb.MSPPrincipal_ROLE,
			Principal:               protoMarshalOrPanic(&mb.MSPRole{Role: role, MspIdentifier: id}),
		}
		sigspolicy[i] = SignedBy(int32(i))
	}

	// create the policy: it requires exactly 1 signature from any of the principals
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       NOutOf(n, sigspolicy),
		Identities: principals,
	}

	return p
}

// SignedByAnyMember returns a policy that requires one valid
// signature from a member of any of the orgs whose ids are
// listed in the supplied string array
func SignedByAnyMember(ids []string) *cb.SignaturePolicyEnvelope {
	return signedByAnyOfGivenRole(mb.MSPRole_MEMBER, ids)
}

// SignedByAnyClient returns a policy that requires one valid
// signature from a client of any of the orgs whose ids are
// listed in the supplied string array
func SignedByAnyClient(ids []string) *cb.SignaturePolicyEnvelope {
	return signedByAnyOfGivenRole(mb.MSPRole_CLIENT, ids)
}

// SignedByAnyPeer returns a policy that requires one valid
// signature from an orderer of any of the orgs whose ids are
// listed in the supplied string array
func SignedByAnyPeer(ids []string) *cb.SignaturePolicyEnvelope {
	return signedByAnyOfGivenRole(mb.MSPRole_PEER, ids)
}

// SignedByAnyAdmin returns a policy that requires one valid
// signature from a admin of any of the orgs whose ids are
// listed in the supplied string array
func SignedByAnyAdmin(ids []string) *cb.SignaturePolicyEnvelope {
	return signedByAnyOfGivenRole(mb.MSPRole_ADMIN, ids)
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
				N:     n,
				Rules: policies,
			},
		},
	}
}

// protoMarshalOrPanic serializes a protobuf message and panics if this
// operation fails
func protoMarshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}

	return data
}
