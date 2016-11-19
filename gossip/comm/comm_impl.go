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
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"crypto/tls"
	"os"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

const (
	defDialTimeout  = time.Second * time.Duration(3)
	defConnTimeout  = time.Second * time.Duration(2)
	defRecvBuffSize = 20
	defSendBuffSize = 20
	sendOverflowErr = "Send buffer overflow"
)

var errSendOverflow = fmt.Errorf(sendOverflowErr)
var dialTimeout = defDialTimeout

func init() {
	rand.Seed(42)
}

// SetDialTimeout sets the dial timeout
func SetDialTimeout(timeout time.Duration) {
	dialTimeout = timeout
}

func (c *commImpl) SetDialOpts(opts ...grpc.DialOption) {
	c.opts = opts
}

// NewCommInstanceWithServer creates a comm instance that creates an underlying gRPC server
func NewCommInstanceWithServer(port int, sec SecurityProvider, pkID common.PKIidType, dialOpts ...grpc.DialOption) (Comm, error) {
	var ll net.Listener
	var s *grpc.Server
	var secOpt grpc.DialOption

	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{grpc.WithTimeout(dialTimeout)}
	}

	if port > 0 {
		s, ll, secOpt = createGRPCLayer(port)
		dialOpts = append(dialOpts, secOpt)
	}

	commInst := &commImpl{
		logger:            util.GetLogger(util.LOGGING_COMM_MODULE, fmt.Sprintf("%d", port)),
		PKIID:             pkID,
		opts:              dialOpts,
		sec:               sec,
		port:              port,
		lsnr:              ll,
		gSrv:              s,
		msgPublisher:      NewChannelDemultiplexer(),
		lock:              &sync.RWMutex{},
		deadEndpoints:     make(chan common.PKIidType, 100),
		stopping:          int32(0),
		exitChan:          make(chan struct{}, 1),
		subscriptions:     make([]chan ReceivedMessage, 0),
		blackListedPKIIDs: make([]common.PKIidType, 0),
	}
	commInst.connStore = newConnStore(commInst, pkID, commInst.logger)

	if port > 0 {
		go func() {
			commInst.stopWG.Add(1)
			defer commInst.stopWG.Done()
			s.Serve(ll)
		}()

		proto.RegisterGossipServer(s, commInst)
	}

	commInst.logger.SetLevel(logging.WARNING)

	return commInst, nil
}

// NewCommInstance creates a new comm instance that binds itself to the given gRPC server
func NewCommInstance(s *grpc.Server, sec SecurityProvider, PKIID common.PKIidType, dialOpts ...grpc.DialOption) (Comm, error) {
	commInst, err := NewCommInstanceWithServer(-1, sec, PKIID, dialOpts...)
	if err != nil {
		return nil, err
	}
	proto.RegisterGossipServer(s, commInst.(*commImpl))
	return commInst, nil
}

type commImpl struct {
	logger            *util.Logger
	sec               SecurityProvider
	opts              []grpc.DialOption
	connStore         *connectionStore
	PKIID             []byte
	port              int
	deadEndpoints     chan common.PKIidType
	msgPublisher      *ChannelDeMultiplexer
	lock              *sync.RWMutex
	lsnr              net.Listener
	gSrv              *grpc.Server
	exitChan          chan struct{}
	stopping          int32
	stopWG            sync.WaitGroup
	subscriptions     []chan ReceivedMessage
	blackListedPKIIDs []common.PKIidType
}

func (c *commImpl) createConnection(endpoint string, expectedPKIID common.PKIidType) (*connection, error) {
	c.logger.Debug("Entering", endpoint, expectedPKIID)
	defer c.logger.Debug("Exiting")
	if c.isStopping() {
		return nil, fmt.Errorf("Stopping")
	}
	cc, err := grpc.Dial(endpoint, append(c.opts, grpc.WithBlock())...)
	if err != nil {
		if cc != nil {
			cc.Close()
		}
		return nil, err
	}

	cl := proto.NewGossipClient(cc)

	if _, err := cl.Ping(context.Background(), &proto.Empty{}); err != nil {
		cc.Close()
		return nil, err
	}

	if stream, err := cl.GossipStream(context.Background()); err == nil {
		pkiID, err := c.authenticateRemotePeer(stream)
		if err == nil {
			if expectedPKIID != nil && !bytes.Equal(pkiID, expectedPKIID) {
				// PKIID is nil when we don't know the remote PKI id's
				c.logger.Warning("Remote endpoint claims to be a different peer, expected", expectedPKIID, "but got", pkiID)
				return nil, fmt.Errorf("Authentication failure")
			}
			conn := newConnection(cl, cc, stream, nil)
			conn.pkiID = pkiID
			conn.logger = c.logger

			h := func(m *proto.GossipMessage) {
				c.logger.Debug("Got message:", m)
				c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
					conn:          conn,
					lock:          conn,
					GossipMessage: m,
				})
			}
			conn.handler = h
			return conn, nil
		}
		return nil, fmt.Errorf("Authentication failure")
	}
	cc.Close()
	return nil, err
}

