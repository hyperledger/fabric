/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	gmocks "github.com/hyperledger/fabric/gossip/comm/mocks"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
	goproto "google.golang.org/protobuf/proto"
)

// makeDataEnvelope builds a valid, decodable gossip envelope carrying a
// DataMsg whose payload occupies payloadBytes.
func makeDataEnvelope(payloadBytes int) *proto.Envelope {
	msg := &proto.GossipMessage{
		Channel: []byte("test"),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{
				Payload: &proto.Payload{
					SeqNum: uint64(time.Now().UnixNano()),
					Data:   make([]byte, payloadBytes),
				},
			},
		},
	}
	raw, err := goproto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return &proto.Envelope{Payload: raw}
}

// The receive pipeline queues are bounded in message count, so without a byte
// limit the retained memory scales with the sender-chosen message size. These
// tests pin the per-connection byte budget: retention must be capped by bytes
// regardless of message size, and oversized retention must drop the connection.

func TestReadFromStreamEnforcesReceiveByteLimit(t *testing.T) {
	for _, payloadSize := range []int{1 << 20, 4 << 20} {
		t.Run(fmt.Sprintf("payload=%dMiB", payloadSize>>20), func(t *testing.T) {
			msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
			budget := 8*msgSize + msgSize/2

			stream := &gmocks.MockStream{}
			stream.On("CloseSend").Return(nil)
			stream.On("Recv").Return(
				func() *proto.Envelope { return makeDataEnvelope(payloadSize) },
				func() error { return nil },
			)

			conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
				RecvBuffSize:         DefRecvBuffSize,
				SendBuffSize:         1,
				MaxRecvBytesInFlight: int(budget),
			})
			conn.logger = flogging.MustGetLogger("test")

			errChan := make(chan error, 1)
			msgChan := make(chan *protoext.SignedGossipMessage, DefRecvBuffSize)
			var wg sync.WaitGroup
			wg.Go(func() {
				conn.readFromStream(errChan, msgChan)
			})

			// Nothing drains msgChan, so the budget must trip once the
			// queued bytes exceed it.
			select {
			case err := <-errChan:
				require.ErrorContains(t, err, "received bytes in flight exceed")
			case <-time.After(30 * time.Second):
				t.Fatal("connection was not dropped after exceeding the receive byte limit")
			}

			// The tripping message is charged as well, hence +1.
			expectedAdmissions := int(budget/msgSize) + 1
			stream.AssertNumberOfCalls(t, "Recv", expectedAdmissions)
			require.Len(t, msgChan, expectedAdmissions-1)
			charged := conn.recvBytesInFlight.Load()
			require.Greater(t, charged, budget)
			require.LessOrEqual(t, charged-msgSize, budget)
			t.Logf("payload=%dMiB: queued bytes capped at %d (budget %d) after %d messages",
				payloadSize>>20, charged-msgSize, budget, expectedAdmissions-1)

			conn.close()
			wg.Wait()
		})
	}
}

func TestServiceConnectionDropsConnectionOverReceiveByteLimit(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
	budget := 8*msgSize + msgSize/2

	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope { return makeDataEnvelope(payloadSize) },
		func() error { return nil },
	)

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: int(budget),
	})
	conn.logger = flogging.MustGetLogger("test")

	// A consumer that processes nothing, mirroring a stalled downstream pipeline.
	handlerGate := make(chan struct{})
	conn.handler = func(*protoext.SignedGossipMessage) {
		<-handlerGate
	}

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- conn.serviceConnection()
	}()

	require.Eventually(t, func() bool {
		select {
		case <-conn.stopChan:
			return true
		default:
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "connection was not closed after exceeding the receive byte limit")

	expectedAdmissions := int(budget/msgSize) + 1
	stream.AssertNumberOfCalls(t, "Recv", expectedAdmissions)
	charged := conn.recvBytesInFlight.Load()
	require.Greater(t, charged, budget)
	require.LessOrEqual(t, charged-msgSize, budget)
	t.Logf("payload=%dMiB: per-connection retention stopped at %d queued bytes (budget %d)",
		payloadSize>>20, charged-msgSize, budget)

	conn.close()
	close(handlerGate) // unblock the handler so serviceConnection can exit
	<-svcDone
}

func TestReceiveByteLimitReleasedAfterHandling(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
	budget := 4 * msgSize
	const totalMessages = 10

	var recvCalls atomic.Int64
	handledCh := make(chan struct{}, totalMessages)
	done := make(chan struct{})

	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope {
			n := recvCalls.Add(1)
			if n > totalMessages {
				<-done // hold the stream open until the test is done
				return nil
			}
			if n >= 3 {
				<-handledCh // keep at most two messages unhandled
			}
			return makeDataEnvelope(payloadSize)
		},
		func() error {
			if recvCalls.Load() > totalMessages {
				return errors.New("no more messages")
			}
			return nil
		},
	)

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: int(budget),
	})
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = func(*protoext.SignedGossipMessage) {
		handledCh <- struct{}{}
	}

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- conn.serviceConnection()
	}()

	// The cumulative bytes exceed the budget several times over, yet a
	// connection whose messages are promptly handled must stay up and the
	// accounting must drain back to zero.
	require.Eventually(t, func() bool {
		return conn.recvBytesInFlight.Load() == 0 && recvCalls.Load() >= totalMessages
	}, 30*time.Second, time.Millisecond, "messages not all handled or accounted bytes not released")
	select {
	case err := <-svcDone:
		require.NoError(t, err)
		require.Fail(t, "connection dropped although the byte budget was never exceeded")
	default:
	}

	conn.close()
	close(done)
	<-svcDone
}
