/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	deliverLimitTestBlocks  = 4
	deliverLimitTestBlkSize = 4 << 10
)

// blockingIterator serves the shared blocks and then blocks, like a ledger
// cursor under BLOCK_UNTIL_READY waiting for a height that never arrives.
type blockingIterator struct {
	blocks []*cb.Block
	idx    int
	closed chan struct{}
	once   sync.Once
}

func (it *blockingIterator) Next() (*cb.Block, cb.Status) {
	if it.idx < len(it.blocks) {
		b := it.blocks[it.idx]
		it.idx++
		return b, cb.Status_SUCCESS
	}
	<-it.closed
	return nil, cb.Status_NOT_FOUND
}

func (it *blockingIterator) Close() { it.once.Do(func() { close(it.closed) }) }

type staticReader struct {
	height uint64
	shared []*cb.Block
}

func (r *staticReader) Iterator(*ab.SeekPosition) (blockledger.Iterator, uint64) {
	return &blockingIterator{blocks: r.shared, closed: make(chan struct{})}, 0
}

func (r *staticReader) Height() uint64 { return r.height }

func (r *staticReader) RetrieveBlockByNumber(n uint64) (*cb.Block, error) {
	if n < uint64(len(r.shared)) {
		return r.shared[n], nil
	}
	return nil, nil
}

// identitiesContaining is a policies.Policy satisfied when any signing
// identity contains one of the given substrings, mirroring how sample
// channel policies classify member and orderer identities.
type identitiesContaining []string

func (substrs identitiesContaining) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	for _, sd := range signatureSet {
		for _, substr := range substrs {
			if bytes.Contains(sd.Identity, []byte(substr)) {
				return nil
			}
		}
	}
	return errors.Errorf("no identity of %v found", []string(substrs))
}

func (identitiesContaining) EvaluateIdentities([]msp.Identity) error { return nil }

// testDeliverPolicyManager resolves /Channel/Readers, satisfied by reader
// and orderer identities, and /Channel/Orderer/Readers, satisfied only by
// orderer identities. That mirrors overlapping MSPs such as the sample
// profiles, where one organization is both the application and the orderer
// org.
type testDeliverPolicyManager struct{}

func (testDeliverPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	switch id {
	case policies.ChannelReaders:
		return identitiesContaining{"reader", "orderer"}, true
	case policies.ChannelOrdererReaders:
		return identitiesContaining{"orderer"}, true
	default:
		return nil, false
	}
}

func (testDeliverPolicyManager) Manager([]string) (policies.Manager, bool) {
	return nil, false
}

type stubOrdererConfig struct {
	state             ab.ConsensusType_State
	consenters        []*cb.Consenter
	consensusType     string
	consensusMetadata []byte
}

func (s stubOrdererConfig) ConsensusType() string {
	if s.consensusType != "" {
		return s.consensusType
	}
	return "BFT"
}
func (s stubOrdererConfig) ConsensusMetadata() []byte              { return s.consensusMetadata }
func (s stubOrdererConfig) ConsensusState() ab.ConsensusType_State { return s.state }
func (s stubOrdererConfig) BatchSize() *ab.BatchSize               { return nil }
func (s stubOrdererConfig) BatchTimeout() time.Duration            { return 0 }
func (s stubOrdererConfig) MaxChannelsCount() uint64               { return 0 }
func (s stubOrdererConfig) Consenters() []*cb.Consenter            { return s.consenters }
func (s stubOrdererConfig) Organizations() map[string]channelconfig.OrdererOrg {
	return nil
}
func (s stubOrdererConfig) Capabilities() channelconfig.OrdererCapabilities { return nil }

// deliverTestChainSupport implements msgprocessor.SigFilterSupport for the
// deliver policy checker.
type deliverTestChainSupport struct {
	policyManager policies.Manager
	ordererConfig channelconfig.Orderer
}

func (s deliverTestChainSupport) PolicyManager() policies.Manager { return s.policyManager }

func (s deliverTestChainSupport) OrdererConfig() (channelconfig.Orderer, bool) {
	return s.ordererConfig, true
}

// deliverTestService exercises the production deliver server construction
// from server.go.
type deliverTestService struct {
	ab.UnimplementedAtomicBroadcastServer
	h           *deliver.Handler
	limiter     *deliverConcurrencyLimiter
	support     msgprocessor.SigFilterSupport
	seekTimeout time.Duration
}

func (s *deliverTestService) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	session := newDeliverSession(s.limiter)
	if !admitPreAuth(stream, session) {
		return nil
	}
	return s.h.Handle(stream.Context(), newDeliverServer(stream, func(string) msgprocessor.SigFilterSupport {
		return s.support
	}, session, s.seekTimeout, nil, &localconfig.Debug{}))
}

func deliverSeekEnvelope(creator string) *cb.Envelope {
	return deliverSeekEnvelopeWith(creator, math.MaxUint64, ab.SeekInfo_BLOCK, nil)
}

func deliverHeaderSeekEnvelope(creator string) *cb.Envelope {
	return deliverSeekEnvelopeWith(creator, math.MaxUint64, ab.SeekInfo_HEADER_WITH_SIG, nil)
}

func deliverSeekEnvelopeTo(creator string, stop uint64) *cb.Envelope {
	return deliverSeekEnvelopeWith(creator, stop, ab.SeekInfo_BLOCK, nil)
}

