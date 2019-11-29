/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCheckPolicyInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{
					Deserializer: &mocks.MockIdentityDeserializer{
						Identity: []byte("Alice"),
						Msg:      []byte("msg1"),
					},
				},
			},
		},
	}
	pc := &policyChecker{channelPolicyManagerGetter: policyManagerGetter}

	err := pc.CheckPolicy("B", "admin", &peer.SignedProposal{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")
}

func TestCheckPolicyBySignedDataInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{
					Deserializer: &mocks.MockIdentityDeserializer{
						Identity: []byte("Alice"),
						Msg:      []byte("msg1"),
					}},
			},
		},
	}
	pc := &policyChecker{channelPolicyManagerGetter: policyManagerGetter}

	err := pc.CheckPolicyBySignedData("", "admin", []*protoutil.SignedData{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid channel ID name during check policy on signed data. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "", []*protoutil.SignedData{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during check policy on signed data on channel [A]. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "admin", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed data during check policy on channel [A] with policy [admin]")

	err = pc.CheckPolicyBySignedData("B", "admin", []*protoutil.SignedData{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")

	err = pc.CheckPolicyBySignedData("A", "admin", []*protoutil.SignedData{{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [A] with policy [admin]")
}

func TestPolicyCheckerInvalidArgs(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{Deserializer: &mocks.MockIdentityDeserializer{
					Identity: []byte("Alice"),
					Msg:      []byte("msg1"),
				}},
			},
			"B": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{Deserializer: &mocks.MockIdentityDeserializer{
					Identity: []byte("Bob"),
					Msg:      []byte("msg2"),
				}},
			},
			"C": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{Deserializer: &mocks.MockIdentityDeserializer{
					Identity: []byte("Alice"),
					Msg:      []byte("msg3"),
				}},
			},
		},
	}
	identityDeserializer := &mocks.MockIdentityDeserializer{
		Identity: []byte("Alice"),
		Msg:      []byte("msg1"),
	}
	pc := NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Check that (non-empty channel, empty policy) fails
	err := pc.CheckPolicy("A", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during check policy on channel [A]. Name must be different from nil.")

	// Check that (empty channel, empty policy) fails
	err = pc.CheckPolicy("", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid policy name during channelless check policy. Name must be different from nil.")

	// Check that (non-empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("A", "A", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed proposal during check policy on channel [A] with policy [A]")

	// Check that (empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("", "A", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signed proposal during channelless check policy with policy [A]")
}

func TestPolicyChecker(t *testing.T) {
	policyManagerGetter := &mocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"A": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{
					Deserializer: &mocks.MockIdentityDeserializer{Identity: []byte("Alice"), Msg: []byte("msg1")},
				},
			},
			"B": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{
					Deserializer: &mocks.MockIdentityDeserializer{
						Identity: []byte("Bob"),
						Msg:      []byte("msg2"),
					},
				},
			},
			"C": &mocks.MockChannelPolicyManager{
				MockPolicy: &mocks.MockPolicy{
					Deserializer: &mocks.MockIdentityDeserializer{
						Identity: []byte("Alice"),
						Msg:      []byte("msg3"),
					},
				},
			},
		},
	}
	identityDeserializer := &mocks.MockIdentityDeserializer{
		Identity: []byte("Alice"),
		Msg:      []byte("msg1"),
	}
	pc := NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Validate Alice signatures against channel A's readers
	sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	policyManagerGetter.Managers["A"].(*mocks.MockChannelPolicyManager).MockPolicy.(*mocks.MockPolicy).Deserializer.(*mocks.MockIdentityDeserializer).Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	err := pc.CheckPolicy("A", "readers", sProp)
	assert.NoError(t, err)

	// Proposal from Alice for channel A should fail against channel B, where Alice is not involved
	err = pc.CheckPolicy("B", "readers", sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [B] with policy [readers]: [Invalid Identity]")

	// Proposal from Alice for channel A should fail against channel C, where Alice is involved but signature is not valid
	err = pc.CheckPolicy("C", "readers", sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [C] with policy [readers]: [Invalid Signature]")

	// Alice is a member of the local MSP, policy check must succeed
	identityDeserializer.Msg = sProp.ProposalBytes
	err = pc.CheckPolicyNoChannel(mgmt.Members, sProp)
	assert.NoError(t, err)

	sProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
	// Bob is not a member of the local MSP, policy check must fail
	err = pc.CheckPolicyNoChannel(mgmt.Members, sProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed deserializing proposal creator during channelless check policy with policy [Members]: [Invalid Identity]")
}

type MockPolicyCheckerFactory struct {
	mock.Mock
}

func (m *MockPolicyCheckerFactory) NewPolicyChecker() PolicyChecker {
	args := m.Called()
	return args.Get(0).(PolicyChecker)
}
