/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

const (
	handshakeTimeout = time.Second * time.Duration(10)
	defDialTimeout   = time.Second * time.Duration(3)
	defConnTimeout   = time.Second * time.Duration(2)
	defRecvBuffSize  = 20
	defSendBuffSize  = 20
)

// SecurityAdvisor defines an external auxiliary object
// that provides security and identity related capabilities
type SecurityAdvisor interface {
	// OrgByPeerIdentity returns the organization identity of the given PeerIdentityType
	OrgByPeerIdentity(api.PeerIdentityType) api.OrgIdentityType
}

// SetDialTimeout sets the dial timeout
func SetDialTimeout(timeout time.Duration) {
	viper.Set("peer.gossip.dialTimeout", timeout)
}

func (c *commImpl) SetDialOpts(opts ...grpc.DialOption) {
	if len(opts) == 0 {
		c.logger.Warning("Given an empty set of grpc.DialOption, aborting")
		return
	}
	c.opts = opts
}

// NewCommInstanceWithServer creates a comm instance that creates an underlying gRPC server
func NewCommInstanceWithServer(port int, idMapper identity.Mapper, peerIdentity api.PeerIdentityType,
	secureDialOpts api.PeerSecureDialOpts, sa api.SecurityAdvisor, dialOpts ...grpc.DialOption) (Comm, error) {

	var ll net.Listener
	var s *grpc.Server
	var certs *common.TLSCertificates

	if port > 0 {
		s, ll, secureDialOpts, certs = createGRPCLayer(port)
	}

	commInst := &commImpl{
		sa:             sa,
		pubSub:         util.NewPubSub(),
		PKIID:          idMapper.GetPKIidOfCert(peerIdentity),
		idMapper:       idMapper,
		logger:         util.GetLogger(util.LoggingCommModule, fmt.Sprintf("%d", port)),
		peerIdentity:   peerIdentity,
		opts:           dialOpts,
		secureDialOpts: secureDialOpts,
		port:           port,
		lsnr:           ll,
		gSrv:           s,
		msgPublisher:   NewChannelDemultiplexer(),
		lock:           &sync.Mutex{},
		deadEndpoints:  make(chan common.PKIidType, 100),
		stopping:       int32(0),
		exitChan:       make(chan struct{}),
		subscriptions:  make([]chan proto.ReceivedMessage, 0),
		dialTimeout:    util.GetDurationOrDefault("peer.gossip.dialTimeout", defDialTimeout),
		tlsCerts:       certs,
	}
	commInst.connStore = newConnStore(commInst, commInst.logger)

	if port > 0 {
		commInst.stopWG.Add(1)
		proto.RegisterGossipServer(s, commInst)
		go func() {
			defer commInst.stopWG.Done()
			s.Serve(ll)
		}()
	}

	return commInst, nil
}