func deliverSeekEnvelopeWith(creator string, stop uint64, contentType ab.SeekInfo_SeekContentType, certHash []byte) *cb.Envelope {
	seek := &ab.SeekInfo{
		Start:       &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
		Stop:        &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: stop}}},
		Behavior:    ab.SeekInfo_BLOCK_UNTIL_READY,
		ContentType: contentType,
	}
	channelHeader := &cb.ChannelHeader{
		Type:      int32(cb.HeaderType_DELIVER_SEEK_INFO),
		ChannelId: "mychannel",
		Timestamp: timestamppb.Now(),
	}
	if len(certHash) != 0 {
		channelHeader.TlsCertHash = certHash
	}
	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader:   protoutil.MarshalOrPanic(channelHeader),
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{Creator: []byte(creator), Nonce: []byte("n")}),
		},
		Data: protoutil.MarshalOrPanic(seek),
	}
	return &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload), Signature: []byte("sig")}
}

type deliverTestServer struct {
	client  ab.AtomicBroadcastClient
	chain   *mock.Chain
	limiter *deliverConcurrencyLimiter
	cleanup func()
}

func makeBlock(num uint64, size int) *cb.Block {
	return &cb.Block{
		Header: &cb.BlockHeader{Number: num},
		Data:   &cb.BlockData{Data: [][]byte{make([]byte, size)}},
	}
}

func deliverLimitTestChain() *mock.Chain {
	blocks := make([]*cb.Block, deliverLimitTestBlocks)
	for i := range blocks {
		blocks[i] = makeBlock(uint64(i), deliverLimitTestBlkSize)
	}
	return deliverLimitTestChainWithBlocks(blocks)
}

func deliverLimitTestChainWithBlocks(blocks []*cb.Block) *mock.Chain {
	chain := &mock.Chain{}
	chain.ReaderReturns(&staticReader{height: uint64(len(blocks)), shared: blocks})
	chain.ErroredReturns(make(chan struct{}))
	chain.SequenceReturns(1)
	return chain
}

// startDeliverTestServer runs a real gRPC server and deliver handler with
// the given concurrency limits over an insecure connection, mirroring the
// production Deliver path.
func startDeliverTestServer(t *testing.T, deliverLimit, headerLimit, replicationLimit, preAuthLimit int, seekTimeout time.Duration) *deliverTestServer {
	t.Helper()
	return startDeliverTestServerWithChain(t, deliverLimitTestChain(), deliverLimit, headerLimit, replicationLimit, preAuthLimit, seekTimeout)
}

func startDeliverTestServerWithChain(t *testing.T, chain *mock.Chain, deliverLimit, headerLimit, replicationLimit, preAuthLimit int, seekTimeout time.Duration) *deliverTestServer {
	t.Helper()

	chainManager := &mock.ChainManager{}
	chainManager.GetChainReturns(chain)
	handler := deliver.NewHandler(chainManager, time.Hour, false, deliver.NewMetrics(&disabled.Provider{}), true)

	conf := &localconfig.TopLevel{
		General: localconfig.General{
			MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
			MaxSendMsgSize: comm.DefaultMaxSendMsgSize,
		},
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv, err := comm.NewGRPCServerFromListener(lis, initializeServerConfig(conf, nil))
	require.NoError(t, err)
	limiter := newDeliverConcurrencyLimiter(deliverLimit, headerLimit, replicationLimit, preAuthLimit, &disabled.Provider{})
	ab.RegisterAtomicBroadcastServer(srv.Server(), &deliverTestService{
		h:           handler,
		limiter:     limiter,
		seekTimeout: seekTimeout,
		support: deliverTestChainSupport{
			policyManager: testDeliverPolicyManager{},
			ordererConfig: stubOrdererConfig{state: ab.ConsensusType_STATE_NORMAL},
		},
	})
	go func() { _ = srv.Start() }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return &deliverTestServer{
		client:  ab.NewAtomicBroadcastClient(conn),
		chain:   chain,
		limiter: limiter,
		cleanup: func() {
			conn.Close()
			srv.Stop()
		},
	}
}

// deliverTLSTestServer is a deliver server requiring mutual TLS, with one
// client certificate registered as a channel consenter.
type deliverTLSTestServer struct {
	chain         *mock.Chain
	ordererClient ab.AtomicBroadcastClient
	peerClient    ab.AtomicBroadcastClient
	ordererHash   []byte
	peerHash      []byte
	cleanup       func()
}

// startDeliverTestServerTLS runs a TLS deliver server whose channel
// consenter set contains the orderer client's certificate, mirroring how
// cluster replication authenticates in production. When requireClientCert
// is false the server requests but does not require client certificates,
// as in production with ClientAuthRequired disabled.
func startDeliverTestServerTLS(t *testing.T, deliverLimit, replicationLimit int, requireClientCert bool) *deliverTLSTestServer {
	t.Helper()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	serverKp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	ordererKp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	peerKp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	dir := t.TempDir()
	writeFile := func(name string, content []byte) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, content, 0o600))
		return path
	}

	chain := deliverLimitTestChain()
	chainManager := &mock.ChainManager{}
	chainManager.GetChainReturns(chain)
	handler := deliver.NewHandler(chainManager, time.Hour, requireClientCert, deliver.NewMetrics(&disabled.Provider{}), true)

	conf := &localconfig.TopLevel{
		General: localconfig.General{
			TLS: localconfig.TLS{
				Enabled:            true,
				ClientAuthRequired: requireClientCert,
				Certificate:        writeFile("server-cert.pem", serverKp.Cert),
				PrivateKey:         writeFile("server-key.pem", serverKp.Key),
				ClientRootCAs:      []string{writeFile("ca.pem", ca.CertBytes())},
			},
			MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
			MaxSendMsgSize: comm.DefaultMaxSendMsgSize,
		},
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv, err := comm.NewGRPCServerFromListener(lis, initializeServerConfig(conf, nil))
	require.NoError(t, err)
	ab.RegisterAtomicBroadcastServer(srv.Server(), &deliverTestService{
		h:       handler,
		limiter: newDeliverConcurrencyLimiter(deliverLimit, -1, replicationLimit, -1, &disabled.Provider{}),
		support: deliverTestChainSupport{
			policyManager: testDeliverPolicyManager{},
			ordererConfig: stubOrdererConfig{
				state:      ab.ConsensusType_STATE_NORMAL,
				consenters: []*cb.Consenter{{ClientTlsCert: ordererKp.Cert, ServerTlsCert: ordererKp.Cert}},
			},
		},
	})
	go func() { _ = srv.Start() }()

	dial := func(kp *tlsgen.CertKeyPair) (*grpc.ClientConn, ab.AtomicBroadcastClient) {
		roots := x509.NewCertPool()
		require.True(t, roots.AppendCertsFromPEM(ca.CertBytes()))
		tlsCert, err := tls.X509KeyPair(kp.Cert, kp.Key)
		require.NoError(t, err)
		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			RootCAs:      roots,
		})))
		require.NoError(t, err)
		return conn, ab.NewAtomicBroadcastClient(conn)
	}

	ordererConn, ordererClient := dial(ordererKp)
	peerConn, peerClient := dial(peerKp)
	ordererHash := sha256.Sum256(ordererKp.TLSCert.Raw)
	peerHash := sha256.Sum256(peerKp.TLSCert.Raw)

	return &deliverTLSTestServer{
		chain:         chain,
		ordererClient: ordererClient,
		peerClient:    peerClient,
		ordererHash:   ordererHash[:],
		peerHash:      peerHash[:],
		cleanup: func() {
			ordererConn.Close()
			peerConn.Close()
			srv.Stop()
		},
	}
}

