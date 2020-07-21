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
	t.Run("until join block", func(t *testing.T) {
		opt := Options{}
		opt.applyDefaults(true)
		assert.Equal(t, defaultPullRetryMinInterval, opt.PullRetryMinInterval)
		assert.Equal(t, defaultPullRetryMaxInterval, opt.PullRetryMaxInterval)
		assert.NotNil(t, opt.Logger)
		assert.NotNil(t, opt.TimeAfter)
	})
	t.Run("after join block", func(t *testing.T) {
		opt := Options{}
		opt.applyDefaults(false)
		assert.Equal(t, defaultPollRefreshMinInterval, opt.PullRetryMinInterval)
		assert.Equal(t, defaultPollRefreshMaxInterval, opt.PullRetryMaxInterval)
		assert.NotNil(t, opt.Logger)
		assert.NotNil(t, opt.TimeAfter)
	})
	t.Run("with alternatives", func(t *testing.T) {
		timeAfterFunc := func(d time.Duration) <-chan time.Time { return make(chan time.Time) }
		opt := Options{
			Logger:               flogging.MustGetLogger("test"),
			PullRetryMinInterval: time.Microsecond,
			PullRetryMaxInterval: time.Hour,
			TimeAfter:            timeAfterFunc,
		}
		opt.applyDefaults(true)
		assert.Equal(t, time.Microsecond, opt.PullRetryMinInterval)
		assert.Equal(t, time.Hour, opt.PullRetryMaxInterval)
		assert.NotNil(t, opt.Logger)
		assert.NotNil(t, opt.TimeAfter)
	})
}
