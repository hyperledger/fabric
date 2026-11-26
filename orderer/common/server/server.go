/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/protobuf/proto"
)

type broadcastSupport struct {
	*multichannel.Registrar
}

func (bs broadcastSupport) BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
	return bs.Registrar.BroadcastChannelSupport(msg)
}

type deliverSupport struct {
	*multichannel.Registrar
}

func (ds deliverSupport) GetChain(chainID string) deliver.Chain {
	chain := ds.Registrar.GetChain(chainID)
	if chain == nil {
		return nil
	}
	return chain
}

type server struct {
	bh                 *broadcast.Handler
	dh                 *deliver.Handler
	debug              *localconfig.Debug
	deliverLimiter     *deliverConcurrencyLimiter
	deliverSeekTimeout time.Duration
	seekTimeoutLog     *rateLimitedLogger
	consenterKeys      consenterKeySet
	getChainSupport    func(string) msgprocessor.SigFilterSupport
	channelNames       func() []string
	*multichannel.Registrar
}

type responseSender struct {
	ab.AtomicBroadcast_DeliverServer
}

func (rs *responseSender) SendStatusResponse(status cb.Status) error {
	reply := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: status},
	}
	return rs.Send(reply)
}

// SendBlockResponse sends block data and ignores pvtDataMap.
func (rs *responseSender) SendBlockResponse(
	block *cb.Block,
	channelID string,
	chain deliver.Chain,
	signedData *protoutil.SignedData,
) error {
	response := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{Block: block},
	}
	return rs.Send(response)
}

func (rs *responseSender) DataType() string {
	return "block"
}

// NewServer creates an ab.AtomicBroadcastServer based on the broadcast target and ledger Reader
func NewServer(
	r *multichannel.Registrar,
	metricsProvider metrics.Provider,
	debug *localconfig.Debug,
	timeWindow time.Duration,
	mutualTLS bool,
	expirationCheckDisabled bool,
	limits localconfig.Limits,
) ab.AtomicBroadcastServer {
	deliverSeekTimeout := limits.DeliverSeekTimeout
	if deliverSeekTimeout == 0 {
		deliverSeekTimeout = localconfig.Defaults.General.Limits.DeliverSeekTimeout
	}
	s := &server{
		dh: deliver.NewHandler(deliverSupport{Registrar: r}, timeWindow, mutualTLS, deliver.NewMetrics(metricsProvider), expirationCheckDisabled),
		bh: &broadcast.Handler{
			SupportRegistrar: broadcastSupport{Registrar: r},
			Metrics:          broadcast.NewMetrics(metricsProvider),
		},
		debug:              debug,
		deliverLimiter:     newDeliverConcurrencyLimiter(limits.Concurrency.DeliverService, limits.Concurrency.DeliverHeaderService, limits.Concurrency.ClusterReplication, limits.Concurrency.DeliverPreAuth, metricsProvider),
		deliverSeekTimeout: deliverSeekTimeout,
		seekTimeoutLog:     &rateLimitedLogger{interval: time.Minute, logf: logger.Warningf},
		Registrar:          r,
	}
	s.getChainSupport = s.chainSupport
	s.channelNames = s.registrarChannelNames
	return s
}

func CreateThrottlers(throttleConfig localconfig.Throttling) (RateLimiter, RateLimiter) {
	clientRateLimiter := newRateLimiter(throttleConfig.Rate, throttleConfig.InactivityTimeout)
	orgRateLimiter := newRateLimiter(throttleConfig.Rate, throttleConfig.InactivityTimeout)
	return clientRateLimiter, orgRateLimiter
}

type msgTracer struct {
	function string
	debug    *localconfig.Debug
}

