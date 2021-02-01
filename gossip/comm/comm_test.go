/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/api/mocks"
	gmocks "github.com/hyperledger/fabric/gossip/comm/mocks"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	util.SetupTestLogging()
	rand.Seed(time.Now().UnixNano())
	factory.InitFactories(nil)
	naiveSec.On("OrgByPeerIdentity", mock.Anything).Return(api.OrgIdentityType{})
}

var testCommConfig = CommConfig{
	DialTimeout:  300 * time.Millisecond,
	ConnTimeout:  DefConnTimeout,
	RecvBuffSize: DefRecvBuffSize,
	SendBuffSize: DefSendBuffSize,
}

func acceptAll(msg interface{}) bool {
	return true
}

var noopPurgeIdentity = func(_ common.PKIidType, _ api.PeerIdentityType) {
}

var (
	naiveSec        = &naiveSecProvider{}
	hmacKey         = []byte{0, 0, 0}
	disabledMetrics = metrics.NewGossipMetrics(&disabled.Provider{}).CommMetrics
)

type naiveSecProvider struct {
	mocks.SecurityAdvisor
}

func (nsp *naiveSecProvider) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return nsp.SecurityAdvisor.Called(identity).Get(0).(api.OrgIdentityType)
}

func (*naiveSecProvider) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (*naiveSecProvider) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveSecProvider) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveSecProvider) VerifyBlock(channelID common.ChannelID, seqNum uint64, signedBlock *cb.Block) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveSecProvider) Sign(msg []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(msg)
	return mac.Sum(nil), nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*naiveSecProvider) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(message)
	expected := mac.Sum(nil)
	if !bytes.Equal(signature, expected) {
		return fmt.Errorf("Wrong certificate:%v, %v", signature, message)
	}
	return nil
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (*naiveSecProvider) VerifyByChannel(_ common.ChannelID, _ api.PeerIdentityType, _, _ []byte) error {
	return nil
}

func newCommInstanceOnlyWithMetrics(t *testing.T, commMetrics *metrics.CommMetrics, sec *naiveSecProvider,
	gRPCServer *comm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, dialOpts ...grpc.DialOption) Comm {
	_, portString, err := net.SplitHostPort(gRPCServer.Address())
	require.NoError(t, err)

	endpoint := fmt.Sprintf("127.0.0.1:%s", portString)
	id := []byte(endpoint)
	identityMapper := identity.NewIdentityMapper(sec, id, noopPurgeIdentity, sec)

	commInst, err := NewCommInstance(gRPCServer.Server(), certs, identityMapper, id, secureDialOpts,
		sec, commMetrics, testCommConfig, dialOpts...)
	require.NoError(t, err)

	go func() {
		err := gRPCServer.Start()
		require.NoError(t, err)
	}()

	return &commGRPC{commInst.(*commImpl), gRPCServer}
}

type commGRPC struct {
	*commImpl
	gRPCServer *comm.GRPCServer
}

func (c *commGRPC) Stop() {
	c.commImpl.Stop()
	c.commImpl.idMapper.Stop()
	c.gRPCServer.Stop()
}

func newCommInstanceOnly(t *testing.T, sec *naiveSecProvider,
	gRPCServer *comm.GRPCServer, certs *common.TLSCertificates,
	secureDialOpts api.PeerSecureDialOpts, dialOpts ...grpc.DialOption) Comm {
	return newCommInstanceOnlyWithMetrics(t, disabledMetrics, sec, gRPCServer, certs, secureDialOpts, dialOpts...)
}

func newCommInstance(t *testing.T, sec *naiveSecProvider) (c Comm, port int) {
	port, gRPCServer, certs, secureDialOpts, dialOpts := util.CreateGRPCLayer()
	comm := newCommInstanceOnly(t, sec, gRPCServer, certs, secureDialOpts, dialOpts...)
	return comm, port
}

type msgMutator func(*protoext.SignedGossipMessage) *protoext.SignedGossipMessage

