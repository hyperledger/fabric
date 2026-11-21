/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-lib-go/common/metrics/metricsfakes"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/stretchr/testify/require"
)

func TestNewDeliverConcurrencyLimiter(t *testing.T) {
	require.Nil(t, newDeliverConcurrencyLimiter(0, 0, 0, &disabled.Provider{}))
	require.Nil(t, newDeliverConcurrencyLimiter(-1, -1, -1, &disabled.Provider{}))

	limiter := newDeliverConcurrencyLimiter(80, 500, 20, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 80, cap(limiter.clientSema))
	require.Equal(t, 500, cap(limiter.headerSema))
	require.Equal(t, 20, cap(limiter.replicationSema))

	limiter = newDeliverConcurrencyLimiter(-1, 500, -1, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Nil(t, limiter.clientSema)
	require.Equal(t, 500, cap(limiter.headerSema))
	require.Nil(t, limiter.replicationSema)
}

func TestDeliverSessionAcquireAndRelease(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, -1, &disabled.Provider{})

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

func TestDeliverSessionHeaderBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, 1, -1, &disabled.Provider{})

	// a client block stream holds the only block permit
	blockSession := newDeliverSession(limiter)
	require.NoError(t, blockSession.Acquire())

	// a header stream draws from its own budget while the block budget is full
	headerSession := newDeliverSession(limiter)
	headerSession.classify(deliverPoolHeader)
	require.NoError(t, headerSession.Acquire())

	// a second header stream is rejected
	anotherHeader := newDeliverSession(limiter)
	anotherHeader.classify(deliverPoolHeader)
	err := anotherHeader.Acquire()
	require.EqualError(t, err, "too many deliver header requests, exceeding concurrency limit (1)")

	// and the full header budget does not free up the block budget
	anotherBlock := newDeliverSession(limiter)
	err = anotherBlock.Acquire()
	require.EqualError(t, err, "too many deliver requests, exceeding concurrency limit (1)")
}

func TestDeliverSessionReplicationBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, 1, &disabled.Provider{})

	// a client stream holds the only client permit
	reader := newDeliverSession(limiter)
	require.NoError(t, reader.Acquire())

	// a replication pull draws from its own budget while the client budget is full
	pull := newDeliverSession(limiter)
	pull.classify(deliverPoolReplication)
	require.NoError(t, pull.Acquire())

	// an orderer identity opening more than the replication budget is rejected
	byzantine := newDeliverSession(limiter)
	byzantine.classify(deliverPoolReplication)
	err := byzantine.Acquire()
	require.EqualError(t, err, "too many replication requests, exceeding concurrency limit (1)")

	pull.Release()
	require.NoError(t, byzantine.Acquire())
}

// The budget classification is evaluated per seek request; it is never
// latched for the lifetime of the stream.
func TestDeliverSessionClassificationIsPerSeek(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, 1, &disabled.Provider{})

	session := newDeliverSession(limiter)
	session.classify(deliverPoolReplication)
	require.NoError(t, session.Acquire())
	session.Release()

	session.classify(deliverPoolBlock)
	require.NoError(t, session.Acquire())

	// the replication budget is untouched by the client permit now held
	pull := newDeliverSession(limiter)
	pull.classify(deliverPoolReplication)
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
	limiter := newDeliverConcurrencyLimiter(-1, -1, 1, &disabled.Provider{})

	for range 10 {
		require.NoError(t, newDeliverSession(limiter).Acquire())
	}

	pull := newDeliverSession(limiter)
	pull.classify(deliverPoolReplication)
	require.NoError(t, pull.Acquire())
	byzantine := newDeliverSession(limiter)
	byzantine.classify(deliverPoolReplication)
	require.EqualError(t, byzantine.Acquire(), "too many replication requests, exceeding concurrency limit (1)")
}

