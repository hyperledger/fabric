/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mocks/sig_filter_support.go --fake-name SigFilterSupport . sigFilterSupport

type sigFilterSupport interface {
	SigFilterSupport
}

//go:generate counterfeiter -o mocks/policy.go --fake-name Policy . policy

type policy interface {
	policies.Policy
}

//go:generate counterfeiter -o mocks/policy_manager.go --fake-name PolicyManager . policyManager

type policyManager interface {
	policies.Manager
}

func init() {
	flogging.ActivateSpec("orderer.common.msgprocessor=DEBUG")
}

func makeEnvelope() *cb.Envelope {
	return &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{}),
			},
		}),
	}
}

func newMockResources(hasPolicy bool, policyErr error) *mocks.Resources {
	policy := &mocks.Policy{}
	policy.EvaluateSignedDataReturns(policyErr)
	policyManager := &mocks.PolicyManager{}
	policyManager.GetPolicyReturns(policy, hasPolicy)
	ordererConfig := newMockOrdererConfig(false, orderer.ConsensusType_STATE_NORMAL)
	resources := &mocks.Resources{}
	resources.PolicyManagerReturns(policyManager)
	resources.OrdererConfigReturns(ordererConfig, true)
	return resources
}

func TestAccept(t *testing.T) {
	mockResources := newMockResources(true, nil)
	assert.Nil(t, NewSigFilter("foo", "bar", mockResources).Apply(makeEnvelope()), "Valid envelope and good policy")
}

func TestMissingPolicy(t *testing.T) {
	mockResources := newMockResources(false, nil)
	err := NewSigFilter("foo", "bar", mockResources).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.Regexp(t, "could not find policy", err.Error())
}

func TestEmptyPayload(t *testing.T) {
	mockResources := newMockResources(true, nil)
	err := NewSigFilter("foo", "bar", mockResources).Apply(&cb.Envelope{})
	assert.Error(t, err)
	assert.Regexp(t, "could not convert message to signedData", err.Error())
}

func TestErrorOnPolicy(t *testing.T) {
	mockResources := newMockResources(true, fmt.Errorf("Error"))
	err := NewSigFilter("foo", "bar", mockResources).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.Equal(t, ErrPermissionDenied, errors.Cause(err))
}

func TestMaintenance(t *testing.T) {
	mockResources := &mocks.Resources{}
	mockPolicyManager := &mocks.PolicyManager{}
	mockPolicyManager.GetPolicyStub = func(name string) (policies.Policy, bool) {
		mockPolicy := &mocks.Policy{}
		if name == policies.ChannelOrdererWriters {
			mockPolicy.EvaluateSignedDataReturns(fmt.Errorf("Error"))
		}
		return mockPolicy, true
	}
	mockResources.PolicyManagerReturns(mockPolicyManager)

	mockResources.OrdererConfigReturns(newMockOrdererConfig(true, orderer.ConsensusType_STATE_MAINTENANCE), true)
	err := NewSigFilter("foo", policies.ChannelOrdererWriters, mockResources).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: permission denied")
	err = NewSigFilter("bar", policies.ChannelOrdererWriters, mockResources).Apply(makeEnvelope())
	assert.Error(t, err)
	assert.EqualError(t, err, "Error: permission denied")

	mockResources.OrdererConfigReturns(newMockOrdererConfig(true, orderer.ConsensusType_STATE_NORMAL), true)
	err = NewSigFilter("foo", policies.ChannelOrdererWriters, mockResources).Apply(makeEnvelope())
	assert.NoError(t, err)
	err = NewSigFilter("bar", policies.ChannelOrdererWriters, mockResources).Apply(makeEnvelope())
	assert.NoError(t, err)
}