type tlsType int

const (
	none tlsType = iota
	oneWayTLS
	mutualTLS
)

func handshaker(port int, endpoint string, comm Comm, t *testing.T, connMutator msgMutator, connType tlsType) <-chan protoext.ReceivedMessage {
	c := &commImpl{}
	cert := GenerateCertificatesOrPanic()
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
	}
	if connType == mutualTLS {
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	ta := credentials.NewTLS(tlsCfg)
	secureOpts := grpc.WithTransportCredentials(ta)
	if connType == none {
		secureOpts = grpc.WithInsecure()
	}
	acceptChan := comm.Accept(acceptAll)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.DialContext(ctx, target, secureOpts, grpc.WithBlock())
	require.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	require.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}

	var clientCertHash []byte
	if len(tlsCfg.Certificates) > 0 {
		clientCertHash = certHashFromRawCert(tlsCfg.Certificates[0].Certificate[0])
	}

	pkiID := common.PKIidType(endpoint)
	require.NoError(t, err, "%v", err)
	msg, _ := c.createConnectionMsg(pkiID, clientCertHash, []byte(endpoint), func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	}, false)
	// Mutate connection message to test negative paths
	msg = connMutator(msg)
	// Send your own connection message
	stream.Send(msg.Envelope)
	// Wait for connection message from the other side
	envelope, err := stream.Recv()
	if err != nil {
		return acceptChan
	}
	require.NoError(t, err, "%v", err)
	msg, err = protoext.EnvelopeToGossipMessage(envelope)
	require.NoError(t, err, "%v", err)
	require.Equal(t, []byte(target), msg.GetConn().PkiId)
	require.Equal(t, extractCertificateHashFromContext(stream.Context()), msg.GetConn().TlsCertHash)
	msg2Send := createGossipMsg()
	nonce := uint64(rand.Int())
	msg2Send.Nonce = nonce
	go stream.Send(msg2Send.Envelope)
	return acceptChan
}

func TestMutualParallelSendWithAck(t *testing.T) {
	// This test tests concurrent and parallel sending of many (1000) messages
	// from 2 instances to one another at the same time.

	msgNum := 1000

	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()

	acceptData := func(o interface{}) bool {
		m := o.(protoext.ReceivedMessage).GetGossipMessage()
		return protoext.IsDataMsg(m.GossipMessage)
	}

	inc1 := comm1.Accept(acceptData)
	inc2 := comm2.Accept(acceptData)

	// Send a message from comm1 to comm2, to make the instances establish a preliminary connection
	comm1.Send(createGossipMsg(), remotePeer(port2))
	// Wait for the message to be received in comm2
	<-inc2

	for i := 0; i < msgNum; i++ {
		go comm1.SendWithAck(createGossipMsg(), time.Second*5, 1, remotePeer(port2))
	}

	for i := 0; i < msgNum; i++ {
		go comm2.SendWithAck(createGossipMsg(), time.Second*5, 1, remotePeer(port1))
	}

	go func() {
		for i := 0; i < msgNum; i++ {
			<-inc1
		}
	}()

	for i := 0; i < msgNum; i++ {
		<-inc2
	}
}

func getAvailablePort(t *testing.T) (port int, endpoint string, ll net.Listener) {
	ll, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint = ll.Addr().String()
	_, portS, err := net.SplitHostPort(endpoint)
	require.NoError(t, err)
	portInt, err := strconv.Atoi(portS)
	require.NoError(t, err)
	return portInt, endpoint, ll
}

