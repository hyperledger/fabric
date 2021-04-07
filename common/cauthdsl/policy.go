/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cauthdsl

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type provider struct {
	deserializer msp.IdentityDeserializer
}

// NewPolicyProvider provides a policy generator for cauthdsl type policies
func NewPolicyProvider(deserializer msp.IdentityDeserializer) policies.Provider {
	return &provider{
		deserializer: deserializer,
	}
}

// NewPolicy creates a new policy based on the policy bytes
func (pr *provider) NewPolicy(data []byte) (policies.Policy, proto.Message, error) {
	sigPolicy := &cb.SignaturePolicyEnvelope{}
	if err := proto.Unmarshal(data, sigPolicy); err != nil {
		return nil, nil, fmt.Errorf("Error unmarshalling to SignaturePolicy: %s", err)
	}

	if sigPolicy.Version != 0 {
		return nil, nil, fmt.Errorf("This evaluator only understands messages of version 0, but version was %d", sigPolicy.Version)
	}

	compiled, err := compile(sigPolicy.Rule, sigPolicy.Identities)
	if err != nil {
		return nil, nil, err
	}

	return &policy{
		evaluator:               compiled,
		deserializer:            pr.deserializer,
		signaturePolicyEnvelope: sigPolicy,
	}, sigPolicy, nil
}

// EnvelopeBasedPolicyProvider allows to create a new policy from SignaturePolicyEnvelope struct instead of []byte
type EnvelopeBasedPolicyProvider struct {
	Deserializer msp.IdentityDeserializer
}

// NewPolicy creates a new policy from the policy envelope
func (pp *EnvelopeBasedPolicyProvider) NewPolicy(sigPolicy *cb.SignaturePolicyEnvelope) (policies.Policy, error) {
	if sigPolicy == nil {
		return nil, errors.New("invalid arguments")
	}

	compiled, err := compile(sigPolicy.Rule, sigPolicy.Identities)
	if err != nil {
		return nil, err
	}

	return &policy{
		evaluator:               compiled,
		deserializer:            pp.Deserializer,
		signaturePolicyEnvelope: sigPolicy,
	}, nil
}

type policy struct {
	signaturePolicyEnvelope *cb.SignaturePolicyEnvelope
	evaluator               func([]msp.Identity, []bool) bool
	deserializer            msp.IdentityDeserializer
}

// EvaluateSignedData takes a set of SignedData and evaluates whether
// 1) the signatures are valid over the related message
// 2) the signing identities satisfy the policy
func (p *policy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	if p == nil {
		return errors.New("no such policy")
	}

	ids := policies.SignatureSetToValidIdentities(signatureSet, p.deserializer)

	return p.EvaluateIdentities(ids)
}

// EvaluateIdentities takes an array of identities and evaluates whether
// they satisfy the policy
func (p *policy) EvaluateIdentities(identities []msp.Identity) error {
	if p == nil {
		return fmt.Errorf("No such policy")
	}

	ok := p.evaluator(identities, make([]bool, len(identities)))
	if !ok {
		return errors.New("signature set did not satisfy policy")
	}
	return nil
}

func (p *policy) Convert() (*cb.SignaturePolicyEnvelope, error) {
	if p.signaturePolicyEnvelope == nil {
		return nil, errors.New("nil policy field")
	}

	return p.signaturePolicyEnvelope, nil
}