func (mt *msgTracer) trace(traceDir string, msg *cb.Envelope, err error) {
	if err != nil {
		return
	}

	now := time.Now().UnixNano()
	path := fmt.Sprintf("%s%c%d_%p.%s", traceDir, os.PathSeparator, now, msg, mt.function)
	logger.Debugf("Writing %s request trace to %s", mt.function, path)
	go func() {
		if msg == nil {
			logger.Debugf("Error marshaling trace msg for %s: proto: Marshal called with nil", path)
			return
		}
		pb, err := proto.Marshal(msg)
		if err != nil {
			logger.Debugf("Error marshaling trace msg for %s: %s", path, err)
			return
		}
		err = os.WriteFile(path, pb, 0o660)
		if err != nil {
			logger.Debugf("Error writing trace msg for %s: %s", path, err)
		}
	}()
}

type broadcastMsgTracer struct {
	ab.AtomicBroadcast_BroadcastServer
	msgTracer
}

func (bmt *broadcastMsgTracer) Recv() (*cb.Envelope, error) {
	msg, err := bmt.AtomicBroadcast_BroadcastServer.Recv()
	if traceDir := bmt.debug.BroadcastTraceDir; traceDir != "" {
		bmt.trace(bmt.debug.BroadcastTraceDir, msg, err)
	}
	return msg, err
}

type deliverMsgTracer struct {
	deliver.Receiver
	msgTracer
}

func (dmt *deliverMsgTracer) Recv() (*cb.Envelope, error) {
	msg, err := dmt.Receiver.Recv()
	if traceDir := dmt.debug.DeliverTraceDir; traceDir != "" {
		dmt.trace(traceDir, msg, err)
	}
	return msg, err
}

// Broadcast receives a stream of messages from a client for ordering
func (s *server) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	logger.Debugf("Starting new Broadcast handler")
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("Broadcast client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing Broadcast stream")
	}()
	return s.bh.Handle(&broadcastMsgTracer{
		AtomicBroadcast_BroadcastServer: srv,
		debug:                           s.debug,
		function:                        "Broadcast",
	})
}

// admitPreAuth takes a pre-auth permit for the stream; on rejection it
// sends a retryable service-unavailable status (so that block pullers back
// off) and returns false.
func admitPreAuth(srv ab.AtomicBroadcast_DeliverServer, session *deliverSession) bool {
	if err := session.AcquirePreAuth(); err != nil {
		if sendErr := (&responseSender{AtomicBroadcast_DeliverServer: srv}).SendStatusResponse(cb.Status_SERVICE_UNAVAILABLE); sendErr != nil {
			logger.Warningf("Failed to send service-unavailable status to deliver client: %s", sendErr)
		}
		return false
	}
	return true
}

// Deliver sends a stream of blocks to a client after ordering
func (s *server) Deliver(srv ab.AtomicBroadcast_DeliverServer) error {
	logger.Debugf("Starting new Deliver handler")
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("Deliver client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing Deliver stream")
	}()

	session := newDeliverSession(s.deliverLimiter)
	if s.channelNames != nil {
		cert := util.ExtractCertificateFromContext(srv.Context())
		session.recognizeConsenter = func() bool { return s.certIsConsenter(cert) }
	}
	if !admitPreAuth(srv, session) {
		return nil
	}
	return s.dh.Handle(srv.Context(), newDeliverServer(srv, s.getChainSupport, session, s.deliverSeekTimeout, s.seekTimeoutLog, s.debug))
}

// registrarChannelNames returns the names of all channels known to the
// registrar, including the system channel.
func (s *server) registrarChannelNames() []string {
	list := s.ChannelList()
	names := make([]string, 0, len(list.Channels)+1)
	for _, channel := range list.Channels {
		names = append(names, channel.Name)
	}
	if list.SystemChannel != nil {
		names = append(names, list.SystemChannel.Name)
	}
	return names
}