func TestHandshake(t *testing.T) {
	signer := func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	}
	mutator := func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		return msg
	}
	assertPositivePath := func(msg protoext.ReceivedMessage, endpoint string) {
		expectedPKIID := common.PKIidType(endpoint)
		require.Equal(t, expectedPKIID, msg.GetConnectionInfo().ID)
		require.Equal(t, api.PeerIdentityType(endpoint), msg.GetConnectionInfo().Identity)
		require.NotNil(t, msg.GetConnectionInfo().Auth)
		sig, _ := (&naiveSecProvider{}).Sign(msg.GetConnectionInfo().Auth.SignedData)
		require.Equal(t, sig, msg.GetConnectionInfo().Auth.Signature)
	}

	// Positive path 1 - check authentication without TLS
	port, endpoint, ll := getAvailablePort(t)
	s := grpc.NewServer()
	id := []byte(endpoint)
	idMapper := identity.NewIdentityMapper(naiveSec, id, noopPurgeIdentity, naiveSec)
	inst, err := NewCommInstance(s, nil, idMapper, api.PeerIdentityType(endpoint), func() []grpc.DialOption {
		return []grpc.DialOption{grpc.WithInsecure()}
	}, naiveSec, disabledMetrics, testCommConfig)
	go s.Serve(ll)
	require.NoError(t, err)
	var msg protoext.ReceivedMessage

	_, tempEndpoint, tempL := getAvailablePort(t)
	acceptChan := handshaker(port, tempEndpoint, inst, t, mutator, none)
	select {
	case <-time.After(time.Duration(time.Second * 4)):
		require.FailNow(t, "Didn't receive a message, seems like handshake failed")
	case msg = <-acceptChan:
	}
	require.Equal(t, common.PKIidType(tempEndpoint), msg.GetConnectionInfo().ID)
	require.Equal(t, api.PeerIdentityType(tempEndpoint), msg.GetConnectionInfo().Identity)
	sig, _ := (&naiveSecProvider{}).Sign(msg.GetConnectionInfo().Auth.SignedData)
	require.Equal(t, sig, msg.GetConnectionInfo().Auth.Signature)

	inst.Stop()
	s.Stop()
	ll.Close()
	tempL.Close()
	time.Sleep(time.Second)

	comm, port := newCommInstance(t, naiveSec)
	defer comm.Stop()
	// Positive path 2: initiating peer sends its own certificate
	_, tempEndpoint, tempL = getAvailablePort(t)
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)

	select {
	case <-time.After(time.Second * 2):
		require.FailNow(t, "Didn't receive a message, seems like handshake failed")
	case msg = <-acceptChan:
	}
	assertPositivePath(msg, tempEndpoint)
	tempL.Close()

	// Negative path: initiating peer doesn't send its own certificate
	_, tempEndpoint, tempL = getAvailablePort(t)
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, oneWayTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()

	// Negative path, signature is wrong
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		msg.Signature = append(msg.Signature, 0)
		return msg
	}
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()

	// Negative path, the PKIid doesn't match the identity
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		msg.GetConn().PkiId = []byte(tempEndpoint)
		// Sign the message again
		msg.Sign(signer)
		return msg
	}
	_, tempEndpoint2, tempL2 := getAvailablePort(t)
	acceptChan = handshaker(port, tempEndpoint2, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()
	tempL2.Close()

	// Negative path, the cert hash isn't what is expected
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		msg.GetConn().TlsCertHash = append(msg.GetConn().TlsCertHash, 0)
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()

	// Negative path, no PKI-ID was sent
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		msg.GetConn().PkiId = nil
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()

	// Negative path, connection message is of a different type
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		msg.Content = &proto.GossipMessage_Empty{
			Empty: &proto.Empty{},
		}
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()

	// Negative path, the peer didn't respond to the handshake in due time
	_, tempEndpoint, tempL = getAvailablePort(t)
	mutator = func(msg *protoext.SignedGossipMessage) *protoext.SignedGossipMessage {
		time.Sleep(time.Second * 5)
		return msg
	}
	acceptChan = handshaker(port, tempEndpoint, comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	require.Equal(t, 0, len(acceptChan))
	tempL.Close()
}

func TestConnectUnexpectedPeer(t *testing.T) {
	// Scenarios: In both scenarios, comm1 connects to comm2 or comm3.
	// and expects to see a PKI-ID which is equal to comm4's PKI-ID.
	// The connection attempt would succeed or fail based on whether comm2 or comm3
	// are in the same org as comm4

	identityByPort := func(port int) api.PeerIdentityType {
		return api.PeerIdentityType(fmt.Sprintf("127.0.0.1:%d", port))
	}

	customNaiveSec := &naiveSecProvider{}

	comm1Port, gRPCServer1, certs1, secureDialOpts1, dialOpts1 := util.CreateGRPCLayer()
	comm2Port, gRPCServer2, certs2, secureDialOpts2, dialOpts2 := util.CreateGRPCLayer()
	comm3Port, gRPCServer3, certs3, secureDialOpts3, dialOpts3 := util.CreateGRPCLayer()
	comm4Port, gRPCServer4, certs4, secureDialOpts4, dialOpts4 := util.CreateGRPCLayer()

	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm1Port)).Return(api.OrgIdentityType("O"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm2Port)).Return(api.OrgIdentityType("A"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm3Port)).Return(api.OrgIdentityType("B"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm4Port)).Return(api.OrgIdentityType("A"))

	comm1 := newCommInstanceOnly(t, customNaiveSec, gRPCServer1, certs1, secureDialOpts1, dialOpts1...)
	comm2 := newCommInstanceOnly(t, naiveSec, gRPCServer2, certs2, secureDialOpts2, dialOpts2...)
	comm3 := newCommInstanceOnly(t, naiveSec, gRPCServer3, certs3, secureDialOpts3, dialOpts3...)
	comm4 := newCommInstanceOnly(t, naiveSec, gRPCServer4, certs4, secureDialOpts4, dialOpts4...)

	defer comm1.Stop()
	defer comm2.Stop()
	defer comm3.Stop()
	defer comm4.Stop()

	messagesForComm1 := comm1.Accept(acceptAll)
	messagesForComm2 := comm2.Accept(acceptAll)
	messagesForComm3 := comm3.Accept(acceptAll)

	// Have comm4 send a message to comm1
	// in order for comm1 to know comm4
	comm4.Send(createGossipMsg(), remotePeer(comm1Port))
	<-messagesForComm1
	// Close the connection with comm4
	comm1.CloseConn(remotePeer(comm4Port))
	// At this point, comm1 knows comm4's identity and organization

	t.Run("Same organization", func(t *testing.T) {
		unexpectedRemotePeer := remotePeer(comm2Port)
		unexpectedRemotePeer.PKIID = remotePeer(comm4Port).PKIID
		comm1.Send(createGossipMsg(), unexpectedRemotePeer)
		select {
		case <-messagesForComm2:
		case <-time.After(time.Second * 5):
			require.Fail(t, "Didn't receive a message within a timely manner")
			util.PrintStackTrace()
		}
	})

	t.Run("Unexpected organization", func(t *testing.T) {
		unexpectedRemotePeer := remotePeer(comm3Port)
		unexpectedRemotePeer.PKIID = remotePeer(comm4Port).PKIID
		comm1.Send(createGossipMsg(), unexpectedRemotePeer)
		select {
		case <-messagesForComm3:
			require.Fail(t, "Message shouldn't have been received")
		case <-time.After(time.Second * 5):
		}
	})
}

