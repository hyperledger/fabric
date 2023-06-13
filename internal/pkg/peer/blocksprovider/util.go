/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"math"
	"math/rand"
	"time"

	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
)

const (
	bftMinBackoffDelay           = 10 * time.Millisecond
	bftMaxBackoffDelay           = 10 * time.Second
	bftBlockRcvTotalBackoffDelay = 20 * time.Second
	bftBlockCensorshipTimeout    = 20 * time.Second
)

type errRefreshEndpoint struct {
	message string
}

func (e *errRefreshEndpoint) Error() string {
	return e.message
}

type errStopping struct {
	message string
}

func (e *errStopping) Error() string {
	return e.message
}

type errFatal struct {
	message string
}

func (e *errFatal) Error() string {
	return e.message
}

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

// shuffle the endpoint slice
func shuffle(a []*orderers.Endpoint) []*orderers.Endpoint {
	n := len(a)
	returnedSlice := make([]*orderers.Endpoint, n)
	indices := rand.Perm(n)
	for i, idx := range indices {
		returnedSlice[i] = a[idx]
	}
	return returnedSlice
}
