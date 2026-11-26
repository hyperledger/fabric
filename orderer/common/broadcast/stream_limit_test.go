/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type atomicBroadcastService struct {
	ab.UnimplementedAtomicBroadcastServer
	h *broadcast.Handler
}

func (s *atomicBroadcastService) Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error {
	return s.h.Handle(stream)
}

func testBroadcastMetrics() *broadcast.Metrics {
	validate := &mock.MetricsHistogram{}
	validate.WithReturns(validate)
	enqueue := &mock.MetricsHistogram{}
	enqueue.WithReturns(enqueue)
	processed := &mock.MetricsCounter{}
	processed.WithReturns(processed)
	return &broadcast.Metrics{
		ValidateDuration: validate,
		EnqueueDuration:  enqueue,
		ProcessedCount:   processed,
	}
}

// startBroadcastServer stands up a real comm.GRPCServer with the production
// message-size limits, exactly as orderer startup wires them.
func startBroadcastServer(t *testing.T, serverConfig comm.ServerConfig, registrar *mock.ChannelSupportRegistrar) *grpc.ClientConn {
	t.Helper()
	srv, err := comm.NewGRPCServer("127.0.0.1:0", serverConfig)
	require.NoError(t, err)
	handler := &broadcast.Handler{SupportRegistrar: registrar, Metrics: testBroadcastMetrics()}
	ab.RegisterAtomicBroadcastServer(srv.Server(), &atomicBroadcastService{h: handler})
	go func() { _ = srv.Start() }()
	conn, err := grpc.NewClient(srv.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
		srv.Stop()
	})
	return conn
}

// With a concurrent-stream limit configured, streams beyond the limit are
// queued client-side (HTTP/2 SETTINGS / REFUSED_STREAM retry), so the server
// never decodes their messages while every slot is occupied.
func TestBroadcastExcessStreamQueuedBeforeDecode(t *testing.T) {
	const maxStreams = 4

	release := make(chan struct{})
	var gateMessages atomic.Int64
	var gateBytes atomic.Int64
	registrar := &mock.ChannelSupportRegistrar{}
	registrar.BroadcastChannelSupportStub = func(env *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
		gateMessages.Add(1)
		gateBytes.Add(int64(len(env.GetPayload())))
		<-release // hold the stream slot until the test lets responses drain
		return nil, false, nil, errors.New("invalid signature")
	}

	conn := startBroadcastServer(t, comm.ServerConfig{
		Logger:               flogging.MustGetLogger("broadcast.test"),
		MaxRecvMsgSize:       comm.DefaultMaxRecvMsgSize,
		MaxSendMsgSize:       comm.DefaultMaxSendMsgSize,
		MaxConcurrentStreams: maxStreams,
	}, registrar)
	client := ab.NewAtomicBroadcastClient(conn)

	env := &cb.Envelope{Payload: make([]byte, 1<<20)} // no signature set

	// occupy every stream slot with an in-flight, unauthenticated message
	streams := make([]ab.AtomicBroadcast_BroadcastClient, 0, maxStreams)
	for range maxStreams {
		stream, err := client.Broadcast(context.Background())
		require.NoError(t, err)
		require.NoError(t, stream.Send(env))
		streams = append(streams, stream)
	}
	require.Eventually(t, func() bool { return gateMessages.Load() == maxStreams },
		10*time.Second, 10*time.Millisecond, "expected all %d streams to reach the message gate", maxStreams)

	// the excess stream is queued at the transport while every slot is
	// occupied; its message must not be decoded server-side
	excessCtx, excessCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer excessCancel()
	excessDone := make(chan error, 1)
	go func() {
		stream, err := client.Broadcast(excessCtx)
		if err == nil {
			if err = stream.Send(env); err == nil {
				var resp *ab.BroadcastResponse
				if resp, err = stream.Recv(); err == nil && resp.GetStatus() != cb.Status_BAD_REQUEST {
					err = errors.Errorf("unexpected status %v", resp.GetStatus())
				}
			}
		}
		excessDone <- err
	}()
	select {
	case err := <-excessDone:
		t.Fatalf("excess stream completed while all slots were occupied: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	require.EqualValues(t, maxStreams, gateMessages.Load(),
		"excess stream message reached the decode/auth gate")
	require.Equal(t, maxStreams*int64(len(env.GetPayload())), gateBytes.Load())
	t.Logf("maxConcurrentStreams=%d: excess stream queued pre-decode; decoded at gate: %d messages, %d bytes",
		maxStreams, gateMessages.Load(), gateBytes.Load())

	// drain the held streams: each gets the post-decode rejection
	close(release)
	for _, stream := range streams {
		resp, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, cb.Status_BAD_REQUEST, resp.GetStatus())
		require.NoError(t, stream.CloseSend())
	}

	// with a slot freed, the queued stream is served: decoded, then rejected
	select {
	case err := <-excessDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("queued stream was not served after a slot was freed")
	}
	require.EqualValues(t, maxStreams+1, gateMessages.Load())
	t.Logf("after draining a slot: queued stream decoded and rejected; decoded at gate: %d messages", gateMessages.Load())
}

// The documented sizing rule holds at the defaults: the transport stream
// cap must cover the broadcast concurrency budgets, or it would throttle
// legitimate broadcasts before the service budgets are reached.
func TestBroadcastDefaultLimitsBoundary(t *testing.T) {
	defaults := localconfig.Defaults.General.Limits.Concurrency
	require.GreaterOrEqual(t, comm.DefaultMaxConcurrentStreams, uint32(defaults.BroadcastService),
		"the default transport stream cap must cover the default broadcast budget")
	require.GreaterOrEqual(t, comm.DefaultMaxConcurrentStreams, uint32(defaults.BroadcastPreAuth),
		"the default transport stream cap must cover the default pre-auth broadcast budget")
	require.Positive(t, defaults.BroadcastPreAuth)
	require.Less(t, defaults.BroadcastPreAuth, defaults.BroadcastService,
		"the pre-auth allowance must be much smaller than the broadcast budget")
}