func TestGetConnectionInfo(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, _ := newCommInstance(t, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	comm2.Send(createGossipMsg(), remotePeer(port1))
	select {
	case <-time.After(time.Second * 10):
		t.Fatal("Didn't receive a message in time")
	case msg := <-m1:
		require.Equal(t, comm2.GetPKIid(), msg.GetConnectionInfo().ID)
		require.NotNil(t, msg.GetSourceEnvelope())
	}
}

func TestCloseConn(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	defer comm1.Stop()
	acceptChan := comm1.Accept(acceptAll)

	cert := GenerateCertificatesOrPanic()
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	}
	ta := credentials.NewTLS(tlsCfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target := fmt.Sprintf("127.0.0.1:%d", port1)
	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(ta), grpc.WithBlock())
	require.NoError(t, err, "%v", err)
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	require.NoError(t, err, "%v", err)
	c := &commImpl{}
	tlsCertHash := certHashFromRawCert(tlsCfg.Certificates[0].Certificate[0])
	connMsg, _ := c.createConnectionMsg(common.PKIidType("pkiID"), tlsCertHash, api.PeerIdentityType("pkiID"), func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	}, false)
	require.NoError(t, stream.Send(connMsg.Envelope))
	stream.Send(createGossipMsg().Envelope)
	select {
	case <-acceptChan:
	case <-time.After(time.Second):
		require.Fail(t, "Didn't receive a message within a timely period")
	}
	comm1.CloseConn(&RemotePeer{PKIID: common.PKIidType("pkiID")})
	time.Sleep(time.Second * 10)
	gotErr := false
	msg2Send := createGossipMsg()
	msg2Send.GetDataMsg().Payload = &proto.Payload{
		Data: make([]byte, 1024*1024),
	}
	protoext.NoopSign(msg2Send.GossipMessage)
	for i := 0; i < DefRecvBuffSize; i++ {
		err := stream.Send(msg2Send.Envelope)
		if err != nil {
			gotErr = true
			break
		}
	}
	require.True(t, gotErr, "Should have failed because connection is closed")
}

