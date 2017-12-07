/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

type orderedSet struct {
	elements []interface{}
}

func (s *orderedSet) add(o interface{}) {
	s.elements = append(s.elements, o)
}

type indiceSet struct {
	indices []int
}

type indiceSets []*indiceSet

func factorial(n int) int {
	m := 1
	for i := 1; i <= n; i++ {
		m *= i
	}
	return m
}

func nChooseK(n, k int) int {
	a := factorial(n)
	b := factorial(n-k) * factorial(k)
	return a / b
}

func chooseKoutOfN(n, k int) indiceSets {
	var res indiceSets
	subGroups := &orderedSet{}
	choose(n, k, 0, nil, subGroups)
	for _, el := range subGroups.elements {
		res = append(res, el.(*indiceSet))
	}
	return res
}

func choose(n int, targetAmount int, i int, currentSubGroup []int, subGroups *orderedSet) {
	// Check if we have enough elements in our current subgroup
	if len(currentSubGroup) == targetAmount {
		subGroups.add(&indiceSet{indices: currentSubGroup})
		return
	}
	// Return early if not enough remaining candidates to pick from
	itemsLeftToPick := n - i
	if targetAmount-len(currentSubGroup) > itemsLeftToPick {
		return
	}
	// We either pick the current element
	choose(n, targetAmount, i+1, append(currentSubGroup, i), subGroups)
	// Or don't pick it
	choose(n, targetAmount, i+1, currentSubGroup, subGroups)
}
