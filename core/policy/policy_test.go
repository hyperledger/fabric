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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")
}

func TestCheckPolicyBySignedDataInvalidArgs(t *testing.T) {
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

	err := pc.CheckPolicyBySignedData("", "admin", []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid channel ID name during check policy on signed data. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "", []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid policy name during check policy on signed data on channel [A]. Name must be different from nil.")

	err = pc.CheckPolicyBySignedData("A", "admin", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid signed data during check policy on channel [A] with policy [admin]")

	err = pc.CheckPolicyBySignedData("B", "admin", []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to get policy manager for channel [B]")

	err = pc.CheckPolicyBySignedData("A", "admin", []*protoutil.SignedData{{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [A] with policy [admin]")
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
	pc := &policyChecker{
		channelPolicyManagerGetter: policyManagerGetter,
		localMSP:                   identityDeserializer,
		principalGetter:            &mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	}

	// Check that (non-empty channel, empty policy) fails
	err := pc.CheckPolicy("A", "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid policy name during check policy on channel [A]. Name must be different from nil.")

	// Check that (empty channel, empty policy) fails
	err = pc.CheckPolicy("", "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid policy name during channelless check policy. Name must be different from nil.")

	// Check that (non-empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("A", "A", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid signed proposal during check policy on channel [A] with policy [A]")

	// Check that (empty channel, non-empty policy, nil proposal) fails
	err = pc.CheckPolicy("", "A", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid signed proposal during channelless check policy with policy [A]")
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
	pc := &policyChecker{
		channelPolicyManagerGetter: policyManagerGetter,
		localMSP:                   identityDeserializer,
		principalGetter:            &mocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	}

	t.Run("CheckPolicy", func(t *testing.T) {
		// Validate Alice signatures against channel A's readers
		sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
		policyManagerGetter.Managers["A"].(*mocks.MockChannelPolicyManager).MockPolicy.(*mocks.MockPolicy).Deserializer.(*mocks.MockIdentityDeserializer).Msg = sProp.ProposalBytes
		sProp.Signature = sProp.ProposalBytes
		err := pc.CheckPolicy("A", "readers", sProp)
		require.NoError(t, err)

		// Proposal from Alice for channel A should fail against channel B, where Alice is not involved
		err = pc.CheckPolicy("B", "readers", sProp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [B] with policy [readers]: [Invalid Identity]")

		// Proposal from Alice for channel A should fail against channel C, where Alice is involved but signature is not valid
		err = pc.CheckPolicy("C", "readers", sProp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed evaluating policy on signed data during check policy on channel [C] with policy [readers]: [Invalid Signature]")
	})

	t.Run("CheckPolicyNoChannel", func(t *testing.T) {
		sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
		sProp.Signature = sProp.ProposalBytes

		// Alice is a member of the local MSP, policy check must succeed
		identityDeserializer.Msg = sProp.ProposalBytes
		err := pc.CheckPolicyNoChannel(Members, sProp)
		require.NoError(t, err)

		sProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Bob"), []byte("msg2"))
		// Bob is not a member of the local MSP, policy check must fail
		err = pc.CheckPolicyNoChannel(Members, sProp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed deserializing proposal creator during channelless check policy with policy [Members]: [Invalid Identity]")
	})

	t.Run("CheckPolicyNoChannel", func(t *testing.T) {
		signedData := &protoutil.SignedData{
			Identity:  []byte("Alice"),
			Data:      []byte("msg1"),
			Signature: []byte("msg1"), // for testing only, signature is same as data to pass MockIdentity.Verify
		}

		// Alice is a member of the local MSP, policy check must succeed
		identityDeserializer.Msg = signedData.Data
		err := pc.CheckPolicyNoChannelBySignedData(Admins, []*protoutil.SignedData{signedData})
		require.NoError(t, err)

		// CheckPolicyNoChannelBySignedData iterates each signed data and returns an error if any data is invalid
		// Bob is not a member of the local MSP, policy check must fail when deserializing the identity
		signedData2 := &protoutil.SignedData{
			Identity:  []byte("Bob"),
			Data:      []byte("msg2"),
			Signature: []byte("msg2"),
		}
		err = pc.CheckPolicyNoChannelBySignedData(Admins, []*protoutil.SignedData{signedData, signedData2})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed deserializing signed data identity during channelless check policy with policy [Admins]: [Invalid Identity]")

		// policy name cannot be empty
		err = pc.CheckPolicyNoChannelBySignedData("", []*protoutil.SignedData{signedData})
		require.EqualError(t, err, "invalid policy name during channelless check policy. Name must be different from nil.")

		// signed data cannot be nil or empty
		err = pc.CheckPolicyNoChannelBySignedData(Admins, nil)
		require.EqualError(t, err, "no signed data during channelless check policy with policy [Admins]")
		err = pc.CheckPolicyNoChannelBySignedData(Admins, []*protoutil.SignedData{})
		require.EqualError(t, err, "no signed data during channelless check policy with policy [Admins]")
	})
}
