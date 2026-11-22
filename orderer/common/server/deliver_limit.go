/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"crypto/x509"
	"encoding/pem"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var deliverSessionsRejected = metrics.CounterOpts{
	Namespace:    "deliver",
	Name:         "sessions_rejected",
	Help:         "The number of deliver sessions rejected because the deliver concurrency limit was reached.",
	LabelNames:   []string{"pool"},
	StatsdFormat: "%{#fqname}.%{pool}",
}

var deliverSessionsHeld = metrics.GaugeOpts{
	Namespace:    "deliver",
	Name:         "sessions_held",
	Help:         "The number of deliver session permits currently held.",
	LabelNames:   []string{"pool"},
	StatsdFormat: "%{#fqname}.%{pool}",
}

// deliverPool identifies which concurrency budget a seek request draws from.
type deliverPool int

const (
	// deliverPoolBlock serves client requests for full blocks; it is the
	// memory-critical budget.
	deliverPoolBlock deliverPool = iota
	// deliverPoolHeader serves header-only requests (BFT censorship
	// monitor); retained memory per stream is small except when config
	// blocks are served, which escalate to the block budget.
	deliverPoolHeader
	// deliverPoolReplication serves orderer-to-orderer block pulls,
	// identified by the consenter client TLS certificate.
	deliverPoolReplication
)

// deliverPoolCount is the number of budgets.
const deliverPoolCount = 3

// deliverConcurrencyLimiter bounds the number of concurrent authenticated
// deliver sessions, split into independent finite budgets per request
// class. Streams consume a budget only while a seek request is being
// served, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	clientSema      semaphore.Semaphore // nil = unlimited
	headerSema      semaphore.Semaphore // nil = unlimited
	replicationSema semaphore.Semaphore // nil = unlimited
	rejected        metrics.Counter
	held            metrics.Gauge
	throttles       [deliverPoolCount]*rateLimitedLogger
}

// newDeliverConcurrencyLimiter returns nil when all limits are not
// positive (unlimited). A zero value selects the configuration default,
// mirroring localconfig normalization.
func newDeliverConcurrencyLimiter(deliverLimit, headerLimit, replicationLimit int, metricsProvider metrics.Provider) *deliverConcurrencyLimiter {
	defaults := localconfig.Defaults.General.Limits.Concurrency
	if deliverLimit == 0 {
		deliverLimit = defaults.DeliverService
	}
	if headerLimit == 0 {
		headerLimit = defaults.DeliverHeaderService
	}
	if replicationLimit == 0 {
		replicationLimit = defaults.ClusterReplication
	}
	if deliverLimit <= 0 && headerLimit <= 0 && replicationLimit <= 0 {
		return nil
	}
	limiter := &deliverConcurrencyLimiter{
		rejected: metricsProvider.NewCounter(deliverSessionsRejected),
		held:     metricsProvider.NewGauge(deliverSessionsHeld),
	}
	for pool := range deliverPoolCount {
		limiter.throttles[pool] = &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf}
	}
	if deliverLimit > 0 {
		limiter.clientSema = semaphore.New(deliverLimit)
	}
	if headerLimit > 0 {
		limiter.headerSema = semaphore.New(headerLimit)
	}
	if replicationLimit > 0 {
		limiter.replicationSema = semaphore.New(replicationLimit)
	}
	return limiter
}

func (l *deliverConcurrencyLimiter) budget(pool deliverPool) (semaphore.Semaphore, string) {
	switch pool {
	case deliverPoolHeader:
		return l.headerSema, "header"
	case deliverPoolReplication:
		return l.replicationSema, "replication"
	default:
		return l.clientSema, "block"
	}
}

// admit takes one permit from sema, or counts and logs a rejection before
// returning an error. A nil semaphore is an unlimited budget.
func (l *deliverConcurrencyLimiter) admit(sema semaphore.Semaphore, pool deliverPool, poolName string) error {
	if sema == nil {
		return nil
	}
	if sema.TryAcquire() {
		l.held.With("pool", poolName).Add(1)
		return nil
	}
	l.rejected.With("pool", poolName).Add(1)
	l.throttles[pool].errorf("Too many %s requests, exceeding concurrency limit (%d)", poolName, cap(sema))
	return errors.Errorf("too many %s requests, exceeding concurrency limit (%d)", poolName, cap(sema))
}

// newDeliverSession returns a session bound to the limiter; a nil limiter
// yields a session whose Acquire always succeeds.
func newDeliverSession(limiter *deliverConcurrencyLimiter) *deliverSession {
	return &deliverSession{limiter: limiter}
}

// deliverSession tracks a single stream's permits, acquired when a seek
// request starts being served and released when the delivery completes or
// the stream ends.
type deliverSession struct {
	limiter *deliverConcurrencyLimiter
	pool    deliverPool

	sema     semaphore.Semaphore
	semaPool string
	// configSema holds a block-budget permit taken by a header session
	// that is serving config blocks, which are sent in full.
	configSema semaphore.Semaphore
}

// classify selects the budget for the next admission decision. It is
// evaluated per seek request, never latched for the lifetime of the stream.
func (s *deliverSession) classify(pool deliverPool) {
	s.pool = pool
}

// Acquire admits the session into its classified budget, and is a no-op
// while the session holds a permit or when no limit is configured for that
// budget.
func (s *deliverSession) Acquire() error {
	if s.limiter == nil || s.sema != nil {
		return nil
	}
	sema, poolName := s.limiter.budget(s.pool)
	if err := s.limiter.admit(sema, s.pool, poolName); err != nil {
		return err
	}
	s.sema = sema
	s.semaPool = poolName
	return nil
}

