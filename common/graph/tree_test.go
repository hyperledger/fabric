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

func TestFindAndExists(t *testing.T) {
	v := NewTreeVertex("1", nil)
	u := v.AddDescendant(NewTreeVertex("2", nil)).AddDescendant(NewTreeVertex("4", nil))
	v.AddDescendant(NewTreeVertex("3", nil)).AddDescendant(NewTreeVertex("5", nil))
	require.Equal(t, u, v.Find("4"))
	require.True(t, v.Exists("4"))
	require.Nil(t, v.Find("10"))
	require.False(t, v.Exists("10"))
	require.Nil(t, u.Find("1"))
	require.False(t, u.Exists("1"))
	require.Equal(t, v, v.Find("1"))
	require.True(t, v.Exists("1"))
}

func TestIsLeaf(t *testing.T) {
	v := NewTreeVertex("1", nil)
	require.True(t, v.AddDescendant(NewTreeVertex("2", nil)).IsLeaf())
	require.False(t, v.IsLeaf())
}

func TestBFS(t *testing.T) {
	v := NewTreeVertex("1", nil)
	v.AddDescendant(NewTreeVertex("2", nil)).AddDescendant(NewTreeVertex("4", nil))
	v.AddDescendant(NewTreeVertex("3", nil)).AddDescendant(NewTreeVertex("5", nil))
	tree := v.ToTree()
	require.Equal(t, v, tree.Root)
	i := tree.BFS()
	j := 1
	for {
		v := i.Next()
		if v == nil {
			require.True(t, j == 6)
			break
		}
		require.Equal(t, fmt.Sprintf("%d", j), v.Id)
		j++
	}
}

func TestClone(t *testing.T) {
	v := NewTreeVertex("1", 1)
	v.AddDescendant(NewTreeVertex("2", 2)).AddDescendant(NewTreeVertex("4", 3))
	v.AddDescendant(NewTreeVertex("3", 4)).AddDescendant(NewTreeVertex("5", 5))

	copy := v.Clone()
	// They are different references
	require.False(t, copy == v)
	// They are equal
	require.Equal(t, v, copy)

	v.AddDescendant(NewTreeVertex("6", 6))
	require.NotEqual(t, v, copy)
}

func TestReplace(t *testing.T) {
	v := &TreeVertex{
		Id: "r",
		Descendants: []*TreeVertex{
			{Id: "D", Descendants: []*TreeVertex{}},
			{Id: "E", Descendants: []*TreeVertex{}},
			{Id: "F", Descendants: []*TreeVertex{}},
		},
	}

	v.replace("D", &TreeVertex{
		Id: "d",
		Descendants: []*TreeVertex{
			{Id: "a", Descendants: []*TreeVertex{}},
			{Id: "b", Descendants: []*TreeVertex{}},
			{Id: "c", Descendants: []*TreeVertex{}},
		},
	})

	require.Equal(t, "r", v.Id)
	require.Equal(t, &TreeVertex{Id: "F", Descendants: []*TreeVertex{}}, v.Descendants[2])
	require.Equal(t, &TreeVertex{Id: "E", Descendants: []*TreeVertex{}}, v.Descendants[1])
	require.Equal(t, "D", v.Descendants[0].Id)
	require.Equal(t, []*TreeVertex{
		{Id: "a", Descendants: []*TreeVertex{}},
		{Id: "b", Descendants: []*TreeVertex{}},
		{Id: "c", Descendants: []*TreeVertex{}},
	}, v.Descendants[0].Descendants)
}
