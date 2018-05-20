/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"testing"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestToPrincipalSet(t *testing.T) {
	var cps ComparablePrincipalSet
	cps = append(cps, NewComparablePrincipal(member("Org1MSP")))
	cps = append(cps, NewComparablePrincipal(member("Org2MSP")))
	expected := policies.PrincipalSet{member("Org1MSP"), member("Org2MSP")}
	assert.Equal(t, expected, cps.ToPrincipalSet())
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

	assert.Equal(t, expected, ComparablePrincipalSets{cps, cps2}.ToPrincipalSets())
}

func TestNewComparablePrincipal(t *testing.T) {
	mspID := "Org1MSP"

	t.Run("Nil input", func(t *testing.T) {
		assert.Nil(t, NewComparablePrincipal(nil))
	})

	t.Run("Invalid principal type", func(t *testing.T) {
		assert.Nil(t, NewComparablePrincipal(identity(mspID)))
	})

	t.Run("Invalid principal input", func(t *testing.T) {
		member := member(mspID)
		member.Principal = append(member.Principal, 0)
		assert.Nil(t, NewComparablePrincipal(member))

		ou := ou(mspID)
		ou.Principal = append(ou.Principal, 0)
		assert.Nil(t, NewComparablePrincipal(ou))
	})

	t.Run("Role", func(t *testing.T) {
		expectedMember := &msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: mspID}
		expectedPrincipal := &ComparablePrincipal{
			role: expectedMember, mspID: mspID,
			principal: &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_ROLE,
				Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: mspID}),
			},
		}
		assert.Equal(t, expectedPrincipal, NewComparablePrincipal(member(mspID)))
	})

	t.Run("OU", func(t *testing.T) {
		expectedOURole := &msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: mspID}
		expectedPrincipal := &ComparablePrincipal{
			ou:    expectedOURole,
			mspID: mspID,
			principal: &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
				Principal:               utils.MarshalOrPanic(&msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: mspID}),
			},
		}
		assert.Equal(t, expectedPrincipal, NewComparablePrincipal(ou(mspID)))
	})
}

func TestIsA(t *testing.T) {
	member1 := NewComparablePrincipal(member("Org1MSP"))
	member2 := NewComparablePrincipal(member("Org2MSP"))
	peer1 := NewComparablePrincipal(peer("Org1MSP"))
	peer2 := NewComparablePrincipal(peer("Org2MSP"))
	ou1 := NewComparablePrincipal(ou("Org1MSP"))
	ou1ButDifferentIssuer := NewComparablePrincipal(ou("Org1MSP"))
	ou1ButDifferentIssuer.ou.CertifiersIdentifier = []byte{1, 2, 3}
	ou2 := NewComparablePrincipal(ou("Org2MSP"))

	t.Run("Nil input", func(t *testing.T) {
		assert.False(t, member1.IsA(nil))
	})

	t.Run("Incorrect state", func(t *testing.T) {
		assert.False(t, (&ComparablePrincipal{}).IsA(member1))
	})

	t.Run("Same MSP ID", func(t *testing.T) {
		assert.True(t, member1.IsA(NewComparablePrincipal(member("Org1MSP"))))
	})

	t.Run("A peer is also a member", func(t *testing.T) {
		assert.True(t, peer1.IsA(member1))
	})

	t.Run("A member isn't a peer", func(t *testing.T) {
		assert.False(t, member1.IsA(peer1))
	})

	t.Run("Different MSP IDs", func(t *testing.T) {
		assert.False(t, member1.IsA(member2))
		assert.False(t, peer2.IsA(peer1))
	})

	t.Run("An OU member is also a member", func(t *testing.T) {
		assert.True(t, peer1.IsA(member1))
	})

	t.Run("A member isn't an OU member", func(t *testing.T) {
		assert.False(t, member1.IsA(peer1))
	})

	t.Run("Same OU", func(t *testing.T) {
		assert.True(t, ou1.IsA(NewComparablePrincipal(ou("Org1MSP"))))
	})

	t.Run("Different OU", func(t *testing.T) {
		assert.False(t, ou1.IsA(ou2))
	})

	t.Run("Same OU, different issuer", func(t *testing.T) {
		assert.False(t, ou1.IsA(ou1ButDifferentIssuer))
	})

	t.Run("OUs and Peers aren't the same", func(t *testing.T) {
		assert.False(t, ou1.IsA(peer1))
	})
}

func TestIsFound(t *testing.T) {
	member1 := NewComparablePrincipal(member("Org1MSP"))
	member2 := NewComparablePrincipal(member("Org2MSP"))
	peer1 := NewComparablePrincipal(peer("Org1MSP"))

	assert.True(t, member1.IsFound(member1, member2))
	assert.False(t, member1.IsFound())
	assert.False(t, member1.IsFound(member2, peer1))
	assert.True(t, peer1.IsFound(member1, member2))
}

func TestNewComparablePrincipalSet(t *testing.T) {
	t.Run("Invalid principal", func(t *testing.T) {
		principals := []*msp.MSPPrincipal{member("Org1MSP"), identity("Org1MSP")}
		assert.Nil(t, NewComparablePrincipalSet(policies.PrincipalSet(principals)))
	})

	t.Run("Valid Principals", func(t *testing.T) {
		member1 := NewComparablePrincipal(member("Org1MSP"))
		peer2 := NewComparablePrincipal(peer("Org2MSP"))
		principals := []*msp.MSPPrincipal{member("Org1MSP"), peer("Org2MSP")}
		cps := NewComparablePrincipalSet(policies.PrincipalSet(principals))
		expected := ComparablePrincipalSet([]*ComparablePrincipal{member1, peer2})
		assert.Equal(t, expected, cps)
	})
}

func member(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: orgName})}
}

func peer(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ROLE,
		Principal:               utils.MarshalOrPanic(&msp.MSPRole{Role: msp.MSPRole_PEER, MspIdentifier: orgName})}
}

func ou(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_ORGANIZATION_UNIT,
		Principal:               utils.MarshalOrPanic(&msp.OrganizationUnit{OrganizationalUnitIdentifier: "ou", MspIdentifier: orgName})}
}

func identity(orgName string) *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_IDENTITY,
		Principal:               utils.MarshalOrPanic(&msp.SerializedIdentity{Mspid: orgName, IdBytes: []byte("identity")}),
	}
}
