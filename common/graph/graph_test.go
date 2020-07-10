/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVertex(t *testing.T) {
	v := NewVertex("1", "1")
	require.Equal(t, "1", v.Data)
	require.Equal(t, "1", v.Id)
	u := NewVertex("2", "2")
	v.AddNeighbor(u)
	require.Contains(t, u.Neighbors(), v)
	require.Contains(t, v.Neighbors(), u)
	require.Equal(t, u, v.NeighborById("2"))
	require.Nil(t, v.NeighborById("3"))
}
