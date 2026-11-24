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
	// blocks are served, which are charged to the block budget per block.
	deliverPoolHeader
	// deliverPoolReplication serves orderer-to-orderer block pulls,
	// identified by the consenter client TLS certificate.
	deliverPoolReplication
)

// preAuthPoolName labels the budget of streams that have not yet been
// authenticated into a request class.
const preAuthPoolName = "preauth"

// deliverConcurrencyLimiter bounds the number of concurrent deliver
// streams, split into independent finite budgets: streams that have not
// sent an authenticated seek request yet (goroutine-bound), and
// authenticated sessions per request class (memory-bound). Streams
// consume an authenticated budget only while a seek request is being
// served, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	preAuthSema     semaphore.Semaphore // nil = unlimited
	clientSema      semaphore.Semaphore // nil = unlimited
	headerSema      semaphore.Semaphore // nil = unlimited
	replicationSema semaphore.Semaphore // nil = unlimited
	rejected        metrics.Counter
	held            metrics.Gauge
	throttles       map[string]*rateLimitedLogger
}

// newDeliverConcurrencyLimiter returns nil when all limits are not
// positive (unlimited). A zero value selects the configuration default,
// mirroring localconfig normalization.
func newDeliverConcurrencyLimiter(deliverLimit, headerLimit, replicationLimit, preAuthLimit int, metricsProvider metrics.Provider) *deliverConcurrencyLimiter {
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
	if preAuthLimit == 0 {
		preAuthLimit = defaults.DeliverPreAuth
	}
	if deliverLimit <= 0 && headerLimit <= 0 && replicationLimit <= 0 && preAuthLimit <= 0 {
		return nil
	}
	limiter := &deliverConcurrencyLimiter{
		rejected:  metricsProvider.NewCounter(deliverSessionsRejected),
		held:      metricsProvider.NewGauge(deliverSessionsHeld),
		throttles: map[string]*rateLimitedLogger{},
	}
	for _, poolName := range []string{preAuthPoolName, "block", "header", "replication"} {
		limiter.throttles[poolName] = &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf}
	}
	if preAuthLimit > 0 {
		limiter.preAuthSema = semaphore.New(preAuthLimit)
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
func (l *deliverConcurrencyLimiter) admit(sema semaphore.Semaphore, poolName string) error {
	if sema == nil {
		return nil
	}
	if sema.TryAcquire() {
		l.held.With("pool", poolName).Add(1)
		return nil
	}
	l.rejected.With("pool", poolName).Add(1)
	l.throttles[poolName].errorf("Too many %s requests, exceeding concurrency limit (%d)", poolName, cap(sema))
	return errors.Errorf("too many %s requests, exceeding concurrency limit (%d)", poolName, cap(sema))
}

// newDeliverSession returns a session bound to the limiter; a nil limiter
// yields a session whose Acquire always succeeds.
func newDeliverSession(limiter *deliverConcurrencyLimiter) *deliverSession {
	return &deliverSession{limiter: limiter}
}

// deliverSession tracks a single stream's permits: a pre-auth permit held
// from stream start until the first authenticated admission, and a
// request-class permit held while a seek request is being served.
type deliverSession struct {
	limiter *deliverConcurrencyLimiter
	pool    deliverPool

	preAuth    semaphore.Semaphore
	sema       semaphore.Semaphore
	semaPool   string
	configSema semaphore.Semaphore
}

// AcquirePreAuth admits the stream into the pre-authentication budget,
// bounding the number of streams whose handler is waiting for a first seek
// request.
func (s *deliverSession) AcquirePreAuth() error {
	if s.limiter == nil || s.limiter.preAuthSema == nil {
		return nil
	}
	if err := s.limiter.admit(s.limiter.preAuthSema, preAuthPoolName); err != nil {
		return err
	}
	s.preAuth = s.limiter.preAuthSema
	return nil
}

func (s *deliverSession) releasePreAuth() {
	if s.limiter != nil && s.preAuth != nil {
		s.limiter.held.With("pool", preAuthPoolName).Add(-1)
		s.preAuth.Release()
		s.preAuth = nil
	}
}

// classify selects the budget for the next admission decision. It is
// evaluated per seek request, never latched for the lifetime of the stream.
func (s *deliverSession) classify(pool deliverPool) {
	s.pool = pool
}

// Acquire admits the session into its classified budget, and is a no-op
// while the session holds a permit or when no limit is configured for that
// budget. On the first admission the pre-auth permit is returned: the
// stream is now bounded by its request class instead.
func (s *deliverSession) Acquire() error {
	if s.limiter == nil || s.sema != nil {
		return nil
	}
	sema, poolName := s.limiter.budget(s.pool)
	if err := s.limiter.admit(sema, poolName); err != nil {
		return err
	}
	s.sema = sema
	s.semaPool = poolName
	s.releasePreAuth()
	return nil
}

// AcquireForBlock escalates a header session that is about to send a
// config block to the block budget: config blocks are delivered in full
// even to header-only streams (the censorship monitor tracks configuration
// with them), so they cost as much memory as a full-block stream while
// being sent.
func (s *deliverSession) AcquireForBlock(_ *cb.Block, isConfigBlock bool) error {
	if s.limiter == nil || s.pool != deliverPoolHeader || !isConfigBlock || s.configSema != nil {
		return nil
	}
	sema, poolName := s.limiter.budget(deliverPoolBlock)
	if err := s.limiter.admit(sema, poolName); err != nil {
		return err
	}
	s.configSema = sema
	return nil
}

// ReleaseForBlock returns the escalation permit taken for a config block
// once the block is sent, so a header stream does not hold the block
// budget beyond the config block itself.
func (s *deliverSession) ReleaseForBlock(_ *cb.Block, _ bool) {
	if s.limiter == nil || s.configSema == nil {
		return
	}
	_, poolName := s.limiter.budget(deliverPoolBlock)
	s.limiter.held.With("pool", poolName).Add(-1)
	s.configSema.Release()
	s.configSema = nil
}

// Release returns the permits, and is a no-op when none are held.
func (s *deliverSession) Release() {
	if s.limiter == nil {
		return
	}
	s.releasePreAuth()
	if s.sema != nil {
		s.limiter.held.With("pool", s.semaPool).Add(-1)
		s.sema.Release()
		s.sema = nil
	}
	s.ReleaseForBlock(nil, false)
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
