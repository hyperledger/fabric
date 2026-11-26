/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	libmetrics "github.com/hyperledger/fabric-lib-go/common/metrics"
	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	gmocks "github.com/hyperledger/fabric/gossip/comm/mocks"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
	defer close(c.exitChan)      // releases the Accept relay
	defer c.msgPublisher.Close() // closes subscriber channels
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
	cs := newConnStore(factory, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
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

// TestOverflowFailsQueuedSends ensures sends outstanding when a connection
// is torn down fail fast via onErr instead of being silently stuck.
func TestOverflowFailsQueuedSends(t *testing.T) {
	payloadSize := 1 << 20
	msgSize := int64(goproto.Size(makeDataEnvelope(payloadSize)))

	sendGate := make(chan struct{})
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Send", mock.Anything).Return(func(*proto.Envelope) error {
		<-sendGate // hold the first send so the rest stay queued
		return errors.New("stream dead")
	})
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
	conn.handler = func(*protoext.SignedGossipMessage) {} // never consumes

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- conn.serviceConnection()
	}()

	errCh := make(chan error, 3)
	msg := createGossipMsg()
	for range 3 {
		conn.send(msg, func(err error) { errCh <- err }, blockingSend)
	}

	select {
	case <-svcDone:
	case <-time.After(30 * time.Second):
		t.Fatal("serviceConnection did not return after the receive byte limit tripped")
	}
	close(sendGate) // let writeToStream exit and fail the queued sends

	// Every outstanding send must fail exactly once, via the writeToStream
	// exit drain or the closed-aware send path.
	for range 3 {
		select {
		case err := <-errCh:
			require.Error(t, err)
		case <-time.After(30 * time.Second):
			t.Fatal("queued send was not failed on overflow")
		}
	}
}

type noopServerStream struct {
	grpc.ServerStream
}

func (noopServerStream) Send(*proto.Envelope) error { return nil }

func (noopServerStream) Recv() (*proto.Envelope, error) { return nil, errors.New("noop") }

// TestReplacedConnectionDrainKeepsSuccessor drives the discovery path: sends
// outstanding on a replaced connection must fail via onErr, but must neither
// publish a death event nor tear down the healthy successor through it.
func TestReplacedConnectionDrainKeepsSuccessor(t *testing.T) {
	pkiID := []byte("pkiID")
	sendGate := make(chan struct{})
	done := make(chan struct{})
	defer close(done)

	oldStream := &gmocks.MockStream{}
	oldStream.On("CloseSend").Return(nil)
	oldStream.On("Send", mock.Anything).Return(func(*proto.Envelope) error {
		<-sendGate // hold the first send so the rest stay queued
		return errors.New("dead stream")
	})
	oldStream.On("Recv").Return(func() *proto.Envelope { <-done; return nil }, func() error {
		<-done
		return errors.New("done")
	})

	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	oldConn := newConnection(nil, nil, oldStream, disabledMetrics, ConnConfig{RecvBuffSize: 1, SendBuffSize: 4})
	oldConn.pkiID = pkiID
	oldConn.logger = flogging.MustGetLogger("test")
	oldConn.handler = func(*protoext.SignedGossipMessage) {}
	cs.pki2Conn[string(pkiID)] = oldConn
	oldConn.evictFromStore = func() { cs.evictConn(oldConn) }
	oldConn.store = cs

	c := &commImpl{
		logger:        flogging.MustGetLogger("test"),
		connStore:     cs,
		deadEndpoints: make(chan common.PKIidType, 100),
		exitChan:      make(chan struct{}),
	}
	defer close(c.exitChan)
	// Mimics discovery's expireDeadMembers -> CloseConn reaction.
	go func() {
		for {
			select {
			case id := <-c.deadEndpoints:
				cs.closeConnByPKIid(id)
			case <-c.exitChan:
				return
			}
		}
	}()

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- oldConn.serviceConnection()
	}()

	peer := &RemotePeer{PKIID: pkiID, Endpoint: "localhost:1234"}
	msg := createGossipMsg()
	for range 3 {
		c.sendToEndpoint(peer, msg, blockingSend)
	}

	// The successor registers; the old connection is closed and its queued
	// sends fail via the writeToStream drain and stream error.
	newConn := cs.onConnected(noopServerStream{}, &protoext.ConnectionInfo{ID: pkiID}, disabledMetrics)
	close(sendGate)

	// The old writer terminates and its queue is drained...
	require.Eventually(t, func() bool {
		oldConn.sendMu.Lock()
		defer oldConn.sendMu.Unlock()
		return oldConn.sendClosed
	}, 30*time.Second, time.Millisecond, "replaced connection's writer did not terminate")
	// ...but it must never publish a death event for the peer.
	require.Never(t, func() bool {
		select {
		case <-c.deadEndpoints:
			return true
		default:
			return false
		}
	}, 3*time.Second, 50*time.Millisecond, "replaced connection must not publish a death event")
	select {
	case <-newConn.stopChan:
		t.Fatal("successor connection was torn down by its predecessor's failed sends")
	default:
	}
	require.Equal(t, 1, cs.connNum())

	// A failure of the still-current successor must still publish one.
	newStream := &gmocks.MockStream{}
	newStream.On("CloseSend").Return(nil)
	newStream.On("Send", mock.Anything).Return(errors.New("dead stream"))
	newStream.On("Recv").Return(func() *proto.Envelope { <-done; return nil }, func() error {
		<-done
		return errors.New("done")
	})
	newConn.gossipStream = newStream
	go newConn.serviceConnection()
	c.sendToEndpoint(peer, msg, blockingSend)
	// The fake consumer only removes the connection once the death event is
	// published, so the store emptying proves the notification still works
	// for a still-current connection.
	require.Eventually(t, func() bool {
		return cs.connNum() == 0
	}, 30*time.Second, time.Millisecond, "current connection's failure must publish a death event")

	<-svcDone
}

