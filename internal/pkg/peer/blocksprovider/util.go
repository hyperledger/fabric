/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"math"
	"time"
)

const (
	bftMinBackoffDelay           = 10 * time.Millisecond
	bftMaxBackoffDelay           = 10 * time.Second
	bftBlockRcvTotalBackoffDelay = 20 * time.Second
	bftBlockCensorshipTimeout    = 20 * time.Second
)

func backOffDuration(base float64, exponent uint, minDur, maxDur time.Duration) time.Duration {
	if base < 1.0 {
		base = 1.0
	}
	if minDur <= 0 {
		minDur = bftMinBackoffDelay
	}
	if maxDur < minDur {
		maxDur = minDur
	}

	fDurNano := math.Pow(base, float64(exponent)) * float64(minDur.Nanoseconds())
	fDurNano = math.Min(fDurNano, float64(maxDur.Nanoseconds()))
	return time.Duration(fDurNano)
}

func backOffSleep(backOffDur time.Duration, stopChan <-chan struct{}) {
	select {
	case <-time.After(backOffDur):
	case <-stopChan:
	}
}
