/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
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
// orderer identities.
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
	state ab.ConsensusType_State
}

func (s stubOrdererConfig) ConsensusType() string                  { return "BFT" }
func (s stubOrdererConfig) ConsensusMetadata() []byte              { return nil }
func (s stubOrdererConfig) ConsensusState() ab.ConsensusType_State { return s.state }
func (s stubOrdererConfig) BatchSize() *ab.BatchSize               { return nil }
func (s stubOrdererConfig) BatchTimeout() time.Duration            { return 0 }
func (s stubOrdererConfig) MaxChannelsCount() uint64               { return 0 }
func (s stubOrdererConfig) Consenters() []*cb.Consenter            { return nil }
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

// deliverTestService mirrors Deliver in server.go, using the production
// policy checker and session limiter wiring.
type deliverTestService struct {
	ab.UnimplementedAtomicBroadcastServer
	h       *deliver.Handler
	limiter *deliverConcurrencyLimiter
	support msgprocessor.SigFilterSupport
}

func (s *deliverTestService) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	session := newDeliverSession(s.limiter)
	ds := &deliver.Server{
		PolicyChecker: newDeliverPolicyChecker(func(string) msgprocessor.SigFilterSupport {
			return s.support
		}, session),
		Receiver:       stream,
		ResponseSender: &deliverResponseSender{stream: stream},
		SessionLimiter: session,
	}
	return s.h.Handle(stream.Context(), ds)
}

type deliverResponseSender struct {
	stream ab.AtomicBroadcast_DeliverServer
}

func (s *deliverResponseSender) SendStatusResponse(status cb.Status) error {
	return s.stream.Send(&ab.DeliverResponse{Type: &ab.DeliverResponse_Status{Status: status}})
}

func (s *deliverResponseSender) SendBlockResponse(b *cb.Block, _ string, _ deliver.Chain, _ *protoutil.SignedData) error {
	return s.stream.Send(&ab.DeliverResponse{Type: &ab.DeliverResponse_Block{Block: b}})
}

func (s *deliverResponseSender) DataType() string { return "block" }

func deliverSeekEnvelope(creator string) *cb.Envelope {
	return deliverSeekEnvelopeTo(creator, math.MaxUint64)
}

func deliverSeekEnvelopeTo(creator string, stop uint64) *cb.Envelope {
	seek := &ab.SeekInfo{
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: stop}}},
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}
	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
				Type:      int32(cb.HeaderType_DELIVER_SEEK_INFO),
				ChannelId: "mychannel",
				Timestamp: timestamppb.Now(),
			}),
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

// startDeliverTestServer runs a real gRPC server and deliver handler with
// the given concurrency limits, mirroring the production Deliver path.
func startDeliverTestServer(t *testing.T, deliverLimit, replicationLimit int) *deliverTestServer {
	t.Helper()

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
		limiter: newDeliverConcurrencyLimiter(deliverLimit, replicationLimit, &disabled.Provider{}),
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

// openDeliver seeks from the oldest block and returns once the first block
// arrives, proving the stream was admitted.
func openDeliver(streamCtx context.Context, client ab.AtomicBroadcastClient, creator string) error {
	stream, err := client.Deliver(streamCtx)
	if err != nil {
		return err
	}
	if err := stream.Send(deliverSeekEnvelope(creator)); err != nil {
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

// requireRejectedUnavailable asserts that a stream is rejected with a
// retryable service unavailable status.
func requireRejectedUnavailable(t *testing.T, client ab.AtomicBroadcastClient, creator string) {
	t.Helper()
	probe, err := client.Deliver(context.Background())
	require.NoError(t, err)
	require.NoError(t, probe.Send(deliverSeekEnvelope(creator)))
	resp, err := probe.Recv()
	require.NoError(t, err)
	require.Equal(t, cb.Status_SERVICE_UNAVAILABLE, resp.GetStatus())
	_, err = probe.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// The orderer bounds concurrent deliver sessions: idle streams consume no
// budget, the first limit authenticated streams are admitted, the next one
// is rejected before an iterator is created, orderer-to-orderer pulls draw
// from their own finite budget, and closing an admitted stream admits a
// new one.
func TestDeliverConcurrencyLimit(t *testing.T) {
	const (
		clientLimit      = 5
		replicationLimit = 2
	)

	ts := startDeliverTestServer(t, clientLimit, replicationLimit)
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

	// fill the client budget with long-lived, slowly consumed streams
	streamCancels := make([]context.CancelFunc, 0, clientLimit)
	for range clientLimit {
		streamCtx, streamCancel := context.WithCancel(ctx)
		require.NoError(t, openDeliver(streamCtx, ts.client, "reader"))
		streamCancels = append(streamCancels, streamCancel)
	}
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// the next client stream is rejected before an iterator is created
	requireRejectedUnavailable(t, ts.client, "reader")
	require.Equal(t, clientLimit, ts.chain.ReaderCallCount())

	// orderer-to-orderer pulls draw from their own budget while the client
	// budget is full
	pullCtx, pullCancel := context.WithCancel(ctx)
	defer pullCancel()
	for range replicationLimit {
		require.NoError(t, openDeliver(pullCtx, ts.client, "orderer"))
	}
	require.Equal(t, clientLimit+replicationLimit, ts.chain.ReaderCallCount())

	// an orderer identity opening more than the replication budget is rejected
	requireRejectedUnavailable(t, ts.client, "orderer")
	require.Equal(t, clientLimit+replicationLimit, ts.chain.ReaderCallCount())

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
	ts := startDeliverTestServer(t, 1, -1)
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

	// stream B is admitted because A's permit was released on completion
	require.NoError(t, openDeliver(ctx, ts.client, "reader"))
}

func TestDeliverConcurrencyUnlimited(t *testing.T) {
	ts := startDeliverTestServer(t, -1, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range 7 {
		require.NoError(t, openDeliver(ctx, ts.client, "reader"))
	}
	require.Equal(t, 7, ts.chain.ReaderCallCount())
}