// TestReceiveAccountingDrainsOnClose stresses the handoff/teardown race:
// closing a connection with an active reader must never leak byte accounting.
func TestReceiveAccountingDrainsOnClose(t *testing.T) {
	for i := range 300 {
		stream := &gmocks.MockStream{}
		stream.On("CloseSend").Return(nil)
		stream.On("Recv").Return(
			func() *proto.Envelope { return makeDataEnvelope(1024) },
			func() error { return nil },
		)

		shared := &sharedRecvBudget{max: 1 << 60}
		conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
			RecvBuffSize:         DefRecvBuffSize,
			SendBuffSize:         1,
			MaxRecvBytesInFlight: 1 << 60,
			sharedRecvBudget:     shared,
		})
		conn.logger = flogging.MustGetLogger("test")
		conn.handler = func(m *protoext.SignedGossipMessage) {
			conn.releaseReceivedBytes(m) // consumed and released immediately
		}

		svcDone := make(chan error, 1)
		go func() {
			svcDone <- conn.serviceConnection()
		}()
		time.Sleep(time.Millisecond)
		conn.close()
		<-svcDone

		require.Eventually(t, func() bool {
			return conn.recvBytesInFlight.Load() == 0 && shared.inFlight.Load() == 0
		}, 10*time.Second, 100*time.Microsecond, "iteration %d leaked receive byte accounting", i)
	}
}

