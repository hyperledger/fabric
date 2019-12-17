/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVertex(t *testing.T) {
	v := NewVertex("1", "1")
	assert.Equal(t, "1", v.Data)
	assert.Equal(t, "1", v.Id)
	u := NewVertex("2", "2")
	v.AddNeighbor(u)
	assert.Contains(t, u.Neighbors(), v)
	assert.Contains(t, v.Neighbors(), u)
	assert.Equal(t, u, v.NeighborById("2"))
	assert.Nil(t, v.NeighborById("3"))
}
