/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"crypto/x509"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-lib-go/common/metrics/metricsfakes"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestNewDeliverConcurrencyLimiter(t *testing.T) {
	// zero values normalize to the configuration defaults
	limiter := newDeliverConcurrencyLimiter(0, 0, 0, 0, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 500, cap(limiter.preAuthSema))
	require.Equal(t, 100, cap(limiter.clientSema))
	require.Equal(t, 1000, cap(limiter.headerSema))
	require.Nil(t, limiter.replicationSema)
	// unlimited replication: recognized consenters bypass the full pre-auth
	// budget instead of drawing from a sized reserve
	require.Nil(t, limiter.consenterPreAuthSema)

	require.Nil(t, newDeliverConcurrencyLimiter(-1, -1, -1, -1, &disabled.Provider{}))

	limiter = newDeliverConcurrencyLimiter(80, 500, 20, -1, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 80, cap(limiter.clientSema))
	require.Equal(t, 500, cap(limiter.headerSema))
	require.Equal(t, 20, cap(limiter.replicationSema))
	// no pre-auth budget, no reserve
	require.Nil(t, limiter.consenterPreAuthSema)

	// the consenter pre-auth reserve is sized by the replication budget
	limiter = newDeliverConcurrencyLimiter(80, 500, 20, 500, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 20, cap(limiter.consenterPreAuthSema))

	limiter = newDeliverConcurrencyLimiter(-1, 500, -1, -1, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Nil(t, limiter.clientSema)
	require.Equal(t, 500, cap(limiter.headerSema))
	require.Nil(t, limiter.replicationSema)
}

func TestDeliverSessionAcquireAndRelease(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, -1, -1, &disabled.Provider{})

	first := newDeliverSession(limiter)
	require.NoError(t, first.Acquire())
	// acquiring again is a no-op while the permit is held
	require.NoError(t, first.Acquire())

	second := newDeliverSession(limiter)
	err := second.Acquire()
	require.EqualError(t, err, "too many block requests, exceeding concurrency limit (1)")

	first.Release()
	// releasing is idempotent
	first.Release()

	require.NoError(t, second.Acquire())
}

func TestDeliverSessionPreAuthBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, -1, -1, 1, &disabled.Provider{})

	// the first silent stream holds the only pre-auth permit
	silent := newDeliverSession(limiter)
	require.NoError(t, silent.AcquirePreAuth())

	// the second silent stream is rejected
	other := newDeliverSession(limiter)
	err := other.AcquirePreAuth()
	require.EqualError(t, err, "too many preauth requests, exceeding concurrency limit (1)")

	// admitting into a request class returns the pre-auth permit
	require.NoError(t, silent.Acquire())
	require.NoError(t, other.AcquirePreAuth())

	// and releasing the stream returns everything
	silent.Release()
	other.Release()
	require.NoError(t, newDeliverSession(limiter).Acquire())
}

// A stream presenting a consenter certificate is admitted through the
// reserved allowance when unauthenticated streams have exhausted the
// shared pre-auth budget; other streams are still rejected.
func TestDeliverSessionConsenterPreAuthReserve(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, -1, 1, 1, &disabled.Provider{})

	// a silent stream fills the shared pre-auth budget
	silent := newDeliverSession(limiter)
	require.NoError(t, silent.AcquirePreAuth())

	// an unauthenticated flood stream is rejected
	flood := newDeliverSession(limiter)
	require.EqualError(t, flood.AcquirePreAuth(), "too many preauth requests, exceeding concurrency limit (1)")

	// a stream presenting a consenter certificate falls back to the
	// reserved allowance
	consenter := newDeliverSession(limiter)
	consenter.recognizeConsenter = func() bool { return true }
	require.NoError(t, consenter.AcquirePreAuth())
	require.Equal(t, consenterPreAuthPoolName, consenter.preAuthPool)

	// the reserve is bounded as well: it is sized by the replication budget
	second := newDeliverSession(limiter)
	second.recognizeConsenter = func() bool { return true }
	require.EqualError(t, second.AcquirePreAuth(), "too many consenter-preauth requests, exceeding concurrency limit (1)")

	// the reserved slot is returned once the consenter stream is admitted
	// into its request class
	consenter.classify(deliverPoolReplication)
	require.NoError(t, consenter.Acquire())
	require.NoError(t, second.AcquirePreAuth())

	// a consenter stream admitted while the shared budget has room uses
	// the shared budget, keeping the reserve free
	silent.Release()
	second.Release()
	third := newDeliverSession(limiter)
	third.recognizeConsenter = func() bool { return true }
	require.NoError(t, third.AcquirePreAuth())
	require.Equal(t, preAuthPoolName, third.preAuthPool)
}