// TestSharedBudgetBystanderSurvivesAfterEviction covers the eviction cascade:
// a bystander whose first send arrives after the flooder was evicted must not
// be dropped while the flooder's delivered charges still clog the budget.
func TestSharedBudgetBystanderSurvivesAfterEviction(t *testing.T) {
	bigPayload := 1 << 20
	bigSize := int64(goproto.Size(makeDataEnvelope(bigPayload)))
	smallSize := int64(goproto.Size(makeDataEnvelope(1024)))

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	c.msgPublisher.AddChannel(func(any) bool { return true }) // stalled subscriber

	shared := &sharedRecvBudget{max: 3 * bigSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	done := make(chan struct{})
	bystanderSecond := make(chan struct{})
	newTestConn := func(pkiID string, payloadSize int, gated bool) *connection {
		var calls atomic.Int64
		stream := &gmocks.MockStream{}
		stream.On("CloseSend").Return(nil)
		stream.On("Recv").Return(
			func() *proto.Envelope {
				n := calls.Add(1)
				if gated {
					if n == 2 {
						select {
						case <-bystanderSecond:
						case <-done:
						}
					}
					if n > 2 {
						<-done // the bystander falls silent after two messages
						return nil
					}
				}
				return makeDataEnvelope(payloadSize)
			},
			func() error {
				if gated && calls.Load() > 2 {
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
		conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: conn.pkiID})
		cs.pki2Conn[pkiID] = conn
		conn.evictFromStore = func() { cs.evictConn(conn) }
		return conn
	}

	flooder := newTestConn("flooder", bigPayload, false)
	bystander := newTestConn("bystander", 1024, true)

	go flooder.serviceConnection()
	go bystander.serviceConnection()

	// The flooder alone exceeds the shared budget and must be evicted.
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

	// The bystander's second message trips the shared budget again, but the
	// largest charge is the dead flooder's, so the bystander must survive.
	close(bystanderSecond)
	require.Eventually(t, func() bool {
		return bystander.recvBytesInFlight.Load() >= 2*smallSize
	}, 30*time.Second, time.Millisecond, "bystander's second message was not admitted")

	select {
	case <-bystander.stopChan:
		t.Fatal("bystander connection was dropped because of another peer's flood")
	default:
	}
	require.Equal(t, 1, cs.connNum())

	bystander.close()
	close(done)
}

// TestSharedBudgetPolicyUsesAggregateDebt pins the node-wide policy: live
// connections overcommit while the overage is retired debt, and the largest
// live connection is evicted only when aggregate live debt exceeds the budget.
// It also proves retired entries are purged on final release without another
// budget overage.
func TestSharedBudgetPolicyUsesAggregateDebt(t *testing.T) {
	msgOf := func() *protoext.SignedGossipMessage {
		msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
		require.NoError(t, err)
		return msg
	}
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))

	shared := &sharedRecvBudget{max: 15 * msgSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	newChargedConn := func(pkiID string) *connection {
		conn := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
		conn.pkiID = common.PKIidType(pkiID)
		conn.logger = flogging.MustGetLogger("test")
		cs.pki2Conn[pkiID] = conn
		conn.evictFromStore = func() { cs.evictConn(conn) }
		conn.store = cs
		return conn
	}
	// chargeDelivered emulates a received message: the receive path charges
	// both the connection and the shared budget on admission.
	chargeDelivered := func(conn *connection, n int) {
		for range n {
			conn.admitReceivedBytes(msgOf())
		}
	}
	// disposeDelivered mirrors the consumer release path.
	disposeDelivered := func(conn *connection, n int) {
		for range n {
			conn.releaseReceivedBytes(msgOf())
		}
	}
	alive := func(conn *connection) bool {
		select {
		case <-conn.stopChan:
			return false
		default:
			return true
		}
	}

	// A retired connection holds 10 bytes of delivered debt.
	dead := newChargedConn("dead")
	chargeDelivered(dead, 10)
	cs.evictConn(dead)
	require.Len(t, cs.deadCharged, 1)
	require.Equal(t, 10*msgSize, shared.inFlight.Load())

	// Live debt 6: the shared counter (16) exceeds the budget, but the live
	// debt alone does not, so nothing may be evicted.
	liveA := newChargedConn("a")
	chargeDelivered(liveA, 6)
	require.True(t, alive(liveA), "overcommit caused by retired debt must not evict live connections")

	// Aggregate live debt 6+10=16 crosses the budget: the fattest live
	// connection must be evicted, the bystander must not.
	liveB := newChargedConn("b")
	chargeDelivered(liveB, 10)
	require.False(t, alive(liveB), "largest live connection past the aggregate cap must be evicted")
	require.True(t, alive(liveA), "bystander connection must survive")
	require.Equal(t, 1, cs.connNum())

	// Consuming the retired connections' last deliveries purges them without
	// any further budget overage.
	disposeDelivered(dead, 10)
	require.Len(t, cs.deadCharged, 1) // the evicted live connection remains
	disposeDelivered(liveB, 10)
	require.Empty(t, cs.deadCharged)
}

// TestBlockingSendTerminatesOnWriterFailure covers a full queue with a
// blocking sender, a writer Send error, and a receive side that stays open:
// the writer must publish termination, drain the queue, and unblock senders.
func TestBlockingSendTerminatesOnWriterFailure(t *testing.T) {
	sendGate := make(chan struct{})
	sendEntered := make(chan struct{})
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Send", mock.Anything).Return(func(*proto.Envelope) error {
		close(sendEntered) // the writer provably holds the first send now
		<-sendGate
		return errors.New("stream dead")
	})

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{RecvBuffSize: 1, SendBuffSize: 1})
	conn.logger = flogging.MustGetLogger("test")
	go conn.writeToStream()

	msg := createGossipMsg()
	conn.send(msg, func(error) {}, blockingSend) // taken by the writer, held in Send
	<-sendEntered
	conn.send(msg, func(error) {}, blockingSend) // queued in the full outBuff

	blockedErr := make(chan error, 1)
	go conn.send(msg, func(err error) { blockedErr <- err }, blockingSend) // waits for room

	close(sendGate) // the writer's Send fails; everything must terminate
	select {
	case err := <-blockedErr:
		require.ErrorIs(t, err, errConnClosed)
	case <-time.After(30 * time.Second):
		t.Fatal("blocking sender did not terminate on writer failure")
	}
	require.Eventually(t, func() bool {
		conn.sendMu.Lock()
		defer conn.sendMu.Unlock()
		return conn.sendClosed
	}, 30*time.Second, time.Millisecond, "writer did not publish termination")
	select {
	case <-conn.stopChan:
		t.Fatal("connection was closed although only the send side failed")
	default:
	}
}

type staticConnFactory struct{ conn *connection }

func (f *staticConnFactory) createConnection(string, common.PKIidType) (*connection, error) {
	return f.conn, nil
}

