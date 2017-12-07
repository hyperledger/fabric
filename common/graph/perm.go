/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

// treePermutations represents possible permutations
// of a tree
type treePermutations struct {
	originalRoot           *TreeVertex                     // The root vertex of all sub-trees
	permutations           []*TreeVertex                   // The accumulated permutations
	descendantPermutations map[*TreeVertex][][]*TreeVertex // Defines the combinations of sub-trees based on the threshold of the current vertex
}

// newTreePermutation creates a new treePermutations object with a given root vertex
func newTreePermutation(root *TreeVertex) *treePermutations {
	return &treePermutations{
		descendantPermutations: make(map[*TreeVertex][][]*TreeVertex),
		originalRoot:           root,
		permutations:           []*TreeVertex{root},
	}
}

// permute returns Trees that their vertices and edges all exist in the original tree who's vertex
// is the 'originalRoot' field of the treePermutations
func (tp *treePermutations) permute() []*Tree {
	tp.computeDescendantPermutations()

	it := newBFSIterator(tp.originalRoot)
	for {
		v := it.Next()
		if v == nil {
			break
		}

		if len(v.Descendants) == 0 {
			continue
		}

		// Iterate over all permutations where v exists
		// and separate them to 2 sets: a indiceSet where it exists and a indiceSet where it doesn't
		var permutationsWhereVexists []*TreeVertex
		var permutationsWhereVdoesntExist []*TreeVertex
		for _, perm := range tp.permutations {
			if perm.Exists(v.Id) {
				permutationsWhereVexists = append(permutationsWhereVexists, perm)
			} else {
				permutationsWhereVdoesntExist = append(permutationsWhereVdoesntExist, perm)
			}
		}

		// Remove the permutations where v exists from the permutations
		tp.permutations = permutationsWhereVdoesntExist

		// Next, we replace each occurrence of a permutation where v exists,
		// with multiple occurrences of its descendants permutations
		for _, perm := range permutationsWhereVexists {
			// For each permutation of v's descendants, clone the graph
			// and create a new graph with v replaced with the permutated graph
			// of v connected to the descendant permutation
			for _, permutation := range tp.descendantPermutations[v] {
				subGraph := &TreeVertex{
					Id:          v.Id,
					Data:        v.Data,
					Descendants: permutation,
				}
				newTree := perm.Clone()
				newTree.replace(v.Id, subGraph)
				// Add the new option to the permutations
				tp.permutations = append(tp.permutations, newTree)
			}
		}
	}

	res := make([]*Tree, len(tp.permutations))
	for i, perm := range tp.permutations {
		res[i] = perm.ToTree()
	}
	return res
}

// computeDescendantPermutations computes all possible combinations of sub-trees
// for all vertices, based on the thresholds.
func (tp *treePermutations) computeDescendantPermutations() {
	it := newBFSIterator(tp.originalRoot)
	for {
		v := it.Next()
		if v == nil {
			return
		}

		// Skip leaves
		if len(v.Descendants) == 0 {
			continue
		}

		// Iterate over all options of selecting the threshold out of the descendants
		for _, el := range chooseKoutOfN(len(v.Descendants), v.Threshold) {
			// And for each such option, append it to the current TreeVertex
			tp.descendantPermutations[v] = append(tp.descendantPermutations[v], v.selectDescendants(el.indices))
		}
	}
}

// selectDescendants returns a subset of descendants according to given indices
func (v *TreeVertex) selectDescendants(indices []int) []*TreeVertex {
	r := make([]*TreeVertex, len(indices))
	i := 0
	for _, index := range indices {
		r[i] = v.Descendants[index]
		i++
	}
	return r
}
