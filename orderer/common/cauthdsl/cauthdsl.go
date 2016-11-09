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
	"bytes"
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
)

// CryptoHelper is used to provide a plugin point for different signature validation types
type CryptoHelper interface {
	VerifySignature(msg []byte, id []byte, signature []byte) bool
}

// SignaturePolicyEvaluator is useful for a chain Reader to stream blocks as they are created
type SignaturePolicyEvaluator struct {
	compiledAuthenticator func([]byte, [][]byte, [][]byte) bool
}

// NewSignaturePolicyEvaluator evaluates a protbuf SignaturePolicy to produce a 'compiled' version which can be invoked in code
func NewSignaturePolicyEvaluator(policy *cb.SignaturePolicyEnvelope, ch CryptoHelper) (*SignaturePolicyEvaluator, error) {
	if policy.Version != 0 {
		return nil, fmt.Errorf("This evaluator only understands messages of version 0, but version was %d", policy.Version)
	}

	compiled, err := compile(policy.Policy, policy.Identities, ch)
	if err != nil {
		return nil, err
	}

	return &SignaturePolicyEvaluator{
		compiledAuthenticator: compiled,
	}, nil
}

// compile recursively builds a go evaluatable function corresponding to the policy specified
func compile(policy *cb.SignaturePolicy, identities [][]byte, ch CryptoHelper) (func([]byte, [][]byte, [][]byte) bool, error) {
	switch t := policy.Type.(type) {
	case *cb.SignaturePolicy_From:
		policies := make([]func([]byte, [][]byte, [][]byte) bool, len(t.From.Policies))
		for i, policy := range t.From.Policies {
			compiledPolicy, err := compile(policy, identities, ch)
			if err != nil {
				return nil, err
			}
			policies[i] = compiledPolicy

		}
		return func(msg []byte, ids [][]byte, signatures [][]byte) bool {
			verified := int32(0)
			for _, policy := range policies {
				if policy(msg, ids, signatures) {
					verified++
				}
			}
			return verified >= t.From.N
		}, nil
	case *cb.SignaturePolicy_SignedBy:
		if t.SignedBy < 0 || t.SignedBy >= int32(len(identities)) {
			return nil, fmt.Errorf("Identity index out of range, requested %d, but identies length is %d", t.SignedBy, len(identities))
		}
		signedByID := identities[t.SignedBy]
		return func(msg []byte, ids [][]byte, signatures [][]byte) bool {
			for i, id := range ids {
				if bytes.Equal(id, signedByID) {
					return ch.VerifySignature(msg, id, signatures[i])
				}
			}
			return false
		}, nil
	default:
		return nil, fmt.Errorf("Unknown type: %T:%v", t, t)
	}

}

// Authenticate returns nil if the authentication policy is satisfied, or an error indicating why the authentication failed
func (ape *SignaturePolicyEvaluator) Authenticate(msg []byte, ids [][]byte, signatures [][]byte) bool {
	return ape.compiledAuthenticator(msg, ids, signatures)
}