// TestIdentityChangeRetiresChargedConn covers the PKI-ID collision path in
// getConnection: the replaced connection must be retired with its charge.
func TestIdentityChangeRetiresChargedConn(t *testing.T) {
	shared := &sharedRecvBudget{max: 1 << 60}

	oldConn := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	oldConn.pkiID = []byte("b")
	oldConn.logger = flogging.MustGetLogger("test")

	newStream := &gmocks.MockStream{}
	newStream.On("CloseSend").Return(nil)
	newStream.On("Recv").Return(nil, errors.New("done"))
	newConn := newConnection(nil, nil, newStream, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	newConn.pkiID = []byte("b")
	newConn.logger = flogging.MustGetLogger("test")
	newConn.handler = func(*protoext.SignedGossipMessage) {}

	cs := newConnStore(&staticConnFactory{conn: newConn}, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, disabledMetrics)
	cs.pki2Conn["b"] = oldConn
	oldConn.store = cs

	// An outstanding delivered charge rides on the old connection.
	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	oldConn.admitReceivedBytes(msg)

	// Dialing expecting pkiID "a" authenticates "b" and collides with the old entry.
	conn, err := cs.getConnection(&RemotePeer{PKIID: []byte("a"), Endpoint: "localhost:1234"})
	require.NoError(t, err)
	require.Same(t, newConn, conn)

	select {
	case <-oldConn.stopChan:
	default:
		t.Fatal("replaced connection was not closed")
	}
	require.Len(t, cs.deadCharged, 1, "replaced connection was not retired with its charge")

	newConn.close()
}

// TestPostTeardownReceiveDoesNotCharge proves that a message returned by Recv
// after teardown is never charged, so it cannot trip an eviction elsewhere.
func TestPostTeardownReceiveDoesNotCharge(t *testing.T) {
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))
	shared := &sharedRecvBudget{max: msgSize + msgSize/2}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	healthy := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	healthy.pkiID = []byte("healthy")
	healthy.logger = flogging.MustGetLogger("test")
	cs.pki2Conn["healthy"] = healthy
	healthy.store = cs
	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	// Delivered charge on the healthy connection: fills the budget to exactly max.
	healthy.admitReceivedBytes(msg)

	// The victim's reader is blocked in Recv while the connection is torn down.
	recvGate := make(chan struct{})
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(func() *proto.Envelope { <-recvGate; return makeDataEnvelope(1024) }, func() error { return nil })
	victim := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{RecvBuffSize: 1, sharedRecvBudget: shared})
	victim.pkiID = []byte("victim")
	victim.logger = flogging.MustGetLogger("test")
	victim.handler = func(*protoext.SignedGossipMessage) {}
	cs.pki2Conn["victim"] = victim
	victim.store = cs

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- victim.serviceConnection()
	}()

	victim.close() // teardown while the reader blocks in Recv
	<-svcDone      // teardown drain has taken ownership

	close(recvGate) // Recv returns a message now; it must never be charged
	require.Eventually(t, func() bool {
		victim.recvMu.Lock()
		defer victim.recvMu.Unlock()
		return victim.recvClosed
	}, 30*time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		return shared.inFlight.Load() == msgSize
	}, 30*time.Second, time.Millisecond, "post-teardown receive changed the shared charge")

	select {
	case <-healthy.stopChan:
		t.Fatal("healthy connection was evicted by a post-teardown receive")
	default:
	}
	require.Zero(t, victim.recvBytesInFlight.Load())

	healthy.close()
}

// TestConnectionEvictedOnReceiveError covers the outbound zombie path: a
// connection whose receive stream dies must be closed, removed from the
// store, and retired with its outstanding charge.
func TestConnectionEvictedOnReceiveError(t *testing.T) {
	shared := &sharedRecvBudget{max: 1 << 60}
	var calls atomic.Int64
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope {
			if calls.Add(1) == 1 {
				return makeDataEnvelope(1024)
			}
			return nil
		},
		func() error {
			if calls.Load() > 1 {
				return errors.New("stream dead")
			}
			return nil
		},
	)

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:     DefRecvBuffSize,
		SendBuffSize:     1,
		sharedRecvBudget: shared,
	})
	conn.pkiID = []byte("pkiID")
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = func(*protoext.SignedGossipMessage) {} // never consumes

	cs := newConnStore(&staticConnFactory{conn: conn}, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	got, err := cs.getConnection(&RemotePeer{PKIID: conn.pkiID, Endpoint: "localhost:1234"})
	require.NoError(t, err)
	require.Same(t, conn, got)

	// The first message is charged and held; the receive stream then dies.
	require.Eventually(t, func() bool {
		return cs.connNum() == 0
	}, 30*time.Second, time.Millisecond, "connection with a dead receive stream was not evicted from the store")
	select {
	case <-conn.stopChan:
	default:
		t.Fatal("connection with a dead receive stream was not closed")
	}
	cs.RLock()
	retired := len(cs.deadCharged)
	cs.RUnlock()
	require.Equal(t, 1, retired, "evicted connection was not retired with its outstanding charge")
}