// A consenter stream admitted through the reserved allowance is not
// counted as a rejection: operators alert on deliver_sessions_rejected,
// and a healthy consenter reconnect burst must not trigger the alert.
func TestDeliverSessionConsenterPreAuthReserveNotCounted(t *testing.T) {
	counter := &metricsfakes.Counter{}
	counter.WithReturns(counter)
	gauge := &metricsfakes.Gauge{}
	gauge.WithReturns(gauge)
	provider := &metricsfakes.Provider{}
	provider.NewCounterReturns(counter)
	provider.NewGaugeReturns(gauge)

	limiter := newDeliverConcurrencyLimiter(-1, -1, 1, 1, provider)

	silent := newDeliverSession(limiter)
	require.NoError(t, silent.AcquirePreAuth())

	consenter := newDeliverSession(limiter)
	consenter.recognizeConsenter = func() bool { return true }
	require.NoError(t, consenter.AcquirePreAuth())
	require.Equal(t, consenterPreAuthPoolName, consenter.preAuthPool)

	require.Zero(t, counter.AddCallCount(),
		"an admitted consenter stream must not be counted as a rejected session")

	// a genuine rejection past both budgets is counted
	other := newDeliverSession(limiter)
	other.recognizeConsenter = func() bool { return true }
	require.Error(t, other.AcquirePreAuth())
	require.Equal(t, 1, counter.AddCallCount())
	require.Equal(t, []string{"pool", consenterPreAuthPoolName}, counter.WithArgsForCall(0))

	flood := newDeliverSession(limiter)
	require.Error(t, flood.AcquirePreAuth())
	require.Equal(t, []string{"pool", preAuthPoolName}, counter.WithArgsForCall(1))
}

// With an unlimited replication budget, recognized consenters bypass the
// exhausted shared pre-auth budget entirely.
func TestDeliverSessionConsenterPreAuthReserveUnlimitedReplication(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, -1, -1, 1, &disabled.Provider{})

	silent := newDeliverSession(limiter)
	require.NoError(t, silent.AcquirePreAuth())

	consenter := newDeliverSession(limiter)
	consenter.recognizeConsenter = func() bool { return true }
	require.NoError(t, consenter.AcquirePreAuth())

	flood := newDeliverSession(limiter)
	require.Error(t, flood.AcquirePreAuth())
}

func TestDeliverSessionComplete(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, -1, 1, &disabled.Provider{})

	session := newDeliverSession(limiter)
	require.NoError(t, session.AcquirePreAuth())
	require.NoError(t, session.Acquire())
	require.Nil(t, session.preAuth, "the pre-auth permit is returned while serving")

	// completing the delivery returns the serving permit and re-acquires
	// the pre-auth permit for the wait
	require.NoError(t, session.Complete())
	require.Nil(t, session.sema)
	require.False(t, session.acquired)
	require.NotNil(t, session.preAuth)

	// a second waiter is bounded out while the first waits
	other := newDeliverSession(limiter)
	require.EqualError(t, other.AcquirePreAuth(), "too many preauth requests, exceeding concurrency limit (1)")

	// the next seek returns to the serving budget and frees the wait slot
	require.NoError(t, session.Acquire())
	require.Nil(t, session.preAuth)
	require.NoError(t, other.AcquirePreAuth())
}

func TestDeliverSessionCompleteRejectedWhenWaitBudgetFull(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, -1, -1, 1, &disabled.Provider{})

	other := newDeliverSession(limiter)
	require.NoError(t, other.AcquirePreAuth())

	session := newDeliverSession(limiter)
	require.NoError(t, session.Acquire())
	err := session.Complete()
	require.EqualError(t, err, "too many preauth requests, exceeding concurrency limit (1)")
	// the serving permit was returned regardless
	require.Nil(t, session.sema)
	require.False(t, session.acquired)
}

func TestDeliverSessionHeaderBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, 1, -1, -1, &disabled.Provider{})

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
	require.EqualError(t, err, "too many header requests, exceeding concurrency limit (1)")

	// and the full header budget does not free up the block budget
	anotherBlock := newDeliverSession(limiter)
	err = anotherBlock.Acquire()
	require.EqualError(t, err, "too many block requests, exceeding concurrency limit (1)")
}

