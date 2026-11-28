/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The effective per-connection stream cap resolves at the documented
// boundaries: an unset value selects the default, and any explicit value
// (including the unlimited sentinel used by the orderer cluster listener)
// is applied verbatim.
func TestEffectiveMaxConcurrentStreams(t *testing.T) {
	t.Parallel()

	require.Equal(t, DefaultMaxConcurrentStreams, effectiveMaxConcurrentStreams(0))
	require.Equal(t, uint32(1), effectiveMaxConcurrentStreams(1))
	require.Equal(t, DefaultMaxConcurrentStreams, effectiveMaxConcurrentStreams(DefaultMaxConcurrentStreams))
	require.Equal(t, UnlimitedMaxConcurrentStreams, effectiveMaxConcurrentStreams(UnlimitedMaxConcurrentStreams))
}
