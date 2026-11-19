/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-lib-go/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/stretchr/testify/require"
)

func TestNewDeliverConcurrencyLimiter(t *testing.T) {
	require.Nil(t, newDeliverConcurrencyLimiter(0, &disabled.Provider{}))
	require.Nil(t, newDeliverConcurrencyLimiter(-1, &disabled.Provider{}))

	limiter := newDeliverConcurrencyLimiter(100, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 100, cap(limiter.sema))
}

func TestDeliverSessionAcquireAndRelease(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, &disabled.Provider{})

	first := limiter.newSession()
	require.NoError(t, first.Acquire())
	// acquiring again is a no-op while the permit is held
	require.NoError(t, first.Acquire())

	second := limiter.newSession()
	err := second.Acquire()
	require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")

	first.Release()
	// releasing is idempotent
	first.Release()

	require.NoError(t, second.Acquire())

	// exempt sessions never consume the budget
	exempt := limiter.newSession()
	exempt.Exempt()
	require.NoError(t, exempt.Acquire())
	require.NotPanics(t, exempt.Release)
}

func TestDeliverSessionUnlimitedWhenNoLimiter(t *testing.T) {
	session := newDeliverConcurrencyLimiter(-1, &disabled.Provider{}).newSession()
	require.NoError(t, session.Acquire())
	require.NotPanics(t, session.Release)
}

func TestDeliverSessionRejectionMetricAndThrottledLog(t *testing.T) {
	counter := &metricsfakes.Counter{}
	provider := &metricsfakes.Provider{}
	provider.NewCounterReturns(counter)
	limiter := newDeliverConcurrencyLimiter(1, provider)

	logCount := 0
	limiter.throttle = &rateLimitedLogger{
		interval: time.Hour,
		logf:     func(string, ...any) { logCount++ },
	}

	held := limiter.newSession()
	require.NoError(t, held.Acquire())

	for range 3 {
		err := limiter.newSession().Acquire()
		require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")
	}
	require.Equal(t, 3, counter.AddCallCount())
	require.Equal(t, 1, logCount, "expected rejection logs to be rate limited")
}

func TestSatisfiesOrdererReadersPolicy(t *testing.T) {
	chain := deliverChainWithOrdererPolicy()

	require.True(t, satisfiesOrdererReadersPolicy(chain, deliverSeekEnvelope("orderer")))
	require.False(t, satisfiesOrdererReadersPolicy(chain, deliverSeekEnvelope("reader")))
	require.False(t, satisfiesOrdererReadersPolicy(chain, nil))
}

// TestNewServerDeliverConcurrencyLimit exercises the production wiring path:
// load the sample config and verify the limiter is installed by default.
func TestNewServerDeliverConcurrencyLimit(t *testing.T) {
	configtest.SetDevFabricConfigPath(t)
	conf, err := localconfig.Load()
	require.NoError(t, err)
	require.Equal(t, 2500, conf.General.Limits.Concurrency.DeliverService)

	s := NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		conf.General.Limits.Concurrency.DeliverService,
	).(*server)
	require.NotNil(t, s.deliverLimiter)
	require.Equal(t, 2500, cap(s.deliverLimiter.sema))

	s = NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		-1,
	).(*server)
	require.Nil(t, s.deliverLimiter)
}
