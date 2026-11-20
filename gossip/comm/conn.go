/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"sync/atomic"

	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	goproto "google.golang.org/protobuf/proto"
)

type handler func(message *protoext.SignedGossipMessage)

type blockingBehavior bool

const (
	blockingSend    = blockingBehavior(true)
	nonBlockingSend = blockingBehavior(false)
)

var errConnClosed = errors.New("connection closed")

type connFactory interface {
	createConnection(endpoint string, pkiID common.PKIidType) (*connection, error)
}

type connectionStore struct {
	config       ConnConfig
	logger       util.Logger            // logger
	isClosing    bool                   // whether this connection store is shutting down
	shutdownOnce sync.Once              // ensure shutdown is only called once per connectionStore
	connFactory  connFactory            // creates a connection to remote peer
	sync.RWMutex                        // synchronize access to shared variables
	pki2Conn     map[string]*connection // mapping between pkiID to connections
	// deadCharged tracks removed connections whose delivered charges are still
	// counted against the shared budget until consumed downstream.
	deadCharged      map[*connection]struct{}
	destinationLocks map[string]*sync.Mutex // mapping between pkiIDs and locks,
	// used to prevent concurrent connection establishment to the same remote endpoint
}

func newConnStore(connFactory connFactory, logger util.Logger, config ConnConfig) *connectionStore {
	return &connectionStore{
		connFactory:      connFactory,
		isClosing:        false,
		pki2Conn:         make(map[string]*connection),
		deadCharged:      make(map[*connection]struct{}),
		destinationLocks: make(map[string]*sync.Mutex),
		logger:           logger,
		config:           config,
	}
}

// retireChargedConn tracks a removed connection while its delivered charges
// drain; callers must hold cs's write lock.
func (cs *connectionStore) retireChargedConn(conn *connection) {
	if conn.recvBytesInFlight.Load() > 0 {
		cs.deadCharged[conn] = struct{}{}
	}
}

func (cs *connectionStore) getConnection(peer *RemotePeer) (*connection, error) {
	cs.RLock()
	isClosing := cs.isClosing
	cs.RUnlock()

	if isClosing {
		return nil, errors.Errorf("conn store is closing")
	}

	pkiID := peer.PKIID
	endpoint := peer.Endpoint

	cs.Lock()
	destinationLock, hasConnected := cs.destinationLocks[string(pkiID)]
	if !hasConnected {
		destinationLock = &sync.Mutex{}
		cs.destinationLocks[string(pkiID)] = destinationLock
	}
	cs.Unlock()

	destinationLock.Lock()

	cs.RLock()
	conn, exists := cs.pki2Conn[string(pkiID)]
	if exists {
		cs.RUnlock()
		destinationLock.Unlock()
		return conn, nil
	}
	cs.RUnlock()

	createdConnection, err := cs.connFactory.createConnection(endpoint, pkiID)
	if err == nil {
		cs.logger.Debugf("Created new connection to %s, %s", endpoint, pkiID)
	}

	destinationLock.Unlock()

	cs.RLock()
	isClosing = cs.isClosing
	cs.RUnlock()
	if isClosing {
		return nil, errors.Errorf("conn store is closing")
	}

	cs.Lock()
	delete(cs.destinationLocks, string(pkiID))
	defer cs.Unlock()

	// check again, maybe someone connected to us during the connection creation?
	conn, exists = cs.pki2Conn[string(pkiID)]

	if exists {
		if createdConnection != nil {
			createdConnection.close()
		}
		return conn, nil
	}

	// no one connected to us AND we failed connecting!
	if err != nil {
		return nil, err
	}

	// At this point we know the latest PKI-ID of the remote peer.
	// Perhaps we are trying to connect to a peer that changed its PKI-ID.
	// Ensure that we are not already connected to that PKI-ID,
	// and if so - close the old connection in order to keep the latest one.
	// We keep the latest one because the remote peer is going to close
	// our old connection anyway once it sensed this connection handshake.
	if conn, exists := cs.pki2Conn[string(createdConnection.pkiID)]; exists {
		conn.close()
	}

	// at this point in the code, we created a connection to a remote peer
	conn = createdConnection
	cs.pki2Conn[string(createdConnection.pkiID)] = conn
	conn.evictFromStore = func() { cs.evictConn(conn) }

	go conn.serviceConnection()

	return conn, nil
}