// TestDeMultiplexDoesNotAllocate pins the hot path: neither untracked nor
// tracked delivery may allocate per message with a typical subscriber count.
func TestDeMultiplexDoesNotAllocate(t *testing.T) {
	demux := NewChannelDemultiplexer()
	defer demux.Close()
	sub := demux.AddChannel(acceptAll)
	go func() {
		for range sub {
		}
	}()

	t.Run("untracked", func(t *testing.T) {
		require.Zero(t, testing.AllocsPerRun(100, func() {
			demux.DeMultiplex(struct{}{})
		}))
	})

	t.Run("tracked", func(t *testing.T) {
		msg := &trackableMessage{}
		require.Zero(t, testing.AllocsPerRun(100, func() {
			demux.DeMultiplexAbortable(msg, nil)
		}))
	})
}

// TestDeliveriesTrackedAcrossFanOut pins the multi-subscriber refcount: the
// charge is released only after every delivery is consumed.
func TestDeliveriesTrackedAcrossFanOut(t *testing.T) {
	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	sub1 := c.msgPublisher.AddChannel(acceptAll)
	sub2 := c.msgPublisher.AddChannel(acceptAll)
	defer c.msgPublisher.Close()

	conn := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{RecvBuffSize: 1, SendBuffSize: 1})
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: []byte("peer")})

	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	require.True(t, conn.admitReceivedBytes(msg)) // charge as readFromStream does
	conn.handler(msg)
	require.Positive(t, conn.recvBytesInFlight.Load())

	(<-sub1).(*ReceivedMessageImpl).Dispose()
	require.Positive(t, conn.recvBytesInFlight.Load(), "charge released before all deliveries were consumed")
	(<-sub2).(*ReceivedMessageImpl).Dispose()
	require.Zero(t, conn.recvBytesInFlight.Load(), "charge held after all deliveries were consumed")
}

// TestAckReleasedWithoutDelivery pins the ack fast path: acknowledgements
// never enter the delivery pipeline and must release their charge at once.
func TestAckReleasedWithoutDelivery(t *testing.T) {
	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	sub := c.msgPublisher.AddChannel(acceptAll)
	defer c.msgPublisher.Close()

	conn := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{RecvBuffSize: 1, SendBuffSize: 1})
	conn.logger = flogging.MustGetLogger("test")
	conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: []byte("peer")})

	ack := &proto.GossipMessage{
		Content: &proto.GossipMessage_Ack{Ack: &proto.Acknowledgement{}},
	}
	signedAck, err := protoext.NoopSign(ack)
	require.NoError(t, err)
	require.True(t, conn.admitReceivedBytes(signedAck)) // charge as readFromStream does
	conn.handler(signedAck)
	require.Zero(t, conn.recvBytesInFlight.Load(), "ack charge was not released")
	require.Empty(t, sub, "ack must not be delivered to subscribers")
}

// TestNewCommInstanceWiresReceiveBudgets covers config propagation through
// the exported constructor into the connection store.
func TestNewCommInstanceWiresReceiveBudgets(t *testing.T) {
	port, gRPCServer, certs, secureDialOpts, dialOpts := util.CreateGRPCLayer()
	defer gRPCServer.Stop()
	endpoint := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	id := []byte(endpoint)
	identityMapper := identity.NewIdentityMapper(naiveSec, id, noopPurgeIdentity, naiveSec)

	config := testCommConfig
	config.MaxRecvBytesInFlight = 1000
	config.MaxRecvBytesTotal = 4000
	inst, err := NewCommInstance(gRPCServer.Server(), certs, identityMapper, id, secureDialOpts,
		naiveSec, disabledMetrics, config, dialOpts...)
	require.NoError(t, err)
	defer inst.Stop()

	ci := inst.(*commImpl)
	require.Equal(t, 1000, ci.connStore.config.MaxRecvBytesInFlight)
	require.NotNil(t, ci.connStore.config.sharedRecvBudget)
	require.Equal(t, int64(4000), ci.connStore.config.sharedRecvBudget.max)
	require.NotNil(t, ci.connStore.config.sharedRecvBudget.evict)
}

type fakeCounter struct{ n atomic.Int64 }

func (c *fakeCounter) With(...string) libmetrics.Counter { return c }
func (c *fakeCounter) Add(delta float64)                 { c.n.Add(int64(delta)) }

type fakeGauge struct{}

