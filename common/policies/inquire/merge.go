/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"reflect"

	"github.com/hyperledger/fabric/common/policies"
)

// ComparablePrincipalSets aggregate ComparablePrincipalSets
type ComparablePrincipalSets []ComparablePrincipalSet

// ToPrincipalSets converts this ComparablePrincipalSets to a PrincipalSets
func (cps ComparablePrincipalSets) ToPrincipalSets() policies.PrincipalSets {
	var res policies.PrincipalSets
	for _, cp := range cps {
		res = append(res, cp.ToPrincipalSet())
	}
	return res
}

// Merge returns ComparablePrincipalSets that the underlying PrincipalSets consist of
// PrincipalSets that satisfy the endorsement policies that both ComparablePrincipalSets were derived of.
// More formally speaking, let EP1 and EP2 be endorsement policies, and
// P1 and P2 be the principal sets that each principal set p in P1 satisfies EP1,
// and each principal set p in P2 satisfies EP2.
// Denote as S1 and S2 the ComparablePrincipalSets derived from EP1 and EP2 respectively.
// Then, S = Merge(S1, S2) wields ComparablePrincipalSets
// such that every ComparablePrincipalSet s in S, satisfies both EP1 and EP2.
func Merge(s1, s2 ComparablePrincipalSets) ComparablePrincipalSets {
	var res ComparablePrincipalSets
	setsIn1ToTheContainingSetsIn2 := computeContainedInMapping(s1, s2)
	setsIn1ThatAreIn2 := s1.OfMapping(setsIn1ToTheContainingSetsIn2, s2)
	// Without loss of generality, remove all principal sets in s1 that
	// are contained by principal sets in s2, in order not to have duplicates
	s1 = s1.ExcludeIndices(setsIn1ToTheContainingSetsIn2)
	setsIn2ToTheContainingSetsIn1 := computeContainedInMapping(s2, s1)
	setsIn2ThatAreIn1 := s2.OfMapping(setsIn2ToTheContainingSetsIn1, s1)
	s2 = s2.ExcludeIndices(setsIn2ToTheContainingSetsIn1)

	// In the interim, the result contains sets from either the first of the second
	// set, that also contain some other set(s) in the other set
	res = append(res, setsIn1ThatAreIn2.ToMergedPrincipalSets()...)
	res = append(res, setsIn2ThatAreIn1.ToMergedPrincipalSets()...)

	// Now, purge the principal sets from the original groups s1 and s2
	// that are found to contain sets from the other group.
	// The motivation is to be left with s1 and s2 that only contain sets that aren't
	// contained or contain any set of the other group.
	s1 = s1.ExcludeIndices(setsIn2ToTheContainingSetsIn1.invert())
	s2 = s2.ExcludeIndices(setsIn1ToTheContainingSetsIn2.invert())

	// We're left with principal sets either in both or in one of the sets
	// that are entirely contained by the other sets, so there is nothing more to be done
	if len(s1) == 0 || len(s2) == 0 {
		return res.Reduce()
	}

	// We're only left with sets that are not contained in the other sets,
	// in both sets. Therefore we should combine them to form principal sets
	// that contain both sets.
	combinedPairs := CartesianProduct(s1, s2)
	res = append(res, combinedPairs.ToMergedPrincipalSets()...)
	return res.Reduce()
}

// CartesianProduct returns a comparablePrincipalSetPairs that is comprised of the combination
// of every possible pair of ComparablePrincipalSet such that the first element is in s1,
// and the second element is in s2.
func CartesianProduct(s1, s2 ComparablePrincipalSets) comparablePrincipalSetPairs {
	var res comparablePrincipalSetPairs
	for _, x := range s1 {
		var set comparablePrincipalSetPairs
		// For every set in the first sets,
		// combine it with every set in the second sets
		for _, y := range s2 {
			set = append(set, comparablePrincipalSetPair{
				contained:  x,
				containing: y,
			})
		}
		res = append(res, set...)
	}
	return res
}

// comparablePrincipalSetPair is a tuple of 2 ComparablePrincipalSets
type comparablePrincipalSetPair struct {
	contained  ComparablePrincipalSet
	containing ComparablePrincipalSet
}

// EnsurePlurality returns a ComparablePrincipalSet such that plurality requirements over
// the contained ComparablePrincipalSet in the comparablePrincipalSetPair hold
func (pair comparablePrincipalSetPair) MergeWithPlurality() ComparablePrincipalSet {
	var principalsToAdd []*ComparablePrincipal
	used := make(map[int]struct{})
	// Iterate over the contained set and for each principal
	for _, principal := range pair.contained {
		var covered bool
		// Search a principal in the containing set to cover the principal in the contained set
		for i, coveringPrincipal := range pair.containing {
			// The principal found shouldn't be used twice
			if _, isUsed := used[i]; isUsed {
				continue
			}
			// All identities that satisfy the found principal, should satisfy the covered principal as well.
			if coveringPrincipal.IsA(principal) {
				used[i] = struct{}{}
				covered = true
				break
			}
		}
		// If we haven't found a cover to the principal, it's because we already used up all the potential candidates
		// among the containing set, so just add it to the principals set to be added later.
		if !covered {
			principalsToAdd = append(principalsToAdd, principal)
		}
	}

	res := pair.containing.Clone()
	res = append(res, principalsToAdd...)
	return res
}