// TestCommSend makes sure that enough messages get through
// eventually. Comm.Send() is both asynchronous and best-effort, so this test
// case assumes some will fail, but that eventually enough messages will get
// through that the test will end.
func TestCommSend(t *testing.T) {
	sendMessages := func(c Comm, peer *RemotePeer, stopChan <-chan struct{}) {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			emptyMsg := createGossipMsg()
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				c.Send(emptyMsg, peer)
			}
		}
	}

	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()

	// Create the receive channel before sending the messages
	ch1 := comm1.Accept(acceptAll)
	ch2 := comm2.Accept(acceptAll)

	// control channels for background senders
	stopch1 := make(chan struct{})
	stopch2 := make(chan struct{})

	go sendMessages(comm1, remotePeer(port2), stopch1)
	go sendMessages(comm2, remotePeer(port1), stopch2)

	c1received := 0
	c2received := 0
	// hopefully in some runs we'll fill both send and receive buffers and
	// drop overflowing messages, but still finish, because the endless
	// stream of messages inexorably gets through unless something is very
	// broken.
	totalMessagesReceived := (DefSendBuffSize + DefRecvBuffSize) * 2
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
RECV:
	for {
		select {
		case <-ch1:
			c1received++
			if c1received == totalMessagesReceived {
				close(stopch2)
			}
		case <-ch2:
			c2received++
			if c2received == totalMessagesReceived {
				close(stopch1)
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for messages to be received.\nc1 got %d messages\nc2 got %d messages", c1received, c2received)
		default:
			if c1received >= totalMessagesReceived && c2received >= totalMessagesReceived {
				break RECV
			}
		}
	}
	t.Logf("c1 got %d messages\nc2 got %d messages", c1received, c2received)
}

type nonResponsivePeer struct {
	*grpc.Server
	port int
}

func newNonResponsivePeer(t *testing.T) *nonResponsivePeer {
	port, gRPCServer, _, _, _ := util.CreateGRPCLayer()
	nrp := &nonResponsivePeer{
		Server: gRPCServer.Server(),
		port:   port,
	}
	proto.RegisterGossipServer(gRPCServer.Server(), nrp)
	return nrp
}

func (bp *nonResponsivePeer) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	time.Sleep(time.Second * 15)
	return &proto.Empty{}, nil
}

func (bp *nonResponsivePeer) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	return nil
}

func (bp *nonResponsivePeer) stop() {
	bp.Server.Stop()
}

