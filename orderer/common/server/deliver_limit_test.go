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
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/stretchr/testify/require"
)

func TestNewDeliverConcurrencyLimiter(t *testing.T) {
	require.Nil(t, newDeliverConcurrencyLimiter(0, 0, &disabled.Provider{}))
	require.Nil(t, newDeliverConcurrencyLimiter(-1, -1, &disabled.Provider{}))

	limiter := newDeliverConcurrencyLimiter(80, 20, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 80, cap(limiter.clientSema))
	require.Equal(t, 20, cap(limiter.replicationSema))

	limiter = newDeliverConcurrencyLimiter(-1, 20, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Nil(t, limiter.clientSema)
	require.Equal(t, 20, cap(limiter.replicationSema))
}

func TestDeliverSessionAcquireAndRelease(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, &disabled.Provider{})

	first := newDeliverSession(limiter)
	require.NoError(t, first.Acquire())
	// acquiring again is a no-op while the permit is held
	require.NoError(t, first.Acquire())

	second := newDeliverSession(limiter)
	err := second.Acquire()
	require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")

	first.Release()
	// releasing is idempotent
	first.Release()

	require.NoError(t, second.Acquire())
}

func TestDeliverSessionReplicationBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, 1, &disabled.Provider{})

	// a client stream holds the only client permit
	reader := newDeliverSession(limiter)
	require.NoError(t, reader.Acquire())

	// a replication pull draws from its own budget while the client budget is full
	pull := newDeliverSession(limiter)
	pull.markReplication(true)
	require.NoError(t, pull.Acquire())

	// an orderer identity opening more than the replication budget is rejected
	byzantine := newDeliverSession(limiter)
	byzantine.markReplication(true)
	err := byzantine.Acquire()
	require.EqualError(t, err, "too many replication requests, exceeding concurrency limit (1)")

	pull.Release()
	require.NoError(t, byzantine.Acquire())
}

// The budget classification is evaluated per seek request; it is never
// latched for the lifetime of the stream.
func TestDeliverSessionClassificationIsPerSeek(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, 1, &disabled.Provider{})

	session := newDeliverSession(limiter)
	session.markReplication(true)
	require.NoError(t, session.Acquire())
	session.Release()

	session.markReplication(false)
	require.NoError(t, session.Acquire())

	// the replication budget is untouched by the client permit now held
	pull := newDeliverSession(limiter)
	pull.markReplication(true)
	require.NoError(t, pull.Acquire())

	// and the client budget is full for other client streams
	other := newDeliverSession(limiter)
	err := other.Acquire()
	require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")
}

func TestDeliverSessionUnlimitedWhenNoLimiter(t *testing.T) {
	session := newDeliverSession(nil)
	require.NoError(t, session.Acquire())
	require.NotPanics(t, session.Release)
}

func TestDeliverSessionUnlimitedPool(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, 1, &disabled.Provider{})

	for range 10 {
		require.NoError(t, newDeliverSession(limiter).Acquire())
	}

	pull := newDeliverSession(limiter)
	pull.markReplication(true)
	require.NoError(t, pull.Acquire())
	byzantine := newDeliverSession(limiter)
	byzantine.markReplication(true)
	require.EqualError(t, byzantine.Acquire(), "too many replication requests, exceeding concurrency limit (1)")
}

func TestDeliverSessionRejectionMetricAndThrottledLog(t *testing.T) {
	counter := &metricsfakes.Counter{}
	provider := &metricsfakes.Provider{}
	provider.NewCounterReturns(counter)
	limiter := newDeliverConcurrencyLimiter(1, -1, provider)

	logCount := 0
	limiter.throttle = &rateLimitedLogger{
		interval: time.Hour,
		logf:     func(string, ...any) { logCount++ },
	}

	held := newDeliverSession(limiter)
	require.NoError(t, held.Acquire())

	for range 3 {
		err := newDeliverSession(limiter).Acquire()
		require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")
	}
	require.Equal(t, 3, counter.AddCallCount())
	require.Equal(t, 1, logCount, "expected rejection logs to be rate limited")
}

func TestSatisfiesOrdererReadersPolicy(t *testing.T) {
	support := deliverTestChainSupport{policyManager: testDeliverPolicyManager{}}

	require.True(t, satisfiesOrdererReadersPolicy(support, deliverSeekEnvelope("orderer")))
	require.False(t, satisfiesOrdererReadersPolicy(support, deliverSeekEnvelope("reader")))
	require.False(t, satisfiesOrdererReadersPolicy(support, nil))
}

func TestNewDeliverPolicyChecker(t *testing.T) {
	support := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{state: ab.ConsensusType_STATE_NORMAL},
	}
	getChain := func(channelID string) msgprocessor.SigFilterSupport {
		if channelID != "mychannel" {
			return nil
		}
		return support
	}

	newSession := func() (*deliverSession, deliver.PolicyChecker) {
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, &disabled.Provider{}))
		return session, newDeliverPolicyChecker(getChain, session)
	}

	t.Run("reader passes and is not classified as replication", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.False(t, session.replication)
	})

	t.Run("orderer passes and is classified as replication", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.True(t, session.replication)
	})

	t.Run("classification is recomputed per seek", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.False(t, session.replication)
	})

	t.Run("unauthorized identity is rejected", func(t *testing.T) {
		session, checker := newSession()
		require.Error(t, checker.CheckPolicy(deliverSeekEnvelope("stranger"), "mychannel"))
		require.False(t, session.replication)
	})

	t.Run("unknown channel is rejected", func(t *testing.T) {
		session, checker := newSession()
		require.EqualError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "no-such-channel"), "channel no-such-channel not found")
		require.False(t, session.replication)
	})
}

// TestNewServerDeliverConcurrencyLimit exercises the production wiring path:
// load the sample config and verify the limiter is installed by default.
func TestNewServerDeliverConcurrencyLimit(t *testing.T) {
	configtest.SetDevFabricConfigPath(t)
	conf, err := localconfig.Load()
	require.NoError(t, err)
	require.Equal(t, 80, conf.General.Limits.Concurrency.DeliverService)
	require.Equal(t, 20, conf.General.Limits.Concurrency.ClusterReplication)

	s := NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		conf.General.Limits.Concurrency,
	).(*server)
	require.NotNil(t, s.deliverLimiter)
	require.Equal(t, 80, cap(s.deliverLimiter.clientSema))
	require.Equal(t, 20, cap(s.deliverLimiter.replicationSema))

	s = NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		localconfig.Concurrency{DeliverService: -1, ClusterReplication: -1},
	).(*server)
	require.Nil(t, s.deliverLimiter)
}
