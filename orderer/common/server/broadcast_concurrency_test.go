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

func rejectingRegistrar() *broadcastmock.ChannelSupportRegistrar {
	registrar := &broadcastmock.ChannelSupportRegistrar{}
	registrar.BroadcastChannelSupportStub = func(env *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
		return nil, false, nil, errors.New("invalid signature")
	}
	return registrar
}

// startBroadcastLimitServer stands up a real comm.GRPCServer with the
// broadcast concurrency limiter wired exactly like initializeServerConfig
// does, and counts streams that passed admission (a rejected stream never
// reaches the inner handler).
func startBroadcastLimitServer(t *testing.T, limiter *broadcastConcurrencyLimiter, registrar *broadcastmock.ChannelSupportRegistrar) (*grpc.ClientConn, *atomic.Int64) {
	t.Helper()

	handler := &broadcast.Handler{SupportRegistrar: registrar, Metrics: broadcastTestMetrics()}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	conf := &localconfig.TopLevel{
		General: localconfig.General{
			MaxRecvMsgSize: comm.DefaultMaxRecvMsgSize,
			MaxSendMsgSize: comm.DefaultMaxSendMsgSize,
		},
	}
	// disable the production-wired limiter; the test installs its own
	conf.General.Limits.Concurrency.BroadcastService = -1
	conf.General.Limits.Concurrency.BroadcastPreAuth = -1
	serverConfig := initializeServerConfig(conf, nil)
	var admitted atomic.Int64
	limiterInterceptor := limiter.interceptor()
	serverConfig.StreamInterceptors = append(serverConfig.StreamInterceptors,
		func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return limiterInterceptor(srv, ss, info, func(srv any, ss grpc.ServerStream) error {
				admitted.Add(1)
				return handler(srv, ss)
			})
		})
	srv, err := comm.NewGRPCServerFromListener(lis, serverConfig)
	require.NoError(t, err)
	ab.RegisterAtomicBroadcastServer(srv.Server(), &broadcastTestService{h: handler})
	go func() { _ = srv.Start() }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn, &admitted
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
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer probeCancel()
	probe, err := dial().Broadcast(probeCtx)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stream, err := clients[0].Broadcast(ctx)
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

// Streams that deliver no message cannot squat the server-wide budget
// forever: their permits expire with the idle timeout, so a legitimate
// client is rejected only while the squatting streams age out.
func TestBroadcastConcurrencyLimitIdleStreamsExpire(t *testing.T) {
	const limit = 3

	conn, admitted := startBroadcastLimitServer(t, newTestBroadcastLimiter(limit, limit, 2*time.Second), rejectingRegistrar())
	client := ab.NewAtomicBroadcastClient(conn)

	env := &cb.Envelope{Payload: make([]byte, 1<<20)}

	// squat every pre-auth permit with streams that never send a message
	squatters := make([]ab.AtomicBroadcast_BroadcastClient, 0, limit)
	defer func() {
		for _, stream := range squatters {
			stream.CloseSend()
		}
	}()
	for range limit {
		stream, err := client.Broadcast(context.Background())
		require.NoError(t, err)
		squatters = append(squatters, stream)
	}
	require.Eventually(t, func() bool { return admitted.Load() == limit },
		10*time.Second, 10*time.Millisecond, "squatters were not admitted")

	// while the squatters hold all permits, a legitimate stream is rejected
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer probeCancel()
	probe, err := client.Broadcast(probeCtx)
	require.NoError(t, err)
	_ = probe.Send(env)
	_, err = probe.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))

	// once the squatters time out, the legitimate client is served
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stream, err := client.Broadcast(ctx)
		if err != nil {
			return false
		}
		defer stream.CloseSend()
		if err := stream.Send(env); err != nil {
			return false
		}
		resp, err := stream.Recv()
		return err == nil && resp.GetStatus() == cb.Status_BAD_REQUEST
	}, 30*time.Second, 100*time.Millisecond)
	t.Logf("broadcastService=%d: %d idle streams squatted all permits; legitimate client rejected (%s) then admitted after the idle timeout",
		limit, limit, codes.ResourceExhausted)
}

