/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	member1 = NewComparablePrincipal(member("Org1MSP"))
	member2 = NewComparablePrincipal(member("Org2MSP"))
	member3 = NewComparablePrincipal(member("Org3MSP"))
	member4 = NewComparablePrincipal(member("Org4MSP"))
	member5 = NewComparablePrincipal(member("Org5MSP"))
	peer1   = NewComparablePrincipal(peer("Org1MSP"))
	peer2   = NewComparablePrincipal(peer("Org2MSP"))
	peer3   = NewComparablePrincipal(peer("Org3MSP"))
	peer4   = NewComparablePrincipal(peer("Org4MSP"))
	peer5   = NewComparablePrincipal(peer("Org5MSP"))
)

func TestString(t *testing.T) {
	cps := ComparablePrincipalSet{member1, member2, NewComparablePrincipal(ou("Org3MSP"))}
	assert.Equal(t, "[Org1MSP.MEMBER, Org2MSP.MEMBER, Org3MSP.ou]", cps.String())
}

func TestClone(t *testing.T) {
	cps := ComparablePrincipalSet{member1, member2}
	clone := cps.Clone()
	assert.Equal(t, cps, clone)
	// Nil out the first entry and ensure that it isn't reflected in the clone
	cps[0] = nil
	assert.False(t, cps[0] == clone[0])
}

func TestMergeInclusiveWithPlurality(t *testing.T) {
	// Scenario:
	// S1 = {Org1.member, Org2.member, Org2.member}, {Org3.peer, Org4.peer}
	// S2 = {Org1.peer, Org2.peer}, {Org3.member, Org4.member}
	// Expected merge result:
	// {Org1.peer, Org2.peer, Org2.member}, {Org3.peer, Org4.peer}

	members12 := ComparablePrincipalSet{member1, member2, member2}
	members34 := ComparablePrincipalSet{member3, member4}
	peers12 := ComparablePrincipalSet{peer1, peer2}
	peers34 := ComparablePrincipalSet{peer3, peer4}

	peers12member2 := ComparablePrincipalSet{peer1, peer2, member2}

	s1 := ComparablePrincipalSets{members12, peers34}
	s2 := ComparablePrincipalSets{peers12, members34}

	merged := Merge(s1, s2)
	expected := ComparablePrincipalSets{peers12member2, peers34}
	assert.Equal(t, expected, merged)

	// Shuffle the order of principal sets and ensure the result is the same
	s1 = ComparablePrincipalSets{peers34, members12}
	s2 = ComparablePrincipalSets{members34, peers12}
	merged = Merge(s1, s2)
	assert.Equal(t, expected, merged)

	// Shuffle the order to the call
	merged = Merge(s2, s1)
	expected = ComparablePrincipalSets{peers34, peers12member2}
	assert.Equal(t, expected, merged)
}

func TestMergeExclusiveWithPlurality(t *testing.T) {
	// Scenario:
	// S1 = {Org1.member, Org2.member, Org2.member}, {Org3.peer, Org4.peer}
	// S2 = {Org2.member, Org3.member}, {Org4.peer, Org5.peer}
	// Expected merge result:
	// {Org1.member, Org2.member, Org3.member}, {Org3.peer, Org4.peer, Org5.peer},
	// {Org1.member, Org2.member, Org4.peer, Org5.peer}, {Org3.peer, Org4.peer, Org2.member, Org3.member}

	members122 := ComparablePrincipalSet{member1, member2, member2}
	members23 := ComparablePrincipalSet{member2, member3}
	peers34 := ComparablePrincipalSet{peer3, peer4}
	peers45 := ComparablePrincipalSet{peer4, peer5}
	members1223 := ComparablePrincipalSet{member1, member2, member2, member3}
	peers345 := ComparablePrincipalSet{peer3, peer4, peer5}
	members122peers45 := ComparablePrincipalSet{member1, member2, member2, peer4, peer5}
	peers34members23 := ComparablePrincipalSet{peer3, peer4, member2, member3}

	s1 := ComparablePrincipalSets{members122, peers34}
	s2 := ComparablePrincipalSets{members23, peers45}
	merged := Merge(s1, s2)
	expected := ComparablePrincipalSets{members1223, peers345, members122peers45, peers34members23}
	assert.True(t, expected.IsEqual(merged))
}

