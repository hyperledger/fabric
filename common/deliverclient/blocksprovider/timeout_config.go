/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import "time"

const (
	BftMinRetryInterval       = 50 * time.Millisecond
	BftMaxRetryInterval       = 10 * time.Second
	BftBlockCensorshipTimeout = 20 * time.Second
)

type TimeoutConfig struct {
	// The initial value of the actual retry interval, which is increased on every failed retry
	MinRetryInterval time.Duration
	// The maximal value of the actual retry interval, which cannot increase beyond this value
	MaxRetryInterval time.Duration
	// The value of the bft censorship detection timeout
	BlockCensorshipTimeout time.Duration
}

func (t *TimeoutConfig) ApplyDefaults() {
	if t.MinRetryInterval <= 0 {
		t.MinRetryInterval = BftMinRetryInterval
	}
	if t.MaxRetryInterval <= 0 {
		t.MaxRetryInterval = BftMaxRetryInterval
	}
	if t.BlockCensorshipTimeout <= 0 {
		t.BlockCensorshipTimeout = BftBlockCensorshipTimeout
	}
}