func (c *commImpl) Send(msg *proto.GossipMessage, peers ...*RemotePeer) {
	if c.isStopping() {
		return
	}

	c.logger.Info("Entering, sending", msg, "to ", len(peers), "peers")

	for _, peer := range peers {
		go func(peer *RemotePeer, msg *proto.GossipMessage) {
			c.sendToEndpoint(peer, msg)
		}(peer, msg)
	}
}

func (c *commImpl) BlackListPKIid(PKIID common.PKIidType) {
	c.logger.Info("Entering", PKIID)
	defer c.logger.Info("Exiting")
	c.lock.Lock()
	defer c.lock.Unlock()
	c.connStore.closeByPKIid(PKIID)
	c.blackListedPKIIDs = append(c.blackListedPKIIDs, PKIID)
}

func (c *commImpl) isPKIblackListed(p common.PKIidType) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	for _, pki := range c.blackListedPKIIDs {
		if bytes.Equal(pki, p) {
			c.logger.Debug(p, ":", true)
			return true
		}
	}
	c.logger.Debug(p, ":", false)
	return false
}

func (c *commImpl) sendToEndpoint(peer *RemotePeer, msg *proto.GossipMessage) {
	if c.isStopping() {
		return
	}
	c.logger.Debug("Entering, Sending to", peer.Endpoint, ", msg:", msg)
	defer c.logger.Debug("Exiting")
	var err error

	conn, err := c.connStore.getConnection(peer)
	if err == nil {
		disConnectOnErr := func(err error) {
			c.logger.Warning(peer, "isn't responsive:", err)
			c.disconnect(peer.PKIID)
		}
		conn.send(msg, disConnectOnErr)
		return
	}
	c.logger.Warning("Failed obtaining connection for", peer, "reason:", err)
	c.disconnect(peer.PKIID)
}

func (c *commImpl) isStopping() bool {
	return atomic.LoadInt32(&c.stopping) == int32(1)
}

func (c *commImpl) Probe(peer *RemotePeer) error {
	if c.isStopping() {
		return fmt.Errorf("Stopping!")
	}
	c.logger.Debug("Entering, endpoint:", peer.Endpoint, "PKIID:", peer.PKIID)
	var err error

	opts := c.opts
	if opts == nil {
		opts = []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(dialTimeout)}
	}
	cc, err := grpc.Dial(peer.Endpoint, append(opts, grpc.WithBlock())...)
	if err != nil {
		c.logger.Debug("Returning", err)
		return err
	}
	defer cc.Close()
	cl := proto.NewGossipClient(cc)
	_, err = cl.Ping(context.Background(), &proto.Empty{})
	c.logger.Debug("Returning", err)
	return err
}

func (c *commImpl) Accept(acceptor common.MessageAcceptor) <-chan ReceivedMessage {
	genericChan := c.msgPublisher.AddChannel(acceptor)
	specificChan := make(chan ReceivedMessage, 10)

	if c.isStopping() {
		c.logger.Warning("Accept() called but comm module is stopping, returning empty channel")
		return specificChan
	}

	c.lock.Lock()
	c.subscriptions = append(c.subscriptions, specificChan)
	c.lock.Unlock()

	go func() {
		defer c.logger.Debug("Exiting Accept() loop")
		defer func() {
			c.logger.Warning("Recovered")
			recover()
		}()

		c.stopWG.Add(1)
		defer c.stopWG.Done()

		for {
			select {
			case msg := <-genericChan:
				specificChan <- msg.(*ReceivedMessageImpl)
				break
			case s := <-c.exitChan:
				c.exitChan <- s
				return
			}
		}
	}()
	return specificChan
}

func (c *commImpl) PresumedDead() <-chan common.PKIidType {
	return c.deadEndpoints
}

func (c *commImpl) CloseConn(peer *RemotePeer) {
	c.logger.Info("Closing connection for", peer)
	c.connStore.closeConn(peer)
}

func (c *commImpl) emptySubscriptions() {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, ch := range c.subscriptions {
		close(ch)
	}
}

func (c *commImpl) Stop() {
	if c.isStopping() {
		return
	}
	atomic.StoreInt32(&c.stopping, int32(1))
	c.logger.Info("Stopping")
	defer c.logger.Info("Stopped")
	if c.gSrv != nil {
		c.gSrv.Stop()
	}
	if c.lsnr != nil {
		c.lsnr.Close()
	}
	c.connStore.shutdown()
	c.logger.Debug("Shut down connection store, connection count:", c.connStore.connNum())
	c.exitChan <- struct{}{}
	c.msgPublisher.Close()
	c.logger.Debug("Shut down publisher")
	c.emptySubscriptions()
	c.logger.Debug("Closed subscriptions, waiting for goroutines to stop...")
	c.stopWG.Wait()
}

func (c *commImpl) GetPKIid() common.PKIidType {
	return c.PKIID
}

func extractRemoteAddress(stream stream) string {
	var remoteAddress string
	p, ok := peer.FromContext(stream.Context())
	if ok {
		if address := p.Addr; address != nil {
			remoteAddress = address.String()
		}
	}
	return remoteAddress
}