func (cs *connectionStore) connNum() int {
	cs.RLock()
	defer cs.RUnlock()
	return len(cs.pki2Conn)
}

func (cs *connectionStore) shutdown() {
	cs.shutdownOnce.Do(func() {
		cs.Lock()
		cs.isClosing = true

		for _, conn := range cs.pki2Conn {
			conn.close()
		}
		cs.pki2Conn = make(map[string]*connection)

		cs.Unlock()
	})
}

// onConnected closes any connection to the remote peer and creates a new connection object to it in order to have only
// one single bi-directional connection between a pair of peers
func (cs *connectionStore) onConnected(serverStream proto.Gossip_GossipStreamServer,
	connInfo *protoext.ConnectionInfo, metrics *metrics.CommMetrics,
) *connection {
	cs.Lock()
	defer cs.Unlock()

	if c, exists := cs.pki2Conn[string(connInfo.ID)]; exists {
		c.close()
		cs.retireChargedConn(c)
	}

	conn := newConnection(nil, nil, serverStream, metrics, cs.config)
	conn.pkiID = connInfo.ID
	conn.info = connInfo
	conn.logger = cs.logger
	cs.pki2Conn[string(connInfo.ID)] = conn
	conn.evictFromStore = func() { cs.evictConn(conn) }
	return conn
}

func (cs *connectionStore) closeConnByPKIid(pkiID common.PKIidType) {
	cs.Lock()
	defer cs.Unlock()
	if conn, exists := cs.pki2Conn[string(pkiID)]; exists {
		conn.close()
		delete(cs.pki2Conn, string(pkiID))
		cs.retireChargedConn(conn)
	}
}

// evictConn removes conn from the store only if it is still the current entry
// for its PKI-ID, so a replaced connection never evicts its successor.
func (cs *connectionStore) evictConn(conn *connection) {
	cs.Lock()
	defer cs.Unlock()
	cs.evictConnLocked(conn)
}

// evictConnLocked is evictConn for callers already holding cs's write lock.
func (cs *connectionStore) evictConnLocked(conn *connection) {
	if current, exists := cs.pki2Conn[string(conn.pkiID)]; exists && current == conn {
		conn.close()
		delete(cs.pki2Conn, string(conn.pkiID))
		cs.retireChargedConn(conn)
	}
}

// evictLargestConnection drops the connection retaining the most unconsumed
// received bytes; targeting the largest holder keeps a flooding peer from
// getting bystander connections dropped when the shared budget is exceeded.
// Charges already delivered by a removed connection outlive it, so when the
// largest charge is dead the store overcommits until subscribers drain instead
// of cascading evictions to bystanders.
func (cs *connectionStore) evictLargestConnection() {
	cs.Lock()
	defer cs.Unlock()

	var victim *connection
	var mostLive, mostDead int64
	for _, conn := range cs.pki2Conn {
		if n := conn.recvBytesInFlight.Load(); n > mostLive {
			mostLive, victim = n, conn
		}
	}
	for conn := range cs.deadCharged {
		n := conn.recvBytesInFlight.Load()
		if n == 0 {
			delete(cs.deadCharged, conn)
			continue
		}
		if n > mostDead {
			mostDead = n
		}
	}

	if mostDead >= mostLive || victim == nil {
		if mostDead > 0 {
			cs.logger.Debugf("Shared receive byte budget is exceeded by %d bytes delivered by removed connections; overcommitting until subscribers drain", mostDead)
		}
		return
	}
	cs.logger.Warningf("Shared receive byte budget exceeded, dropping connection to %s holding %d unconsumed bytes", victim.pkiID, mostLive)
	victim.close()
	cs.evictConnLocked(victim)
}

func newConnection(cl proto.GossipClient, c *grpc.ClientConn, s stream, metrics *metrics.CommMetrics, config ConnConfig) *connection {
	connection := &connection{
		metrics:              metrics,
		outBuff:              make(chan *msgSending, config.SendBuffSize),
		cl:                   cl,
		conn:                 c,
		gossipStream:         s,
		stopChan:             make(chan struct{}, 1),
		recvBuffSize:         config.RecvBuffSize,
		maxRecvBytesInFlight: int64(config.MaxRecvBytesInFlight),
		sharedRecvBudget:     config.sharedRecvBudget,
	}
	return connection
}

