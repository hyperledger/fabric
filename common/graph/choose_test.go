/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package graph

import (
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
