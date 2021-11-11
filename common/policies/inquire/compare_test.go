/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestToPrincipalSet(t *testing.T) {
	var cps ComparablePrincipalSet
	cps = append(cps, NewComparablePrincipal(member("Org1MSP")))
	cps = append(cps, NewComparablePrincipal(member("Org2MSP")))
	expected := policies.PrincipalSet{member("Org1MSP"), member("Org2MSP")}
	require.Equal(t, expected, cps.ToPrincipalSet())
}

func TestToPrincipalSets(t *testing.T) {
	var cps, cps2 ComparablePrincipalSet

	cps = append(cps, NewComparablePrincipal(member("Org1MSP")))
	cps = append(cps, NewComparablePrincipal(member("Org2MSP")))

	cps2 = append(cps2, NewComparablePrincipal(member("Org3MSP")))
	cps2 = append(cps2, NewComparablePrincipal(member("Org4MSP")))

	expected := policies.PrincipalSets{
		{member("Org1MSP"), member("Org2MSP")},
		{member("Org3MSP"), member("Org4MSP")},
	}

	require.Equal(t, expected, ComparablePrincipalSets{cps, cps2}.ToPrincipalSets())
}

func TestNewComparablePrincipal(t *testing.T) {
	mspID := "Org1MSP"

	t.Run("Nil input", func(t *testing.T) {
		require.Nil(t, NewComparablePrincipal(nil))
	})

	t.Run("Identity", func(t *testing.T) {
		expectedPrincipal := &ComparablePrincipal{
			mspID:   mspID,
			idBytes: []byte("identity"),
			principal: &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_IDENTITY,
				Principal:               protoutil.MarshalOrPanic(&msp.SerializedIdentity{IdBytes: []byte("identity"), Mspid: mspID}),
			},
		}

		require.Equal(t, expectedPrincipal, NewComparablePrincipal(identity(mspID, []byte("identity"))))
	})

	t.Run("Invalid principal input", func(t *testing.T) {
		member := member(mspID)
		member.Principal = append(member.Principal, 0)
		require.Nil(t, NewComparablePrincipal(member))

		ou := ou(mspID)
		ou.Principal = append(ou.Principal, 0)
		require.Nil(t, NewComparablePrincipal(ou))
	})

	t.Run("Role", func(t *testing.T) {
		expectedMember := &msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: mspID}
		expectedPrincipal := &ComparablePrincipal{
			role: expectedMember, mspID: mspID,
			principal: &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_ROLE,
				Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: mspID}),
			},
		}
		require.Equal(t, expectedPrincipal, NewComparablePrincipal(member(mspID)))
	})

	t.Run("OU", func(t *testing.T) {
		expectedOURole := &msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: mspID}
		expectedPrincipal := &ComparablePrincipal{
			ou:    expectedOURole,
			mspID: mspID,
			principal: &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
				Principal:               protoutil.MarshalOrPanic(&msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: mspID}),
			},
		}
		require.Equal(t, expectedPrincipal, NewComparablePrincipal(ou(mspID)))
	})
}

