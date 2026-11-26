/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"crypto/x509"
	"encoding/pem"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/channelconfig"
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

// consenterPreAuthPoolName labels the pre-auth allowance reserved for
// streams that present a consenter TLS certificate, so that a flood of
// unauthenticated streams cannot starve cluster replication before the
// first seek request arrives and classification can run.
const consenterPreAuthPoolName = "consenter-preauth"

// deliverConcurrencyLimiter bounds the number of concurrent deliver
// streams, split into independent finite budgets: streams that have not
// sent an authenticated seek request yet (goroutine-bound), and
// authenticated sessions per request class (memory-bound). Streams
// consume an authenticated budget only while a seek request is being
// served, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	preAuthSema semaphore.Semaphore // nil = unlimited
	// consenterPreAuthSema is the pre-auth allowance reserved for streams
	// presenting a consenter certificate; nil when the replication budget
	// is unlimited, in which case recognized consenters bypass the
	// pre-auth budget entirely once it is full.
	consenterPreAuthSema semaphore.Semaphore
	clientSema           semaphore.Semaphore // nil = unlimited
	headerSema           semaphore.Semaphore // nil = unlimited
	replicationSema      semaphore.Semaphore // nil = unlimited
	rejected             metrics.Counter
	held                 metrics.Gauge
	throttles            map[string]*rateLimitedLogger
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
	for _, poolName := range []string{preAuthPoolName, consenterPreAuthPoolName, "block", "header", "replication"} {
		limiter.throttles[poolName] = &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf}
	}
	if preAuthLimit > 0 {
		limiter.preAuthSema = semaphore.New(preAuthLimit)
		// The reserved allowance is sized by the replication budget: a
		// consenter connection admitted through it needs a replication
		// permit after classification anyway.
		if replicationLimit > 0 {
			limiter.consenterPreAuthSema = semaphore.New(replicationLimit)
		}
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
// whenever the stream is not serving a seek request, and a request-class
// permit held while a seek request is being served.
type deliverSession struct {
	limiter *deliverConcurrencyLimiter
	pool    deliverPool

	// recognizeConsenter reports whether the stream presented the TLS
	// certificate of a channel consenter. It is consulted lazily, only when
	// the shared pre-auth budget is full, and the result is latched.
	recognizeConsenter func() bool
	consenterChecked   bool
	isConsenter        bool

	preAuth     semaphore.Semaphore
	preAuthPool string
	acquired    bool
	sema        semaphore.Semaphore
	semaPool    string
	configSema  semaphore.Semaphore
}

// AcquirePreAuth admits the stream into the pre-authentication budget,
// bounding the number of streams whose handler is waiting for a first seek
// request.
func (s *deliverSession) AcquirePreAuth() error {
	return s.acquirePreAuth()
}

func (s *deliverSession) releasePreAuth() {
	if s.limiter != nil && s.preAuth != nil {
		s.limiter.held.With("pool", s.preAuthPool).Add(-1)
		s.preAuth.Release()
		s.preAuth = nil
		s.preAuthPool = ""
	}
}

// classify selects the budget for the next admission decision. It is
// evaluated per seek request, never latched for the lifetime of the
// stream. It reports whether the class changed while a seek is being
// served (e.g. after a channel configuration update); the caller then
// terminates the seek so the client readmits into the new budget, instead
// of transferring the permit mid-flight.
func (s *deliverSession) classify(pool deliverPool) bool {
	changed := s.acquired && pool != s.pool
	s.pool = pool
	return changed
}

// Acquire admits the session into its classified budget, and is a no-op
// once the session is admitted or when no limit is configured for that
// budget. On the first admission the pre-auth permit is returned: the
// stream is now bounded by its request class instead.
func (s *deliverSession) Acquire() error {
	if s.limiter == nil || s.acquired {
		return nil
	}
	sema, poolName := s.limiter.budget(s.pool)
	if err := s.limiter.admit(sema, poolName); err != nil {
		return err
	}
	s.acquired = true
	s.sema = sema
	s.semaPool = poolName
	s.releasePreAuth()
	return nil
}

// Complete is called when a delivery completes: the serving permits are
// returned, and the stream is admitted back into the pre-auth budget while
// it waits for the next seek request. An error means the stream cannot
// stay open and is closed.
func (s *deliverSession) Complete() error {
	if s.limiter == nil {
		return nil
	}
	s.releaseServing()
	return s.acquirePreAuth()
}

// acquirePreAuth admits the stream into the pre-auth budget, and is a
// no-op when the stream already holds the permit or the budget is
// unlimited. When the shared budget is full, a stream that presented a
// consenter certificate falls back to the reserved consenter allowance, so
// a flood of unauthenticated streams cannot starve cluster replication
// before the first seek request arrives.
func (s *deliverSession) acquirePreAuth() error {
	if s.limiter == nil || s.limiter.preAuthSema == nil || s.preAuth != nil {
		return nil
	}
	err := s.limiter.admit(s.limiter.preAuthSema, preAuthPoolName)
	if err == nil {
		s.preAuth = s.limiter.preAuthSema
		s.preAuthPool = preAuthPoolName
		return nil
	}
	if !s.recognizedConsenter() {
		return err
	}
	if err := s.limiter.admit(s.limiter.consenterPreAuthSema, consenterPreAuthPoolName); err != nil {
		return err
	}
	s.preAuth = s.limiter.consenterPreAuthSema
	s.preAuthPool = consenterPreAuthPoolName
	return nil
}

// recognizedConsenter reports whether the stream presented a consenter
// certificate, evaluating the (potentially expensive) recognition only
// once.
func (s *deliverSession) recognizedConsenter() bool {
	if !s.consenterChecked {
		s.consenterChecked = true
		s.isConsenter = s.recognizeConsenter != nil && s.recognizeConsenter()
	}
	return s.isConsenter
}

// AcquireForBlock escalates a header session that is about to send a
// config block to the block budget: config blocks are delivered in full
// even to header-only streams (the censorship monitor tracks configuration
// with them), so they cost as much memory as a full-block stream. The
// escalation permit is held for the rest of the seek, not released once
// the block is sent: gRPC queues oversized messages without backpressure,
// so a permit returned at send time would let retained buffers bypass the
// block budget.
func (s *deliverSession) AcquireForBlock(isConfigBlock bool) error {
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

// releaseServing returns the serving and config-escalation permits.
func (s *deliverSession) releaseServing() {
	if s.sema != nil {
		s.limiter.held.With("pool", s.semaPool).Add(-1)
		s.sema.Release()
		s.sema = nil
	}
	s.acquired = false
	s.releaseConfigEscalation()
}

func (s *deliverSession) releaseConfigEscalation() {
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
	s.releaseServing()
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
		// deliverBlocks rejects envelopes whose payload cannot be parsed, so
		// a parse failure here charges the memory-critical budget.
		payload, _ := protoutil.UnmarshalPayload(env.GetPayload())
		pool := deliverPoolBlock
		switch {
		case isClusterReplication(chain, clientCert, payload, env):
			pool = deliverPoolReplication
		case seekContentType(payload) == ab.SeekInfo_HEADER_WITH_SIG:
			pool = deliverPoolHeader
		}
		if session.classify(pool) {
			return errors.New("deliver request class changed after a channel configuration update; resubmit the seek request")
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
func isClusterReplication(chain msgprocessor.SigFilterSupport, clientCert *x509.Certificate, payload *cb.Payload, env *cb.Envelope) bool {
	if clientCert == nil {
		return false
	}
	if !satisfiesOrdererReadersPolicy(chain, payload, env) {
		return false
	}
	ordererConf, ok := chain.OrdererConfig()
	if !ok {
		return false
	}
	return certMatchesConsenter(clientCert, consenterTLSCerts(ordererConf))
}

// consenterTLSCerts returns the client and server TLS certificates of the
// channel consenters. BFT consenters are read from the Orderers value;
// etcdraft channels carry no Orderers value, so their consenters are
// unmarshaled from the consensus metadata the same way the etcdraft
// consenter reads them.
func consenterTLSCerts(ordererConf channelconfig.Orderer) [][]byte {
	var certs [][]byte
	for _, consenter := range ordererConf.Consenters() {
		certs = append(certs, consenter.GetClientTlsCert(), consenter.GetServerTlsCert())
	}
	metadata := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(ordererConf.ConsensusMetadata(), metadata); err == nil {
		for _, consenter := range metadata.GetConsenters() {
			certs = append(certs, consenter.GetClientTlsCert(), consenter.GetServerTlsCert())
		}
	}
	return certs
}

// certMatchesConsenter reports whether the client certificate public key
// matches any of the consenter TLS certificates.
func certMatchesConsenter(clientCert *x509.Certificate, consenterCerts [][]byte) bool {
	clientCertPEM := []byte(cluster.DERtoPEM(clientCert.Raw))
	for _, consenterCert := range consenterCerts {
		if block, _ := pem.Decode(consenterCert); block == nil {
			consenterCert = []byte(cluster.DERtoPEM(consenterCert))
		}
		if equal, err := cluster.CompareCertPublicKeys(clientCertPEM, consenterCert); err == nil && equal {
			return true
		}
	}
	return false
}

// satisfiesOrdererReadersPolicy reports whether the envelope satisfies
// /Channel/Orderer/Readers. It deliberately does not log: a negative result
// is the common case for client requests.
func satisfiesOrdererReadersPolicy(chain msgprocessor.SigFilterSupport, payload *cb.Payload, env *cb.Envelope) bool {
	policy, ok := chain.PolicyManager().GetPolicy(policies.ChannelOrdererReaders)
	if !ok {
		return false
	}
	signatureHeader, err := protoutil.UnmarshalSignatureHeader(payload.GetHeader().GetSignatureHeader())
	if err != nil {
		return false
	}
	signedData := []*protoutil.SignedData{{Data: env.GetPayload(), Identity: signatureHeader.GetCreator(), Signature: env.GetSignature()}}
	return policy.EvaluateSignedData(signedData) == nil
}

// seekContentType extracts the content type of the seek request, defaulting
// to full blocks when the payload cannot be parsed; deliverBlocks rejects
// such envelopes anyway, so they are charged to the memory-critical budget.
func seekContentType(payload *cb.Payload) ab.SeekInfo_SeekContentType {
	seekInfo := &ab.SeekInfo{}
	if err := proto.Unmarshal(payload.GetData(), seekInfo); err != nil {
		return ab.SeekInfo_BLOCK
	}
	return seekInfo.GetContentType()
}

// seekTimeoutReceiver fails the stream when no message arrives within the
// timeout while the handler waits for a seek request, so idle streams
// cannot hold goroutines forever. The timeout applies to every receive:
// while a seek is being served the handler does not call Recv, so
// long-lived deliveries are unaffected.
type seekTimeoutReceiver struct {
	deliver.Receiver
	timeout  time.Duration
	throttle *rateLimitedLogger
}

func (r *seekTimeoutReceiver) Recv() (*cb.Envelope, error) {
	if r.timeout <= 0 {
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
		return result.env, result.err
	case <-timer.C:
		// logged here, rate-limited: the handler suppresses its per-stream
		// warning for this error so a burst of idle streams cannot flood
		// the log
		r.throttle.errorf("No seek request received within %v, closing the deliver stream", r.timeout)
		return nil, loggedRecvError{error: errors.Errorf("no seek request received within %v", r.timeout)}
	}
}

// loggedRecvError marks a Recv error the receiver already logged through
// the rate limiter, so the deliver handler does not warn about it again.
type loggedRecvError struct{ error }

func (loggedRecvError) RecvErrorLogged() {}

// rateLimitedLogger logs at most one message per interval; rejections are
// counted by the sessions_rejected metric regardless.
type rateLimitedLogger struct {
	interval time.Duration
	logf     func(template string, args ...any)

	mu   sync.Mutex
	last time.Time
}

// errorf takes the rate-limiting decision under the mutex but logs after
// releasing it, so a blocked log sink cannot stall admission decisions.
func (r *rateLimitedLogger) errorf(template string, args ...any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if now := time.Now(); now.Sub(r.last) < r.interval {
		r.mu.Unlock()
		return
	}
	r.last = time.Now()
	r.mu.Unlock()
	r.logf(template, args...)
}