func TestNonResponsivePing(t *testing.T) {
	c, _ := newCommInstance(t, naiveSec)
	defer c.Stop()
	nonRespPeer := newNonResponsivePeer(t)
	defer nonRespPeer.stop()
	s := make(chan struct{})
	go func() {
		c.Probe(remotePeer(nonRespPeer.port))
		s <- struct{}{}
	}()
	select {
	case <-time.After(time.Second * 10):
		require.Fail(t, "Request wasn't cancelled on time")
	case <-s:
	}
}

func TestResponses(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, _ := newCommInstance(t, naiveSec)

	defer comm1.Stop()
	defer comm2.Stop()

	wg := sync.WaitGroup{}

	msg := createGossipMsg()
	wg.Add(1)
	go func() {
		inChan := comm1.Accept(acceptAll)
		wg.Done()
		for m := range inChan {
			reply := createGossipMsg()
			reply.Nonce = m.GetGossipMessage().Nonce + 1
			m.Respond(reply.GossipMessage)
		}
	}()
	expectedNOnce := uint64(msg.Nonce + 1)
	responsesFromComm1 := comm2.Accept(acceptAll)

	ticker := time.NewTicker(10 * time.Second)
	wg.Wait()
	comm2.Send(msg, remotePeer(port1))

	select {
	case <-ticker.C:
		require.Fail(t, "Haven't got response from comm1 within a timely manner")
		break
	case resp := <-responsesFromComm1:
		ticker.Stop()
		require.Equal(t, expectedNOnce, resp.GetGossipMessage().Nonce)
		break
	}
}

// TestAccept makes sure that accept filters work. The probability of the parity
// of all nonces being 0 or 1 is very low.
func TestAccept(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, _ := newCommInstance(t, naiveSec)

	evenNONCESelector := func(m interface{}) bool {
		return m.(protoext.ReceivedMessage).GetGossipMessage().Nonce%2 == 0
	}

	oddNONCESelector := func(m interface{}) bool {
		return m.(protoext.ReceivedMessage).GetGossipMessage().Nonce%2 != 0
	}

	evenNONCES := comm1.Accept(evenNONCESelector)
	oddNONCES := comm1.Accept(oddNONCESelector)

	var evenResults []uint64
	var oddResults []uint64

	out := make(chan uint64)
	sem := make(chan struct{})

	readIntoSlice := func(a *[]uint64, ch <-chan protoext.ReceivedMessage) {
		for m := range ch {
			*a = append(*a, m.GetGossipMessage().Nonce)
			select {
			case out <- m.GetGossipMessage().Nonce:
			default: // avoid blocking when we stop reading from out
			}
		}
		sem <- struct{}{}
	}

	go readIntoSlice(&evenResults, evenNONCES)
	go readIntoSlice(&oddResults, oddNONCES)

	stopSend := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopSend:
				return
			default:
				comm2.Send(createGossipMsg(), remotePeer(port1))
			}
		}
	}()

	waitForMessages(t, out, (DefSendBuffSize+DefRecvBuffSize)*2, "Didn't receive all messages sent")
	close(stopSend)

	comm1.Stop()
	comm2.Stop()

	<-sem
	<-sem

	t.Logf("%d even nonces received", len(evenResults))
	t.Logf("%d  odd nonces received", len(oddResults))

	require.NotEmpty(t, evenResults)
	require.NotEmpty(t, oddResults)

	remainderPredicate := func(a []uint64, rem uint64) {
		for _, n := range a {
			require.Equal(t, n%2, rem)
		}
	}

	remainderPredicate(evenResults, 0)
	remainderPredicate(oddResults, 1)
}

func TestReConnections(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)

	reader := func(out chan uint64, in <-chan protoext.ReceivedMessage) {
		for {
			msg := <-in
			if msg == nil {
				return
			}
			out <- msg.GetGossipMessage().Nonce
		}
	}

	out1 := make(chan uint64, 10)
	out2 := make(chan uint64, 10)

	go reader(out1, comm1.Accept(acceptAll))
	go reader(out2, comm2.Accept(acceptAll))

	// comm1 connects to comm2
	comm1.Send(createGossipMsg(), remotePeer(port2))
	waitForMessages(t, out2, 1, "Comm2 didn't receive a message from comm1 in a timely manner")
	// comm2 sends to comm1
	comm2.Send(createGossipMsg(), remotePeer(port1))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")
	comm1.Stop()

	comm1, port1 = newCommInstance(t, naiveSec)
	out1 = make(chan uint64, 1)
	go reader(out1, comm1.Accept(acceptAll))
	comm2.Send(createGossipMsg(), remotePeer(port1))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")
	comm1.Stop()
	comm2.Stop()
}

