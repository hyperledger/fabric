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

func TestPermute(t *testing.T) {
	vR := NewTreeVertex("r", nil)
	vR.Threshold = 2

	vD := vR.AddDescendant(NewTreeVertex("D", nil))
	vD.Threshold = 2
	for _, id := range []string{"A", "B", "C"} {
		vD.AddDescendant(NewTreeVertex(id, nil))
	}

	vE := vR.AddDescendant(NewTreeVertex("E", nil))
	vE.Threshold = 2
	for _, id := range []string{"a", "b", "c"} {
		vE.AddDescendant(NewTreeVertex(id, nil))
	}

	vF := vR.AddDescendant(NewTreeVertex("F", nil))
	vF.Threshold = 2
	for _, id := range []string{"1", "2", "3"} {
		vF.AddDescendant(NewTreeVertex(id, nil))
	}

	permutations := vR.ToTree().Permute(1000)
	// For a sub-tree with r-(D,E) we have 9 combinations (3 combinations of each sub-tree where D and E are the roots)
	// For a sub-tree with r-(D,F) we have 9 combinations from the same logic
	// For a sub-tree with r-(E,F) we have 9 combinations too
	// Total 27 combinations
	require.Equal(t, 27, len(permutations))

	listCombination := func(i Iterator) []string {
		var traversal []string
		for {
			v := i.Next()
			if v == nil {
				break
			}
			traversal = append(traversal, v.Id)
		}
		return traversal
	}

	// First combination is a left most traversal on the combination graph
	expectedScan := []string{"r", "D", "E", "A", "B", "a", "b"}
	require.Equal(t, expectedScan, listCombination(permutations[0].BFS()))

	// Last combination is a right most traversal on the combination graph
	expectedScan = []string{"r", "E", "F", "b", "c", "2", "3"}
	require.Equal(t, expectedScan, listCombination(permutations[26].BFS()))
}

func TestPermuteTooManyCombinations(t *testing.T) {
	root := NewTreeVertex("r", nil)
	root.Threshold = 500
	for i := 0; i < 1000; i++ {
		root.AddDescendant(NewTreeVertex(fmt.Sprintf("%d", i), nil))
	}
	permutations := root.ToTree().Permute(501)
	require.Len(t, permutations, 501)
}
