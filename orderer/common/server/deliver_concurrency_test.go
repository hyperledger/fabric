/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
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
	"github.com/hyperledger/fabric/internal/pkg/comm"
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

// deliverTestService mirrors Deliver in server.go: the policy check passes
// for every caller and the stream is handed to the shared deliver handler.
type deliverTestService struct {
	ab.UnimplementedAtomicBroadcastServer
	h *deliver.Handler
}

func (s *deliverTestService) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	ds := &deliver.Server{
		PolicyChecker:  deliver.PolicyCheckerFunc(func(*cb.Envelope, string) error { return nil }),
		Receiver:       stream,
		ResponseSender: &deliverResponseSender{stream: stream},
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

func deliverSeekEnvelope() *cb.Envelope {
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
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{Creator: []byte("reader"), Nonce: []byte("n")}),
		},
		Data: protoutil.MarshalOrPanic(seek),
	}
	return &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload), Signature: []byte("sig")}
}

// The orderer must bound concurrent deliver streams: the first limit streams
// are admitted, the next one is rejected before an iterator is created, and
// closing an admitted stream admits a new one.
func TestDeliverConcurrencyLimit(t *testing.T) {
	const limit = 5

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
	conf.General.Limits.Concurrency.DeliverService = limit

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv, err := comm.NewGRPCServerFromListener(lis, initializeServerConfig(conf, nil))
	require.NoError(t, err)
	ab.RegisterAtomicBroadcastServer(srv.Server(), &deliverTestService{h: handler})
	go func() { _ = srv.Start() }()
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	client := ab.NewAtomicBroadcastClient(conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// openDeliver seeks from the oldest block and returns once the first
	// block arrives, proving the stream was admitted.
	openDeliver := func(streamCtx context.Context) error {
		stream, err := client.Deliver(streamCtx)
		if err != nil {
			return err
		}
		if err := stream.Send(deliverSeekEnvelope()); err != nil {
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

	// fill all permits with long-lived, slowly consumed streams
	streamCancels := make([]context.CancelFunc, 0, limit)
	for range limit {
		streamCtx, streamCancel := context.WithCancel(ctx)
		require.NoError(t, openDeliver(streamCtx))
		streamCancels = append(streamCancels, streamCancel)
	}
	require.Equal(t, limit, chain.ReaderCallCount())

	// the next stream is rejected by the concurrency limiter
	probe, err := client.Deliver(ctx)
	require.NoError(t, err)
	_ = probe.Send(deliverSeekEnvelope())
	_, err = probe.Recv()
	require.ErrorContains(t, err, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", ab.AtomicBroadcast_Deliver_FullMethodName, limit))
	require.Equal(t, limit, chain.ReaderCallCount())

	// closing an admitted stream releases its permit
	streamCancels[0]()
	require.Eventually(t, func() bool {
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()
		return openDeliver(streamCtx) == nil
	}, 30*time.Second, 50*time.Millisecond)
}