func (fakeGauge) With(...string) libmetrics.Gauge { return fakeGauge{} }
func (fakeGauge) Add(float64)                     {}
func (fakeGauge) Set(float64)                     {}

type fakeHistogram struct{}

func (fakeHistogram) With(...string) libmetrics.Histogram { return fakeHistogram{} }
func (fakeHistogram) Observe(float64)                     {}

type fakeMetricsProvider struct {
	counters sync.Map
}

func (p *fakeMetricsProvider) NewCounter(o libmetrics.CounterOpts) libmetrics.Counter {
	c := &fakeCounter{}
	p.counters.Store(o.Name, c)
	return c
}
func (p *fakeMetricsProvider) NewGauge(libmetrics.GaugeOpts) libmetrics.Gauge { return fakeGauge{} }
func (p *fakeMetricsProvider) NewHistogram(libmetrics.HistogramOpts) libmetrics.Histogram {
	return fakeHistogram{}
}

func (p *fakeMetricsProvider) counter(name string) *fakeCounter {
	v, ok := p.counters.Load(name)
	if !ok {
		return &fakeCounter{}
	}
	return v.(*fakeCounter)
}

// TestReceiveBudgetMetrics pins the eviction and overcommit counters.
func TestReceiveBudgetMetrics(t *testing.T) {
	provider := &fakeMetricsProvider{}
	commMetrics := metrics.NewGossipMetrics(provider).CommMetrics

	// Per-connection drop on overflow.
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope { return makeDataEnvelope(1024) },
		func() error { return nil },
	)
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))
	conn := newConnection(nil, nil, stream, commMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: int(2 * msgSize),
	})
	conn.logger = flogging.MustGetLogger("test")

	errChan := make(chan error, 1)
	msgChan := make(chan *protoext.SignedGossipMessage, DefRecvBuffSize)
	go conn.readFromStream(errChan, msgChan)
	require.Error(t, <-errChan)
	require.EqualValues(t, 1, provider.counter("receive_budget_drops").n.Load())
	conn.close()

	// Node-wide eviction and overcommit.
	shared := &sharedRecvBudget{max: 15 * msgSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, commMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	chargeDelivered := func(conn *connection, n int) {
		for range n {
			conn.admitReceivedBytes(&protoext.SignedGossipMessage{Envelope: makeDataEnvelope(1024)})
		}
	}

	dead := newConnection(nil, nil, &gmocks.MockStream{}, commMetrics, ConnConfig{sharedRecvBudget: shared})
	dead.pkiID = []byte("dead")
	dead.logger = flogging.MustGetLogger("test")
	cs.pki2Conn["dead"] = dead
	dead.store = cs
	chargeDelivered(dead, 10)
	cs.evictConn(dead)

	// Overcommit: retired debt pushes the shared counter over the budget while
	// live debt stays under it.
	live := newConnection(nil, nil, &gmocks.MockStream{}, commMetrics, ConnConfig{sharedRecvBudget: shared})
	live.pkiID = []byte("live")
	live.logger = flogging.MustGetLogger("test")
	cs.pki2Conn["live"] = live
	live.store = cs
	chargeDelivered(live, 6)
	require.Positive(t, provider.counter("receive_shared_overcommit").n.Load())

	// Aggregate live debt past the budget: the fattest live connection evicted.
	chargeDelivered(live, 10)
	require.EqualValues(t, 1, provider.counter("receive_shared_evictions").n.Load())
	select {
	case <-live.stopChan:
	default:
		t.Fatal("largest live connection was not evicted")
	}
}