// ConnConfig is the configuration required to initialize a new conn
type ConnConfig struct {
	RecvBuffSize int
	SendBuffSize int
	// MaxRecvBytesInFlight bounds the wire bytes of received messages a
	// connection may hold unconsumed before the connection is closed.
	// Non-positive disables the limit.
	MaxRecvBytesInFlight int
	// sharedRecvBudget bounds unconsumed received bytes across all
	// connections of the comm instance; nil disables it.
	sharedRecvBudget *sharedRecvBudget
}

// sharedRecvBudget bounds received-but-unconsumed bytes across connections.
// On excess, the connection holding the most in-flight bytes is evicted, so
// bystander connections are never dropped because of another peer's flood.
type sharedRecvBudget struct {
	inFlight atomic.Int64
	max      int64
	// evict drops the connection with the most in-flight bytes.
	evict func()
}

func (b *sharedRecvBudget) admit(size int64) {
	if b == nil || b.max <= 0 {
		return
	}
	if b.inFlight.Add(size) > b.max && b.evict != nil {
		b.evict()
	}
}

func (b *sharedRecvBudget) release(size int64) {
	if b == nil || b.max <= 0 {
		return
	}
	b.inFlight.Add(-size)
}

type connection struct {
	recvBuffSize int
	// recvBytesInFlight tracks received bytes not yet consumed downstream, so
	// a stalled pipeline cannot pin unbounded memory.
	recvBytesInFlight    atomic.Int64
	maxRecvBytesInFlight int64
	sharedRecvBudget     *sharedRecvBudget
	// evictFromStore removes this connection from the connection store if it
	// is still the current entry; set once the connection is registered.
	evictFromStore func()
	// recvMu serializes the readFromStream handoff against the teardown drain;
	// recvClosed makes the reader release new messages itself after teardown.
	recvMu     sync.Mutex
	recvClosed bool
	// sendMu serializes outBuff handoff against the writeToStream exit drain;
	// sendClosed makes senders fail new messages instead of enqueueing them.
	sendMu       sync.Mutex
	sendClosed   bool
	metrics      *metrics.CommMetrics
	cancel       context.CancelFunc
	info         *protoext.ConnectionInfo
	outBuff      chan *msgSending
	logger       util.Logger        // logger
	pkiID        common.PKIidType   // pkiID of the remote endpoint
	handler      handler            // function to invoke upon a message reception
	conn         *grpc.ClientConn   // gRPC connection to remote endpoint
	cl           proto.GossipClient // gRPC stub of remote endpoint
	gossipStream stream             // there can only be one
	stopChan     chan struct{}      // a method to stop the server-side gRPC call from a different go-routine
	stopOnce     sync.Once          // once to ensure close is called only once
}

func (conn *connection) close() {
	conn.stopOnce.Do(func() {
		close(conn.stopChan)

		if conn.conn != nil {
			conn.conn.Close()
		}

		if conn.cancel != nil {
			conn.cancel()
		}
	})
}

func (conn *connection) send(msg *protoext.SignedGossipMessage, onErr func(error), shouldBlock blockingBehavior) {
	m := &msgSending{
		envelope: msg.Envelope,
		onErr:    onErr,
	}

	conn.sendMu.Lock()
	defer conn.sendMu.Unlock()
	if conn.sendClosed {
		conn.logger.Debugf("Aborting send() to %s because connection is closed", conn.info.Endpoint)
		go m.onErr(errConnClosed)
		return
	}
	select {
	case conn.outBuff <- m:
		// room in channel, successfully sent message, nothing to do
	case <-conn.stopChan:
		conn.logger.Debugf("Aborting send() to %s because connection is closing", conn.info.Endpoint)
		go m.onErr(errConnClosed)
	default: // did not send
		if shouldBlock {
			select {
			case conn.outBuff <- m: // try again, and wait to send
			case <-conn.stopChan: // stop blocking if the connection is closing
				go m.onErr(errConnClosed)
			}
		} else {
			conn.metrics.BufferOverflow.Add(1)
			conn.logger.Debugf("Buffer to %s overflowed, dropping message %s", conn.info.Endpoint, msg)
		}
	}
}