// certIsConsenter reports whether cert's public key belongs to a consenter
// of any known channel. It runs only when the shared pre-auth budget is
// exhausted, to let cluster replication connections into the reserved
// allowance. Recognition is one hash probe against the cached consenter
// key set; the set is rebuilt only when a channel is added or a channel's
// config sequence advances, so a flood of distinct certificates cannot
// trigger repeated channel scans or exhaust a per-certificate cache.
func (s *server) certIsConsenter(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	if !s.consenterKeys.current(s.consenterSequences()) {
		s.rebuildConsenterKeys()
	}
	return s.consenterKeys.lookup(cert)
}

// consenterSequences returns the config sequence of every known channel,
// used to detect configuration changes that invalidate the consenter set.
func (s *server) consenterSequences() map[string]uint64 {
	sequences := map[string]uint64{}
	for _, name := range s.channelNames() {
		chain := s.getChainSupport(name)
		if chain == nil {
			continue
		}
		// Every channel is recorded, so channel add/remove invalidates
		// even when the chain cannot report its config sequence.
		sequences[name] = 0
		if sequencer, ok := chain.(interface{ Sequence() uint64 }); ok {
			sequences[name] = sequencer.Sequence()
		}
	}
	return sequences
}

func (s *server) rebuildConsenterKeys() {
	keys := map[[32]byte]struct{}{}
	for _, name := range s.channelNames() {
		chain := s.getChainSupport(name)
		if chain == nil {
			continue
		}
		ordererConf, ok := chain.OrdererConfig()
		if !ok {
			continue
		}
		for _, consenterCert := range consenterTLSCerts(ordererConf) {
			if key, ok := consenterPublicKeyHash(consenterCert); ok {
				keys[key] = struct{}{}
			}
		}
	}
	s.consenterKeys.rebuild(s.consenterSequences(), keys)
}

// consenterKeySet caches the public keys of all channels' consenters.
type consenterKeySet struct {
	mu        sync.RWMutex
	keys      map[[32]byte]struct{}
	sequences map[string]uint64
}

func (c *consenterKeySet) current(sequences map[string]uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sequences != nil && maps.Equal(c.sequences, sequences)
}

func (c *consenterKeySet) lookup(cert *x509.Certificate) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.keys[sha256.Sum256(cert.RawSubjectPublicKeyInfo)]
	return ok
}

func (c *consenterKeySet) rebuild(sequences map[string]uint64, keys map[[32]byte]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = keys
	c.sequences = sequences
}

// consenterPublicKeyHash parses a consenter TLS certificate (PEM or DER)
// and returns the hash of its public key.
func consenterPublicKeyHash(certBytes []byte) ([32]byte, bool) {
	der := certBytes
	if block, _ := pem.Decode(certBytes); block != nil {
		der = block.Bytes
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(parsed.RawSubjectPublicKeyInfo), true
}

// chainSupport adapts the registrar to the signature the deliver policy
// checker expects, translating a missing channel to a nil interface.
func (s *server) chainSupport(channelID string) msgprocessor.SigFilterSupport {
	if chain := s.GetChain(channelID); chain != nil {
		return chain
	}
	return nil
}

// newDeliverServer builds the deliver server for a stream: the policy
// checker classifies each seek request into a concurrency budget and the
// session limiter admits it after authentication.
func newDeliverServer(srv ab.AtomicBroadcast_DeliverServer, getChain func(string) msgprocessor.SigFilterSupport, session *deliverSession, seekTimeout time.Duration, timeoutLog *rateLimitedLogger, dbg *localconfig.Debug) *deliver.Server {
	return &deliver.Server{
		PolicyChecker: newDeliverPolicyChecker(getChain, util.ExtractCertificateFromContext(srv.Context()), session),
		Receiver: &deliverMsgTracer{
			Receiver: &seekTimeoutReceiver{Receiver: srv, timeout: seekTimeout, throttle: timeoutLog},
			debug:    dbg,
			function: "Deliver",
		},
		ResponseSender: &responseSender{
			AtomicBroadcast_DeliverServer: srv,
		},
		SessionLimiter: session,
	}
}