// openDeliverEnvelope sends a seek envelope and returns once the first
// block arrives, proving the stream was admitted.
func openDeliverEnvelope(streamCtx context.Context, client ab.AtomicBroadcastClient, env *cb.Envelope) error {
	stream, err := client.Deliver(streamCtx)
	if err != nil {
		return err
	}
	if err := stream.Send(env); err != nil {
		return err
	}
	resp, err := stream.Recv()
	if err != nil {
		return err
	}
	if resp.GetBlock() == nil {
		return errors.Errorf("expected block, got %T", resp.GetType())
	}
	return nil
}

func openDeliver(streamCtx context.Context, client ab.AtomicBroadcastClient, creator string) error {
	return openDeliverEnvelope(streamCtx, client, deliverSeekEnvelope(creator))
}

// requireRejectedUnavailable asserts that a stream is rejected with a
// retryable service unavailable status.
func requireRejectedUnavailable(t *testing.T, client ab.AtomicBroadcastClient, env *cb.Envelope) {
	t.Helper()
	probe, err := client.Deliver(context.Background())
	require.NoError(t, err)
	require.NoError(t, probe.Send(env))
	resp, err := probe.Recv()
	require.NoError(t, err)
	require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, resp.GetStatus())
	_, err = probe.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// The orderer bounds concurrent deliver sessions per request class: idle