// TestUnmatchedMessageDoesNotEvictBystander proves that large decodable
// messages matching no subscriber release both charges at handoff: they never
// accumulate against the shared budget and never evict a bystander.
func TestUnmatchedMessageDoesNotEvictBystander(t *testing.T) {
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	c.msgPublisher.AddChannel(func(any) bool { return false }) // matches nothing
	defer c.msgPublisher.Close()

	// The budget stays far above the flooder's transient in-queue charges, so
	// nothing may be evicted.
	shared := &sharedRecvBudget{max: 100 * msgSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	bystander := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	bystander.pkiID = []byte("bystander")
	bystander.logger = flogging.MustGetLogger("test")
	cs.pki2Conn["bystander"] = bystander
	bystander.store = cs

	var recvCalls atomic.Int64
	flooderStream := &gmocks.MockStream{}
	flooderStream.On("CloseSend").Return(nil)
	flooderStream.On("Recv").Return(
		func() *proto.Envelope {
			recvCalls.Add(1)
			return makeDataEnvelope(1024)
		},
		func() error { return nil },
	)
	flooder := newConnection(nil, nil, flooderStream, disabledMetrics, ConnConfig{
		RecvBuffSize:     DefRecvBuffSize,
		SendBuffSize:     1,
		sharedRecvBudget: shared,
	})
	flooder.pkiID = []byte("flooder")
	flooder.logger = flogging.MustGetLogger("test")
	flooder.handler = c.publishingHandler(flooder, &protoext.ConnectionInfo{ID: flooder.pkiID})
	cs.pki2Conn["flooder"] = flooder
	flooder.evictFromStore = func() { cs.evictConn(flooder) }
	flooder.store = cs

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- flooder.serviceConnection()
	}()

	// The flooder streams many unmatched messages; nobody may be evicted.
	require.Eventually(t, func() bool {
		return recvCalls.Load() > 20
	}, 30*time.Second, time.Millisecond, "flooded messages never arrived")
	require.Equal(t, 2, cs.connNum())
	select {
	case <-bystander.stopChan:
		t.Fatal("bystander connection was dropped by unmatched-message floods")
	default:
	}

	// Once the flood stops, both counters drain completely: nothing was retained.
	flooder.close()
	<-svcDone
	require.Eventually(t, func() bool {
		return flooder.recvBytesInFlight.Load() == 0 && shared.inFlight.Load() == 0
	}, 30*time.Second, time.Millisecond, "unmatched messages were not released at handoff")
	bystander.close()
}

// TestPerConnTripDoesNotEvictOther proves that a message tripping only the
// per-connection budget drops that connection without touching the shared
// budget or any other connection.
func TestPerConnTripDoesNotEvictOther(t *testing.T) {
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))
	shared := &sharedRecvBudget{max: 100 * msgSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	healthy := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	healthy.pkiID = []byte("healthy")
	healthy.logger = flogging.MustGetLogger("test")
	healthy.handler = func(*protoext.SignedGossipMessage) {}
	cs.pki2Conn["healthy"] = healthy
	healthy.store = cs

	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(
		func() *proto.Envelope { return makeDataEnvelope(1024) },
		func() error { return nil },
	)
	victim := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{
		RecvBuffSize:         DefRecvBuffSize,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: int(2 * msgSize),
		sharedRecvBudget:     shared,
	})
	victim.pkiID = []byte("victim")
	victim.logger = flogging.MustGetLogger("test")
	victim.handler = func(*protoext.SignedGossipMessage) {} // never consumes
	cs.pki2Conn["victim"] = victim
	victim.evictFromStore = func() { cs.evictConn(victim) }
	victim.store = cs

	svcDone := make(chan error, 1)
	go func() {
		svcDone <- victim.serviceConnection()
	}()

	require.Eventually(t, func() bool {
		select {
		case <-victim.stopChan:
			return true
		default:
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "connection did not trip its per-connection budget")
	<-svcDone

	// Only the two messages still retained by the connection stay charged;
	// the tripping message never touched the shared budget.
	require.Equal(t, 2*msgSize, shared.inFlight.Load(), "per-connection trip charged the shared budget for a never-retained message")
	select {
	case <-healthy.stopChan:
		t.Fatal("healthy connection was evicted by another connection's per-connection trip")
	default:
	}
	healthy.close()
}

// TestSharedBudgetLogsOvercommitOncePerEpisode pins the throttle: while
// retired debt keeps the budget exceeded, the eviction scan runs once per
// episode, not per admission.
func TestSharedBudgetLogsOvercommitOncePerEpisode(t *testing.T) {
	msgSize := int64(goproto.Size(makeDataEnvelope(1024)))
	var evictCalls atomic.Int32
	shared := &sharedRecvBudget{max: 4 * msgSize}
	shared.evict = func() { evictCalls.Add(1) }

	// Retired debt at the budget: 4 messages retired with delivered charges.
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, disabledMetrics)
	dead := newConnection(nil, nil, &gmocks.MockStream{}, disabledMetrics, ConnConfig{sharedRecvBudget: shared})
	dead.pkiID = []byte("dead")
	dead.logger = flogging.MustGetLogger("test")
	cs.pki2Conn["dead"] = dead
	dead.store = cs
	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	for range 4 {
		dead.admitReceivedBytes(msg)
	}
	cs.evictConn(dead)
	require.EqualValues(t, 0, evictCalls.Load(), "no scan expected before the budget is exceeded")

	// Sustained live admissions while the budget is exceeded by retired debt:
	// one scan per episode, not per admission.
	for range 4 {
		shared.admit(msgSize)
	}
	require.EqualValues(t, 1, evictCalls.Load(), "overcommit scan must run once per episode, not per admission")

	// A new episode after draining below the budget scans again.
	for range 4 {
		shared.release(msgSize)
	}
	shared.admit(msgSize)
	require.EqualValues(t, 2, evictCalls.Load(), "a new overcommit episode must scan once more")
}

