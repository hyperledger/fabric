/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
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
//go:generate mockery -dir ../../common/policies/ -name ChannelPolicyManagerGetter -case underscore -output mocks/
//go:generate mockery -dir ../../common/policies/ -name Manager -case underscore -output mocks/
//go:generate mockery -dir . -name ChannelPolicyReferenceProvider -case underscore -output mocks/
//go:generate mockery -dir . -name SignaturePolicyProvider -case underscore -output mocks/

type ApplicationPolicyEvaluator struct {
	signaturePolicyProvider        SignaturePolicyProvider
	channelPolicyReferenceProvider ChannelPolicyReferenceProvider
}

type ChannelPolicyReferenceProviderImpl struct {
	policies.Manager
}

func (c *ChannelPolicyReferenceProviderImpl) NewPolicy(channelConfigPolicyReference string) (policies.Policy, error) {
	p, ok := c.GetPolicy(channelConfigPolicyReference)
	if !ok {
		return nil, errors.Errorf("failed to retrieve policy for reference %s", channelConfigPolicyReference)
	}

	return p, nil
}

// New returns an evaluator for application policies
func New(deserializer msp.IdentityDeserializer, channel string, channelPolicyManagerGetter policies.ChannelPolicyManagerGetter) (*ApplicationPolicyEvaluator, error) {
	cpp, ok := channelPolicyManagerGetter.Manager(channel)
	if !ok {
		return nil, errors.Errorf("failed to retrieve policy manager for channel %s", channel)
	}

	return &ApplicationPolicyEvaluator{
		signaturePolicyProvider:        &cauthdsl.ProviderFromStruct{Deserializer: deserializer},
		channelPolicyReferenceProvider: &ChannelPolicyReferenceProviderImpl{Manager: cpp},
	}, nil
}

func (a *ApplicationPolicyEvaluator) evaluateSignaturePolicy(signaturePolicy *common.SignaturePolicyEnvelope, signatureSet []*protoutil.SignedData) error {
	p, err := a.signaturePolicyProvider.NewPolicy(signaturePolicy)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for signature policy")
	}

	return p.Evaluate(signatureSet)
}

func (a *ApplicationPolicyEvaluator) evaluateChannelConfigPolicyReference(channelConfigPolicyReference string, signatureSet []*protoutil.SignedData) error {
	p, err := a.channelPolicyReferenceProvider.NewPolicy(channelConfigPolicyReference)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for channel reference policy")
	}

	return p.Evaluate(signatureSet)
}

func (a *ApplicationPolicyEvaluator) Evaluate(policyBytes []byte, signatureSet []*protoutil.SignedData) error {
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