func TestDeliverSessionReplicationBudget(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(1, -1, 1, -1, &disabled.Provider{})

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

func TestDeliverSessionUnlimitedWhenNoLimiter(t *testing.T) {
	session := newDeliverSession(nil)
	require.NoError(t, session.Acquire())
	require.NotPanics(t, session.Release)
}

func TestDeliverSessionUnlimitedPool(t *testing.T) {
	limiter := newDeliverConcurrencyLimiter(-1, -1, 1, -1, &disabled.Provider{})

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
	counter.WithReturns(counter)
	gauge := &metricsfakes.Gauge{}
	gauge.WithReturns(gauge)
	provider := &metricsfakes.Provider{}
	provider.NewCounterReturns(counter)
	provider.NewGaugeReturns(gauge)
	limiter := newDeliverConcurrencyLimiter(1, -1, -1, -1, provider)

	logCount := 0
	limiter.throttles["block"] = &rateLimitedLogger{
		interval: time.Hour,
		logf:     func(string, ...any) { logCount++ },
	}

	held := newDeliverSession(limiter)
	require.NoError(t, held.Acquire())
	require.Equal(t, []string{"pool", "block"}, gauge.WithArgsForCall(0))

	for range 3 {
		err := newDeliverSession(limiter).Acquire()
		require.EqualError(t, err, "too many block requests, exceeding concurrency limit (1)")
	}
	require.Equal(t, 3, counter.AddCallCount())
	require.Equal(t, []string{"pool", "block"}, counter.WithArgsForCall(0))
	require.Equal(t, 1, logCount, "expected rejection logs to be rate limited")
}

// deliverSeekPayload unmarshals the payload of a test seek envelope.
func deliverSeekPayload(t *testing.T, env *cb.Envelope) *cb.Payload {
	t.Helper()
	payload, err := protoutil.UnmarshalPayload(env.GetPayload())
	require.NoError(t, err)
	return payload
}

func TestSeekContentType(t *testing.T) {
	require.Equal(t, ab.SeekInfo_HEADER_WITH_SIG, seekContentType(deliverSeekPayload(t, deliverHeaderSeekEnvelope("reader"))))
	require.Equal(t, ab.SeekInfo_BLOCK, seekContentType(deliverSeekPayload(t, deliverSeekEnvelope("reader"))))
	require.Equal(t, ab.SeekInfo_BLOCK, seekContentType(nil))
}

func TestSatisfiesOrdererReadersPolicy(t *testing.T) {
	support := deliverTestChainSupport{policyManager: testDeliverPolicyManager{}}

	ordererEnv := deliverSeekEnvelope("orderer")
	require.True(t, satisfiesOrdererReadersPolicy(support, deliverSeekPayload(t, ordererEnv), ordererEnv))
	readerEnv := deliverSeekEnvelope("reader")
	require.False(t, satisfiesOrdererReadersPolicy(support, deliverSeekPayload(t, readerEnv), readerEnv))
	require.False(t, satisfiesOrdererReadersPolicy(support, nil, nil))
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
	isReplication := func(support deliverTestChainSupport, cert *x509.Certificate, env *cb.Envelope) bool {
		ordererConf, ok := support.OrdererConfig()
		require.True(t, ok)
		return isClusterReplication(support, ordererConf, cert, deliverSeekPayload(t, env), env)
	}

	// a consenter client certificate with an orderer-signed envelope is replication
	require.True(t, isReplication(support, ordererCert, deliverSeekEnvelope("orderer")))
	// the same envelope from another certificate is not, even though the
	// identity satisfies the orderer readers policy (overlapping MSP)
	require.False(t, isReplication(support, otherCert, deliverSeekEnvelope("orderer")))
	// without a presented client certificate there is nothing to bind
	require.False(t, isReplication(support, nil, deliverSeekEnvelope("orderer")))
	// a reader-signed envelope is not replication even from a consenter
	require.False(t, isReplication(support, ordererCert, deliverSeekEnvelope("reader")))

	// consenter certificates stored in DER form are normalized as well
	derSupport := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{
			state:      ab.ConsensusType_STATE_NORMAL,
			consenters: []*cb.Consenter{{ClientTlsCert: ordererCert.Raw}},
		},
	}
	require.True(t, isReplication(derSupport, ordererCert, deliverSeekEnvelope("orderer")))

	// etcdraft channels carry no Orderers value; their consenters live in
	// the consensus metadata
	raftSupport := deliverTestChainSupport{
		policyManager: testDeliverPolicyManager{},
		ordererConfig: stubOrdererConfig{
			state:         ab.ConsensusType_STATE_NORMAL,
			consensusType: "etcdraft",
			consensusMetadata: protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{
				Consenters: []*etcdraft.Consenter{{ClientTlsCert: ordererCertPEM, ServerTlsCert: ordererCertPEM}},
			}),
		},
	}
	require.True(t, isReplication(raftSupport, ordererCert, deliverSeekEnvelope("orderer")))
	require.False(t, isReplication(raftSupport, otherCert, deliverSeekEnvelope("orderer")))
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

	newSession := func() (*deliverSession, deliver.PolicyChecker) {
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, -1, &disabled.Provider{}))
		return session, newDeliverPolicyChecker(getChain, ordererCert, session)
	}

	t.Run("reader is classified into the block budget", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("header seek is classified into the header budget", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverHeaderSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolHeader, session.pool)
	})

	t.Run("orderer with consenter certificate is classified into the replication budget", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.Equal(t, deliverPoolReplication, session.pool)
	})

	t.Run("orderer without a client certificate falls back to the block budget", func(t *testing.T) {
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, -1, &disabled.Provider{}))
		checker := newDeliverPolicyChecker(getChain, nil, session)
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("classification is recomputed per seek", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("raft consenter certificate is classified into the replication budget", func(t *testing.T) {
		raftSupport := deliverTestChainSupport{
			policyManager: testDeliverPolicyManager{},
			ordererConfig: stubOrdererConfig{
				state:         ab.ConsensusType_STATE_NORMAL,
				consensusType: "etcdraft",
				consensusMetadata: protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{
					Consenters: []*etcdraft.Consenter{{ClientTlsCert: ordererCertPEM}},
				}),
			},
		}
		session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, -1, &disabled.Provider{}))
		checker := newDeliverPolicyChecker(func(string) msgprocessor.SigFilterSupport { return raftSupport }, ordererCert, session)
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.Equal(t, deliverPoolReplication, session.pool)
	})

	t.Run("classification change while serving requires readmission", func(t *testing.T) {
		session, checker := newSession()
		require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
		require.NoError(t, session.Acquire())
		// the policy check re-runs after a channel configuration update; if
		// the request class changed, the seek is terminated instead of
		// transferring the permit between budgets
		err := checker.CheckPolicy(deliverSeekEnvelope("reader"), "mychannel")
		require.ErrorContains(t, err, "resubmit the seek request")
		session.Release()
	})

	t.Run("unauthorized identity is rejected", func(t *testing.T) {
		session, checker := newSession()
		require.Error(t, checker.CheckPolicy(deliverSeekEnvelope("stranger"), "mychannel"))
		require.Equal(t, deliverPoolBlock, session.pool)
	})

	t.Run("unknown channel is rejected", func(t *testing.T) {
		_, checker := newSession()
		require.EqualError(t, checker.CheckPolicy(deliverSeekEnvelope("reader"), "no-such-channel"), "channel no-such-channel not found")
	})
}

