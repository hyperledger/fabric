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

package utils

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
)

// MakeConfigurationItem makes a ConfigurationItem.
func MakeConfigurationItem(ch *cb.ChainHeader, configItemType cb.ConfigurationItem_ConfigurationType, lastModified uint64, modPolicyID string, key string, value []byte) *cb.ConfigurationItem {
	return &cb.ConfigurationItem{
		Header:             ch,
		Type:               configItemType,
		LastModified:       lastModified,
		ModificationPolicy: modPolicyID,
		Key:                key,
		Value:              value,
	}
}

// MakeConfigurationEnvelope makes a ConfigurationEnvelope.
func MakeConfigurationEnvelope(items ...*cb.SignedConfigurationItem) *cb.ConfigurationEnvelope {
	return &cb.ConfigurationEnvelope{Items: items}
}

// MakePolicyOrPanic creates a Policy proto message out of a SignaturePolicyEnvelope, and panics if this operation fails.
// NOTE Expand this as more policy types as supported.
func MakePolicyOrPanic(policyEnvelope interface{}) *cb.Policy {
	switch pe := policyEnvelope.(type) {
	case *cb.SignaturePolicyEnvelope:
		return &cb.Policy{
			Type: &cb.Policy_SignaturePolicy{
				SignaturePolicy: pe,
			},
		}
	default:
		panic("Unknown policy envelope type given")
	}
}

// MakePolicy creates a Policy proto message out of a SignaturePolicyEnvelope.
// NOTE Expand this as more policy types as supported.
func MakePolicy(policyEnvelope interface{}) (*cb.Policy, error) {
	switch pe := policyEnvelope.(type) {
	case *cb.SignaturePolicyEnvelope:
		return &cb.Policy{
			Type: &cb.Policy_SignaturePolicy{
				SignaturePolicy: pe,
			},
		}, nil
	default:
		return nil, fmt.Errorf("Unknown policy envelope type given")
	}
}
