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
	// The default minimal retry interval when pulling blocks
	defaultPullRetryMinInterval time.Duration = 50 * time.Millisecond
	// The default maximal retry interval when pulling blocks
	defaultPullRetryMaxInterval time.Duration = 60 * time.Second

	// The default minimal height polling interval when pulling blocks after the join block
	defaultHeightPollMinInterval time.Duration = 500 * time.Millisecond
	// The default maximal height polling interval when pulling blocks after the join block
	defaultHeightPollMaxInterval time.Duration = 10 * time.Second
)

// Options contains some configuration options relevant to the follower.Chain.
type Options struct {
	Logger                *flogging.FabricLogger
	PullRetryMinInterval  time.Duration
	PullRetryMaxInterval  time.Duration
	HeightPollMinInterval time.Duration
	HeightPollMaxInterval time.Duration
	Cert                  []byte
	TimeAfter             TimeAfter // If nil, time.After is selected
}

func (o *Options) applyDefaults() {
	if o.Logger == nil {
		o.Logger = flogging.MustGetLogger("orderer.common.follower")
	}

	if o.PullRetryMinInterval <= 0 {
		o.PullRetryMinInterval = defaultPullRetryMinInterval
	}
	if o.PullRetryMaxInterval <= 0 {
		o.PullRetryMaxInterval = defaultPullRetryMaxInterval
	}
	if o.PullRetryMaxInterval > math.MaxInt64/2 {
		o.PullRetryMaxInterval = math.MaxInt64 / 2
	}
	if o.PullRetryMinInterval > o.PullRetryMaxInterval {
		o.PullRetryMaxInterval = o.PullRetryMinInterval
	}

	if o.HeightPollMinInterval <= 0 {
		o.HeightPollMinInterval = defaultHeightPollMinInterval
	}
	if o.HeightPollMaxInterval <= 0 {
		o.HeightPollMaxInterval = defaultHeightPollMaxInterval
	}
	if o.HeightPollMaxInterval > math.MaxInt64/2 {
		o.HeightPollMaxInterval = math.MaxInt64 / 2
	}
	if o.HeightPollMinInterval > o.HeightPollMaxInterval {
		o.HeightPollMaxInterval = o.HeightPollMinInterval
	}

	if o.TimeAfter == nil {
		o.TimeAfter = time.After
	}
}
