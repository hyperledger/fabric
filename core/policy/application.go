/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Policy -case underscore -output mocks/

// Policy is the local interface that used to generate mocks for foreign interface.
type Policy interface {
	policies.Policy
}

//go:generate mockery -dir . -name ChannelPolicyManagerGetter -case underscore -output mocks/

// ChannelPolicyManagerGetter is the local interface that used to generate mocks for foreign interface.
type ChannelPolicyManagerGetter interface {
	policies.ChannelPolicyManagerGetter
}

//go:generate mockery -dir . -name PolicyManager -case underscore -output mocks/

// PolicyManager is the local interface that used to generate mocks for foreign interface.
type PolicyManager interface {
	policies.Manager
}

//go:generate mockery -dir . -name IdentityDeserializer -case underscore -output mocks/

// IdentityDeserializer is the local interface that used to generate mocks for foreign interface.
type IdentityDeserializer interface {
	msp.IdentityDeserializer
}

//go:generate mockery -dir . -name Identity -case underscore -output mocks/

// Identity is the local interface that used to generate mocks for foreign interface.
type Identity interface {
	msp.Identity
}

//go:generate mockery -dir . -name ChannelPolicyReferenceProvider -case underscore -output mocks/
//go:generate mockery -dir . -name SignaturePolicyProvider -case underscore -output mocks/

// SignaturePolicyProvider provides the backing implementation of a policy.
type SignaturePolicyProvider interface {
	// NewPolicy creates a new policy based on the policy bytes.
	NewPolicy(signaturePolicy *common.SignaturePolicyEnvelope) (policies.Policy, error)
}

// ChannelPolicyReferenceProvider is used to determine if a set of signature is valid and complies with a policy.
type ChannelPolicyReferenceProvider interface {
	// NewPolicy creates a new policy based on the policy bytes.
	NewPolicy(channelConfigPolicyReference string) (policies.Policy, error)
}

type ApplicationPolicyEvaluator struct {
	signaturePolicyProvider        SignaturePolicyProvider
	channelPolicyReferenceProvider ChannelPolicyReferenceProvider
}

// Manager defines functions to interface with the policy manager of a channel
type Manager interface {
	// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
	GetPolicy(id string) (policies.Policy, bool)
}

type ChannelPolicyReferenceProviderImpl struct {
	Manager
}

func (c *ChannelPolicyReferenceProviderImpl) NewPolicy(channelConfigPolicyReference string) (policies.Policy, error) {
	p, ok := c.GetPolicy(channelConfigPolicyReference)
	if !ok {
		return nil, errors.Errorf("failed to retrieve policy for reference %s", channelConfigPolicyReference)
	}

	return p, nil
}

// dynamicPolicyManager implements a policy manager that
// always acts on the latest config for this channel
type dynamicPolicyManager struct {
	channelPolicyManagerGetter policies.ChannelPolicyManagerGetter
	channelID                  string
}

func (d *dynamicPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	mgr := d.channelPolicyManagerGetter.Manager(d.channelID)
	if mgr == nil {
		// this will never happen - if we are here we
		// managed to retrieve the policy manager for
		// this channel once, and so by the way the
		// channel config is managed, we cannot fail.
		panic("programming error")
	}

	return mgr.GetPolicy(id)
}

// New returns an evaluator for application policies
func New(deserializer msp.IdentityDeserializer, channel string, channelPolicyManagerGetter policies.ChannelPolicyManagerGetter) (*ApplicationPolicyEvaluator, error) {
	mgr := channelPolicyManagerGetter.Manager(channel)
	if mgr == nil {
		return nil, errors.Errorf("failed to retrieve policy manager for channel %s", channel)
	}

	return &ApplicationPolicyEvaluator{
		signaturePolicyProvider: &cauthdsl.EnvelopeBasedPolicyProvider{Deserializer: deserializer},
		channelPolicyReferenceProvider: &ChannelPolicyReferenceProviderImpl{Manager: &dynamicPolicyManager{
			channelID:                  channel,
			channelPolicyManagerGetter: channelPolicyManagerGetter,
		}},
	}, nil
}

func (a *ApplicationPolicyEvaluator) evaluateSignaturePolicy(signaturePolicy *common.SignaturePolicyEnvelope, signatureSet []*protoutil.SignedData) error {
	p, err := a.signaturePolicyProvider.NewPolicy(signaturePolicy)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for signature policy")
	}

	return p.EvaluateSignedData(signatureSet)
}

func (a *ApplicationPolicyEvaluator) evaluateChannelConfigPolicyReference(channelConfigPolicyReference string, signatureSet []*protoutil.SignedData) error {
	p, err := a.channelPolicyReferenceProvider.NewPolicy(channelConfigPolicyReference)
	if err != nil {
		return errors.WithMessage(err, "could not create evaluator for channel reference policy")
	}

	return p.EvaluateSignedData(signatureSet)
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