// AcquireForBlock escalates a header session that is about to send a config
// block to the block budget: config blocks are delivered in full even to
// header-only streams (the censorship monitor tracks configuration with
// them), so they cost as much memory as a full-block stream.
func (s *deliverSession) AcquireForBlock(block *cb.Block) error {
	if s.limiter == nil || s.pool != deliverPoolHeader || s.configSema != nil {
		return nil
	}
	if !protoutil.IsConfigBlock(block) {
		return nil
	}
	sema, poolName := s.limiter.budget(deliverPoolBlock)
	if err := s.limiter.admit(sema, deliverPoolBlock, poolName); err != nil {
		return err
	}
	s.configSema = sema
	return nil
}

// Release returns the permits, and is a no-op when none are held.
func (s *deliverSession) Release() {
	if s.limiter == nil {
		return
	}
	if s.sema != nil {
		s.limiter.held.With("pool", s.semaPool).Add(-1)
		s.sema.Release()
		s.sema = nil
	}
	if s.configSema != nil {
		_, poolName := s.limiter.budget(deliverPoolBlock)
		s.limiter.held.With("pool", poolName).Add(-1)
		s.configSema.Release()
		s.configSema = nil
	}
}

// newDeliverPolicyChecker returns the policy checker applied to every seek
// request: the channel readers policy (orderer readers in maintenance mode)
// must pass, after which the request is classified into a concurrency
// budget. The classification is recomputed per seek request.
func newDeliverPolicyChecker(getChain func(string) msgprocessor.SigFilterSupport, clientCert *x509.Certificate, session *deliverSession) deliver.PolicyChecker {
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
		session.classify(deliverPoolBlock)
		switch {
		case isClusterReplication(chain, clientCert, env):
			session.classify(deliverPoolReplication)
		case seekContentType(env) == ab.SeekInfo_HEADER_WITH_SIG:
			session.classify(deliverPoolHeader)
		}
		return nil
	})
}

// isClusterReplication reports whether the request is an orderer-to-orderer
// block pull: the envelope must satisfy /Channel/Orderer/Readers and the
// connection must present the client TLS certificate of a consenter of the
// channel. The policy check alone proves only an identity class - in
// overlapping MSPs (orderer and application orgs sharing one) any member
// satisfies it, so without the transport binding every orderer-org peer
// would draw from the replication budget. The TLS server requests client
// certificates whenever TLS is enabled, so the binding works whenever the
// client presents a certificate; without TLS the request falls back to the
// client budgets.
func isClusterReplication(chain msgprocessor.SigFilterSupport, clientCert *x509.Certificate, env *cb.Envelope) bool {
	if clientCert == nil {
		return false
	}
	if !satisfiesOrdererReadersPolicy(chain, env) {
		return false
	}
	ordererConf, ok := chain.OrdererConfig()
	if !ok {
		return false
	}
	clientCertPEM := []byte(cluster.DERtoPEM(clientCert.Raw))
	for _, consenter := range ordererConf.Consenters() {
		for _, consenterCert := range [][]byte{consenter.GetClientTlsCert(), consenter.GetServerTlsCert()} {
			if block, _ := pem.Decode(consenterCert); block == nil {
				consenterCert = []byte(cluster.DERtoPEM(consenterCert))
			}
			if equal, err := cluster.CompareCertPublicKeys(clientCertPEM, consenterCert); err == nil && equal {
				return true
			}
		}
	}
	return false
}

// satisfiesOrdererReadersPolicy reports whether the envelope satisfies
// /Channel/Orderer/Readers. It deliberately does not log: a negative result
// is the common case for client requests.
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

// seekContentType extracts the content type of the seek request, defaulting
// to full blocks when the envelope cannot be parsed; deliverBlocks rejects
// such envelopes anyway, so they are charged to the memory-critical budget.
func seekContentType(env *cb.Envelope) ab.SeekInfo_SeekContentType {
	payload := &cb.Payload{}
	if err := proto.Unmarshal(env.GetPayload(), payload); err != nil {
		return ab.SeekInfo_BLOCK
	}
	seekInfo := &ab.SeekInfo{}
	if err := proto.Unmarshal(payload.GetData(), seekInfo); err != nil {
		return ab.SeekInfo_BLOCK
	}
	return seekInfo.GetContentType()
}

// firstMessageTimeoutReceiver fails the stream when the first message does
// not arrive within the timeout, so clients cannot hold handler goroutines
// with streams that never send a seek request.
type firstMessageTimeoutReceiver struct {
	deliver.Receiver
	timeout  time.Duration
	received atomic.Bool
}

func (r *firstMessageTimeoutReceiver) Recv() (*cb.Envelope, error) {
	if r.timeout <= 0 || r.received.Load() {
		return r.Receiver.Recv()
	}
	type recvResult struct {
		env *cb.Envelope
		err error
	}
	// the inner Recv unblocks when the stream is torn down after the error
	resultCh := make(chan recvResult, 1)
	go func() {
		env, err := r.Receiver.Recv()
		resultCh <- recvResult{env: env, err: err}
	}()
	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	select {
	case result := <-resultCh:
		r.received.Store(true)
		return result.env, result.err
	case <-timer.C:
		return nil, errors.Errorf("no message received within %v", r.timeout)
	}
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
