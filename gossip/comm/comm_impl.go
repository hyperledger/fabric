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
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

const (
	handshakeTimeout = time.Second * 10
	DefDialTimeout   = time.Second * 3
	DefConnTimeout   = time.Second * 2
	DefRecvBuffSize  = 20
	DefSendBuffSize  = 20
)

var errProbe = errors.New("probe")

// SecurityAdvisor defines an external auxiliary object
// that provides security and identity related capabilities
type SecurityAdvisor interface {
	// OrgByPeerIdentity returns the organization identity of the given PeerIdentityType
	OrgByPeerIdentity(api.PeerIdentityType) api.OrgIdentityType
}

func (c *commImpl) SetDialOpts(opts ...grpc.DialOption) {
	if len(opts) == 0 {
		c.logger.Warning("Given an empty set of grpc.DialOption, aborting")
		return
	}
	c.opts = opts
}

// NewCommInstance creates a new comm instance that binds itself to the given gRPC server
func NewCommInstance(s *grpc.Server, certs *common.TLSCertificates, idStore identity.Mapper,
	peerIdentity api.PeerIdentityType, secureDialOpts api.PeerSecureDialOpts, sa api.SecurityAdvisor,
	commMetrics *metrics.CommMetrics, config CommConfig, dialOpts ...grpc.DialOption) (Comm, error) {
	commInst := &commImpl{
		sa:              sa,
		pubSub:          util.NewPubSub(),
		PKIID:           idStore.GetPKIidOfCert(peerIdentity),
		idMapper:        idStore,
		logger:          util.GetLogger(util.CommLogger, ""),
		peerIdentity:    peerIdentity,
		opts:            dialOpts,
		secureDialOpts:  secureDialOpts,
		msgPublisher:    NewChannelDemultiplexer(),
		lock:            &sync.Mutex{},
		deadEndpoints:   make(chan common.PKIidType, 100),
		identityChanges: make(chan common.PKIidType, 1),
		stopping:        int32(0),
		exitChan:        make(chan struct{}),
		subscriptions:   make([]chan protoext.ReceivedMessage, 0),
		tlsCerts:        certs,
		metrics:         commMetrics,
		dialTimeout:     config.DialTimeout,
		connTimeout:     config.ConnTimeout,
		recvBuffSize:    config.RecvBuffSize,
		sendBuffSize:    config.SendBuffSize,
	}

	connConfig := ConnConfig{
		RecvBuffSize: config.RecvBuffSize,
		SendBuffSize: config.SendBuffSize,
	}

	commInst.connStore = newConnStore(commInst, commInst.logger, connConfig)

	proto.RegisterGossipServer(s, commInst)

	return commInst, nil
}

// CommConfig is the configuration required to initialize a new comm
type CommConfig struct {
	DialTimeout  time.Duration // Dial timeout
	ConnTimeout  time.Duration // Connection timeout
	RecvBuffSize int           // Buffer size of received messages
	SendBuffSize int           // Buffer size of sending messages
}

type commImpl struct {
	sa              api.SecurityAdvisor
	tlsCerts        *common.TLSCertificates
	pubSub          *util.PubSub
	peerIdentity    api.PeerIdentityType
	idMapper        identity.Mapper
	logger          util.Logger
	opts            []grpc.DialOption
	secureDialOpts  func() []grpc.DialOption
	connStore       *connectionStore
	PKIID           []byte
	deadEndpoints   chan common.PKIidType
	identityChanges chan common.PKIidType
	msgPublisher    *ChannelDeMultiplexer
	lock            *sync.Mutex
	exitChan        chan struct{}
	stopWG          sync.WaitGroup
	subscriptions   []chan protoext.ReceivedMessage
	stopping        int32
	metrics         *metrics.CommMetrics
	dialTimeout     time.Duration
	connTimeout     time.Duration
	recvBuffSize    int
	sendBuffSize    int
}

