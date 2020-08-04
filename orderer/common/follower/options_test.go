/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestOptions_applyDefaults(t *testing.T) {
	t.Run("on empty object", func(t *testing.T) {
		opt := Options{}
		opt.applyDefaults()
		assert.Equal(t, defaultPullRetryMinInterval, opt.PullRetryMinInterval)
		assert.Equal(t, defaultPullRetryMaxInterval, opt.PullRetryMaxInterval)
		assert.Equal(t, defaultHeightPollMinInterval, opt.HeightPollMinInterval)
		assert.Equal(t, defaultHeightPollMaxInterval, opt.HeightPollMaxInterval)
		assert.NotNil(t, opt.Logger)
		assert.NotNil(t, opt.TimeAfter)
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
		assert.Equal(t, time.Microsecond, opt.PullRetryMinInterval)
		assert.Equal(t, time.Hour, opt.PullRetryMaxInterval)
		assert.Equal(t, time.Millisecond, opt.HeightPollMinInterval)
		assert.Equal(t, time.Minute, opt.HeightPollMaxInterval)
		assert.NotNil(t, opt.Logger)
		assert.NotNil(t, opt.TimeAfter)
	})
}
