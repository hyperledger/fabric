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
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"

	"github.com/golang/protobuf/proto"
)

// AcceptAllPolicy always evaluates to true
var AcceptAllPolicy *ab.SignaturePolicyEnvelope

// MarshaledAcceptAllPolicy is the Marshaled version of AcceptAllPolicy
var MarshaledAcceptAllPolicy []byte

// RejectAllPolicy always evaluates to false
var RejectAllPolicy *ab.SignaturePolicyEnvelope

// MarshaledRejectAllPolicy is the Marshaled version of RejectAllPolicy
var MarshaledRejectAllPolicy []byte

func init() {
	var err error

	AcceptAllPolicy = Envelope(NOutOf(0, []*ab.SignaturePolicy{}), [][]byte{})
	MarshaledAcceptAllPolicy, err = proto.Marshal(AcceptAllPolicy)
	if err != nil {
		panic("Error marshaling trueEnvelope")
	}

	RejectAllPolicy = Envelope(NOutOf(1, []*ab.SignaturePolicy{}), [][]byte{})
	MarshaledRejectAllPolicy, err = proto.Marshal(RejectAllPolicy)
	if err != nil {
		panic("Error marshaling falseEnvelope")
	}
}

// Envelope builds an envelope message embedding a SignaturePolicy
func Envelope(policy *ab.SignaturePolicy, identities [][]byte) *ab.SignaturePolicyEnvelope {
	return &ab.SignaturePolicyEnvelope{
		Version:    0,
		Policy:     policy,
		Identities: identities,
	}
}

// SignedBy creates a SignaturePolicy requiring a given signer's signature
func SignedBy(index int32) *ab.SignaturePolicy {
	return &ab.SignaturePolicy{
		Type: &ab.SignaturePolicy_SignedBy{
			SignedBy: index,
		},
	}
}

// And is a convenience method which utilizes NOutOf to produce And equivalent behavior
func And(lhs, rhs *ab.SignaturePolicy) *ab.SignaturePolicy {
	return NOutOf(2, []*ab.SignaturePolicy{lhs, rhs})
}

// Or is a convenience method which utilizes NOutOf to produce Or equivalent behavior
func Or(lhs, rhs *ab.SignaturePolicy) *ab.SignaturePolicy {
	return NOutOf(1, []*ab.SignaturePolicy{lhs, rhs})
}

// NOutOf creates a policy which requires N out of the slice of policies to evaluate to true
func NOutOf(n int32, policies []*ab.SignaturePolicy) *ab.SignaturePolicy {
	return &ab.SignaturePolicy{
		Type: &ab.SignaturePolicy_From{
			From: &ab.SignaturePolicy_NOutOf{
				N:        n,
				Policies: policies,
			},
		},
	}
}
