/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoffDuration(t *testing.T) {
	dur := backOffDuration(2.0, 0, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, BftMinRetryInterval, dur)

	dur = backOffDuration(2.0, 1, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, 2*BftMinRetryInterval, dur)

	dur = backOffDuration(2.0, 2, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, 4*BftMinRetryInterval, dur)

	// large exponent -> dur=max
	dur = backOffDuration(2.0, 20, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, BftMaxRetryInterval, dur)

	// very large exponent -> dur=max
	dur = backOffDuration(2.0, 1000000, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, BftMaxRetryInterval, dur)

	// max < min -> max=min
	dur = backOffDuration(2.0, 0, BftMinRetryInterval, BftMinRetryInterval/2)
	require.Equal(t, BftMinRetryInterval, dur)

	// min <= 0 -> min = BftMinRetryInterval
	dur = backOffDuration(2.0, 0, -10*BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, BftMinRetryInterval, dur)

	// base < 1.0 -> base = 1.0
	dur = backOffDuration(0.5, 8, BftMinRetryInterval, BftMaxRetryInterval)
	require.Equal(t, BftMinRetryInterval, dur)
}

func TestNumRetries(t *testing.T) {
	// 2,4,8
	n := numRetries2Max(2.0, time.Second, 8*time.Second)
	require.Equal(t, 3, n)

	// 2,4,8,10
	n = numRetries2Max(2.0, time.Second, 10*time.Second)
	require.Equal(t, 4, n)

	n = numRetries2Max(2.0, time.Second, time.Second)
	require.Equal(t, 0, n)

	n = numRetries2Max(2.0, 2*time.Second, time.Second)
	require.Equal(t, 0, n)
}