// In maintenance mode the signature filter already evaluates
// /Channel/Orderer/Readers, so classification must not evaluate it again:
// each evaluation verifies signatures.
func TestDeliverPolicyCheckerEvaluatesOrdererReadersOnce(t *testing.T) {
	_, otherCert, ordererCertPEM := deliverLimitTestCerts(t)

	calls := 0
	policyManager := countingPolicyManager{policies: map[string]policies.Policy{
		policies.ChannelReaders:        countingPolicy{Policy: identitiesContaining{"orderer"}, calls: &calls},
		policies.ChannelOrdererReaders: countingPolicy{Policy: identitiesContaining{"orderer"}, calls: &calls},
	}}
	support := deliverTestChainSupport{
		policyManager: policyManager,
		ordererConfig: stubOrdererConfig{
			state:      ab.ConsensusType_STATE_MAINTENANCE,
			consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM}},
		},
	}

	session := newDeliverSession(newDeliverConcurrencyLimiter(1, 1, 1, -1, &disabled.Provider{}))
	checker := newDeliverPolicyChecker(func(string) msgprocessor.SigFilterSupport { return support }, otherCert, session)
	require.NoError(t, checker.CheckPolicy(deliverSeekEnvelope("orderer"), "mychannel"))
	require.Equal(t, 1, calls, "/Channel/Orderer/Readers is evaluated once per seek in maintenance mode")
	require.Equal(t, deliverPoolBlock, session.pool, "the certificate is not a consenter's")
}

