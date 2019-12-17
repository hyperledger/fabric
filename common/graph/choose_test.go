/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCombinationsExceed(t *testing.T) {
	// 20 choose 5 is 15504.
	assert.False(t, CombinationsExceed(20, 5, 15504))
	assert.False(t, CombinationsExceed(20, 5, 15505))
	assert.True(t, CombinationsExceed(20, 5, 15503))

	// A huge number of combinations doesn't overflow.
	assert.True(t, CombinationsExceed(10000, 500, 9000))

	// N < K returns false
	assert.False(t, CombinationsExceed(20, 30, 0))
}