// Reproduces the raft integration suite's TPS measurement pattern
// (integration/raft/cft_test.go measureTPS): 50 broadcast streams are
// opened to one orderer, all idle until released together, then sent.
// The default pre-auth allowance must admit every one of them.
func TestBroadcastDefaultPreAuthAllowsTPSFanout(t *testing.T) {
	const fanout = 50

	limiter := newTestBroadcastLimiter(0, 0, 30*time.Second) // zero = defaults
	require.GreaterOrEqual(t, cap(limiter.preAuthSema), fanout,
		"the default pre-auth allowance must cover the TPS fan-out of 50")
	conn, admitted := startBroadcastLimitServer(t, limiter, rejectingRegistrar())
	client := ab.NewAtomicBroadcastClient(conn)

	env := &cb.Envelope{Payload: make([]byte, 1<<20)}

	streams := make([]ab.AtomicBroadcast_BroadcastClient, 0, fanout)
	defer func() {
		for _, stream := range streams {
			stream.CloseSend()
		}
	}()
	for range fanout {
		stream, err := client.Broadcast(context.Background())
		require.NoError(t, err)
		streams = append(streams, stream)
	}
	require.Eventually(t, func() bool { return admitted.Load() == fanout },
		10*time.Second, 10*time.Millisecond, "all %d idle TPS streams must be admitted", fanout)

	// release all streams at once: every message is decoded and answered
	for _, stream := range streams {
		require.NoError(t, stream.Send(env))
	}
	for _, stream := range streams {
		resp, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, cb.Status_BAD_REQUEST, resp.GetStatus())
	}
}

// Idle streams draw only from the pre-authentication allowance: more idle
// streams than the entire main budget are admitted concurrently, and a
// client that sends its message is served from the untouched main budget
// while the squatters are still open.
func TestBroadcastConcurrencyLimitIdleStreamsSpareMainBudget(t *testing.T) {
	const mainLimit = 2
	const preAuthLimit = 5

	release := make(chan struct{})
	var gateMessages atomic.Int64
	registrar := &broadcastmock.ChannelSupportRegistrar{}
	registrar.BroadcastChannelSupportStub = func(env *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
		gateMessages.Add(1)
		<-release // hold the main permit until the test drains the streams
		return nil, false, nil, errors.New("invalid signature")
	}

	limiter := newTestBroadcastLimiter(mainLimit, preAuthLimit, 30*time.Second)
	conn, admitted := startBroadcastLimitServer(t, limiter, registrar)
	client := ab.NewAtomicBroadcastClient(conn)

	env := &cb.Envelope{Payload: make([]byte, 1<<20)}

	// a stream whose message is being processed holds one main permit
	active, err := client.Broadcast(context.Background())
	require.NoError(t, err)
	defer active.CloseSend()
	require.NoError(t, active.Send(env))
	require.Eventually(t, func() bool { return gateMessages.Load() == 1 },
		10*time.Second, 10*time.Millisecond, "first stream must reach the message gate")

	// squat every pre-auth permit; with a main permit charged at admission
	// (the pre-fix design) all but mainLimit-1 of these would be rejected
	squatters := make([]ab.AtomicBroadcast_BroadcastClient, 0, preAuthLimit)
	defer func() {
		for _, stream := range squatters {
			stream.CloseSend()
		}
	}()
	for range preAuthLimit {
		stream, err := client.Broadcast(context.Background())
		require.NoError(t, err)
		squatters = append(squatters, stream)
	}
	require.Eventually(t, func() bool { return admitted.Load() == preAuthLimit+1 },
		10*time.Second, 10*time.Millisecond, "idle squatters were not admitted past the main budget")

	// a new stream is rejected only at the pre-auth gate
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer probeCancel()
	probe, err := client.Broadcast(probeCtx)
	require.NoError(t, err)
	_ = probe.Send(env)
	_, err = probe.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))

	require.Len(t, limiter.preAuthSema, preAuthLimit, "squatters must hold every pre-auth permit")

	// freeing one pre-auth permit admits a new stream whose message is
	// processed from the main budget, even though the remaining squatters
	// still outnumber it
	require.NoError(t, squatters[0].CloseSend())
	require.Eventually(t, func() bool { return len(limiter.preAuthSema) < preAuthLimit },
		10*time.Second, 10*time.Millisecond, "closing a squatter must free its pre-auth permit")
	late, err := client.Broadcast(context.Background())
	require.NoError(t, err)
	defer late.CloseSend()
	require.NoError(t, late.Send(env))
	require.Eventually(t, func() bool { return gateMessages.Load() == 2 },
		10*time.Second, 10*time.Millisecond, "main budget must remain available while idle squatters hold pre-auth permits")

	close(release)
	for _, stream := range []ab.AtomicBroadcast_BroadcastClient{active, late} {
		resp, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, cb.Status_BAD_REQUEST, resp.GetStatus())
	}
}