type countingPolicy struct {
	policies.Policy
	calls *int
}

func (p countingPolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	*p.calls++
	return p.Policy.EvaluateSignedData(signatureSet)
}

type countingPolicyManager struct {
	policies map[string]policies.Policy
}

func (m countingPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	policy, ok := m.policies[id]
	return policy, ok
}

func (m countingPolicyManager) Manager([]string) (policies.Manager, bool) {
	return nil, false
}

func TestSeekTimeoutReceiver(t *testing.T) {
	t.Run("times out every idle wait and rate-limits the warning", func(t *testing.T) {
		blocking := &blockingReceiver{done: make(chan struct{})}
		defer close(blocking.done)
		logCount := 0
		receiver := &seekTimeoutReceiver{
			Receiver: blocking,
			timeout:  20 * time.Millisecond,
			throttle: &rateLimitedLogger{interval: time.Hour, logf: func(string, ...any) { logCount++ }},
		}
		start := time.Now()
		_, err := receiver.Recv()
		require.ErrorContains(t, err, "no seek request received within 20ms")
		require.Less(t, time.Since(start), 10*time.Second)
		// the error is pre-logged, so the handler must not warn about it again
		var logged deliver.RecvErrorLogged
		require.ErrorAs(t, err, &logged)
		// the timeout re-arms on the next wait, not only the first
		_, err = receiver.Recv()
		require.ErrorContains(t, err, "no seek request received within 20ms")
		require.Equal(t, 1, logCount, "expected timeout warnings to be rate limited")
	})

	t.Run("passes messages through", func(t *testing.T) {
		stub := &stubReceiver{env: &cb.Envelope{Payload: []byte("x")}}
		receiver := &seekTimeoutReceiver{
			Receiver: stub,
			timeout:  time.Hour,
		}
		env, err := receiver.Recv()
		require.NoError(t, err)
		require.Same(t, stub.env, env)
		_, err = receiver.Recv()
		require.NoError(t, err)
	})

	t.Run("disabled by non-positive timeout", func(t *testing.T) {
		stub := &stubReceiver{err: io.EOF}
		receiver := &seekTimeoutReceiver{
			Receiver: stub,
			timeout:  -1,
		}
		_, err := receiver.Recv()
		require.ErrorIs(t, err, io.EOF)
	})
}

type blockingReceiver struct {
	done chan struct{}
}

func (b *blockingReceiver) Recv() (*cb.Envelope, error) {
	<-b.done
	return nil, io.EOF
}

type stubReceiver struct {
	env *cb.Envelope
	err error
}

func (s *stubReceiver) Recv() (*cb.Envelope, error) {
	return s.env, s.err
}

// TestNewServerDeliverConcurrencyLimit exercises the production wiring path:
// load the sample config and verify the limiter is installed by default.
func TestNewServerDeliverConcurrencyLimit(t *testing.T) {
	configtest.SetDevFabricConfigPath(t)
	conf, err := localconfig.Load()
	require.NoError(t, err)
	require.Equal(t, 100, conf.General.Limits.Concurrency.DeliverService)
	require.Equal(t, 1000, conf.General.Limits.Concurrency.DeliverHeaderService)
	require.Equal(t, -1, conf.General.Limits.Concurrency.ClusterReplication)
	require.Equal(t, 30*time.Second, conf.General.Limits.DeliverSeekTimeout)

	s := NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		conf.General.Limits,
	).(*server)
	require.NotNil(t, s.deliverLimiter)
	require.Equal(t, 100, cap(s.deliverLimiter.clientSema))
	require.Equal(t, 1000, cap(s.deliverLimiter.headerSema))
	require.Nil(t, s.deliverLimiter.replicationSema)
	require.Equal(t, 30*time.Second, s.deliverSeekTimeout)
	require.NotNil(t, s.seekTimeoutLog)
	require.NotNil(t, s.channelNames)

	s = NewServer(
		&multichannel.Registrar{},
		&disabled.Provider{},
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		false,
		true,
		localconfig.Limits{Concurrency: localconfig.Concurrency{DeliverService: -1, DeliverHeaderService: -1, ClusterReplication: -1, DeliverPreAuth: -1}},
	).(*server)
	require.Nil(t, s.deliverLimiter)
}

func makeConfigBlock(num uint64, size int) *cb.Block {
	return &cb.Block{
		Header: &cb.BlockHeader{Number: num},
		Data: &cb.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)}),
				},
				Data: make([]byte, size),
			}),
		})}},
	}
}