// streams consume no budget, block streams are admitted up to the client
// limit, header streams draw from their own budget, and closing an
// admitted stream admits a new one.
func TestDeliverConcurrencyLimit(t *testing.T) {
	const (
		clientLimit = 5
		headerLimit = 2
	)

	ts := startDeliverTestServer(t, clientLimit, headerLimit, -1, -1, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// idle streams that never send a seek request must not consume the budget
	idleCtx, idleCancel := context.WithCancel(ctx)
	defer idleCancel()
	for range 3 {
		_, err := ts.client.Deliver(idleCtx)
		require.NoError(t, err)
	}

	// fill the client budget with long-lived, slowly consumed block streams
	streamCancels := make([]context.CancelFunc, 0, clientLimit)
	for range clientLimit {
		streamCtx, streamCancel := context.WithCancel(ctx)
		require.NoError(t, openDeliver(streamCtx, ts.client, "reader"))
		streamCancels = append(streamCancels, streamCancel)
	}
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// the next block stream is rejected before an iterator is created
	requireRejectedUnavailable(t, ts.client, deliverSeekEnvelope("reader"))
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// header streams draw from their own budget while the block budget is full
	headerCtx, headerCancel := context.WithCancel(ctx)
	defer headerCancel()
	for range headerLimit {
		require.NoError(t, openDeliverEnvelope(headerCtx, ts.client, deliverHeaderSeekEnvelope("reader")))
	}
	require.Equal(t, clientLimit+headerLimit, ts.chain.ReaderCallCount())

	// the header budget is finite as well
	requireRejectedUnavailable(t, ts.client, deliverHeaderSeekEnvelope("reader"))
	require.Equal(t, clientLimit+headerLimit, ts.chain.ReaderCallCount())

	// without mutual TLS, orderer identities fall back to the client budget
	requireRejectedUnavailable(t, ts.client, deliverSeekEnvelope("orderer"))
	require.Equal(t, clientLimit+headerLimit, ts.chain.ReaderCallCount())

	// closing an admitted stream releases its permit
	streamCancels[0]()
	require.Eventually(t, func() bool {
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()
		return openDeliver(streamCtx, ts.client, "reader") == nil
	}, 30*time.Second, 50*time.Millisecond)
}

// A completed delivery releases the permit, so a stream idling after a
// finished seek does not hold the budget.
func TestDeliverConcurrencyPermitReleasedAfterCompletedDelivery(t *testing.T) {
	ts := startDeliverTestServer(t, 1, -1, -1, -1, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// stream A seeks a finite range and then idles on the open stream
	streamA, err := ts.client.Deliver(ctx)
	require.NoError(t, err)
	require.NoError(t, streamA.Send(deliverSeekEnvelopeTo("reader", deliverLimitTestBlocks-1)))
	for {
		resp, err := streamA.Recv()
		require.NoError(t, err)
		if _, isStatus := resp.GetType().(*ab.DeliverResponse_Status); isStatus {
			require.Equal(t, cb.Status_SUCCESS, resp.GetStatus())
			break
		}
	}

	// stream B is admitted because A's permit was released on completion;
	// the release runs after the status is sent, so retry past the race
	require.Eventually(t, func() bool {
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()
		return openDeliver(streamCtx, ts.client, "reader") == nil
	}, 30*time.Second, 50*time.Millisecond)
}

// Under mutual TLS, orderer-to-orderer pulls are identified by the
// consenter client certificate and draw from the replication budget, so
// client floods cannot starve them; peers whose envelopes satisfy the
// orderer readers policy still draw from the client budget.
func TestDeliverConcurrencyReplicationRequiresConsenterCertificate(t *testing.T) {
	const (
		clientLimit      = 2
		replicationLimit = 2
	)

	ts := startDeliverTestServerTLS(t, clientLimit, replicationLimit, true)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ordererEnv := deliverSeekEnvelopeWith("orderer", math.MaxUint64, ab.SeekInfo_BLOCK, ts.ordererHash)
	peerOrdererEnv := deliverSeekEnvelopeWith("orderer", math.MaxUint64, ab.SeekInfo_BLOCK, ts.peerHash)
	peerReaderEnv := deliverSeekEnvelopeWith("reader", math.MaxUint64, ab.SeekInfo_BLOCK, ts.peerHash)

	// fill the client budget with ordinary peer streams
	for range clientLimit {
		require.NoError(t, openDeliverEnvelope(ctx, ts.peerClient, peerReaderEnv))
	}
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// a peer presenting an orderer-signed envelope still draws from the
	// full client budget and cannot consume the replication reservation
	requireRejectedUnavailable(t, ts.peerClient, peerOrdererEnv)
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// a consenter pull draws from its own budget and is not starved
	for range replicationLimit {
		require.NoError(t, openDeliverEnvelope(ctx, ts.ordererClient, ordererEnv))
	}
	require.Equal(t, clientLimit+replicationLimit, ts.chain.ReaderCallCount())

	// an orderer identity opening more than the replication budget is rejected
	requireRejectedUnavailable(t, ts.ordererClient, ordererEnv)
	require.Equal(t, clientLimit+replicationLimit, ts.chain.ReaderCallCount())
}

// Config blocks are sent in full even to header-only streams (the
// censorship monitor tracks configuration with them). A parked header
// stream holds only its header permit, even with a config block queued
// unread, so it cannot starve block delivery.
func TestDeliverConcurrencyHeaderConfigBlockDelivered(t *testing.T) {
	const clientLimit = 1

	blocks := []*cb.Block{makeConfigBlock(0, 128<<10)}
	for i := 1; i < deliverLimitTestBlocks; i++ {
		blocks = append(blocks, makeBlock(uint64(i), deliverLimitTestBlkSize))
	}
	ts := startDeliverTestServerWithChain(t, deliverLimitTestChainWithBlocks(blocks), clientLimit, 10, -1, -1, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	headerCtx, headerCancel := context.WithCancel(ctx)
	defer headerCancel()
	headerStream, err := ts.client.Deliver(headerCtx)
	require.NoError(t, err)
	require.NoError(t, headerStream.Send(deliverHeaderSeekEnvelope("reader")))

	// the header stream is admitted and holds its header permit; with the
	// config block possibly still queued unread, a block stream is
	// admitted into the single-slot block budget
	require.Eventually(t, func() bool {
		return len(ts.limiter.headerSema) == 1
	}, 10*time.Second, 5*time.Millisecond)
	require.NoError(t, openDeliver(ctx, ts.client, "reader"))

	// and the parked header stream is served the config block in full
	resp, err := headerStream.Recv()
	require.NoError(t, err)
	require.True(t, protoutil.IsConfigBlock(resp.GetBlock()))
	require.Len(t, resp.GetBlock().GetData().GetData(), 1)
}

// A stream that never sends a seek request is closed after the configured
// seek timeout instead of holding a handler goroutine forever.
func TestDeliverSeekTimeout(t *testing.T) {
	ts := startDeliverTestServer(t, -1, -1, -1, -1, 100*time.Millisecond)
	defer ts.cleanup()

	stream, err := ts.client.Deliver(context.Background())
	require.NoError(t, err)
	_, err = stream.Recv()
	require.ErrorContains(t, err, "no seek request received within 100ms")
}

// The timeout re-arms while the handler waits for the next seek request, so
// a stream that goes idle after a completed delivery is torn down as well.
func TestDeliverSeekTimeoutAfterCompletedDelivery(t *testing.T) {
	ts := startDeliverTestServer(t, -1, -1, -1, -1, 100*time.Millisecond)
	defer ts.cleanup()

	stream, err := ts.client.Deliver(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(deliverSeekEnvelopeTo("reader", deliverLimitTestBlocks-1)))
	for {
		resp, err := stream.Recv()
		require.NoError(t, err)
		if _, isStatus := resp.GetType().(*ab.DeliverResponse_Status); isStatus {
			require.Equal(t, cb.Status_SUCCESS, resp.GetStatus())
			break
		}
	}

	// the delivery completed and the stream now idles; it is closed after
	// the seek timeout
	_, err = stream.Recv()
	require.ErrorContains(t, err, "no seek request received within 100ms")
}

// With TLS enabled but ClientAuthRequired disabled (the sample default),
// the server still requests client certificates, so replication
// classification works for pulls that present a consenter certificate.
func TestDeliverConcurrencyReplicationWithOptionalClientCert(t *testing.T) {
	const (
		clientLimit      = 1
		replicationLimit = 1
	)

	ts := startDeliverTestServerTLS(t, clientLimit, replicationLimit, false)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// without mutual TLS binding, envelopes carry no TLS cert hash
	ordererEnv := deliverSeekEnvelopeWith("orderer", math.MaxUint64, ab.SeekInfo_BLOCK, nil)
	peerReaderEnv := deliverSeekEnvelopeWith("reader", math.MaxUint64, ab.SeekInfo_BLOCK, nil)

	// fill the client budget with an ordinary peer stream
	require.NoError(t, openDeliverEnvelope(ctx, ts.peerClient, peerReaderEnv))
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// a consenter pull is still classified into its own budget
	require.NoError(t, openDeliverEnvelope(ctx, ts.ordererClient, ordererEnv))
	require.Equal(t, clientLimit+replicationLimit, ts.chain.ReaderCallCount())

	// and both budgets reject past their limits
	requireRejectedUnavailable(t, ts.peerClient, peerReaderEnv)
	requireRejectedUnavailable(t, ts.ordererClient, ordererEnv)
}

// fakeDeliverServerStream implements ab.AtomicBroadcast_DeliverServer over
// channels, to drive the production server.Deliver directly.
type fakeDeliverServerStream struct {
	grpc.ServerStream
	ctx    context.Context
	recvCh chan deliverRecvResult
	sendCh chan *ab.DeliverResponse
}

type deliverRecvResult struct {
	env *cb.Envelope
	err error
}

func (f *fakeDeliverServerStream) Context() context.Context { return f.ctx }

func (f *fakeDeliverServerStream) Recv() (*cb.Envelope, error) {
	result := <-f.recvCh
	return result.env, result.err
}

func (f *fakeDeliverServerStream) Send(resp *ab.DeliverResponse) error {
	f.sendCh <- resp
	return nil
}

// The production server.Deliver wires the deliver handler with the policy
// checker and session limiter: an admitted request consumes the
// production limiter, and an excess request is rejected with a retryable
// status. Dropping the SessionLimiter from the construction would admit
// the excess request and fail this test.
func TestServerDeliverWiring(t *testing.T) {
	chain := deliverLimitTestChain()
	chainManager := &mock.ChainManager{}
	chainManager.GetChainReturns(chain)
	handler := deliver.NewHandler(chainManager, time.Hour, false, deliver.NewMetrics(&disabled.Provider{}), true)
	s := &server{
		dh:                 handler,
		debug:              &localconfig.Debug{},
		deliverLimiter:     newDeliverConcurrencyLimiter(1, -1, -1, -1, &disabled.Provider{}),
		deliverSeekTimeout: -1,
		Registrar:          &multichannel.Registrar{},
		getChainSupport: func(string) msgprocessor.SigFilterSupport {
			return deliverTestChainSupport{
				policyManager: testDeliverPolicyManager{},
				ordererConfig: stubOrdererConfig{state: ab.ConsensusType_STATE_NORMAL},
			}
		},
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	stream1 := &fakeDeliverServerStream{
		ctx:    ctx1,
		recvCh: make(chan deliverRecvResult, 1),
		sendCh: make(chan *ab.DeliverResponse, 8),
	}
	done1 := make(chan error, 1)
	go func() { done1 <- s.Deliver(stream1) }()
	stream1.recvCh <- deliverRecvResult{env: deliverSeekEnvelope("reader")}
	resp := <-stream1.sendCh
	require.NotNil(t, resp.GetBlock())

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	stream2 := &fakeDeliverServerStream{
		ctx:    ctx2,
		recvCh: make(chan deliverRecvResult, 1),
		sendCh: make(chan *ab.DeliverResponse, 2),
	}
	done2 := make(chan error, 1)
	go func() { done2 <- s.Deliver(stream2) }()
	stream2.recvCh <- deliverRecvResult{env: deliverSeekEnvelope("reader")}
	resp = <-stream2.sendCh
	require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, resp.GetStatus())
	require.NoError(t, <-done2)

	cancel1()
	require.Error(t, <-done1)
}

// A stream waiting between completed deliveries holds a pre-auth slot, so
// the number of such idle streams is bounded, not only timed out.
func TestDeliverConcurrencyBetweenSeeksBound(t *testing.T) {
	const preAuthLimit = 1

	ts := startDeliverTestServer(t, -1, -1, -1, preAuthLimit, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// stream A completes a finite seek and idles between seeks
	streamA, err := ts.client.Deliver(ctx)
	require.NoError(t, err)
	require.NoError(t, streamA.Send(deliverSeekEnvelopeTo("reader", deliverLimitTestBlocks-1)))
	for {
		resp, err := streamA.Recv()
		require.NoError(t, err)
		if _, isStatus := resp.GetType().(*ab.DeliverResponse_Status); isStatus {
			require.Equal(t, cb.Status_SUCCESS, resp.GetStatus())
			break
		}
	}
	require.Eventually(t, func() bool {
		return len(ts.limiter.preAuthSema) == preAuthLimit
	}, 10*time.Second, 5*time.Millisecond)

	// a new stream is bounded out while A waits between seeks
	requireRejectedUnavailable(t, ts.client, deliverSeekEnvelope("reader"))

	// closing A frees the wait slot
	require.NoError(t, streamA.CloseSend())
	require.Eventually(t, func() bool {
		return len(ts.limiter.preAuthSema) == 0
	}, 10*time.Second, 5*time.Millisecond)
	require.NoError(t, openDeliver(ctx, ts.client, "reader"))
}

// A burst of seek timeouts tears down the streams and their goroutines.
func TestDeliverSeekTimeoutGoroutines(t *testing.T) {
	ts := startDeliverTestServer(t, -1, -1, -1, -1, 50*time.Millisecond)
	defer ts.cleanup()

	// establish the client connection so its transport goroutines are
	// counted in the baseline
	require.NoError(t, openDeliver(context.Background(), ts.client, "reader"))
	before := runtime.NumGoroutine()
	const burst = 20
	for range burst {
		stream, err := ts.client.Deliver(context.Background())
		require.NoError(t, err)
		go func() { _, _ = stream.Recv() }()
	}
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before+5
	}, 15*time.Second, 50*time.Millisecond)
}

// Streams that never send a seek request are bounded by the pre-auth
// budget, not only by the seek timeout: the third silent stream is
// rejected, and so is an authenticated one until a slot frees.
func TestDeliverConcurrencyPreAuthBound(t *testing.T) {
	const preAuthLimit = 2

	ts := startDeliverTestServer(t, -1, -1, -1, preAuthLimit, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	silentCtx, silentCancel := context.WithCancel(ctx)
	// client streams are lazy: a background Recv flushes the headers so the
	// server-side handler starts and occupies a pre-auth slot
	for range preAuthLimit {
		stream, err := ts.client.Deliver(silentCtx)
		require.NoError(t, err)
		go func() { _, _ = stream.Recv() }()
	}
	require.Eventually(t, func() bool {
		return len(ts.limiter.preAuthSema) == preAuthLimit
	}, 10*time.Second, 5*time.Millisecond)

	// a third silent stream is rejected out of the pre-auth budget with a
	// retryable status, consistent with the authenticated path
	requireRejectedUnavailable(t, ts.client, deliverSeekEnvelope("reader"))

	// and an authenticated reader is rejected the same way until a silent
	// stream goes away
	require.Equal(t, 0, ts.chain.ReaderCallCount())

	silentCancel()
	require.Eventually(t, func() bool {
		return len(ts.limiter.preAuthSema) == 0
	}, 10*time.Second, 5*time.Millisecond)
	require.NoError(t, openDeliver(ctx, ts.client, "reader"))
}

// gatedReader serves blocks one at a time, waiting for a gate signal
// before each block beyond the first, so a test can change the channel
// configuration mid-seek.
type gatedReader struct {
	blocks []*cb.Block
	gate   chan struct{}
}

func (r *gatedReader) Iterator(*ab.SeekPosition) (blockledger.Iterator, uint64) {
	return &gatedIterator{blocks: r.blocks, gate: r.gate}, 0
}

func (r *gatedReader) Height() uint64 { return uint64(len(r.blocks)) }

func (r *gatedReader) RetrieveBlockByNumber(n uint64) (*cb.Block, error) {
	if n < uint64(len(r.blocks)) {
		return r.blocks[n], nil
	}
	return nil, nil
}

type gatedIterator struct {
	blocks []*cb.Block
	gate   chan struct{}
	idx    int
}

func (it *gatedIterator) Next() (*cb.Block, cb.Status) {
	if it.idx >= len(it.blocks) {
		return nil, cb.Status_NOT_FOUND
	}
	if it.idx > 0 {
		<-it.gate
	}
	block := it.blocks[it.idx]
	it.idx++
	return block, cb.Status_SUCCESS
}

func (it *gatedIterator) Close() {}

// mutableChainSupport lets a test swap the orderer config, as a channel
// configuration update does.
type mutableChainSupport struct {
	policyManager policies.Manager
	ordererConfig atomic.Value
}

func (s *mutableChainSupport) PolicyManager() policies.Manager { return s.policyManager }

func (s *mutableChainSupport) OrdererConfig() (channelconfig.Orderer, bool) {
	return s.ordererConfig.Load().(channelconfig.Orderer), true
}

// A channel configuration update mid-seek re-runs classification: the
// handler re-evaluates the policy check on every block when the config
// sequence advances, so the changed-class guard fires while the seek is
// being served, not only between seeks. The seek is terminated so the
// client readmits into the new budget.
func TestDeliverClassificationChangeMidSeekTerminatesSeek(t *testing.T) {
	ordererCert, _, ordererCertPEM := deliverLimitTestCerts(t)

	var seq atomic.Uint64
	seq.Store(1)
	support := &mutableChainSupport{policyManager: testDeliverPolicyManager{}}
	support.ordererConfig.Store(channelconfig.Orderer(stubOrdererConfig{
		state:      ab.ConsensusType_STATE_NORMAL,
		consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM}},
	}))

	blocks := []*cb.Block{makeBlock(0, deliverLimitTestBlkSize), makeBlock(1, deliverLimitTestBlkSize)}
	gate := make(chan struct{}, 1)
	chain := &mock.Chain{}
	chain.ReaderReturns(&gatedReader{blocks: blocks, gate: gate})
	chain.ErroredReturns(make(chan struct{}))
	chain.SequenceStub = func() uint64 { return seq.Load() }

	chainManager := &mock.ChainManager{}
	chainManager.GetChainReturns(chain)
	handler := deliver.NewHandler(chainManager, time.Hour, false, deliver.NewMetrics(&disabled.Provider{}), true)
	s := &server{
		dh:                 handler,
		debug:              &localconfig.Debug{},
		deliverLimiter:     newDeliverConcurrencyLimiter(1, -1, 1, -1, &disabled.Provider{}),
		deliverSeekTimeout: -1,
		seekTimeoutLog:     &rateLimitedLogger{interval: time.Minute, logf: func(string, ...any) {}},
		Registrar:          &multichannel.Registrar{},
		getChainSupport:    func(string) msgprocessor.SigFilterSupport { return support },
	}

	consenterCtx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{ordererCert}}},
	})
	stream := &fakeDeliverServerStream{
		ctx:    consenterCtx,
		recvCh: make(chan deliverRecvResult, 1),
		sendCh: make(chan *ab.DeliverResponse, 4),
	}
	done := make(chan error, 1)
	go func() { done <- s.Deliver(stream) }()
	stream.recvCh <- deliverRecvResult{env: deliverSeekEnvelopeTo("orderer", 1)}

	// the replication-classified seek delivers the first block
	resp := <-stream.sendCh
	require.NotNil(t, resp.GetBlock())
	require.Len(t, s.deliverLimiter.replicationSema, 1)

	// a config update removes the consenter; the next block's policy
	// re-evaluation re-runs classification, the class changes, and the
	// seek is terminated instead of silently switching budgets
	seq.Store(2)
	support.ordererConfig.Store(channelconfig.Orderer(stubOrdererConfig{state: ab.ConsensusType_STATE_NORMAL}))
	gate <- struct{}{}
	resp = <-stream.sendCh
	// retryable: FORBIDDEN is read by cluster pullers as self-eviction
	require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, resp.GetStatus())
	require.NoError(t, <-done)
	require.Empty(t, s.deliverLimiter.replicationSema)
}

