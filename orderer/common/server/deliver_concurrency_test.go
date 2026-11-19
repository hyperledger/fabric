/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
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

// ordererReadersPolicyManager serves a single policy, /Channel/Orderer/Readers,
// satisfied only by identities containing "orderer", mirroring how orderer
// nodes are classified by channel MSP NodeOUs in production.
type ordererReadersPolicyManager struct{}

func (ordererReadersPolicyManager) GetPolicy(id string) (policies.Policy, bool) {
	if id != policies.ChannelOrdererReaders {
		return nil, false
	}
	return ordererReadersPolicy{}, true
}

func (ordererReadersPolicyManager) Manager([]string) (policies.Manager, bool) {
	return nil, false
}

type ordererReadersPolicy struct{}

func (ordererReadersPolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	for _, sd := range signatureSet {
		if bytes.Contains(sd.Identity, []byte("orderer")) {
			return nil
		}
	}
	return errors.Errorf("identity is not an orderer")
}

func (ordererReadersPolicy) EvaluateIdentities([]msp.Identity) error { return nil }

// deliverChainWithOrdererPolicy returns a mock chain whose policy manager
// resolves /Channel/Orderer/Readers.
func deliverChainWithOrdererPolicy() *mock.Chain {
	chain := &mock.Chain{}
	chain.PolicyManagerReturns(ordererReadersPolicyManager{})
	return chain
}

// deliverTestService mirrors Deliver in server.go: the channel readers
// policy check is mocked to pass for every caller, orderer-to-orderer pulls
// are exempted, and sessions are admitted by the shared deliver handler
// through its SessionLimiter hook.
type deliverTestService struct {
	ab.UnimplementedAtomicBroadcastServer
	h       *deliver.Handler
	limiter *deliverConcurrencyLimiter
	chain   deliver.Chain
}

func (s *deliverTestService) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	session := s.limiter.newSession()
	ds := &deliver.Server{
		PolicyChecker: deliver.PolicyCheckerFunc(func(env *cb.Envelope, _ string) error {
			if satisfiesOrdererReadersPolicy(s.chain, env) {
				session.Exempt()
			}
			return nil
		}),
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
	seek := &ab.SeekInfo{
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
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
// the given concurrency limit, mirroring the production Deliver path.
func startDeliverTestServer(t *testing.T, limit int) *deliverTestServer {
	t.Helper()

	shared := make([]*cb.Block, deliverLimitTestBlocks)
	for i := range shared {
		shared[i] = &cb.Block{
			Header: &cb.BlockHeader{Number: uint64(i)},
			Data:   &cb.BlockData{Data: [][]byte{make([]byte, deliverLimitTestBlkSize)}},
		}
	}
	chain := deliverChainWithOrdererPolicy()
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
		limiter: newDeliverConcurrencyLimiter(limit, &disabled.Provider{}),
		chain:   chain,
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

// The orderer bounds concurrent deliver sessions: idle streams consume no
// budget, the first limit authenticated streams are admitted, the next one
// is rejected before an iterator is created, orderer-to-orderer pulls are
// exempt, and closing an admitted stream admits a new one.
func TestDeliverConcurrencyLimit(t *testing.T) {
	const limit = 5

	ts := startDeliverTestServer(t, limit)
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

	// fill the budget with long-lived, slowly consumed client streams
	streamCancels := make([]context.CancelFunc, 0, limit)
	for range limit {
		streamCtx, streamCancel := context.WithCancel(ctx)
		require.NoError(t, openDeliver(streamCtx, ts.client, "reader"))
		streamCancels = append(streamCancels, streamCancel)
	}
	require.Equal(t, limit, ts.chain.ReaderCallCount())

	// the next client stream is rejected before an iterator is created
	probe, err := ts.client.Deliver(ctx)
	require.NoError(t, err)
	_ = probe.Send(deliverSeekEnvelope("reader"))
	_, err = probe.Recv()
	require.ErrorContains(t, err, fmt.Sprintf("too many deliver requests, exceeding concurrency limit (%d)", limit))
	require.Equal(t, limit, ts.chain.ReaderCallCount())

	// an orderer-to-orderer block pull is admitted despite the full budget
	pullCtx, pullCancel := context.WithCancel(ctx)
	defer pullCancel()
	require.NoError(t, openDeliver(pullCtx, ts.client, "orderer"))

	// closing an admitted stream releases its permit
	streamCancels[0]()
	require.Eventually(t, func() bool {
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()
		return openDeliver(streamCtx, ts.client, "reader") == nil
	}, 30*time.Second, 50*time.Millisecond)
}

func TestDeliverConcurrencyUnlimited(t *testing.T) {
	ts := startDeliverTestServer(t, -1)
	defer ts.cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range 7 {
		require.NoError(t, openDeliver(ctx, ts.client, "reader"))
	}
	require.Equal(t, 7, ts.chain.ReaderCallCount())
}