func (c *commImpl) createConnection(endpoint string, expectedPKIID common.PKIidType) (*connection, error) {
	var err error
	var cc *grpc.ClientConn
	var stream proto.Gossip_GossipStreamClient
	var pkiID common.PKIidType
	var connInfo *protoext.ConnectionInfo
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
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()
	cc, err = grpc.DialContext(ctx, endpoint, dialOpts...)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cl := proto.NewGossipClient(cc)

	ctx, cancel = context.WithTimeout(context.Background(), c.connTimeout)
	defer cancel()
	if _, err = cl.Ping(ctx, &proto.Empty{}); err != nil {
		cc.Close()
		return nil, errors.WithStack(err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	if stream, err = cl.GossipStream(ctx); err == nil {
		connInfo, err = c.authenticateRemotePeer(stream, true, false)
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
					cancel()
					return nil, errors.New("authentication failure")
				} else {
					c.logger.Infof("Peer %s changed its PKI-ID from %s to %s", endpoint, expectedPKIID, pkiID)
					c.identityChanges <- expectedPKIID
				}
			}
			connConfig := ConnConfig{
				RecvBuffSize: c.recvBuffSize,
				SendBuffSize: c.sendBuffSize,
			}
			conn := newConnection(cl, cc, stream, c.metrics, connConfig)
			conn.pkiID = pkiID
			conn.info = connInfo
			conn.logger = c.logger
			conn.cancel = cancel

			h := func(m *protoext.SignedGossipMessage) {
				c.logger.Debug("Got message:", m)
				c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
					conn:                conn,
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
	cancel()
	return nil, errors.WithStack(err)
}

func (c *commImpl) Send(msg *protoext.SignedGossipMessage, peers ...*RemotePeer) {
	if c.isStopping() || len(peers) == 0 {
		return
	}
	c.logger.Debug("Entering, sending", msg, "to ", len(peers), "peers")

	for _, peer := range peers {
		go func(peer *RemotePeer, msg *protoext.SignedGossipMessage) {
			c.sendToEndpoint(peer, msg, nonBlockingSend)
		}(peer, msg)
	}
}

func (c *commImpl) sendToEndpoint(peer *RemotePeer, msg *protoext.SignedGossipMessage, shouldBlock blockingBehavior) {
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
			conn.close()
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
		return errors.New("stopping")
	}
	c.logger.Debug("Entering, endpoint:", endpoint, "PKIID:", pkiID)
	dialOpts = append(dialOpts, c.secureDialOpts()...)
	dialOpts = append(dialOpts, grpc.WithBlock())
	dialOpts = append(dialOpts, c.opts...)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()
	cc, err := grpc.DialContext(ctx, remotePeer.Endpoint, dialOpts...)
	if err != nil {
		c.logger.Debugf("Returning %v", err)
		return err
	}
	defer cc.Close()
	cl := proto.NewGossipClient(cc)
	ctx, cancel = context.WithTimeout(context.Background(), c.connTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()
	cc, err := grpc.DialContext(ctx, remotePeer.Endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}
	defer cc.Close()

	cl := proto.NewGossipClient(cc)
	ctx, cancel = context.WithTimeout(context.Background(), c.connTimeout)
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
	connInfo, err := c.authenticateRemotePeer(stream, true, true)
	if err != nil {
		c.logger.Warningf("Authentication failed: %v", err)
		return nil, err
	}
	if len(remotePeer.PKIID) > 0 && !bytes.Equal(connInfo.ID, remotePeer.PKIID) {
		return nil, errors.New("PKI-ID of remote peer doesn't match expected PKI-ID")
	}
	return connInfo.Identity, nil
}

func (c *commImpl) Accept(acceptor common.MessageAcceptor) <-chan protoext.ReceivedMessage {
	genericChan := c.msgPublisher.AddChannel(acceptor)
	specificChan := make(chan protoext.ReceivedMessage, 10)

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
			case msg, channelOpen := <-genericChan:
				if !channelOpen {
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

func (c *commImpl) IdentitySwitch() <-chan common.PKIidType {
	return c.identityChanges
}

func (c *commImpl) CloseConn(peer *RemotePeer) {
	c.logger.Debug("Closing connection for", peer)
	c.connStore.closeConnByPKIid(peer.PKIID)
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

func (c *commImpl) authenticateRemotePeer(stream stream, initiator, isProbe bool) (*protoext.ConnectionInfo, error) {
	ctx := stream.Context()
	remoteAddress := extractRemoteAddress(stream)
	remoteCertHash := extractCertificateHashFromContext(ctx)
	var err error
	var cMsg *protoext.SignedGossipMessage
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
		return nil, errors.New("no TLS certificate")
	}

	cMsg, err = c.createConnectionMsg(c.PKIID, selfCertHash, c.peerIdentity, signer, isProbe)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Sending", cMsg, "to", remoteAddress)
	stream.Send(cMsg.Envelope)
	m, err := readWithTimeout(stream, c.connTimeout, remoteAddress)
	if err != nil {
		c.logger.Warningf("Failed reading message from %s, reason: %v", remoteAddress, err)
		return nil, err
	}
	receivedMsg := m.GetConn()
	if receivedMsg == nil {
		c.logger.Warning("Expected connection message from", remoteAddress, "but got", receivedMsg)
		return nil, errors.New("wrong type")
	}

	if receivedMsg.PkiId == nil {
		c.logger.Warningf("%s didn't send a pkiID", remoteAddress)
		return nil, errors.New("no PKI-ID")
	}

	c.logger.Debug("Received", receivedMsg, "from", remoteAddress)
	err = c.idMapper.Put(receivedMsg.PkiId, receivedMsg.Identity)
	if err != nil {
		c.logger.Warningf("Identity store rejected %s : %v", remoteAddress, err)
		return nil, err
	}

	connInfo := &protoext.ConnectionInfo{
		ID:       receivedMsg.PkiId,
		Identity: receivedMsg.Identity,
		Endpoint: remoteAddress,
		Auth: &protoext.AuthInfo{
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
		pkiID := c.idMapper.GetPKIidOfCert(peerIdentity)
		return c.idMapper.Verify(pkiID, signature, message)
	}
	err = m.Verify(receivedMsg.Identity, verifier)
	if err != nil {
		c.logger.Errorf("Failed verifying signature from %s : %v", remoteAddress, err)
		return nil, err
	}

	c.logger.Debug("Authenticated", remoteAddress)

	if receivedMsg.Probe {
		return connInfo, errProbe
	}

	return connInfo, nil
}

// SendWithAck sends a message to remote peers, waiting for acknowledgement from minAck of them, or until a certain timeout expires
func (c *commImpl) SendWithAck(msg *protoext.SignedGossipMessage, timeout time.Duration, minAck int, peers ...*RemotePeer) AggregatedSendResult {
	if len(peers) == 0 {
		return nil
	}
	var err error

	// Roll a random NONCE to be used as a send ID to differentiate
	// between different invocations
	msg.Nonce = util.RandomUInt64()
	// Replace the envelope in the message to update the NONCE
	msg, err = protoext.NoopSign(msg.GossipMessage)

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
	sndFunc := func(peer *RemotePeer, msg *protoext.SignedGossipMessage) {
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
				return errors.Errorf("received a message of type %s, expected *proto.Acknowledgement", reflect.TypeOf(msg))
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
		return errors.New("shutting down")
	}
	connInfo, err := c.authenticateRemotePeer(stream, false, false)

	if err == errProbe {
		c.logger.Infof("Peer %s (%s) probed us", connInfo.ID, connInfo.Endpoint)
		return nil
	}

	if err != nil {
		c.logger.Errorf("Authentication failed: %v", err)
		return err
	}
	c.logger.Debug("Servicing", extractRemoteAddress(stream))

	conn := c.connStore.onConnected(stream, connInfo, c.metrics)

	h := func(m *protoext.SignedGossipMessage) {
		c.msgPublisher.DeMultiplex(&ReceivedMessageImpl{
			conn:                conn,
			SignedGossipMessage: m,
			connInfo:            connInfo,
		})
	}

	conn.handler = interceptAcks(h, connInfo.ID, c.pubSub)

	defer func() {
		c.logger.Debug("Client", extractRemoteAddress(stream), " disconnected")
		c.connStore.closeConnByPKIid(connInfo.ID)
	}()

	return conn.serviceConnection()
}

func (c *commImpl) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func (c *commImpl) disconnect(pkiID common.PKIidType) {
	select {
	case c.deadEndpoints <- pkiID:
	case <-c.exitChan:
		return
	}

	c.connStore.closeConnByPKIid(pkiID)
}

func readWithTimeout(stream stream, timeout time.Duration, address string) (*protoext.SignedGossipMessage, error) {
	incChan := make(chan *protoext.SignedGossipMessage, 1)
	errChan := make(chan error, 1)
	go func() {
		if m, err := stream.Recv(); err == nil {
			msg, err := protoext.EnvelopeToGossipMessage(m)
			if err != nil {
				errChan <- err
				return
			}
			incChan <- msg
		}
	}()
	select {
	case <-time.After(timeout):
		return nil, errors.Errorf("timed out waiting for connection message from %s", address)
	case m := <-incChan:
		return m, nil
	case err := <-errChan:
		return nil, errors.WithStack(err)
	}
}

func (c *commImpl) createConnectionMsg(pkiID common.PKIidType, certHash []byte, cert api.PeerIdentityType, signer protoext.Signer, isProbe bool) (*protoext.SignedGossipMessage, error) {
	m := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Conn{
			Conn: &proto.ConnEstablish{
				TlsCertHash: certHash,
				Identity:    cert,
				PkiId:       pkiID,
				Probe:       isProbe,
			},
		},
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
	return sMsg, errors.WithStack(err)
}

type stream interface {
	Send(envelope *proto.Envelope) error
	Recv() (*proto.Envelope, error)
	Context() context.Context
}

func topicForAck(nonce uint64, pkiID common.PKIidType) string {
	return fmt.Sprintf("%d %s", nonce, hex.EncodeToString(pkiID))
}