func (conn *connection) serviceConnection() error {
	errChan := make(chan error, 1)
	msgChan := make(chan *protoext.SignedGossipMessage, conn.recvBuffSize)
	defer func() {
		// Take ownership of msgChan: once recvClosed is set, the reader
		// releases newly admitted messages itself instead of enqueueing them,
		// so nothing charged is left behind.
		conn.recvMu.Lock()
		conn.recvClosed = true
		for {
			select {
			case msg := <-msgChan:
				conn.releaseReceivedBytes(msg)
			default:
				conn.recvMu.Unlock()
				return
			}
		}
	}()
	// Call stream.Recv() asynchronously in readFromStream(),
	// and wait for either the Recv() call to end,
	// or a signal to close the connection, which exits
	// the method and makes the Recv() call to fail in the
	// readFromStream() method
	go conn.readFromStream(errChan, msgChan)

	go conn.writeToStream()

	for {
		select {
		case <-conn.stopChan:
			conn.logger.Debug("Closing reading from stream")
			return nil
		case err := <-errChan:
			return err
		case msg := <-msgChan:
			conn.handler(msg)
		}
	}
}

func (conn *connection) writeToStream() {
	defer func() {
		// Become the only owner of outBuff: fail everything still queued so
		// onErr callbacks (e.g. SendWithAck waiters) are not stuck.
		conn.sendMu.Lock()
		conn.sendClosed = true
		for {
			select {
			case m := <-conn.outBuff:
				go m.onErr(errConnClosed)
			default:
				conn.sendMu.Unlock()
				return
			}
		}
	}()
	stream := conn.gossipStream
	for {
		select {
		case m := <-conn.outBuff:
			err := stream.Send(m.envelope)
			if err != nil {
				go m.onErr(err)
				return
			}
			conn.metrics.SentMessages.Add(1)
		case <-conn.stopChan:
			conn.logger.Debug("Closing writing to stream")
			return
		}
	}
}

func (conn *connection) readFromStream(errChan chan error, msgChan chan *protoext.SignedGossipMessage) {
	stream := conn.gossipStream
	for {
		select {
		case <-conn.stopChan:
			return
		default:
			envelope, err := stream.Recv()
			if err != nil {
				errChan <- err
				conn.logger.Debugf("Got error, aborting: %v", err)
				return
			}
			conn.metrics.ReceivedMessages.Add(1)
			msg, err := protoext.EnvelopeToGossipMessage(envelope)
			if err != nil {
				errChan <- err
				conn.logger.Warningf("Got error, aborting: %v", err)
				return
			}
			if !conn.admitReceivedBytes(msg) {
				conn.releaseReceivedBytes(msg) // never delivered
				err = errors.Errorf("received bytes in flight exceed the per-connection limit of %d bytes", conn.maxRecvBytesInFlight)
				conn.logger.Warningf("Dropping connection: %v", err)
				errChan <- err
				// The handler may be wedged on a stalled subscriber, so tear
				// the connection down and evict it instead of waiting.
				conn.close()
				if conn.evictFromStore != nil {
					conn.evictFromStore()
				}
				return
			}
			conn.recvMu.Lock()
			if conn.recvClosed {
				// Teardown owns msgChan now; release instead of enqueueing.
				conn.recvMu.Unlock()
				conn.releaseReceivedBytes(msg)
				return
			}
			select {
			case <-conn.stopChan:
				conn.recvMu.Unlock()
				conn.releaseReceivedBytes(msg)
				return
			case msgChan <- msg:
				conn.recvMu.Unlock()
			}
		}
	}
}

// admitReceivedBytes charges the message's wire size to the shared budget and
// the connection's receive budget; false means the connection's own budget is
// exceeded and the connection must be dropped to bound retained memory.
func (conn *connection) admitReceivedBytes(msg *protoext.SignedGossipMessage) bool {
	size := int64(goproto.Size(msg.Envelope))
	inFlight := conn.recvBytesInFlight.Add(size)
	conn.sharedRecvBudget.admit(size)
	if conn.maxRecvBytesInFlight <= 0 {
		return true
	}
	return inFlight <= conn.maxRecvBytesInFlight
}

// releaseReceivedBytes returns the message's wire size to both budgets once
// the message will not be (or no longer is) retained by the receive path.
func (conn *connection) releaseReceivedBytes(msg *protoext.SignedGossipMessage) {
	size := int64(goproto.Size(msg.Envelope))
	conn.sharedRecvBudget.release(size)
	conn.recvBytesInFlight.Add(-size)
}

type msgSending struct {
	envelope *proto.Envelope
	onErr    func(error)
}

//go:generate mockery -dir . -name MockStream -case underscore -output mocks/

type MockStream interface {
	proto.Gossip_GossipStreamClient
}
