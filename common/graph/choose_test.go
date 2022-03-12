/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombinationsExceed(t *testing.T) {
	// 20 choose 5 is 15504.
	require.False(t, CombinationsExceed(20, 5, 15504))
	require.False(t, CombinationsExceed(20, 5, 15505))
	require.True(t, CombinationsExceed(20, 5, 15503))

	// A huge number of combinations doesn't overflow.
	require.True(t, CombinationsExceed(10000, 500, 9000))

	// N < K returns false
	require.False(t, CombinationsExceed(20, 30, 0))
}

func TestChooseKoutOfN(t *testing.T) {
	expectedSets := indiceSets{
		&indiceSet{[]int{0, 1, 2, 3}},
		&indiceSet{[]int{0, 1, 2, 4}},
		&indiceSet{[]int{0, 1, 2, 5}},
		&indiceSet{[]int{0, 1, 3, 4}},
		&indiceSet{[]int{0, 1, 3, 5}},
		&indiceSet{[]int{0, 1, 4, 5}},
		&indiceSet{[]int{0, 2, 3, 4}},
		&indiceSet{[]int{0, 2, 3, 5}},
		&indiceSet{[]int{0, 2, 4, 5}},
		&indiceSet{[]int{0, 3, 4, 5}},
		&indiceSet{[]int{1, 2, 3, 4}},
		&indiceSet{[]int{1, 2, 3, 5}},
		&indiceSet{[]int{1, 2, 4, 5}},
		&indiceSet{[]int{1, 3, 4, 5}},
		&indiceSet{[]int{2, 3, 4, 5}},
	}
	require.Equal(t, indiceSetsToStrings(expectedSets), indiceSetsToStrings(chooseKoutOfN(6, 4)))
}

func indiceSetsToStrings(sets indiceSets) []string {
	var res []string
	for _, set := range sets {
		res = append(res, fmt.Sprintf("%v", set.indices))
	}
	return res
}