// comparablePrincipalSetPairs aggregates []comparablePrincipalSetPairs
type comparablePrincipalSetPairs []comparablePrincipalSetPair

// ToPrincipalSets converts the comparablePrincipalSetPairs to ComparablePrincipalSets
// while taking into account plurality of each pair
func (pairs comparablePrincipalSetPairs) ToMergedPrincipalSets() ComparablePrincipalSets {
	var res ComparablePrincipalSets
	for _, pair := range pairs {
		res = append(res, pair.MergeWithPlurality())
	}
	return res
}

// OfMapping returns comparablePrincipalSetPairs comprising only of the indices found in the given keys
func (cps ComparablePrincipalSets) OfMapping(mapping map[int][]int, sets2 ComparablePrincipalSets) comparablePrincipalSetPairs {
	var res []comparablePrincipalSetPair
	for i, js := range mapping {
		for _, j := range js {
			res = append(res, comparablePrincipalSetPair{
				contained:  cps[i],
				containing: sets2[j],
			})
		}
	}
	return res
}

// Reduce returns the ComparablePrincipalSets in a form such that no element contains another element.
// Every element that contains some other element is omitted from the result.
func (cps ComparablePrincipalSets) Reduce() ComparablePrincipalSets {
	var res ComparablePrincipalSets
	for i, s1 := range cps {
		var isContaining bool
		for j, s2 := range cps {
			if i == j {
				continue
			}
			if s2.IsSubset(s1) {
				isContaining = true
			}
		}
		if !isContaining {
			res = append(res, s1)
		}
	}
	return res
}

// ExcludeIndices returns a ComparablePrincipalSets without the given indices found in the keys
func (cps ComparablePrincipalSets) ExcludeIndices(mapping map[int][]int) ComparablePrincipalSets {
	var res ComparablePrincipalSets
	for i, set := range cps {
		if _, exists := mapping[i]; exists {
			continue
		}
		res = append(res, set)
	}
	return res
}

// Contains returns whether this ComparablePrincipalSet contains the given ComparablePrincipal.
// A ComparablePrincipalSet X contains a ComparablePrincipal y if
// there is a ComparablePrincipal x in X such that x.IsA(y).
// From here it follows that every signature set that satisfies X, also satisfies y.
func (cps ComparablePrincipalSet) Contains(s *ComparablePrincipal) bool {
	for _, cp := range cps {
		if cp.IsA(s) {
			return true
		}
	}
	return false
}

// IsContainedIn returns whether this ComparablePrincipalSet is contained in the given ComparablePrincipalSet.
// More formally- a ComparablePrincipalSet X is said to be contained in ComparablePrincipalSet Y
// if for each ComparablePrincipalSet x in X there is a ComparablePrincipalSet y in Y such that y.IsA(x) is true.
// If a ComparablePrincipalSet X is contained by a ComparablePrincipalSet Y then if a signature set satisfies Y,
// it also satisfies X, because for each x in X there is a y in Y such that there exists a signature of a corresponding
// identity such that the identity satisfies y, and therefore satisfies x too.
func (cps ComparablePrincipalSet) IsContainedIn(set ComparablePrincipalSet) bool {
	for _, cp := range cps {
		if !set.Contains(cp) {
			return false
		}
	}
	return true
}

// computeContainedInMapping returns a mapping from the indices in the first ComparablePrincipalSets
// to the indices in the second ComparablePrincipalSets that the corresponding ComparablePrincipalSets in
// the first ComparablePrincipalSets are contained in the second ComparablePrincipalSets given.
func computeContainedInMapping(s1, s2 []ComparablePrincipalSet) intMapping {
	mapping := make(map[int][]int)
	for i, ps1 := range s1 {
		for j, ps2 := range s2 {
			if !ps1.IsContainedIn(ps2) {
				continue
			}
			mapping[i] = append(mapping[i], j)
		}
	}
	return mapping
}

// intMapping maps integers to sets of integers
type intMapping map[int][]int

func (im intMapping) invert() intMapping {
	res := make(intMapping)
	for i, js := range im {
		for _, j := range js {
			res[j] = append(res[j], i)
		}
	}
	return res
}

// IsSubset returns whether this ComparablePrincipalSet is a subset of the given ComparablePrincipalSet
func (cps ComparablePrincipalSet) IsSubset(sets ComparablePrincipalSet) bool {
	for _, p1 := range cps {
		var found bool
		for _, p2 := range sets {
			if reflect.DeepEqual(p1, p2) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
