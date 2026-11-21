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
	"sync"
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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	state      ab.ConsensusType_State
	consenters []*cb.Consenter
}

func (s stubOrdererConfig) ConsensusType() string                  { return "BFT" }
func (s stubOrdererConfig) ConsensusMetadata() []byte              { return nil }
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
	h       *deliver.Handler
	limiter *deliverConcurrencyLimiter
	support msgprocessor.SigFilterSupport
}

func (s *deliverTestService) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	return s.h.Handle(stream.Context(), newDeliverServer(stream, func(string) msgprocessor.SigFilterSupport {
		return s.support
	}, s.limiter, &localconfig.Debug{}))
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
	cleanup func()
}

func deliverLimitTestChain() *mock.Chain {
	shared := make([]*cb.Block, deliverLimitTestBlocks)
	for i := range shared {
		shared[i] = &cb.Block{
			Header: &cb.BlockHeader{Number: uint64(i)},
			Data:   &cb.BlockData{Data: [][]byte{make([]byte, deliverLimitTestBlkSize)}},
		}
	}
	chain := &mock.Chain{}
	chain.ReaderReturns(&staticReader{height: deliverLimitTestBlocks, shared: shared})
	chain.ErroredReturns(make(chan struct{}))
	chain.SequenceReturns(1)
	return chain
}

// startDeliverTestServer runs a real gRPC server and deliver handler with
// the given concurrency limits over an insecure connection, mirroring the
// production Deliver path.
func startDeliverTestServer(t *testing.T, deliverLimit, headerLimit, replicationLimit int) *deliverTestServer {
	t.Helper()

	chain := deliverLimitTestChain()
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
	ab.RegisterAtomicBroadcastServer(srv.Server(), &deliverTestService{
		h:       handler,
		limiter: newDeliverConcurrencyLimiter(deliverLimit, headerLimit, replicationLimit, &disabled.Provider{}),
		support: deliverTestChainSupport{
			policyManager: testDeliverPolicyManager{},
			ordererConfig: stubOrdererConfig{state: ab.ConsensusType_STATE_NORMAL},
		},
	})
	go func() { _ = srv.Start() }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return &deliverTestServer{
		client: ab.NewAtomicBroadcastClient(conn),
		chain:  chain,
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

// startDeliverTestServerTLS runs a mutual-TLS deliver server whose channel
// consenter set contains the orderer client's certificate, mirroring how
// cluster replication authenticates in production.
func startDeliverTestServerTLS(t *testing.T, deliverLimit, replicationLimit int) *deliverTLSTestServer {
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
	handler := deliver.NewHandler(chainManager, time.Hour, true, deliver.NewMetrics(&disabled.Provider{}), true)

	conf := &localconfig.TopLevel{
		General: localconfig.General{
			TLS: localconfig.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
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
		limiter: newDeliverConcurrencyLimiter(deliverLimit, -1, replicationLimit, &disabled.Provider{}),
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

	ts := startDeliverTestServer(t, clientLimit, headerLimit, -1)
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

// A BFT censorship monitor opens one long-lived header stream to every
// orderer per channel, so the header budget must absorb a fan-out of
// peers times channels while the block budget stays small.
func TestDeliverConcurrencyBFTHeaderFanout(t *testing.T) {
	const (
		clientLimit = 5
		headerLimit = 100
	)

	ts := startDeliverTestServer(t, clientLimit, headerLimit, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// a fan-out well above the block budget is admitted for header streams
	for range 90 {
		require.NoError(t, openDeliverEnvelope(ctx, ts.client, deliverHeaderSeekEnvelope("reader")))
	}
	require.Equal(t, 90, ts.chain.ReaderCallCount())

	// block streams still draw from the small client budget
	for range clientLimit {
		require.NoError(t, openDeliver(ctx, ts.client, "reader"))
	}
	requireRejectedUnavailable(t, ts.client, deliverSeekEnvelope("reader"))
	require.Equal(t, 90+clientLimit, ts.chain.ReaderCallCount())
}

// A completed delivery releases the permit, so a stream idling after a
// finished seek does not hold the budget.
func TestDeliverConcurrencyPermitReleasedAfterCompletedDelivery(t *testing.T) {
	ts := startDeliverTestServer(t, 1, -1, -1)
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

func TestDeliverConcurrencyUnlimited(t *testing.T) {
	ts := startDeliverTestServer(t, -1, -1, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range 7 {
		require.NoError(t, openDeliver(ctx, ts.client, "reader"))
	}
	require.Equal(t, 7, ts.chain.ReaderCallCount())
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

	ts := startDeliverTestServerTLS(t, clientLimit, replicationLimit)
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

// The replication budget covers concurrent catch-up pulls at the
// documented boundary of (orderers - 1) x channels.
func TestDeliverConcurrencyReplicationBoundary(t *testing.T) {
	const (
		orderers = 4
		channels = 7
		boundary = (orderers - 1) * channels
	)

	ts := startDeliverTestServerTLS(t, -1, boundary)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ordererEnv := deliverSeekEnvelopeWith("orderer", math.MaxUint64, ab.SeekInfo_BLOCK, ts.ordererHash)
	for range boundary {
		require.NoError(t, openDeliverEnvelope(ctx, ts.ordererClient, ordererEnv))
	}
	require.Equal(t, boundary, ts.chain.ReaderCallCount())

	requireRejectedUnavailable(t, ts.ordererClient, ordererEnv)
	require.Equal(t, boundary, ts.chain.ReaderCallCount())
}
