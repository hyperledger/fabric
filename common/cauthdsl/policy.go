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
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
)

type provider struct {
	deserializer msp.IdentityDeserializer
}

// NewProviderImpl provides a policy generator for cauthdsl type policies
func NewPolicyProvider(deserializer msp.IdentityDeserializer) policies.Provider {
	return &provider{
		deserializer: deserializer,
	}
}

// NewPolicy creates a new policy based on the policy bytes
func (pr *provider) NewPolicy(data []byte) (policies.Policy, proto.Message, error) {
	sigPolicy := &cb.SignaturePolicyEnvelope{}
	if err := proto.Unmarshal(data, sigPolicy); err != nil {
		return nil, nil, fmt.Errorf("Error unmarshaling to SignaturePolicy: %s", err)
	}

	if sigPolicy.Version != 0 {
		return nil, nil, fmt.Errorf("This evaluator only understands messages of version 0, but version was %d", sigPolicy.Version)
	}

	compiled, err := compile(sigPolicy.Policy, sigPolicy.Identities, pr.deserializer)
	if err != nil {
		return nil, nil, err
	}

	return &policy{
		evaluator: compiled,
	}, sigPolicy, nil

}

type policy struct {
	evaluator func([]*cb.SignedData, []bool) bool
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (p *policy) Evaluate(signatureSet []*cb.SignedData) error {
	if p == nil {
		return fmt.Errorf("No such policy")
	}

	ok := p.evaluator(signatureSet, make([]bool, len(signatureSet)))
	if !ok {
		return errors.New("Failed to authenticate policy")
	}
	return nil
}