// TestRetiredDebtResidue pins the release ordering: the final dispose of a
// retired connection must subtract its shared retired debt before the
// zero-counter purge clears the retired flag.
func TestRetiredDebtResidue(t *testing.T) {
	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	size := int64(goproto.Size(msg.Envelope))

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	defer c.msgPublisher.Close()
	sub := c.msgPublisher.AddChannel(func(any) bool { return true })

	b := &sharedRecvBudget{max: 100 * size}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: b}, disabledMetrics)
	conn := newConnection(nil, nil, nil, disabledMetrics, ConnConfig{
		MaxRecvBytesInFlight: 1000 * int(size),
		sharedRecvBudget:     b,
	})
	conn.pkiID = []byte("peer")
	conn.logger = flogging.MustGetLogger("test")
	conn.store = cs
	cs.pki2Conn["peer"] = conn
	conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: conn.pkiID})

	require.True(t, conn.admitReceivedBytes(msg))
	conn.handler(msg)
	rmi := (<-sub).(*ReceivedMessageImpl)

	// The peer disconnects while the subscriber still holds the message.
	cs.Lock()
	cs.retireChargedConn(conn)
	cs.Unlock()
	require.Equal(t, size, b.deadTotal.Load())

	rmi.Dispose()

	require.Zero(t, conn.recvBytesInFlight.Load())
	require.Zero(t, b.inFlight.Load())
	require.Zero(t, b.deadTotal.Load(), "retired debt was not released on the final Dispose")
	require.Empty(t, cs.deadCharged)
}

// TestTotalBudgetCoversQueuedMessages proves the node-wide budget also bounds
// messages queued behind a blocked handler when the per-connection limit is
// disabled: the total is charged at admission, so the connection is dropped.
func TestTotalBudgetCoversQueuedMessages(t *testing.T) {
	size := int64(goproto.Size(makeDataEnvelope(1024)))
	shared := &sharedRecvBudget{max: 2 * size}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{sharedRecvBudget: shared}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	defer c.msgPublisher.Close()

	// Fill the subscriber so the next handoff blocks after charging.
	c.msgPublisher.AddChannel(func(any) bool { return true })
	for range 10 {
		c.msgPublisher.DeMultiplex(&protoext.SignedGossipMessage{Envelope: makeDataEnvelope(1024)})
	}

	conn := newConnection(nil, nil, nil, disabledMetrics, ConnConfig{
		RecvBuffSize:         4,
		SendBuffSize:         1,
		MaxRecvBytesInFlight: 0, // disable the per-connection limit
		sharedRecvBudget:     shared,
	})
	conn.pkiID = []byte("peer")
	conn.logger = flogging.MustGetLogger("test")
	conn.store = cs
	cs.pki2Conn["peer"] = conn
	conn.evictFromStore = func() { cs.evictConn(conn) }
	conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: conn.pkiID})

	msg, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
	require.NoError(t, err)
	require.True(t, conn.admitReceivedBytes(msg))
	handlerDone := make(chan struct{})
	go func() {
		conn.handler(msg) // blocks in the demux with the shared budget charged
		close(handlerDone)
	}()
	require.Eventually(t, func() bool { return shared.inFlight.Load() == size }, 30*time.Second, time.Millisecond)

	// The reader keeps admitting into msgChan behind the blocked handler.
	for range 4 {
		queued, err := protoext.EnvelopeToGossipMessage(makeDataEnvelope(1024))
		require.NoError(t, err)
		require.True(t, conn.admitReceivedBytes(queued))
	}

	// The node-wide budget must stop the connection once the queued and
	// handler-held bytes cross it.
	select {
	case <-conn.stopChan:
	default:
		t.Fatal("node-wide budget did not cover messages queued behind a blocked handler")
	}

	conn.close()
	<-handlerDone
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
// drops the live connection holding the most in-flight bytes, not bystanders.
func TestSharedBudgetEvictsLargestConnection(t *testing.T) {
	bigPayload := 1 << 20
	bigSize := int64(goproto.Size(makeDataEnvelope(bigPayload)))

	c := &commImpl{
		msgPublisher: NewChannelDemultiplexer(),
		pubSub:       util.NewPubSub(),
		logger:       flogging.MustGetLogger("test"),
	}
	c.msgPublisher.AddChannel(func(any) bool { return true }) // stalled subscriber

	shared := &sharedRecvBudget{max: 8 * bigSize}
	cs := newConnStore(nil, flogging.MustGetLogger("test"), ConnConfig{}, disabledMetrics)
	shared.evict = func() { cs.evictLargestConnection(shared.max) }

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
		conn.handler = c.publishingHandler(conn, &protoext.ConnectionInfo{ID: conn.pkiID})
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