func TestProbe(t *testing.T) {
	comm1, port1 := newCommInstance(t, naiveSec)
	defer comm1.Stop()
	comm2, port2 := newCommInstance(t, naiveSec)
	time.Sleep(time.Duration(1) * time.Second)
	require.NoError(t, comm1.Probe(remotePeer(port2)))
	_, err := comm1.Handshake(remotePeer(port2))
	require.NoError(t, err)
	tempPort, _, ll := getAvailablePort(t)
	defer ll.Close()
	require.Error(t, comm1.Probe(remotePeer(tempPort)))
	_, err = comm1.Handshake(remotePeer(tempPort))
	require.Error(t, err)
	comm2.Stop()
	time.Sleep(time.Duration(1) * time.Second)
	require.Error(t, comm1.Probe(remotePeer(port2)))
	_, err = comm1.Handshake(remotePeer(port2))
	require.Error(t, err)
	comm2, port2 = newCommInstance(t, naiveSec)
	defer comm2.Stop()
	time.Sleep(time.Duration(1) * time.Second)
	require.NoError(t, comm2.Probe(remotePeer(port1)))
	_, err = comm2.Handshake(remotePeer(port1))
	require.NoError(t, err)
	require.NoError(t, comm1.Probe(remotePeer(port2)))
	_, err = comm1.Handshake(remotePeer(port2))
	require.NoError(t, err)
	// Now try a deep probe with an expected PKI-ID that doesn't match
	wrongRemotePeer := remotePeer(port2)
	if wrongRemotePeer.PKIID[0] == 0 {
		wrongRemotePeer.PKIID[0] = 1
	} else {
		wrongRemotePeer.PKIID[0] = 0
	}
	_, err = comm1.Handshake(wrongRemotePeer)
	require.Error(t, err)
	// Try a deep probe with a nil PKI-ID
	endpoint := fmt.Sprintf("127.0.0.1:%d", port2)
	id, err := comm1.Handshake(&RemotePeer{Endpoint: endpoint})
	require.NoError(t, err)
	require.Equal(t, api.PeerIdentityType(endpoint), id)
}

func TestPresumedDead(t *testing.T) {
	comm1, _ := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Wait()
		comm1.Send(createGossipMsg(), remotePeer(port2))
	}()

	ticker := time.NewTicker(time.Duration(10) * time.Second)
	acceptCh := comm2.Accept(acceptAll)
	wg.Done()
	select {
	case <-acceptCh:
		ticker.Stop()
	case <-ticker.C:
		require.Fail(t, "Didn't get first message")
	}

	comm2.Stop()
	go func() {
		for i := 0; i < 5; i++ {
			comm1.Send(createGossipMsg(), remotePeer(port2))
			time.Sleep(time.Millisecond * 200)
		}
	}()

	ticker = time.NewTicker(time.Second * time.Duration(3))
	select {
	case <-ticker.C:
		require.Fail(t, "Didn't get a presumed dead message within a timely manner")
		break
	case <-comm1.PresumedDead():
		ticker.Stop()
		break
	}
}

