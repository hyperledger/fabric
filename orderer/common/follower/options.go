/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"math"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
)

const (
	// The default minimal retry interval when pulling blocks until the join block
	defaultPullRetryMinInterval time.Duration = 50 * time.Millisecond
	// The default maximal retry interval when pulling blocks until the join block
	defaultPullRetryMaxInterval time.Duration = 60 * time.Second

	// The default minimal polling interval when pulling blocks after the join block
	defaultPollRefreshMinInterval time.Duration = 500 * time.Millisecond
	// The default maximal polling interval when pulling blocks after the join block
	defaultPollRefreshMaxInterval time.Duration = 10 * time.Second
)

// Options contains some configuration options relevant to the follower.Chain.
type Options struct {
	Logger               *flogging.FabricLogger
	PullRetryMinInterval time.Duration
	PullRetryMaxInterval time.Duration
	Cert                 []byte
	TimeAfter            TimeAfter // If nil, time.After is selected
}

func (o *Options) applyDefaults(untilJoin bool) {
	if o.Logger == nil {
		o.Logger = flogging.MustGetLogger("orderer.common.follower")
	}

	if o.PullRetryMinInterval <= 0 {
		if untilJoin {
			o.PullRetryMinInterval = defaultPullRetryMinInterval
		} else {
			o.PullRetryMinInterval = defaultPollRefreshMinInterval
		}
	}

	if o.PullRetryMaxInterval <= 0 {
		if untilJoin {
			o.PullRetryMaxInterval = defaultPullRetryMaxInterval
		} else {
			o.PullRetryMaxInterval = defaultPollRefreshMaxInterval
		}
	}

	if o.PullRetryMaxInterval > math.MaxInt64/2 {
		o.PullRetryMaxInterval = math.MaxInt64 / 2
	}

	if o.PullRetryMinInterval > o.PullRetryMaxInterval {
		o.PullRetryMaxInterval = o.PullRetryMinInterval
	}

	if o.TimeAfter == nil {
		o.TimeAfter = time.After
	}
}
