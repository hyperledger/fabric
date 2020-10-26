/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/require"
)

func TestOptions_applyDefaults(t *testing.T) {
	t.Run("on empty object", func(t *testing.T) {
		opt := Options{}
		opt.applyDefaults()
		require.Equal(t, defaultPullRetryMinInterval, opt.PullRetryMinInterval)
		require.Equal(t, defaultPullRetryMaxInterval, opt.PullRetryMaxInterval)
		require.Equal(t, defaultHeightPollMinInterval, opt.HeightPollMinInterval)
		require.Equal(t, defaultHeightPollMaxInterval, opt.HeightPollMaxInterval)
		require.NotNil(t, opt.Logger)
		require.NotNil(t, opt.TimeAfter)
	})
	t.Run("with alternatives", func(t *testing.T) {
		timeAfterFunc := func(d time.Duration) <-chan time.Time { return make(chan time.Time) }
		opt := Options{
			Logger:                flogging.MustGetLogger("test"),
			PullRetryMinInterval:  time.Microsecond,
			PullRetryMaxInterval:  time.Hour,
			HeightPollMinInterval: time.Millisecond,
			HeightPollMaxInterval: time.Minute,

			TimeAfter: timeAfterFunc,
		}
		opt.applyDefaults()
		require.Equal(t, time.Microsecond, opt.PullRetryMinInterval)
		require.Equal(t, time.Hour, opt.PullRetryMaxInterval)
		require.Equal(t, time.Millisecond, opt.HeightPollMinInterval)
		require.Equal(t, time.Minute, opt.HeightPollMaxInterval)
		require.NotNil(t, opt.Logger)
		require.NotNil(t, opt.TimeAfter)
	})
}