// NewCommInstance creates a new comm instance that binds itself to the given gRPC server
func NewCommInstance(s *grpc.Server, certs *common.TLSCertificates, idStore identity.Mapper,
	peerIdentity api.PeerIdentityType, secureDialOpts api.PeerSecureDialOpts, sa api.SecurityAdvisor,
	dialOpts ...grpc.DialOption) (Comm, error) {

	commInst, err := NewCommInstanceWithServer(-1, idStore, peerIdentity, secureDialOpts, sa, dialOpts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	commInst.(*commImpl).tlsCerts = certs

	proto.RegisterGossipServer(s, commInst.(*commImpl))

	return commInst, nil
}

type commImpl struct {
	sa             api.SecurityAdvisor
	tlsCerts       *common.TLSCertificates
	pubSub         *util.PubSub
	peerIdentity   api.PeerIdentityType
	idMapper       identity.Mapper
	logger         util.Logger
	opts           []grpc.DialOption
	secureDialOpts func() []grpc.DialOption
	connStore      *connectionStore
	PKIID          []byte
	deadEndpoints  chan common.PKIidType
	msgPublisher   *ChannelDeMultiplexer
	lock           *sync.Mutex
	lsnr           net.Listener
	gSrv           *grpc.Server
	exitChan       chan struct{}
	stopWG         sync.WaitGroup
	subscriptions  []chan proto.ReceivedMessage
	port           int
	stopping       int32
	dialTimeout    time.Duration
}

func (c *commImpl) createConnection(endpoint string, expectedPKIID common.PKIidType) (*connection, error) {
	var err error
	var cc *grpc.ClientConn
	var stream proto.Gossip_GossipStreamClient
	var pkiID common.PKIidType
	var connInfo *proto.ConnectionInfo
	var dialOpts []grpc.DialOption

	c.logger.Debug("Entering", endpoint, expectedPKIID)
	defer c.logger.Debug("Exiting")

	if c.isStopping() {
		return nil, errors.New("Stopping")
	}
	dialOpts = append(dialOpts, c.secureDialOpts()...)
	dialOpts = append(dialOpts, grpc.WithBlock())
	dialOpts = append(dialOpts, c.opts...)
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, c.dialTimeout)
	cc, err = grpc.DialContext(ctx, endpoint, dialOpts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cl := proto.NewGossipClient(cc)

	ctx, cancel := context.WithTimeout(context.Background(), defConnTimeout)
	defer cancel()
	if _, err = cl.Ping(ctx, &proto.Empty{}); err != nil {
		cc.Close()
		return nil, errors.WithStack(err)
	}

	ctx, cf := context.WithCancel(context.Background())
	if stream, err = cl.GossipStream(ctx); err == nil {
		connInfo, err = c.authenticateRemotePeer(stream, true)
		if err == nil {
			pkiID = connInfo.ID
			// PKIID is nil when we don't know the remote PKI id's
			if expectedPKIID != nil && !bytes.Equal(pkiID, expectedPKIID) {
				actualOrg := c.sa.OrgByPeerIdentity(connInfo.Identity)
				// If the identity isn't present, it's nil - therefore OrgByPeerIdentity would
				// return nil too and thus would be different than the actual organization
				identity, _ := c.idMapper.Get(expectedPKIID)
				oldOrg := c.sa.OrgByPeerIdentity(identity)
				if !bytes.Equal(actualOrg, oldOrg) {
					c.logger.Warning("Remote endpoint claims to be a different peer, expected", expectedPKIID, "but got", pkiID)
					cc.Close()
					return nil, errors.New("authentication failure")
				}
			}
			conn := newConnection(cl, cc, stream, nil)
			conn.pkiID = pkiID
			conn.info = connInfo
			conn.logger = c.logger
			conn.cancel = cf

			h := func(m *proto.SignedGossipMessage) {
				c.logger.Debug("Got message:", m)
				c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
					conn:                conn,
					lock:                conn,
					SignedGossipMessage: m,
					connInfo:            connInfo,
				})
			}
			conn.handler = interceptAcks(h, connInfo.ID, c.pubSub)
			return conn, nil
		}
		c.logger.Warningf("Authentication failed: %+v", err)
	}
	cc.Close()
	return nil, errors.WithStack(err)
}

func (c *commImpl) Send(msg *proto.SignedGossipMessage, peers ...*RemotePeer) {
	if c.isStopping() || len(peers) == 0 {
		return
	}
	c.logger.Debug("Entering, sending", msg, "to ", len(peers), "peers")

	for _, peer := range peers {
		go func(peer *RemotePeer, msg *proto.SignedGossipMessage) {
			c.sendToEndpoint(peer, msg, nonBlockingSend)
		}(peer, msg)
	}
}

func (c *commImpl) sendToEndpoint(peer *RemotePeer, msg *proto.SignedGossipMessage, shouldBlock blockingBehavior) {
	if c.isStopping() {
		return
	}
	c.logger.Debug("Entering, Sending to", peer.Endpoint, ", msg:", msg)
	defer c.logger.Debug("Exiting")
	var err error

	conn, err := c.connStore.getConnection(peer)
	if err == nil {
		disConnectOnErr := func(err error) {
			c.logger.Warningf("%v isn't responsive: %v", peer, err)
			c.disconnect(peer.PKIID)
		}
		conn.send(msg, disConnectOnErr, shouldBlock)
		return
	}
	c.logger.Warningf("Failed obtaining connection for %v reason: %v", peer, err)
	c.disconnect(peer.PKIID)
}

func (c *commImpl) isStopping() bool {
	return atomic.LoadInt32(&c.stopping) == int32(1)
}