// countingChainSupport counts OrdererConfig reads - the expensive part of
// consenter recognition, which unmarshals consensus metadata - and exposes
// a config sequence for topology invalidation.
type countingChainSupport struct {
	msgprocessor.SigFilterSupport
	ordererConfigReads *atomic.Int64
	sequence           *atomic.Uint64
}

func (c *countingChainSupport) OrdererConfig() (channelconfig.Orderer, bool) {
	c.ordererConfigReads.Add(1)
	return c.SigFilterSupport.OrdererConfig()
}

func (c *countingChainSupport) Sequence() uint64 { return c.sequence.Load() }

// certIsConsenter recognizes consenters with one hash probe against a
// cached public-key set; the consenter certificates of every channel are
// read only when the channel topology changes, however many distinct
// certificates arrive.
func TestCertIsConsenterCachedTopology(t *testing.T) {
	ordererCert, otherCert, ordererCertPEM := deliverLimitTestCerts(t)

	support := &mutableChainSupport{policyManager: testDeliverPolicyManager{}}
	support.ordererConfig.Store(channelconfig.Orderer(stubOrdererConfig{
		consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM}},
	}))
	reads := &atomic.Int64{}
	seq := &atomic.Uint64{}
	seq.Store(1)
	s := &server{
		channelNames: func() []string { return []string{"mychannel"} },
		getChainSupport: func(string) msgprocessor.SigFilterSupport {
			return &countingChainSupport{SigFilterSupport: support, ordererConfigReads: reads, sequence: seq}
		},
	}

	require.True(t, s.certIsConsenter(ordererCert))
	require.False(t, s.certIsConsenter(otherCert))
	require.Equal(t, int64(1), reads.Load(), "the first lookup reads the channel's consenter certificates once")

	// repeated lookups with an unchanged topology do not rescan, however
	// many distinct certificates arrive
	for range 10 {
		require.True(t, s.certIsConsenter(ordererCert))
		require.False(t, s.certIsConsenter(otherCert))
	}
	require.Equal(t, int64(1), reads.Load())

	// a channel configuration change rebuilds the set
	support.ordererConfig.Store(channelconfig.Orderer(stubOrdererConfig{consenters: nil}))
	seq.Store(2)
	require.False(t, s.certIsConsenter(ordererCert))
	require.Equal(t, int64(2), reads.Load())
}

