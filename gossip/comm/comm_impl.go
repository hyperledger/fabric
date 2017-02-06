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
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/gossip"
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
	if len(opts) == 0 {
		c.logger.Warning("Given an empty set of grpc.DialOption, aborting")
		return
	}
	c.opts = opts
}

// NewCommInstanceWithServer creates a comm instance that creates an underlying gRPC server
func NewCommInstanceWithServer(port int, idMapper identity.Mapper, peerIdentity api.PeerIdentityType, dialOpts ...grpc.DialOption) (Comm, error) {
	var ll net.Listener
	var s *grpc.Server
	var secOpt grpc.DialOption
	var certHash []byte

	if len(dialOpts) == 0 {
		dialOpts = []grpc.DialOption{grpc.WithTimeout(dialTimeout)}
	}

	if port > 0 {
		s, ll, secOpt, certHash = createGRPCLayer(port)
		dialOpts = append(dialOpts, secOpt)
	}

	commInst := &commImpl{
		selfCertHash:      certHash,
		PKIID:             idMapper.GetPKIidOfCert(peerIdentity),
		idMapper:          idMapper,
		logger:            util.GetLogger(util.LoggingCommModule, fmt.Sprintf("%d", port)),
		peerIdentity:      peerIdentity,
		opts:              dialOpts,
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
	commInst.connStore = newConnStore(commInst, commInst.logger)
	commInst.idMapper.Put(idMapper.GetPKIidOfCert(peerIdentity), peerIdentity)

	if port > 0 {
		go func() {
			commInst.stopWG.Add(1)
			defer commInst.stopWG.Done()
			s.Serve(ll)
		}()
		proto.RegisterGossipServer(s, commInst)
	}

	return commInst, nil
}

// NewCommInstance creates a new comm instance that binds itself to the given gRPC server
func NewCommInstance(s *grpc.Server, cert *tls.Certificate, idStore identity.Mapper, peerIdentity api.PeerIdentityType, dialOpts ...grpc.DialOption) (Comm, error) {
	commInst, err := NewCommInstanceWithServer(-1, idStore, peerIdentity, dialOpts...)
	if err != nil {
		return nil, err
	}

	if cert != nil {
		inst := commInst.(*commImpl)
		if len(cert.Certificate) == 0 {
			inst.logger.Panic("Certificate supplied but certificate chain is empty")
		} else {
			inst.selfCertHash = certHashFromRawCert(cert.Certificate[0])
		}
	}

	proto.RegisterGossipServer(s, commInst.(*commImpl))

	return commInst, nil
}

type commImpl struct {
	selfCertHash      []byte
	peerIdentity      api.PeerIdentityType
	idMapper          identity.Mapper
	logger            *logging.Logger
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
	var err error
	var cc *grpc.ClientConn
	var stream proto.Gossip_GossipStreamClient
	var pkiID common.PKIidType

	c.logger.Debug("Entering", endpoint, expectedPKIID)
	defer c.logger.Debug("Exiting")

	if c.isStopping() {
		return nil, fmt.Errorf("Stopping")
	}
	cc, err = grpc.Dial(endpoint, append(c.opts, grpc.WithBlock())...)
	if err != nil {
		return nil, err
	}

	cl := proto.NewGossipClient(cc)

	if _, err = cl.Ping(context.Background(), &proto.Empty{}); err != nil {
		cc.Close()
		return nil, err
	}

	if stream, err = cl.GossipStream(context.Background()); err == nil {
		pkiID, err = c.authenticateRemotePeer(stream)
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

func (c *commImpl) Probe(remotePeer *RemotePeer) error {
	endpoint := remotePeer.Endpoint
	pkiID := remotePeer.PKIID
	if c.isStopping() {
		return fmt.Errorf("Stopping")
	}
	c.logger.Debug("Entering, endpoint:", endpoint, "PKIID:", pkiID)
	cc, err := grpc.Dial(remotePeer.Endpoint, append(c.opts, grpc.WithBlock())...)
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
	remoteCertHash := extractCertificateHashFromContext(ctx)
	var err error
	var cMsg *proto.GossipMessage
	var signer proto.Signer

	// If TLS is detected, sign the hash of our cert to bind our TLS cert
	// to the gRPC session
	if remoteCertHash != nil && c.selfCertHash != nil {
		signer = func(msg []byte) ([]byte, error) {
			return c.idMapper.Sign(msg)
		}
	} else { // If we don't use TLS, we have no unique text to sign,
		//  so don't sign anything
		signer = func(msg []byte) ([]byte, error) {
			return msg, nil
		}
	}

	cMsg = c.createConnectionMsg(c.PKIID, c.selfCertHash, c.peerIdentity, signer)

	c.logger.Debug("Sending", cMsg, "to", remoteAddress)
	stream.Send(cMsg)
	m := readWithTimeout(stream, defConnTimeout)
	if m == nil {
		c.logger.Warning("Timed out waiting for connection message from", remoteAddress)
		return nil, fmt.Errorf("Timed out")
	}
	receivedMsg := m.GetConn()
	if receivedMsg == nil {
		c.logger.Warning("Expected connection message but got", receivedMsg)
		return nil, fmt.Errorf("Wrong type")
	}

	if receivedMsg.PkiID == nil {
		c.logger.Warning("%s didn't send a pkiID")
		return nil, fmt.Errorf("%s didn't send a pkiID", remoteAddress)
	}

	if c.isPKIblackListed(receivedMsg.PkiID) {
		c.logger.Warning("Connection attempt from", remoteAddress, "but it is black-listed")
		return nil, fmt.Errorf("Black-listed")
	}
	c.logger.Debug("Received", receivedMsg, "from", remoteAddress)
	err = c.idMapper.Put(receivedMsg.PkiID, receivedMsg.Cert)
	if err != nil {
		c.logger.Warning("Identity store rejected", remoteAddress, ":", err)
		return nil, err
	}

	// if TLS is detected, verify remote peer
	if remoteCertHash != nil && c.selfCertHash != nil {
		if !bytes.Equal(remoteCertHash, receivedMsg.Hash) {
			return nil, fmt.Errorf("Expected %v in remote hash, but got %v", remoteCertHash, receivedMsg.Hash)
		}
		verifier := func(peerIdentity []byte, signature, message []byte) error {
			pkiID := c.idMapper.GetPKIidOfCert(api.PeerIdentityType(peerIdentity))
			return c.idMapper.Verify(pkiID, signature, message)
		}
		err = m.Verify(receivedMsg.Cert, verifier)
		if err != nil {
			c.logger.Error("Failed verifying signature from", remoteAddress, ":", err)
			return nil, err
		}
	}

	c.logger.Debug("Authenticated", remoteAddress)
	return receivedMsg.PkiID, nil
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
		conn.close()
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

func (c *commImpl) createConnectionMsg(pkiID common.PKIidType, hash []byte, cert api.PeerIdentityType, signer proto.Signer) *proto.GossipMessage {
	m := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Conn{
			Conn: &proto.ConnEstablish{
				Hash:  hash,
				Cert:  cert,
				PkiID: pkiID,
			},
		},
	}
	if err := m.Sign(signer); err != nil {
		c.logger.Panicf("Gossip failed to sign a message using the peer identity.\n Halting execution.\nActual error: %v", err)
	}

	return m
}

type stream interface {
	Send(*proto.GossipMessage) error
	Recv() (*proto.GossipMessage, error)
	grpc.Stream
}

func createGRPCLayer(port int) (*grpc.Server, net.Listener, grpc.DialOption, []byte) {
	var returnedCertHash []byte
	var s *grpc.Server
	var ll net.Listener
	var err error
	var serverOpts []grpc.ServerOption
	var dialOpts grpc.DialOption

	keyFileName := fmt.Sprintf("key.%d.pem", rand.Int63())
	certFileName := fmt.Sprintf("cert.%d.pem", rand.Int63())

	defer os.Remove(keyFileName)
	defer os.Remove(certFileName)

	err = generateCertificates(keyFileName, certFileName)
	if err == nil {
		cert, err := tls.LoadX509KeyPair(certFileName, keyFileName)
		if err != nil {
			panic(err)
		}

		if len(cert.Certificate) == 0 {
			panic(fmt.Errorf("Certificate chain is nil"))
		}

		returnedCertHash = certHashFromRawCert(cert.Certificate[0])

		tlsConf := &tls.Config{
			Certificates:       []tls.Certificate{cert},
			ClientAuth:         tls.RequestClientCert,
			InsecureSkipVerify: true,
		}
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConf)))
		ta := credentials.NewTLS(&tls.Config{
			Certificates:       []tls.Certificate{cert},
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
	return s, ll, dialOpts, returnedCertHash
}
