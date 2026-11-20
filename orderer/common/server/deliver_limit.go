/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var deliverSessionsRejected = metrics.CounterOpts{
	Namespace: "deliver",
	Name:      "sessions_rejected",
	Help:      "The number of deliver sessions rejected because the deliver concurrency limit was reached.",
}

// deliverConcurrencyLimiter bounds the number of concurrent authenticated
// deliver sessions. Client streams and orderer-to-orderer replication pulls
// draw from separate finite budgets, so client floods cannot starve
// replication and compromised orderer identities cannot exceed the
// replication budget. Streams consume a budget only while a seek request is
// being served, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	clientSema      semaphore.Semaphore // nil = unlimited
	replicationSema semaphore.Semaphore // nil = unlimited
	rejected        metrics.Counter
	throttle        *rateLimitedLogger
}

// newDeliverConcurrencyLimiter returns nil when both limits are not
// positive (unlimited).
func newDeliverConcurrencyLimiter(deliverLimit, replicationLimit int, metricsProvider metrics.Provider) *deliverConcurrencyLimiter {
	if deliverLimit <= 0 && replicationLimit <= 0 {
		return nil
	}
	limiter := &deliverConcurrencyLimiter{
		rejected: metricsProvider.NewCounter(deliverSessionsRejected),
		throttle: &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf},
	}
	if deliverLimit > 0 {
		limiter.clientSema = semaphore.New(deliverLimit)
	}
	if replicationLimit > 0 {
		limiter.replicationSema = semaphore.New(replicationLimit)
	}
	return limiter
}

// newDeliverSession returns a session bound to the limiter; a nil limiter
// yields a session whose Acquire always succeeds.
func newDeliverSession(limiter *deliverConcurrencyLimiter) *deliverSession {
	return &deliverSession{limiter: limiter}
}

// deliverSession tracks a single stream's permit, acquired when a seek
// request starts being served and released when the delivery completes or
// the stream ends.
type deliverSession struct {
	limiter     *deliverConcurrencyLimiter
	replication bool // the current seek draws from the replication budget
	sema        semaphore.Semaphore
}

// markReplication classifies the next admission decision. It is evaluated
// per seek request, never latched for the lifetime of the stream.
func (s *deliverSession) markReplication(replication bool) {
	s.replication = replication
}

// Acquire admits the session into the budget matching its classification,
// and is a no-op while the session holds a permit or when no limit is
// configured for its budget.
func (s *deliverSession) Acquire() error {
	if s.limiter == nil || s.sema != nil {
		return nil
	}
	sema := s.limiter.clientSema
	pool := "deliver"
	if s.replication {
		sema = s.limiter.replicationSema
		pool = "replication"
	}
	if sema == nil {
		return nil
	}
	if sema.TryAcquire() {
		s.sema = sema
		return nil
	}
	s.limiter.rejected.Add(1)
	s.limiter.throttle.errorf("Too many %s requests, exceeding concurrency limit (%d)", pool, cap(sema))
	return errors.Errorf("too many %s requests, exceeding concurrency limit (%d)", pool, cap(sema))
}

// Release returns the permit, and is a no-op when none is held.
func (s *deliverSession) Release() {
	if s.sema != nil {
		s.sema.Release()
		s.sema = nil
	}
}

// newDeliverPolicyChecker returns the policy checker applied to every seek
// request: the channel readers policy (orderer readers in maintenance mode)
// must pass, and requests signed by an identity satisfying
// /Channel/Orderer/Readers are classified into the replication budget.
func newDeliverPolicyChecker(getChain func(string) msgprocessor.SigFilterSupport, session *deliverSession) deliver.PolicyChecker {
	return deliver.PolicyCheckerFunc(func(env *cb.Envelope, channelID string) error {
		chain := getChain(channelID)
		if chain == nil {
			return errors.Errorf("channel %s not found", channelID)
		}
		// In maintenance mode, we typically require the signature of /Channel/Orderer/Readers.
		// This will block Deliver requests from peers (which normally satisfy /Channel/Readers).
		sf := msgprocessor.NewSigFilter(policies.ChannelReaders, policies.ChannelOrdererReaders, chain)
		if err := sf.Apply(env); err != nil {
			return err
		}
		session.markReplication(satisfiesOrdererReadersPolicy(chain, env))
		return nil
	})
}

// satisfiesOrdererReadersPolicy reports whether the envelope satisfies
// /Channel/Orderer/Readers. That is an identity class, not proof of an
// orderer-to-orderer pull: any member of an orderer org matches, so
// matching requests are bounded by the replication budget rather than
// exempted. It deliberately does not log: a negative result is the common
// case for client requests.
func satisfiesOrdererReadersPolicy(chain msgprocessor.SigFilterSupport, env *cb.Envelope) bool {
	policy, ok := chain.PolicyManager().GetPolicy(policies.ChannelOrdererReaders)
	if !ok {
		return false
	}
	signedData, err := protoutil.EnvelopeAsSignedData(env)
	if err != nil {
		return false
	}
	return policy.EvaluateSignedData(signedData) == nil
}

// rateLimitedLogger logs at most one message per interval; rejections are
// counted by the sessions_rejected metric regardless.
type rateLimitedLogger struct {
	interval time.Duration
	logf     func(template string, args ...any)

	mu   sync.Mutex
	last time.Time
}

func (r *rateLimitedLogger) errorf(template string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now := time.Now(); now.Sub(r.last) < r.interval {
		return
	}
	r.last = time.Now()
	r.logf(template, args...)
}