func TestReadFromStream(t *testing.T) {
	stream := &gmocks.MockStream{}
	stream.On("CloseSend").Return(nil)
	stream.On("Recv").Return(&proto.Envelope{Payload: []byte{1}}, nil).Once()
	stream.On("Recv").Return(nil, errors.New("stream closed")).Once()

	conn := newConnection(nil, nil, stream, disabledMetrics, ConnConfig{1, 1})
	conn.logger = flogging.MustGetLogger("test")

	errChan := make(chan error, 2)
	msgChan := make(chan *protoext.SignedGossipMessage, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn.readFromStream(errChan, msgChan)
	}()

	select {
	case <-msgChan:
		require.Fail(t, "malformed message shouldn't have been received")
	case <-time.After(time.Millisecond * 100):
		require.Len(t, errChan, 1)
	}

	conn.close()
	wg.Wait()
}

func TestSendBadEnvelope(t *testing.T) {
	comm1, port := newCommInstance(t, naiveSec)
	defer comm1.Stop()

	stream, err := establishSession(t, port)
	require.NoError(t, err)

	inc := comm1.Accept(acceptAll)

	goodMsg := createGossipMsg()
	err = stream.Send(goodMsg.Envelope)
	require.NoError(t, err)

	select {
	case goodMsgReceived := <-inc:
		require.Equal(t, goodMsg.Envelope.Payload, goodMsgReceived.GetSourceEnvelope().Payload)
	case <-time.After(time.Minute):
		require.Fail(t, "Didn't receive message within a timely manner")
		return
	}

	// Next, we corrupt a message and send it until the stream is closed forcefully from the remote peer
	start := time.Now()
	for {
		badMsg := createGossipMsg()
		badMsg.Envelope.Payload = []byte{1}
		err = stream.Send(badMsg.Envelope)
		if err != nil {
			require.Equal(t, io.EOF, err)
			break
		}
		if time.Now().After(start.Add(time.Second * 30)) {
			require.Fail(t, "Didn't close stream within a timely manner")
			return
		}
	}
}

func establishSession(t *testing.T, port int) (proto.Gossip_GossipStreamClient, error) {
	cert := GenerateCertificatesOrPanic()
	secureOpts := grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.DialContext(ctx, endpoint, secureOpts, grpc.WithBlock())
	require.NoError(t, err, "%v", err)
	if err != nil {
		return nil, err
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	require.NoError(t, err, "%v", err)
	if err != nil {
		return nil, err
	}

	clientCertHash := certHashFromRawCert(cert.Certificate[0])
	pkiID := common.PKIidType([]byte{1, 2, 3})
	c := &commImpl{}
	require.NoError(t, err, "%v", err)
	msg, _ := c.createConnectionMsg(pkiID, clientCertHash, []byte{1, 2, 3}, func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	}, false)
	// Send your own connection message
	stream.Send(msg.Envelope)
	// Wait for connection message from the other side
	envelope, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	require.NotNil(t, envelope)
	return stream, nil
}

func createGossipMsg() *protoext.SignedGossipMessage {
	msg, _ := protoext.NoopSign(&proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: uint64(rand.Int()),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{},
		},
	})
	return msg
}

func remotePeer(port int) *RemotePeer {
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	return &RemotePeer{Endpoint: endpoint, PKIID: []byte(endpoint)}
}

func waitForMessages(t *testing.T, msgChan chan uint64, count int, errMsg string) {
	c := 0
	waiting := true
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	for waiting {
		select {
		case <-msgChan:
			c++
			if c == count {
				waiting = false
			}
		case <-ticker.C:
			waiting = false
		}
	}
	require.Equal(t, count, c, errMsg)
}

func TestConcurrentCloseSend(t *testing.T) {
	var stopping int32

	comm1, _ := newCommInstance(t, naiveSec)
	comm2, port2 := newCommInstance(t, naiveSec)
	m := comm2.Accept(acceptAll)
	comm1.Send(createGossipMsg(), remotePeer(port2))
	<-m
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)

		comm1.Send(createGossipMsg(), remotePeer(port2))
		close(ready)

		for atomic.LoadInt32(&stopping) == int32(0) {
			comm1.Send(createGossipMsg(), remotePeer(port2))
		}
	}()
	<-ready
	comm2.Stop()
	atomic.StoreInt32(&stopping, int32(1))
	<-done
}