// certIsConsenter is safe for concurrent use: parallel lookups race with
// rebuilds triggered by topology changes.
func TestConsenterKeySetConcurrent(t *testing.T) {
	ordererCert, otherCert, ordererCertPEM := deliverLimitTestCerts(t)

	support := &mutableChainSupport{policyManager: testDeliverPolicyManager{}}
	support.ordererConfig.Store(channelconfig.Orderer(stubOrdererConfig{
		consenters: []*cb.Consenter{{ClientTlsCert: ordererCertPEM}},
	}))
	reads := &atomic.Int64{}
	seq := &atomic.Uint64{}
	s := &server{
		channelNames: func() []string { return []string{"mychannel"} },
		getChainSupport: func(string) msgprocessor.SigFilterSupport {
			return &countingChainSupport{SigFilterSupport: support, ordererConfigReads: reads, sequence: seq}
		},
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					s.certIsConsenter(ordererCert)
					s.certIsConsenter(otherCert)
				}
			}
		})
	}
	// concurrent rebuild triggers
	for range 10 {
		seq.Add(1)
	}
	close(stop)
	wg.Wait()

	// with a stable topology the cached set is consistent
	require.True(t, s.certIsConsenter(ordererCert))
	require.False(t, s.certIsConsenter(otherCert))
}

