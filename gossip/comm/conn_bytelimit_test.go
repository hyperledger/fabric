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
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
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
// tests pin the byte budgets: retention must be capped by bytes regardless of
// message size, and exceeding connections must be dropped and evicted.

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

			// The tripping message is released again, hence +1.
			expectedAdmissions := int(budget/msgSize) + 1
			stream.AssertNumberOfCalls(t, "Recv", expectedAdmissions)
			require.Len(t, msgChan, expectedAdmissions-1)
			charged := conn.recvBytesInFlight.Load()
			require.LessOrEqual(t, charged, budget)
			require.Greater(t, charged+msgSize, budget)
			t.Logf("payload=%dMiB: queued bytes capped at %d (budget %d) after %d messages",
				payloadSize>>20, charged, budget, expectedAdmissions-1)

			conn.close()
			wg.Wait()
		})
	}
}

// TestServiceConnectionTerminatesOnOverflow exercises the production receive
// path: a real demultiplexer whose subscriber is stalled. The overflow must
// abort the wedged delivery, terminate serviceConnection and close the
// connection, and the accounting must drain once messages are consumed.
func TestServiceConnectionTerminatesOnOverflow(t *testing.T) {
	for _, payloadSize := range []int{1 << 20, 4 << 20} {
		t.Run(fmt.Sprintf("payload=%dMiB", payloadSize>>20), func(t *testing.T) {
			msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
			budget := 16 * msgSize

			c := &commImpl{
				msgPublisher: NewChannelDemultiplexer(),
				pubSub:       util.NewPubSub(),
				logger:       flogging.MustGetLogger("test"),
			}
			sub := c.msgPublisher.AddChannel(func(any) bool { return true }) // never drained

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
			conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: []byte("peer")})

			svcDone := make(chan error, 1)
			go func() {
				svcDone <- conn.serviceConnection()
			}()

			// The connection must terminate even though the handler wedges
			// on the stalled subscriber.
			select {
			case <-svcDone:
			case <-time.After(30 * time.Second):
				t.Fatal("serviceConnection did not return after the receive byte limit tripped")
			}

			// Tripping happens on the first admission that crosses the budget.
			expectedAdmissions := int(budget/msgSize) + 1
			stream.AssertNumberOfCalls(t, "Recv", expectedAdmissions)

			select {
			case <-conn.stopChan:
			default:
				t.Fatal("connection was not closed after exceeding the receive byte limit")
			}

			// Undelivered and undrained charges were released on the way out;
			// only messages sitting in the stalled subscriber remain charged.
			charged := conn.recvBytesInFlight.Load()
			require.LessOrEqual(t, charged, 10*msgSize)
			t.Logf("payload=%dMiB: terminated with %d bytes charged to the stalled subscriber (budget %d)",
				payloadSize>>20, charged, budget)

			// Consuming the stalled subscriber's queue releases the rest.
			for {
				select {
				case m := <-sub:
					m.(*ReceivedMessageImpl).Dispose()
				default:
					require.Zero(t, conn.recvBytesInFlight.Load())
					return
				}
			}
		})
	}
}

// TestReceiveByteLimitReleasedOnConsume runs the full production delivery
// path (demultiplexer, Accept relay, consumer) with a consumer that keeps up:
// a connection whose cumulative traffic exceeds the budget must stay up, and
// the accounting must drain back to zero.
func TestReceiveByteLimitReleasedOnConsume(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
	budget := 4 * msgSize
	const totalMessages = 10

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		lock:         &sync.Mutex{},
		exitChan:     make(chan struct{}),
		logger:       flogging.MustGetLogger("test"),
	}
	inc := c.Accept(func(any) bool { return true })

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

	shared := &sharedRecvBudget{max: 1 << 60}
	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: int(budget),
		sharedRecvBudget:     shared,
	})
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: []byte("peer")})

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for {
			select {
			case m := <-inc:
				DisposeMessage(m)
				handledCh <- struct{}{}
			case <-done:
				return
			}
		}
	}()

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- conn.serviceConnection()
	}()

	// The cumulative bytes exceed the budget several times over, yet a
	// connection whose messages are promptly consumed must stay up and both
	// budgets must drain back to zero.
	require.Eventually(t, func() bool {
		return conn.recvBytesInFlight.Load() == 0 &&
			shared.inFlight.Load() == 0 &&
			recvCalls.Load() > totalMessages
	}, 30*time.Second, time.Millisecond, "accounting did not drain after all messages were consumed")
	select {
	case err := <-svcDone:
		require.NoError(t, err)
		require.Fail(t, "connection dropped although the byte budget was never exceeded")
	default:
	}

	conn.close()
	close(done)
	<-svcDone
	<-consumerDone
}

