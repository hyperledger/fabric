/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

// Provider provides the backing implementation of a policy
type SignaturePolicyProvider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(signaturePolicy *common.SignaturePolicyEnvelope) (policies.Policy, error)
}

// ChannelPolicyReference is used to determine if a set of signature is valid and complies with a policy
type ChannelPolicyReferenceProvider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(channelConfigPolicyReference string) (policies.Policy, error)
}

//go:generate mockery -dir ../../common/policies/ -name Policy -case underscore -output mocks/
//go:generate mockery -dir . -name ChannelPolicyReferenceProvider -case underscore -output mocks/
//go:generate mockery -dir . -name SignaturePolicyProvider -case underscore -output mocks/

type ApplicationPolicyEvaluator struct {
	channel                        string
	signaturePolicyProvider        SignaturePolicyProvider
	channelPolicyReferenceProvider ChannelPolicyReferenceProvider
}

func (a *ApplicationPolicyEvaluator) evaluateSignaturePolicy(signaturePolicy *common.SignaturePolicyEnvelope, signatureSet []*common.SignedData) error {
	p, err := a.signaturePolicyProvider.NewPolicy(signaturePolicy)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for signature policy")
	}

	return p.Evaluate(signatureSet)
}

func (a *ApplicationPolicyEvaluator) evaluateChannelConfigPolicyReference(channelConfigPolicyReference string, signatureSet []*common.SignedData) error {
	p, err := a.channelPolicyReferenceProvider.NewPolicy(channelConfigPolicyReference)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for channel reference policy")
	}

	return p.Evaluate(signatureSet)
}

func (a *ApplicationPolicyEvaluator) Evaluate(policyBytes []byte, signatureSet []*common.SignedData) error {
	p := &peer.ApplicationPolicy{}
	err := proto.Unmarshal(policyBytes, p)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal ApplicationPolicy bytes")
	}

	switch policy := p.Type.(type) {
	case *peer.ApplicationPolicy_SignaturePolicy:
		return a.evaluateSignaturePolicy(policy.SignaturePolicy, signatureSet)
	case *peer.ApplicationPolicy_ChannelConfigPolicyReference:
		return a.evaluateChannelConfigPolicyReference(policy.ChannelConfigPolicyReference, signatureSet)
	default:
		return errors.Errorf("unsupported policy type %T", policy)
	}
}