func TestDeliverSessionRejectionMetricAndThrottledLog(t *testing.T) {
	counter := &metricsfakes.Counter{}
	provider := &metricsfakes.Provider{}
	provider.NewCounterReturns(counter)
	limiter := newDeliverConcurrencyLimiter(1, -1, -1, provider)

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

func TestSeekContentType(t *testing.T) {
	require.Equal(t, ab.SeekInfo_HEADER_WITH_SIG, seekContentType(deliverHeaderSeekEnvelope("reader")))
	require.Equal(t, ab.SeekInfo_BLOCK, seekContentType(deliverSeekEnvelope("reader")))
	require.Equal(t, ab.SeekInfo_BLOCK, seekContentType(&cb.Envelope{Payload: []byte("garbage")}))
}

func TestSatisfiesOrdererReadersPolicy(t *testing.T) {
	support := deliverTestChainSupport{policyManager: testDeliverPolicyManager{}}

	require.True(t, satisfiesOrdererReadersPolicy(support, deliverSeekEnvelope("orderer")))
	require.False(t, satisfiesOrdererReadersPolicy(support, deliverSeekEnvelope("reader")))
	require.False(t, satisfiesOrdererReadersPolicy(support, nil))
}

// deliverLimitTestCerts returns the x509 certificates of an orderer client
// and an unrelated client from the same CA.
func deliverLimitTestCerts(t *testing.T) (ordererCert, otherCert *x509.Certificate, ordererCertPEM []byte) {
	t.Helper()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	ordererKp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	otherKp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	return ordererKp.TLSCert, otherKp.TLSCert, ordererKp.Cert
}

func TestIsClusterReplication(t *testing.T) {
	ordererCert, otherCert, ordererCertPEM := deliverLimitTestCerts(t)
	support := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{
			state:      ab.ConsensusType_STATE_NORMAL,
			consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM, ServerTlsCert: ordererCertPEM}},
		},
	}

	// a consenter client certificate with an orderer-signed envelope is replication
	require.True(t, isClusterReplication(support, ordererCert, deliverSeekEnvelope("orderer")))
	// the same envelope from another certificate is not, even though the
	// identity satisfies the orderer readers policy (overlapping MSP)
	require.False(t, isClusterReplication(support, otherCert, deliverSeekEnvelope("orderer")))
	// without mutual TLS there is no client certificate to bind
	require.False(t, isClusterReplication(support, nil, deliverSeekEnvelope("orderer")))
	// a reader-signed envelope is not replication even from a consenter
	require.False(t, isClusterReplication(support, ordererCert, deliverSeekEnvelope("reader")))

	// consenter certificates stored in DER form are normalized as well
	derSupport := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{
			state:      ab.ConsensusType_STATE_NORMAL,
			consenters: []*cb.Consenter{{ClientTlsCert: ordererCert.Raw}},
		},
	}
	require.True(t, isClusterReplication(derSupport, ordererCert, deliverSeekEnvelope("orderer")))
}

func TestNewDeliverPolicyChecker(t *testing.T) {
	ordererCert, _, ordererCertPEM := deliverLimitTestCerts(t)
	support := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{
			state:      ab.ConsensusType_STATE_NORMAL,
			consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM}},
		},
	}
	getChain := func(channelID string) msgprocessor.SigFilterSupport {
		if channelID != "mychannel" {
			return nil
		}
		return support
	}

	newSession := func() (*deliverSession, deliver.PolicyChecker, *x509.Certificate) {
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, &disabled.Provider{}))
		return session, newDeliverPolicyChecker(getChain, ordererCert, session), ordererCert
	}

	t.Run("reader is classified into the block budget", func(t *testing.T) {
		session, checker, _ := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("header seek is classified into the header budget", func(t *testing.T) {
		session, checker, _ := newSession()
		require.NoError(t, checker.CheckPolicy(deliverHeaderSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolHeader, session.pool)
	})

	t.Run("orderer with consenter certificate is classified into the replication budget", func(t *testing.T) {
		session, checker, _ := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.Equal(t, deliverPoolReplication, session.pool)
	})

	t.Run("orderer without a client certificate falls back to the block budget", func(t *testing.T) {
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, &disabled.Provider{}))
		checker := newDeliverPolicyChecker(getChain, nil, session)
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("classification is recomputed per seek", func(t *testing.T) {
		session, checker, _ := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("unauthorized identity is rejected", func(t *testing.T) {
		session, checker, _ := newSession()
		require.Error(t, checker.CheckPolicy(deliverSeekEnvelope("stranger"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("unknown channel is rejected", func(t *testing.T) {
		_, checker, _ := newSession()
		require.EqualError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "no-such-channel"), "channel no-such-channel not found")
	})
}

// TestNewServerDeliverConcurrencyLimit exercises the production wiring path:
// load the sample config and verify the limiter is installed by default.
func TestNewServerDeliverConcurrencyLimit(t *testing.T) {
	configtest.SetDevFabricConfigPath(t)
	conf, err := localconfig.Load()
	require.NoError(t, err)
	require.Equal(t, 80, conf.General.Limits.Concurrency.DeliverService)
	require.Equal(t, 500, conf.General.Limits.Concurrency.DeliverHeaderService)
	require.Equal(t, -1, conf.General.Limits.Concurrency.ClusterReplication)

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
	require.Equal(t, 500, cap(s.deliverLimiter.headerSema))
	require.Nil(t, s.deliverLimiter.replicationSema)

	s = NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		localconfig.Concurrency{DeliverService: -1, DeliverHeaderService: -1, ClusterReplication: -1},
	).(*server)
	require.Nil(t, s.deliverLimiter)
}