func TestIsA(t *testing.T) {
	id1 := NewComparablePrincipal(identity("Org1MSP", []byte("identity")))
	id2 := NewComparablePrincipal(identity("Org2MSP", []byte("identity")))
	id3 := NewComparablePrincipal(identity("Org1MSP", []byte("identity2")))
	member1 := NewComparablePrincipal(member("Org1MSP"))
	member2 := NewComparablePrincipal(member("Org2MSP"))
	peer1 := NewComparablePrincipal(peer("Org1MSP"))
	peer2 := NewComparablePrincipal(peer("Org2MSP"))
	ou1 := NewComparablePrincipal(ou("Org1MSP"))
	ou1ButDifferentIssuer := NewComparablePrincipal(ou("Org1MSP"))
	ou1ButDifferentIssuer.ou.CertifiersIdentifier = []byte{1, 2, 3}
	ou2 := NewComparablePrincipal(ou("Org2MSP"))

	t.Run("Nil input", func(t *testing.T) {
		require.False(t, member1.IsA(nil))
	})

	t.Run("Incorrect state", func(t *testing.T) {
		require.False(t, (&ComparablePrincipal{}).IsA(member1))
	})

	t.Run("Same MSP ID", func(t *testing.T) {
		require.True(t, member1.IsA(NewComparablePrincipal(member("Org1MSP"))))
	})

	t.Run("A peer is also a member", func(t *testing.T) {
		require.True(t, peer1.IsA(member1))
	})

	t.Run("A member isn't a peer", func(t *testing.T) {
		require.False(t, member1.IsA(peer1))
	})

	t.Run("Different MSP IDs", func(t *testing.T) {
		require.False(t, member1.IsA(member2))
		require.False(t, peer2.IsA(peer1))
	})

	t.Run("An OU member is also a member", func(t *testing.T) {
		require.True(t, peer1.IsA(member1))
	})

	t.Run("A member isn't an OU member", func(t *testing.T) {
		require.False(t, member1.IsA(peer1))
	})

	t.Run("Same OU", func(t *testing.T) {
		require.True(t, ou1.IsA(NewComparablePrincipal(ou("Org1MSP"))))
	})

	t.Run("Different OU", func(t *testing.T) {
		require.False(t, ou1.IsA(ou2))
	})

	t.Run("Same OU, different issuer", func(t *testing.T) {
		require.False(t, ou1.IsA(ou1ButDifferentIssuer))
	})

	t.Run("OUs and Peers aren't the same", func(t *testing.T) {
		require.False(t, ou1.IsA(peer1))
	})
	t.Run("Same identity", func(t *testing.T) {
		require.True(t, id1.IsA(id1))
	})
	t.Run("Different identity bytes", func(t *testing.T) {
		require.False(t, id1.IsA(id3))
	})
	t.Run("Different MSP ID", func(t *testing.T) {
		require.False(t, id1.IsA(id2))
	})
}

func TestIsFound(t *testing.T) {
	member1 := NewComparablePrincipal(member("Org1MSP"))
	member2 := NewComparablePrincipal(member("Org2MSP"))
	peer1 := NewComparablePrincipal(peer("Org1MSP"))

	require.True(t, member1.IsFound(member1, member2))
	require.False(t, member1.IsFound())
	require.False(t, member1.IsFound(member2, peer1))
	require.True(t, peer1.IsFound(member1, member2))
}

func TestNewComparablePrincipalSet(t *testing.T) {
	t.Run("Invalid principal", func(t *testing.T) {
		idCP := NewComparablePrincipal(identity("Org1MSP", []byte("identity")))
		principals := []*msp.MSPPrincipal{member("Org1MSP"), identity("Org1MSP", []byte("identity"))}
		cps := NewComparablePrincipalSet(principals)
		expected := ComparablePrincipalSet([]*ComparablePrincipal{member1, idCP})
		require.Equal(t, expected, cps)
	})

	t.Run("Valid Principals", func(t *testing.T) {
		member1 := NewComparablePrincipal(member("Org1MSP"))
		peer2 := NewComparablePrincipal(peer("Org2MSP"))
		principals := []*msp.MSPPrincipal{member("Org1MSP"), peer("Org2MSP")}
		cps := NewComparablePrincipalSet(policies.PrincipalSet(principals))
		expected := ComparablePrincipalSet([]*ComparablePrincipal{member1, peer2})
		require.Equal(t, expected, cps)
	})
}

func member(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: orgName}),
	}
}

func peer(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               protoutil.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: orgName}),
	}
}

func ou(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               protoutil.MarshalOrPanic(&msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: orgName}),
	}
}

func identity(orgName string, idBytes []byte) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               protoutil.MarshalOrPanic(&msp.SerializedIdentity{Mspid: orgName, IdBytes: idBytes}),
	}
}
