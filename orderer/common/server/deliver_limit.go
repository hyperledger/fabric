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
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var deliverSessionsRejected = metrics.CounterOpts{
	Namespace: "deliver",
	Name:      "sessions_rejected",
	Help:      "The number of deliver sessions rejected because the deliver concurrency limit was reached.",
}

// deliverPool identifies which concurrency budget a seek request draws from.
type deliverPool int

const (
	// deliverPoolBlock serves client requests for full blocks; it is the
	// memory-critical budget.
	deliverPoolBlock deliverPool = iota
	// deliverPoolHeader serves header-only requests (BFT censorship
	// monitor); retained memory per stream is small.
	deliverPoolHeader
	// deliverPoolReplication serves orderer-to-orderer block pulls,
	// identified by the consenter client TLS certificate.
	deliverPoolReplication
)

// deliverConcurrencyLimiter bounds the number of concurrent authenticated
// deliver sessions, split into independent finite budgets per request
// class. Streams consume a budget only while a seek request is being
// served, so idle connections cannot exhaust it.
type deliverConcurrencyLimiter struct {
	clientSema      semaphore.Semaphore // nil = unlimited
	headerSema      semaphore.Semaphore // nil = unlimited
	replicationSema semaphore.Semaphore // nil = unlimited
	rejected        metrics.Counter
	throttle        *rateLimitedLogger
}

// newDeliverConcurrencyLimiter returns nil when all limits are not
// positive (unlimited).
func newDeliverConcurrencyLimiter(deliverLimit, headerLimit, replicationLimit int, metricsProvider metrics.Provider) *deliverConcurrencyLimiter {
	if deliverLimit <= 0 && headerLimit <= 0 && replicationLimit <= 0 {
		return nil
	}
	limiter := &deliverConcurrencyLimiter{
		rejected: metricsProvider.NewCounter(deliverSessionsRejected),
		throttle: &rateLimitedLogger{interval: time.Minute, logf: logger.Errorf},
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

// newDeliverSession returns a session bound to the limiter; a nil limiter
// yields a session whose Acquire always succeeds.
func newDeliverSession(limiter *deliverConcurrencyLimiter) *deliverSession {
	return &deliverSession{limiter: limiter}
}

// deliverSession tracks a single stream's permit, acquired when a seek
// request starts being served and released when the delivery completes or
// the stream ends.
type deliverSession struct {
	limiter *deliverConcurrencyLimiter
	pool    deliverPool
	sema    semaphore.Semaphore
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
	sema := s.limiter.clientSema
	pool := "deliver"
	switch s.pool {
	case deliverPoolHeader:
		sema = s.limiter.headerSema
		pool = "deliver header"
	case deliverPoolReplication:
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
// would draw from the replication budget. Without mutual TLS there is no
// client certificate and the request falls back to the client budget.
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