func TestMergePartialExclusiveWithPlurality(t *testing.T) {
	// Scenario:
	// S1 = {Org1.member, Org2.member}, {Org3.peer, Org4.peer, Org5.member}
	// S2 = {Org2.member, Org3.member}, {Org3.peer, Org4.member, Org4.member}, {Org4.peer, Org5.member}
	// The naive solution would be:
	// {Org1.member, Org2.member, Org3.member}, {Org3.peer, Org4.peer, Org4.member, Org5.member}, {Org3.peer, Org4.peer, Org5.member}
	// However - {Org3.peer, Org4.peer, Org4.member, Org5.member} contains {Org3.peer, Org4.peer, Org5.member}
	// So the result should be optimized, hence {Org3.peer, Org4.peer, Org4.member, Org5.member} will be omitted.
	// Expected merge result:
	// {Org1.member, Org2.member, Org3.member}, {Org3.peer, Org4.peer, Org5.member}

	members12 := ComparablePrincipalSet{member1, member2}
	members23 := ComparablePrincipalSet{member2, member3}
	peers34member5 := ComparablePrincipalSet{peer3, peer4, member5}
	peer3members44 := ComparablePrincipalSet{peer3, member4, member4}
	peer4member5 := ComparablePrincipalSet{peer4, member5}
	members123 := ComparablePrincipalSet{member1, member2, member3}

	s1 := ComparablePrincipalSets{members12, peers34member5}
	s2 := ComparablePrincipalSets{members23, peer3members44, peer4member5}
	merged := Merge(s1, s2)
	expected := ComparablePrincipalSets{members123, peers34member5}
	assert.True(t, expected.IsEqual(merged))
}

func TestMergeWithPlurality(t *testing.T) {
	pair := comparablePrincipalSetPair{
		contained:  ComparablePrincipalSet{peer3, member4, member4},
		containing: ComparablePrincipalSet{peer3, peer4, member5},
	}
	merged := pair.MergeWithPlurality()
	expected := ComparablePrincipalSet{peer3, peer4, member5, member4}
	assert.Equal(t, expected, merged)
}

func TestMergeSubsetPrincipalSets(t *testing.T) {
	member1And2 := ComparablePrincipalSets{ComparablePrincipalSet{member1, member2}}
	member1Or2 := ComparablePrincipalSets{ComparablePrincipalSet{member1}, ComparablePrincipalSet{member2}}
	merged := Merge(member1And2, member1Or2)
	expected := member1And2
	assert.True(t, expected.IsEqual(merged))
}

func TestEqual(t *testing.T) {
	member1 := NewComparablePrincipal(member("Org1MSP"))
	member2 := NewComparablePrincipal(member("Org2MSP"))
	anotherMember1 := NewComparablePrincipal(member("Org1MSP"))
	assert.False(t, member1.Equal(member2))
	assert.True(t, member1.Equal(anotherMember1))
}

func TestIsSubset(t *testing.T) {
	members12 := ComparablePrincipalSet{member1, member2, member2}
	members321 := ComparablePrincipalSet{member3, member2, member1}
	members13 := ComparablePrincipalSet{member1, member3}
	assert.True(t, members12.IsSubset(members12))
	assert.True(t, members12.IsSubset(members321))
	assert.False(t, members12.IsSubset(members13))
}

func TestReduce(t *testing.T) {
	members12 := ComparablePrincipalSet{member1, member2}
	members123 := ComparablePrincipalSet{member1, member2, member3}
	members12peers45 := ComparablePrincipalSet{member1, member2, peer4, peer5}
	peers45 := ComparablePrincipalSet{peer4, peer5}
	peers34 := ComparablePrincipalSet{peer3, peer4}
	s := ComparablePrincipalSets{members12, peers34, members123, members123, members12peers45, peers45}
	expected := ComparablePrincipalSets{members12, peers34, peers45}
	assert.Equal(t, expected, s.Reduce())
}

// IsEqual returns whether this ComparablePrincipalSets contains the elements of the given ComparablePrincipalSets
// in some order
func (cps ComparablePrincipalSets) IsEqual(sets ComparablePrincipalSets) bool {
	return cps.IsSubset(sets) && sets.IsSubset(cps)
}

// IsSubset returns whether this ComparablePrincipalSets is a subset of the given ComparablePrincipalSets
func (cps ComparablePrincipalSets) IsSubset(sets ComparablePrincipalSets) bool {
	for _, sets1 := range cps {
		var found bool
		for _, sets2 := range sets {
			if sets1.IsEqual(sets2) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsEqual returns whether this ComparablePrincipalSet contains the elements of the given ComparablePrincipalSet
// in some order
func (cps ComparablePrincipalSet) IsEqual(otherSet ComparablePrincipalSet) bool {
	return cps.IsSubset(otherSet) && otherSet.IsSubset(cps)
}
