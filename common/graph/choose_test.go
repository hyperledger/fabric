/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChoose(t *testing.T) {
	assert.Equal(t, 24, factorial(4))
	assert.Equal(t, 1, factorial(0))
	assert.Equal(t, 1, factorial(1))
	assert.Equal(t, 15504, nChooseK(20, 5))
	for n := 1; n < 20; n++ {
		for k := 1; k < n; k++ {
			g := chooseKoutOfN(n, k)
			assert.Equal(t, nChooseK(n, k), len(g), "n=%d, k=%d", n, k)
		}
	}
}
