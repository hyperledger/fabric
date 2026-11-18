/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	broadcastmock "github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// broadcastTestService wires Broadcast to the real broadcast handler, like
// server.go does, with a registrar standing in for the post-decode auth gate.
type broadcastTestService struct {
	ab.UnimplementedAtomicBroadcastServer
	h *broadcast.Handler
}

func (s *broadcastTestService) Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error {
	return s.h.Handle(stream)
}

func broadcastTestMetrics() *broadcast.Metrics {
	validate := &broadcastmock.MetricsHistogram{}
	validate.WithReturns(validate)
	enqueue := &broadcastmock.MetricsHistogram{}
	enqueue.WithReturns(enqueue)
	processed := &broadcastmock.MetricsCounter{}
	processed.WithReturns(processed)
	return &broadcast.Metrics{
		ValidateDuration: validate,
		EnqueueDuration:  enqueue,
		ProcessedCount:   processed,
	}
}

// The broadcast concurrency limit applies server-wide, across connections:
// with every permit held by an in-flight unauthenticated message spread over
// several connections, a stream on yet another connection is rejected before
// its message is decoded, and draining a stream frees its permit.
func TestBroadcastConcurrencyLimitAcrossConnections(t *testing.T) {
	const limit = 5

	release := make(chan struct{})
	var gateMessages atomic.Int64
	var gateBytes atomic.Int64
	registrar := &broadcastmock.ChannelSupportRegistrar{}
	registrar.BroadcastChannelSupportStub = func(env *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
		gateMessages.Add(1)
		gateBytes.Add(int64(len(env.GetPayload())))
		<-release // hold the permit until the test lets responses drain
		return nil, false, nil, errors.New("invalid signature")
	}
	handler := &broadcast.Handler{SupportRegistrar: registrar, Metrics: broadcastTestMetrics()}

	conf := &localconfig.TopLevel{
		General: localconfig.General{
			MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
			MaxSendMsgSize: comm.DefaultMaxSendMsgSize,
		},
	}
	conf.General.Limits.Concurrency.BroadcastService = limit

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv, err := comm.NewGRPCServerFromListener(lis, initializeServerConfig(conf, nil))
	require.NoError(t, err)
	ab.RegisterAtomicBroadcastServer(srv.Server(), &broadcastTestService{h: handler})
	go func() { _ = srv.Start() }()
	defer srv.Stop()

	dial := func() ab.AtomicBroadcastClient {
		conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		return ab.NewAtomicBroadcastClient(conn)
	}

	env := &cb.Envelope{Payload: make([]byte, 1<<20)} // no signature set

	// fill every permit with an in-flight, unauthenticated message, spread
	// over several connections
	clients := []ab.AtomicBroadcastClient{dial(), dial(), dial()}
	streams := make([]ab.AtomicBroadcast_BroadcastClient, 0, limit)
	for i := range limit {
		stream, err := clients[i%len(clients)].Broadcast(context.Background())
		require.NoError(t, err)
		require.NoError(t, stream.Send(env))
		streams = append(streams, stream)
	}
	require.Eventually(t, func() bool { return gateMessages.Load() == limit },
		10*time.Second, 10*time.Millisecond, "expected all %d streams to reach the message gate", limit)

	// a stream on yet another connection is rejected before any decode
	probe, err := dial().Broadcast(context.Background())
	require.NoError(t, err)
	_ = probe.Send(env)
	_, err = probe.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.ErrorContains(t, err, "exceeding concurrency limit")
	require.EqualValues(t, limit, gateMessages.Load())
	require.Equal(t, int64(limit)*int64(len(env.GetPayload())), gateBytes.Load())
	t.Logf("broadcastService=%d: %d connections saturated the server-wide budget; excess connection rejected pre-decode (%d messages, %d bytes decoded at gate)",
		limit, len(clients), gateMessages.Load(), gateBytes.Load())

	// drain the admitted streams: each gets the post-decode rejection
	close(release)
	for _, stream := range streams {
		resp, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, cb.Status_BAD_REQUEST, resp.GetStatus())
		require.NoError(t, stream.CloseSend())
	}

	// freed permits admit a new stream on any connection
	require.Eventually(t, func() bool {
		stream, err := clients[0].Broadcast(context.Background())
		if err != nil {
			return false
		}
		defer stream.CloseSend()
		if err := stream.Send(env); err != nil {
			return false
		}
		resp, err := stream.Recv()
		return err == nil && resp.GetStatus() == cb.Status_BAD_REQUEST
	}, 30*time.Second, 50*time.Millisecond)
	require.EqualValues(t, limit+1, gateMessages.Load())
}
