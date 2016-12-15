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
	VerifySignature(signedData *cb.SignedData) error
}

// compile recursively builds a go evaluatable function corresponding to the policy specified
func compile(policy *cb.SignaturePolicy, identities [][]byte, ch CryptoHelper) (func([]*cb.SignedData) bool, error) {
	switch t := policy.Type.(type) {
	case *cb.SignaturePolicy_From:
		policies := make([]func([]*cb.SignedData) bool, len(t.From.Policies))
		for i, policy := range t.From.Policies {
			compiledPolicy, err := compile(policy, identities, ch)
			if err != nil {
				return nil, err
			}
			policies[i] = compiledPolicy

		}
		return func(signedData []*cb.SignedData) bool {
			verified := int32(0)
			for _, policy := range policies {
				if policy(signedData) {
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
		return func(signedData []*cb.SignedData) bool {
			for _, sd := range signedData {
				if bytes.Equal(sd.Identity, signedByID) {
					return ch.VerifySignature(sd) == nil
				}
			}
			return false
		}, nil
	default:
		return nil, fmt.Errorf("Unknown type: %T:%v", t, t)
	}
}