func (c *commImpl) Probe(remotePeer *RemotePeer) error {
	var dialOpts []grpc.DialOption
	endpoint := remotePeer.Endpoint
	pkiID := remotePeer.PKIID
	if c.isStopping() {
		return fmt.Errorf("Stopping")
	}
	c.logger.Debug("Entering, endpoint:", endpoint, "PKIID:", pkiID)
	dialOpts = append(dialOpts, c.secureDialOpts()...)
	dialOpts = append(dialOpts, grpc.WithBlock())
	dialOpts = append(dialOpts, c.opts...)
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, c.dialTimeout)
	cc, err := grpc.DialContext(ctx, remotePeer.Endpoint, dialOpts...)
	if err != nil {
		c.logger.Debugf("Returning %v", err)
		return err
	}
	defer cc.Close()
	cl := proto.NewGossipClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), defConnTimeout)
	defer cancel()
	_, err = cl.Ping(ctx, &proto.Empty{})
	c.logger.Debugf("Returning %v", err)
	return err
}

func (c *commImpl) Handshake(remotePeer *RemotePeer) (api.PeerIdentityType, error) {
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, c.secureDialOpts()...)
	dialOpts = append(dialOpts, grpc.WithBlock())
	dialOpts = append(dialOpts, c.opts...)
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, c.dialTimeout)
	cc, err := grpc.DialContext(ctx, remotePeer.Endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}
	defer cc.Close()

	cl := proto.NewGossipClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), defConnTimeout)
	defer cancel()
	if _, err = cl.Ping(ctx, &proto.Empty{}); err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	stream, err := cl.GossipStream(ctx)
	if err != nil {
		return nil, err
	}
	connInfo, err := c.authenticateRemotePeer(stream, true)
	if err != nil {
		c.logger.Warningf("Authentication failed: %v", err)
		return nil, err
	}
	if len(remotePeer.PKIID) > 0 && !bytes.Equal(connInfo.ID, remotePeer.PKIID) {
		return nil, fmt.Errorf("PKI-ID of remote peer doesn't match expected PKI-ID")
	}
	return connInfo.Identity, nil
}

