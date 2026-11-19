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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var deliverSessionsRejected = metrics.CounterOpts{
	Namespace: "deliver",
	Name:      "sessions_rejected",
	Help:      "The number of deliver sessions rejected because the deliver concurrency limit was reached.",
}

// deliverConcurrencyLimiter bounds the number of concurrent authenticated
// deliver sessions. Streams only consume the budget once they are
// authenticated, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	sema     semaphore.Semaphore
	rejected metrics.Counter
	throttle *rateLimitedLogger
}

// newDeliverConcurrencyLimiter returns nil when the limit is not positive
// (unlimited).
func newDeliverConcurrencyLimiter(limit int, metricsProvider metrics.Provider) *deliverConcurrencyLimiter {
	if limit <= 0 {
		return nil
	}
	return &deliverConcurrencyLimiter{
		sema:     semaphore.New(limit),
		rejected: metricsProvider.NewCounter(deliverSessionsRejected),
		throttle: &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf},
	}
}

func (l *deliverConcurrencyLimiter) newSession() *deliverSession {
	return &deliverSession{limiter: l}
}

// deliverSession tracks a single stream's permit, acquired lazily on the
// first authenticated seek and released when the stream ends.
type deliverSession struct {
	limiter *deliverConcurrencyLimiter
	exempt  bool
	held    bool
}

// Exempt marks the session as excluded from the concurrency budget.
func (s *deliverSession) Exempt() {
	s.exempt = true
}

// Acquire admits the session, and is a no-op once the session holds a
// permit, when it is exempt, or when no limit is configured.
func (s *deliverSession) Acquire() error {
	if s.held || s.exempt || s.limiter == nil {
		return nil
	}
	if s.limiter.sema.TryAcquire() {
		s.held = true
		return nil
	}
	s.limiter.rejected.Add(1)
	s.limiter.throttle.errorf("Too many deliver requests, exceeding concurrency limit (%d)", cap(s.limiter.sema))
	return errors.Errorf("too many deliver requests, exceeding concurrency limit (%d)", cap(s.limiter.sema))
}

// Release returns the permit, and is a no-op when none is held.
func (s *deliverSession) Release() {
	if s.held {
		s.limiter.sema.Release()
		s.held = false
	}
}

// satisfiesOrdererReadersPolicy reports whether the envelope is signed by an
// identity satisfying /Channel/Orderer/Readers, which identifies
// orderer-to-orderer block pulls. It deliberately does not log: callers are
// expected to fall back to the regular client path on a negative result.
func satisfiesOrdererReadersPolicy(chain deliver.Chain, env *cb.Envelope) bool {
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