type overflowConnFactory struct {
	config  ConnConfig
	payload int
	pkiID   common.PKIidType
	created atomic.Int32
}

func (f *overflowConnFactory) createConnection(string, common.PKIidType) (*connection, error) {
	f.created.Add(1)
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope { return makeDataEnvelope(f.payload) },
		func() error { return nil },
	)
	conn := newConnection(nil, nil, stream, disabledMetrics, f.config)
	conn.pkiID = f.pkiID
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = func(*protoext.SignedGossipMessage) {} // never consumes
	return conn, nil
}

// TestEvictedFromStoreOnOverflow covers the outbound path: once a connection
// trips the budget it must be removed from the connection store, so the next
// send dials a fresh connection instead of reusing the dead one.
func TestEvictedFromStoreOnOverflow(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))
	factory := &overflowConnFactory{
		config: ConnConfig{
			RecvBuffSize:         DefRecvBuffSize,
			SendBuffSize:         1,
			MaxRecvBytesInFlight: int(2 * msgSize),
		},
		payload: payloadSize,
		pkiID:   []byte("pkiID"),
	}
	cs := newConnStore(factory, flogging.MustGetLogger("test"), ConnConfig{})
	peer := &RemotePeer{PKIID: factory.pkiID, Endpoint: "localhost:1234"}

	conn, err := cs.getConnection(peer)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return cs.connNum() == 0
	}, 30*time.Second, time.Millisecond, "overflowing connection was not evicted from the store")

	select {
	case <-conn.stopChan:
	default:
		t.Fatal("overflowing connection was not closed")
	}

	// The next send dials anew instead of getting the dead connection back.
	conn2, err := cs.getConnection(peer)
	require.NoError(t, err)
	require.NotSame(t, conn, conn2)
	require.EqualValues(t, 2, factory.created.Load())
	conn2.close()
}

// TestOverflowFailsQueuedSends ensures sends still queued when a connection
// is dropped on overflow fail fast via onErr instead of being silently stuck.
func TestOverflowFailsQueuedSends(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))

	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope { return makeDataEnvelope(payloadSize) },
		func() error { return nil },
	)

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         4,
		MaxRecvBytesInFlight: int(2 * msgSize),
	})
	conn.logger = flogging.MustGetLogger("test")

	errCh := make(chan error, 3)
	msg := createGossipMsg()
	for range 3 { // nobody drains outBuff: writeToStream is not running
		conn.send(msg, func(err error) { errCh <- err }, blockingSend)
	}

	errRecv := make(chan error, 1)
	msgChan := make(chan *protoext.SignedGossipMessage, DefRecvBuffSize)
	go conn.readFromStream(errRecv, msgChan)

	select {
	case err := <-errRecv:
		require.ErrorContains(t, err, "received bytes in flight exceed")
	case <-time.After(30 * time.Second):
		t.Fatal("connection did not trip the receive byte limit")
	}
	for range 3 {
		select {
		case err := <-errCh:
			require.ErrorContains(t, err, "received bytes in flight exceed")
		case <-time.After(30 * time.Second):
			t.Fatal("queued send was not failed on overflow")
		}
	}
}

type trackableMessage struct {
	tracked  atomic.Int32
	disposed atomic.Int32
}

func (m *trackableMessage) trackDeliveries(n int) { m.tracked.Store(int32(n)) }
func (m *trackableMessage) Dispose()              { m.disposed.Add(1) }