func (c *commImpl) Accept(acceptor common.MessageAcceptor) <-chan proto.ReceivedMessage {
	genericChan := c.msgPublisher.AddChannel(acceptor)
	specificChan := make(chan proto.ReceivedMessage, 10)

	if c.isStopping() {
		c.logger.Warning("Accept() called but comm module is stopping, returning empty channel")
		return specificChan
	}

	c.lock.Lock()
	c.subscriptions = append(c.subscriptions, specificChan)
	c.lock.Unlock()

	c.stopWG.Add(1)
	go func() {
		defer c.logger.Debug("Exiting Accept() loop")

		defer c.stopWG.Done()

		for {
			select {
			case msg := <-genericChan:
				if msg == nil {
					return
				}
				select {
				case specificChan <- msg.(*ReceivedMessageImpl):
				case <-c.exitChan:
					return
				}
			case <-c.exitChan:
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
	c.logger.Debug("Closing connection for", peer)
	c.connStore.closeConn(peer)
}

func (c *commImpl) closeSubscriptions() {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, ch := range c.subscriptions {
		close(ch)
	}
}

func (c *commImpl) Stop() {
	if !atomic.CompareAndSwapInt32(&c.stopping, 0, int32(1)) {
		return
	}
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
	c.msgPublisher.Close()
	close(c.exitChan)
	c.stopWG.Wait()
	c.closeSubscriptions()
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

func (c *commImpl) authenticateRemotePeer(stream stream, initiator bool) (*proto.ConnectionInfo, error) {
	ctx := stream.Context()
	remoteAddress := extractRemoteAddress(stream)
	remoteCertHash := extractCertificateHashFromContext(ctx)
	var err error
	var cMsg *proto.SignedGossipMessage
	useTLS := c.tlsCerts != nil
	var selfCertHash []byte

	if useTLS {
		certReference := c.tlsCerts.TLSServerCert
		if initiator {
			certReference = c.tlsCerts.TLSClientCert
		}
		selfCertHash = certHashFromRawCert(certReference.Load().(*tls.Certificate).Certificate[0])
	}

	signer := func(msg []byte) ([]byte, error) {
		return c.idMapper.Sign(msg)
	}

	// TLS enabled but not detected on other side
	if useTLS && len(remoteCertHash) == 0 {
		c.logger.Warningf("%s didn't send TLS certificate", remoteAddress)
		return nil, fmt.Errorf("No TLS certificate")
	}

	cMsg, err = c.createConnectionMsg(c.PKIID, selfCertHash, c.peerIdentity, signer)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Sending", cMsg, "to", remoteAddress)
	stream.Send(cMsg.Envelope)
	m, err := readWithTimeout(stream, util.GetDurationOrDefault("peer.gossip.connTimeout", defConnTimeout), remoteAddress)
	if err != nil {
		c.logger.Warningf("Failed reading messge from %s, reason: %v", remoteAddress, err)
		return nil, err
	}
	receivedMsg := m.GetConn()
	if receivedMsg == nil {
		c.logger.Warning("Expected connection message from", remoteAddress, "but got", receivedMsg)
		return nil, fmt.Errorf("Wrong type")
	}

	if receivedMsg.PkiId == nil {
		c.logger.Warningf("%s didn't send a pkiID", remoteAddress)
		return nil, fmt.Errorf("No PKI-ID")
	}

	c.logger.Debug("Received", receivedMsg, "from", remoteAddress)
	err = c.idMapper.Put(receivedMsg.PkiId, receivedMsg.Identity)
	if err != nil {
		c.logger.Warningf("Identity store rejected %s : %v", remoteAddress, err)
		return nil, err
	}

	connInfo := &proto.ConnectionInfo{
		ID:       receivedMsg.PkiId,
		Identity: receivedMsg.Identity,
		Endpoint: remoteAddress,
		Auth: &proto.AuthInfo{
			Signature:  m.Signature,
			SignedData: m.Payload,
		},
	}

	// if TLS is enabled and detected, verify remote peer
	if useTLS {
		// If the remote peer sent its TLS certificate, make sure it actually matches the TLS cert
		// that the peer used.
		if !bytes.Equal(remoteCertHash, receivedMsg.TlsCertHash) {
			return nil, errors.Errorf("Expected %v in remote hash of TLS cert, but got %v", remoteCertHash, receivedMsg.TlsCertHash)
		}
	}
	// Final step - verify the signature on the connection message itself
	verifier := func(peerIdentity []byte, signature, message []byte) error {
		pkiID := c.idMapper.GetPKIidOfCert(api.PeerIdentityType(peerIdentity))
		return c.idMapper.Verify(pkiID, signature, message)
	}
	err = m.Verify(receivedMsg.Identity, verifier)
	if err != nil {
		c.logger.Errorf("Failed verifying signature from %s : %v", remoteAddress, err)
		return nil, err
	}

	c.logger.Debug("Authenticated", remoteAddress)

	return connInfo, nil
}

// SendWithAck sends a message to remote peers, waiting for acknowledgement from minAck of them, or until a certain timeout expires
func (c *commImpl) SendWithAck(msg *proto.SignedGossipMessage, timeout time.Duration, minAck int, peers ...*RemotePeer) AggregatedSendResult {
	if len(peers) == 0 {
		return nil
	}
	var err error

	// Roll a random NONCE to be used as a send ID to differentiate
	// between different invocations
	msg.Nonce = util.RandomUInt64()
	// Replace the envelope in the message to update the NONCE
	msg, err = msg.NoopSign()

	if c.isStopping() || err != nil {
		if err == nil {
			err = errors.New("comm is stopping")
		}
		results := []SendResult{}
		for _, p := range peers {
			results = append(results, SendResult{
				error:      err,
				RemotePeer: *p,
			})
		}
		return results
	}
	c.logger.Debug("Entering, sending", msg, "to ", len(peers), "peers")
	sndFunc := func(peer *RemotePeer, msg *proto.SignedGossipMessage) {
		c.sendToEndpoint(peer, msg, blockingSend)
	}
	// Subscribe to acks
	subscriptions := make(map[string]func() error)
	for _, p := range peers {
		topic := topicForAck(msg.Nonce, p.PKIID)
		sub := c.pubSub.Subscribe(topic, timeout)
		subscriptions[string(p.PKIID)] = func() error {
			msg, err := sub.Listen()
			if err != nil {
				return err
			}
			if msg, isAck := msg.(*proto.Acknowledgement); !isAck {
				return fmt.Errorf("Received a message of type %s, expected *proto.Acknowledgement", reflect.TypeOf(msg))
			} else {
				if msg.Error != "" {
					return errors.New(msg.Error)
				}
			}
			return nil
		}
	}
	waitForAck := func(p *RemotePeer) error {
		return subscriptions[string(p.PKIID)]()
	}
	ackOperation := newAckSendOperation(sndFunc, waitForAck)
	return ackOperation.send(msg, minAck, peers...)
}

func (c *commImpl) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	if c.isStopping() {
		return fmt.Errorf("Shutting down")
	}
	connInfo, err := c.authenticateRemotePeer(stream, false)
	if err != nil {
		c.logger.Errorf("Authentication failed: %v", err)
		return err
	}
	c.logger.Debug("Servicing", extractRemoteAddress(stream))

	conn := c.connStore.onConnected(stream, connInfo)

	// if connStore denied the connection, it means we already have a connection to that peer
	// so close this stream
	if conn == nil {
		return nil
	}

	h := func(m *proto.SignedGossipMessage) {
		c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
			conn:                conn,
			lock:                conn,
			SignedGossipMessage: m,
			connInfo:            connInfo,
		})
	}

	conn.handler = interceptAcks(h, connInfo.ID, c.pubSub)

	defer func() {
		c.logger.Debug("Client", extractRemoteAddress(stream), " disconnected")
		c.connStore.closeByPKIid(connInfo.ID)
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

func readWithTimeout(stream interface{}, timeout time.Duration, address string) (*proto.SignedGossipMessage, error) {
	incChan := make(chan *proto.SignedGossipMessage, 1)
	errChan := make(chan error, 1)
	go func() {
		if srvStr, isServerStr := stream.(proto.Gossip_GossipStreamServer); isServerStr {
			if m, err := srvStr.Recv(); err == nil {
				msg, err := m.ToGossipMessage()
				if err != nil {
					errChan <- err
					return
				}
				incChan <- msg
			}
		} else if clStr, isClientStr := stream.(proto.Gossip_GossipStreamClient); isClientStr {
			if m, err := clStr.Recv(); err == nil {
				msg, err := m.ToGossipMessage()
				if err != nil {
					errChan <- err
					return
				}
				incChan <- msg
			}
		} else {
			panic(errors.Errorf("Stream isn't a GossipStreamServer or a GossipStreamClient, but %v. Aborting", reflect.TypeOf(stream)))
		}
	}()
	select {
	case <-time.NewTicker(timeout).C:
		return nil, errors.Errorf("Timed out waiting for connection message from %s", address)
	case m := <-incChan:
		return m, nil
	case err := <-errChan:
		return nil, errors.WithStack(err)
	}
}

func (c *commImpl) createConnectionMsg(pkiID common.PKIidType, certHash []byte, cert api.PeerIdentityType, signer proto.Signer) (*proto.SignedGossipMessage, error) {
	m := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Conn{
			Conn: &proto.ConnEstablish{
				TlsCertHash: certHash,
				Identity:    cert,
				PkiId:       pkiID,
			},
		},
	}
	sMsg := &proto.SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
	return sMsg, errors.WithStack(err)
}

type stream interface {
	Send(envelope *proto.Envelope) error
	Recv() (*proto.Envelope, error)
	grpc.Stream
}

func createGRPCLayer(port int) (*grpc.Server, net.Listener, api.PeerSecureDialOpts, *common.TLSCertificates) {
	var s *grpc.Server
	var ll net.Listener
	var err error
	var serverOpts []grpc.ServerOption
	var dialOpts []grpc.DialOption

	clientCert := GenerateCertificatesOrPanic()
	serverCert := GenerateCertificatesOrPanic()

	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{serverCert},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
	}
	serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConf)))
	ta := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true,
	})
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(ta))

	listenAddress := fmt.Sprintf("%s:%d", "", port)
	ll, err = net.Listen("tcp", listenAddress)
	if err != nil {
		panic(err)
	}
	secureDialOpts := func() []grpc.DialOption {
		return dialOpts
	}
	s = grpc.NewServer(serverOpts...)
	certs := &common.TLSCertificates{}
	certs.TLSServerCert.Store(&serverCert)
	certs.TLSClientCert.Store(&clientCert)
	return s, ll, secureDialOpts, certs
}

func topicForAck(nonce uint64, pkiID common.PKIidType) string {
	return fmt.Sprintf("%d %s", nonce, hex.EncodeToString(pkiID))
}