// A flood of silent streams exhausts the shared pre-auth budget, but a
// connection presenting a consenter certificate is admitted through the
// reserved allowance and pulls blocks, while other new streams are still
// rejected.
func TestDeliverPreAuthReserveStarvation(t *testing.T) {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	consenterKp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	chain := deliverLimitTestChain()
	chainManager := &mock.ChainManager{}
	chainManager.GetChainReturns(chain)
	handler := deliver.NewHandler(chainManager, time.Hour, false, deliver.NewMetrics(&disabled.Provider{}), true)
	s := &server{
		dh:                 handler,
		debug:              &localconfig.Debug{},
		deliverLimiter:     newDeliverConcurrencyLimiter(-1, -1, -1, 1, &disabled.Provider{}),
		deliverSeekTimeout: -1,
		seekTimeoutLog:     &rateLimitedLogger{interval: time.Minute, logf: func(string, ...any) {}},
		Registrar:          &multichannel.Registrar{},
		getChainSupport: func(string) msgprocessor.SigFilterSupport {
			return deliverTestChainSupport{
				policyManager: testDeliverPolicyManager{},
				ordererConfig: stubOrdererConfig{
					state:      ab.ConsensusType_STATE_NORMAL,
					consenters: []*cb.Consenter{{ClientTlsCert: consenterKp.Cert}},
				},
			}
		},
		channelNames: func() []string { return []string{"mychannel"} },
	}

	newStream := func(ctx context.Context, recvBuf, sendBuf int) (*fakeDeliverServerStream, chan error) {
		stream := &fakeDeliverServerStream{
			ctx:    ctx,
			recvCh: make(chan deliverRecvResult, recvBuf),
			sendCh: make(chan *ab.DeliverResponse, sendBuf),
		}
		done := make(chan error, 1)
		go func() { done <- s.Deliver(stream) }()
		return stream, done
	}

	// a silent stream fills the shared pre-auth budget
	silent, silentDone := newStream(t.Context(), 1, 1)
	require.Eventually(t, func() bool {
		return len(s.deliverLimiter.preAuthSema) == 1
	}, 10*time.Second, 5*time.Millisecond)

	// a stream without a consenter certificate is rejected
	other, otherDone := newStream(context.Background(), 1, 1)
	resp := <-other.sendCh
	require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, resp.GetStatus())
	require.NoError(t, <-otherDone)

	// a stream presenting a consenter certificate is admitted through the
	// reserved allowance and served blocks
	consenterCtx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{consenterKp.TLSCert}}},
	})
	consenter, consenterDone := newStream(consenterCtx, 2, 8)
	consenter.recvCh <- deliverRecvResult{env: deliverSeekEnvelopeTo("orderer", deliverLimitTestBlocks-1)}
	for {
		resp = <-consenter.sendCh
		if _, isStatus := resp.GetType().(*ab.DeliverResponse_Status); isStatus {
			require.Equal(t, cb.Status_SUCCESS, resp.GetStatus())
			break
		}
		require.NotNil(t, resp.GetBlock())
	}
	consenter.recvCh <- deliverRecvResult{err: io.EOF}
	require.NoError(t, <-consenterDone)

	// the shared budget is untouched by the reserved admission
	require.Len(t, s.deliverLimiter.preAuthSema, 1)
	silent.recvCh <- deliverRecvResult{err: io.EOF}
	require.NoError(t, <-silentDone)
}