// TestDeMultiplexAbortable pins the delivery-tracking contract: the delivery
// count is reported before the first delivery, and aborted or undeliverable
// deliveries are disposed by the demultiplexer itself.
func TestDeMultiplexAbortable(t *testing.T) {
	t.Run("abort disposes the undelivered", func(t *testing.T) {
		demux := NewChannelDemultiplexer()
		defer demux.Close()
		sub := demux.AddChannel(acceptAll)
		for range 10 { // fill the subscription buffer
			demux.DeMultiplex(struct{}{})
		}

		msg := &trackableMessage{}
		abort := make(chan struct{})
		returned := make(chan struct{})
		go func() {
			demux.DeMultiplexAbortable(msg, abort)
			close(returned)
		}()
		close(abort)
		select {
		case <-returned:
		case <-time.After(30 * time.Second):
			t.Fatal("DeMultiplexAbortable did not return after abort")
		}
		require.EqualValues(t, 1, msg.tracked.Load())
		require.EqualValues(t, 1, msg.disposed.Load())
		require.Len(t, sub, 10)
	})

	t.Run("delivery is tracked not disposed", func(t *testing.T) {
		demux := NewChannelDemultiplexer()
		defer demux.Close()
		sub := demux.AddChannel(acceptAll)

		msg := &trackableMessage{}
		demux.DeMultiplexAbortable(msg, nil)
		require.EqualValues(t, 1, msg.tracked.Load())
		require.Zero(t, msg.disposed.Load())
		require.Len(t, sub, 1)
	})

	t.Run("closed demultiplexer reports zero deliveries", func(t *testing.T) {
		demux := NewChannelDemultiplexer()
		demux.AddChannel(acceptAll)
		demux.Close()

		msg := &trackableMessage{}
		demux.DeMultiplexAbortable(msg, nil)
		require.Zero(t, msg.tracked.Load())
		require.Zero(t, msg.disposed.Load())
	})
}

// TestSharedBudgetEvictsLargestConnection ensures a node-wide budget excess
// drops the connection holding the most in-flight bytes, not bystanders.
func TestSharedBudgetEvictsLargestConnection(t *testing.T) {
	bigPayload := 1 << 20
	bigSize := int64(goproto.Size(makeDataEnvelope(bigPayload)))

	shared := &sharedRecvBudget{max: 12 * bigSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{})
	shared.evict = cs.evictLargestConnection

	done := make(chan struct{})
	newTestConn := func(pkiID string, payloadSize int, once bool) *connection {
		var calls atomic.Int64
		stream := &gmocks.MockStream{}
		stream.On("CloseSend").Return(nil)
		stream.On("Recv").Return(
			func() *proto.Envelope {
				if once && calls.Add(1) > 1 {
					<-done // the bystander falls silent after one message
					return nil
				}
				return makeDataEnvelope(payloadSize)
			},
			func() error {
				if once && calls.Load() > 1 {
					return errors.New("no more messages")
				}
				return nil
			},
		)
		conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
			RecvBuffSize:     DefRecvBuffSize,
			SendBuffSize:     1,
			sharedRecvBudget: shared,
		})
		conn.pkiID = common.PKIidType(pkiID)
		conn.logger = flogging.MustGetLogger("test")
		conn.handler = func(*protoext.SignedGossipMessage) {} // never consumes
		cs.pki2Conn[pkiID] = conn
		conn.evictFromStore = func() { cs.evictConn(conn) }
		return conn
	}

	flooder := newTestConn("flooder", bigPayload, false)
	bystander := newTestConn("bystander", 1024, true)

	go flooder.serviceConnection()
	go bystander.serviceConnection()

	// The flooder alone exceeds the shared budget and must be evicted; the
	// bystander's connection must survive.
	require.Eventually(t, func() bool {
		select {
		case <-flooder.stopChan:
			return true
		default:
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "flooding connection was not evicted on shared budget excess")
	require.Eventually(t, func() bool {
		return cs.connNum() == 1
	}, 30*time.Second, time.Millisecond, "flooding connection was not removed from the store")

	select {
	case <-bystander.stopChan:
		t.Fatal("bystander connection was dropped because of another peer's flood")
	default:
	}

	bystander.close()
	close(done)
}
