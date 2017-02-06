/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package comm

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

type handler func(*proto.GossipMessage)

type connFactory interface {
	createConnection(endpoint string, pkiID common.PKIidType) (*connection, error)
}

type connectionStore struct {
	logger           *logging.Logger          // logger
	isClosing        bool                     // whether this connection store is shutting down
	connFactory      connFactory              // creates a connection to remote peer
	sync.RWMutex                              // synchronize access to shared variables
	pki2Conn         map[string]*connection   // mapping between pkiID to connections
	destinationLocks map[string]*sync.RWMutex //mapping between pkiIDs and locks,
	// used to prevent concurrent connection establishment to the same remote endpoint
}

func newConnStore(connFactory connFactory, logger *logging.Logger) *connectionStore {
	return &connectionStore{
		connFactory:      connFactory,
		isClosing:        false,
		pki2Conn:         make(map[string]*connection),
		destinationLocks: make(map[string]*sync.RWMutex),
		logger:           logger,
	}
}

func (cs *connectionStore) getConnection(peer *RemotePeer) (*connection, error) {
	cs.RLock()
	isClosing := cs.isClosing
	cs.RUnlock()

	if isClosing {
		return nil, fmt.Errorf("Shutting down")
	}

	pkiID := peer.PKIID
	endpoint := peer.Endpoint

	cs.Lock()
	destinationLock, hasConnected := cs.destinationLocks[string(pkiID)]
	if !hasConnected {
		destinationLock = &sync.RWMutex{}
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

	destinationLock.Unlock()

	if cs.isClosing {
		return nil, fmt.Errorf("ConnStore is closing")
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

	// at this point in the code, we created a connection to a remote peer
	conn = createdConnection
	cs.pki2Conn[string(createdConnection.pkiID)] = conn

	go conn.serviceConnection()

	return conn, nil
}

func (cs *connectionStore) connNum() int {
	cs.RLock()
	defer cs.RUnlock()
	return len(cs.pki2Conn)
}

func (cs *connectionStore) closeConn(peer *RemotePeer) {
	cs.Lock()
	defer cs.Unlock()

	if conn, exists := cs.pki2Conn[string(peer.PKIID)]; exists {
		conn.close()
		delete(cs.pki2Conn, string(conn.pkiID))
	}
}

func (cs *connectionStore) shutdown() {
	cs.Lock()
	cs.isClosing = true
	pkiIds2conn := cs.pki2Conn
	cs.Unlock()

	wg := sync.WaitGroup{}
	for _, conn := range pkiIds2conn {
		wg.Add(1)
		go func(conn *connection) {
			cs.closeByPKIid(conn.pkiID)
			wg.Done()
		}(conn)
	}
	wg.Wait()
}

func (cs *connectionStore) onConnected(serverStream proto.Gossip_GossipStreamServer, pkiID common.PKIidType) *connection {
	cs.Lock()
	defer cs.Unlock()

	if c, exists := cs.pki2Conn[string(pkiID)]; exists {
		c.close()
	}

	return cs.registerConn(pkiID, serverStream)
}

func (cs *connectionStore) registerConn(pkiID common.PKIidType, serverStream proto.Gossip_GossipStreamServer) *connection {
	conn := newConnection(nil, nil, nil, serverStream)
	conn.pkiID = pkiID
	conn.logger = cs.logger
	cs.pki2Conn[string(pkiID)] = conn
	return conn
}

func (cs *connectionStore) closeByPKIid(pkiID common.PKIidType) {
	cs.Lock()
	defer cs.Unlock()
	if conn, exists := cs.pki2Conn[string(pkiID)]; exists {
		conn.close()
		delete(cs.pki2Conn, string(pkiID))
	}
}

func newConnection(cl proto.GossipClient, c *grpc.ClientConn, cs proto.Gossip_GossipStreamClient, ss proto.Gossip_GossipStreamServer) *connection {
	connection := &connection{
		outBuff:      make(chan *msgSending, defSendBuffSize),
		cl:           cl,
		conn:         c,
		clientStream: cs,
		serverStream: ss,
		stopFlag:     int32(0),
		stopChan:     make(chan struct{}, 1),
	}

	return connection
}

type connection struct {
	outBuff      chan *msgSending
	logger       *logging.Logger                 // logger
	pkiID        common.PKIidType                // pkiID of the remote endpoint
	handler      handler                         // function to invoke upon a message reception
	conn         *grpc.ClientConn                // gRPC connection to remote endpoint
	cl           proto.GossipClient              // gRPC stub of remote endpoint
	clientStream proto.Gossip_GossipStreamClient // client-side stream to remote endpoint
	serverStream proto.Gossip_GossipStreamServer // server-side stream to remote endpoint
	stopFlag     int32                           // indicates whether this connection is in process of stopping
	stopChan     chan struct{}                   // a method to stop the server-side gRPC call from a different go-routine
	sync.RWMutex                                 // synchronizes access to shared variables
}

func (conn *connection) close() {
	if conn.toDie() {
		return
	}

	amIFirst := atomic.CompareAndSwapInt32(&conn.stopFlag, int32(0), int32(1))
	if !amIFirst {
		return
	}

	conn.stopChan <- struct{}{}

	conn.Lock()

	if conn.clientStream != nil {
		conn.clientStream.CloseSend()
	}
	if conn.conn != nil {
		conn.conn.Close()
	}

	conn.Unlock()

}

func (conn *connection) toDie() bool {
	return atomic.LoadInt32(&(conn.stopFlag)) == int32(1)
}

func (conn *connection) send(msg *proto.GossipMessage, onErr func(error)) {
	conn.Lock()
	defer conn.Unlock()

	if len(conn.outBuff) == defSendBuffSize {
		go onErr(errSendOverflow)
		return
	}

	m := &msgSending{
		msg:   msg,
		onErr: onErr,
	}

	conn.outBuff <- m
}

func (conn *connection) serviceConnection() error {
	errChan := make(chan error, 1)
	msgChan := make(chan *proto.GossipMessage, defRecvBuffSize)
	defer close(msgChan)

	// Call stream.Recv() asynchronously in readFromStream(),
	// and wait for either the Recv() call to end,
	// or a signal to close the connection, which exits
	// the method and makes the Recv() call to fail in the
	// readFromStream() method
	go conn.readFromStream(errChan, msgChan)

	go conn.writeToStream()

	for !conn.toDie() {
		select {
		case stop := <-conn.stopChan:
			conn.logger.Warning("Closing reading from stream")
			conn.stopChan <- stop
			return nil
		case err := <-errChan:
			return err
		case msg := <-msgChan:
			conn.handler(msg)
		}
	}
	return nil
}

func (conn *connection) writeToStream() {
	for !conn.toDie() {
		stream := conn.getStream()
		if stream == nil {
			conn.logger.Error(conn.pkiID, "Stream is nil, aborting!")
			return
		}
		select {
		case m := <-conn.outBuff:
			err := stream.Send(m.msg)
			if err != nil {
				go m.onErr(err)
				return
			}
			break
		case stop := <-conn.stopChan:
			conn.logger.Warning("Closing writing to stream")
			conn.stopChan <- stop
			return
		}
	}
}

func (conn *connection) readFromStream(errChan chan error, msgChan chan *proto.GossipMessage) {
	defer func() {
		recover()
	}() // msgChan might be closed
	for !conn.toDie() {
		stream := conn.getStream()
		if stream == nil {
			conn.logger.Error(conn.pkiID, "Stream is nil, aborting!")
			errChan <- fmt.Errorf("Stream is nil")
			return
		}
		msg, err := stream.Recv()
		if conn.toDie() {
			conn.logger.Warning(conn.pkiID, "canceling read because closing")
			return
		}
		if err != nil {
			errChan <- err
			conn.logger.Warning(conn.pkiID, "Got error, aborting:", err)
			return
		}
		msgChan <- msg
	}
}

func (conn *connection) getStream() stream {
	conn.Lock()
	defer conn.Unlock()

	if conn.clientStream != nil && conn.serverStream != nil {
		e := "Both client and server stream are not nil, something went wrong"
		conn.logger.Error(e)
	}

	if conn.clientStream != nil {
		return conn.clientStream
	}

	if conn.serverStream != nil {
		return conn.serverStream
	}

	return nil
}

type msgSending struct {
	msg   *proto.GossipMessage
	onErr func(error)
}