func (c *commImpl) authenticateRemotePeer(stream stream) (common.PKIidType, error) {
	ctx := stream.Context()
	remoteAddress := extractRemoteAddress(stream)
	tlsUnique := ExtractTLSUnique(ctx)
	var sig []byte
	var err error
	if tlsUnique != nil && c.sec.IsEnabled() {
		sig, err = c.sec.Sign(tlsUnique)
		if err != nil {
			c.logger.Error("Failed signing TLS-Unique:", err)
			return nil, err
		}
	}

	cMsg := createConnectionMsg(c.PKIID, sig)
	stream.Send(cMsg)
	m := readWithTimeout(stream, defConnTimeout)
	if m == nil {
		c.logger.Warning("Timed out waiting for connection message from", remoteAddress)
		return nil, fmt.Errorf("Timed out")
	}
	connMsg := m.GetConn()
	if connMsg == nil {
		c.logger.Warning("Expected connection message but got", connMsg)
		return nil, fmt.Errorf("Wrong type")
	}
	if c.isPKIblackListed(connMsg.PkiID) {
		c.logger.Warning("Connection attempt from", remoteAddress, "but it is black-listed")
		return nil, fmt.Errorf("Black-listed")
	}

	if tlsUnique != nil && c.sec.IsEnabled() {
		err = c.sec.Verify(connMsg.PkiID, connMsg.Sig, tlsUnique)
		if err != nil {
			c.logger.Error("Failed verifying signature from", remoteAddress, ":", err)
			return nil, err
		}
	}

	if connMsg.PkiID == nil {
		return nil, fmt.Errorf("%s didn't send a pkiID", "Didn't send a pkiID")
	}

	c.logger.Debug("Authenticated", remoteAddress)
	return connMsg.PkiID, nil

}

func (c *commImpl) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	if c.isStopping() {
		return fmt.Errorf("Shutting down")
	}
	PKIID, err := c.authenticateRemotePeer(stream)
	if err != nil {
		c.logger.Error("Authentication failed")
		return err
	}
	c.logger.Info("Servicing", extractRemoteAddress(stream))

	conn := c.connStore.onConnected(stream, PKIID)

	// if connStore denied the connection, it means we already have a connection to that peer
	// so close this stream
	if conn == nil {
		return nil
	}

	h := func(m *proto.GossipMessage) {
		c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
			conn:          conn,
			lock:          conn,
			GossipMessage: m,
		})
	}

	conn.handler = h

	defer func() {
		c.logger.Info("Client", extractRemoteAddress(stream), " disconnected")
		c.connStore.closeByPKIid(PKIID)
	}()

	return conn.serviceConnection()
}

func (c *commImpl) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func (c *commImpl) disconnect(pkiID common.PKIidType) {
	if c.isStopping() {
		return
	}
	c.deadEndpoints <- pkiID
	c.connStore.closeByPKIid(pkiID)
}

func readWithTimeout(stream interface{}, timeout time.Duration) *proto.GossipMessage {
	incChan := make(chan *proto.GossipMessage, 1)
	go func() {
		if srvStr, isServerStr := stream.(proto.Gossip_GossipStreamServer); isServerStr {
			if m, err := srvStr.Recv(); err == nil {
				incChan <- m
			}
		}
		if clStr, isClientStr := stream.(proto.Gossip_GossipStreamClient); isClientStr {
			if m, err := clStr.Recv(); err == nil {
				incChan <- m
			}
		}
	}()
	select {
	case <-time.NewTicker(timeout).C:
		return nil
	case m := <-incChan:
		return m
	}
}

func createConnectionMsg(pkiID common.PKIidType, sig []byte) *proto.GossipMessage {
	return &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Conn{
			Conn: &proto.ConnEstablish{
				PkiID: pkiID,
				Sig:   sig,
			},
		},
	}
}

type stream interface {
	Send(*proto.GossipMessage) error
	Recv() (*proto.GossipMessage, error)
	grpc.Stream
}

func createGRPCLayer(port int) (*grpc.Server, net.Listener, grpc.DialOption) {
	var s *grpc.Server
	var ll net.Listener
	var err error
	var serverOpts []grpc.ServerOption
	var dialOpts grpc.DialOption

	keyFileName := fmt.Sprintf("key.%d.pem", time.Now().UnixNano())
	certFileName := fmt.Sprintf("cert.%d.pem", time.Now().UnixNano())

	defer os.Remove(keyFileName)
	defer os.Remove(certFileName)

	err = generateCertificates(keyFileName, certFileName)
	if err == nil {
		var creds credentials.TransportCredentials
		creds, err = credentials.NewServerTLSFromFile(certFileName, keyFileName)
		serverOpts = append(serverOpts, grpc.Creds(creds))
		ta := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})
		dialOpts = grpc.WithTransportCredentials(&authCreds{tlsCreds: ta})
	} else {
		dialOpts = grpc.WithInsecure()
	}

	listenAddress := fmt.Sprintf("%s:%d", "", port)
	ll, err = net.Listen("tcp", listenAddress)
	if err != nil {
		panic(err)
	}

	s = grpc.NewServer(serverOpts...)
	return s, ll, dialOpts
}
