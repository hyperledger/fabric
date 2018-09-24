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
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	util.SetupTestLogging()
	rand.Seed(time.Now().UnixNano())
	factory.InitFactories(nil)
	naiveSec.On("OrgByPeerIdentity", mock.Anything).Return(api.OrgIdentityType{})
}

func acceptAll(msg interface{}) bool {
	return true
}

var noopPurgeIdentity = func(_ common.PKIidType, _ api.PeerIdentityType) {

}

var (
	naiveSec = &naiveSecProvider{}
	hmacKey  = []byte{0, 0, 0}
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
func (*naiveSecProvider) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
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
func (*naiveSecProvider) VerifyByChannel(_ common.ChainID, _ api.PeerIdentityType, _, _ []byte) error {
	return nil
}

func newCommInstance(port int, sec *naiveSecProvider) (Comm, error) {
	endpoint := fmt.Sprintf("localhost:%d", port)
	id := []byte(endpoint)
	inst, err := NewCommInstanceWithServer(port, identity.NewIdentityMapper(sec, id, noopPurgeIdentity, sec), id, nil, sec)
	return inst, err
}

type msgMutator func(*proto.SignedGossipMessage) *proto.SignedGossipMessage

type tlsType int

const (
	none tlsType = iota
	oneWayTLS
	mutualTLS
)

func handshaker(endpoint string, comm Comm, t *testing.T, connMutator msgMutator, connType tlsType) <-chan proto.ReceivedMessage {
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
	conn, err := grpc.DialContext(ctx, "localhost:9611", secureOpts, grpc.WithBlock())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	if err != nil {
		return nil
	}

	var clientCertHash []byte
	if len(tlsCfg.Certificates) > 0 {
		clientCertHash = certHashFromRawCert(tlsCfg.Certificates[0].Certificate[0])
	}

	pkiID := common.PKIidType(endpoint)
	assert.NoError(t, err, "%v", err)
	msg, _ := c.createConnectionMsg(pkiID, clientCertHash, []byte(endpoint), func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	})
	// Mutate connection message to test negative paths
	msg = connMutator(msg)
	// Send your own connection message
	stream.Send(msg.Envelope)
	// Wait for connection message from the other side
	envelope, err := stream.Recv()
	if err != nil {
		return acceptChan
	}
	assert.NoError(t, err, "%v", err)
	msg, err = envelope.ToGossipMessage()
	assert.NoError(t, err, "%v", err)
	assert.Equal(t, []byte("localhost:9611"), msg.GetConn().PkiId)
	assert.Equal(t, extractCertificateHashFromContext(stream.Context()), msg.GetConn().TlsCertHash)
	msg2Send := createGossipMsg()
	nonce := uint64(rand.Int())
	msg2Send.Nonce = nonce
	go stream.Send(msg2Send.Envelope)
	return acceptChan
}

func TestViperConfig(t *testing.T) {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %s", err))
	}

	assert.Equal(t, time.Duration(2)*time.Second, util.GetDurationOrDefault("peer.gossip.connTimeout", 0))
	assert.Equal(t, time.Duration(300)*time.Millisecond, util.GetDurationOrDefault("peer.gossip.dialTimeout", 0))
	assert.Equal(t, 20, util.GetIntOrDefault("peer.gossip.recvBuffSize", 0))
	assert.Equal(t, 200, util.GetIntOrDefault("peer.gossip.sendBuffSize", 0))
}

func TestMutualParallelSendWithAck(t *testing.T) {
	t.Parallel()

	// This test tests concurrent and parallel sending of many (1000) messages
	// from 2 instances to one another at the same time.

	msgNum := 1000

	comm1, _ := newCommInstance(15201, naiveSec)
	comm2, _ := newCommInstance(15202, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()

	acceptData := func(o interface{}) bool {
		return o.(proto.ReceivedMessage).GetGossipMessage().IsDataMsg()
	}

	inc1 := comm1.Accept(acceptData)
	inc2 := comm2.Accept(acceptData)

	// Send a message from comm1 to comm2, to make the instances establish a preliminary connection
	comm1.Send(createGossipMsg(), remotePeer(15202))
	// Wait for the message to be received in comm2
	<-inc2

	for i := 0; i < msgNum; i++ {
		go comm1.SendWithAck(createGossipMsg(), time.Second*5, 1, remotePeer(15202))
	}

	for i := 0; i < msgNum; i++ {
		go comm2.SendWithAck(createGossipMsg(), time.Second*5, 1, remotePeer(15201))
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

func TestHandshake(t *testing.T) {
	t.Parallel()
	signer := func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	}
	mutator := func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		return msg
	}
	assertPositivePath := func(msg proto.ReceivedMessage, endpoint string) {
		expectedPKIID := common.PKIidType(endpoint)
		assert.Equal(t, expectedPKIID, msg.GetConnectionInfo().ID)
		assert.Equal(t, api.PeerIdentityType(endpoint), msg.GetConnectionInfo().Identity)
		assert.NotNil(t, msg.GetConnectionInfo().Auth)
		sig, _ := (&naiveSecProvider{}).Sign(msg.GetConnectionInfo().Auth.SignedData)
		assert.Equal(t, sig, msg.GetConnectionInfo().Auth.Signature)
	}

	// Positive path 1 - check authentication without TLS
	ll, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 9611))
	assert.NoError(t, err)
	s := grpc.NewServer()
	id := []byte("localhost:9611")
	idMapper := identity.NewIdentityMapper(naiveSec, id, noopPurgeIdentity, naiveSec)
	inst, err := NewCommInstance(s, nil, idMapper, api.PeerIdentityType("localhost:9611"), func() []grpc.DialOption {
		return []grpc.DialOption{grpc.WithInsecure()}
	}, naiveSec)
	go s.Serve(ll)
	assert.NoError(t, err)
	var msg proto.ReceivedMessage

	acceptChan := handshaker("localhost:9608", inst, t, mutator, none)
	select {
	case <-time.After(time.Duration(time.Second * 4)):
		assert.FailNow(t, "Didn't receive a message, seems like handshake failed")
	case msg = <-acceptChan:
	}
	assert.Equal(t, common.PKIidType("localhost:9608"), msg.GetConnectionInfo().ID)
	assert.Equal(t, api.PeerIdentityType("localhost:9608"), msg.GetConnectionInfo().Identity)
	sig, _ := (&naiveSecProvider{}).Sign(msg.GetConnectionInfo().Auth.SignedData)
	assert.Equal(t, sig, msg.GetConnectionInfo().Auth.Signature)

	inst.Stop()
	s.Stop()
	ll.Close()
	time.Sleep(time.Second)

	comm, err := newCommInstance(9611, naiveSec)
	assert.NoError(t, err)
	defer comm.Stop()
	// Positive path 2: initiating peer sends its own certificate
	acceptChan = handshaker("localhost:9609", comm, t, mutator, mutualTLS)

	select {
	case <-time.After(time.Second * 2):
		assert.FailNow(t, "Didn't receive a message, seems like handshake failed")
	case msg = <-acceptChan:
	}
	assertPositivePath(msg, "localhost:9609")

	// Negative path: initiating peer doesn't send its own certificate
	acceptChan = handshaker("localhost:9610", comm, t, mutator, oneWayTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))
	// Negative path, signature is wrong
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		msg.Signature = append(msg.Signature, 0)
		return msg
	}
	acceptChan = handshaker("localhost:9612", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// Negative path, the PKIid doesn't match the identity
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		msg.GetConn().PkiId = []byte("localhost:9650")
		// Sign the message again
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker("localhost:9613", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// Negative path, the cert hash isn't what is expected
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		msg.GetConn().TlsCertHash = append(msg.GetConn().TlsCertHash, 0)
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker("localhost:9615", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// Negative path, no PKI-ID was sent
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		msg.GetConn().PkiId = nil
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker("localhost:9616", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// Negative path, connection message is of a different type
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		msg.Content = &proto.GossipMessage_Empty{
			Empty: &proto.Empty{},
		}
		msg.Sign(signer)
		return msg
	}
	acceptChan = handshaker("localhost:9617", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))

	// Negative path, the peer didn't respond to the handshake in due time
	mutator = func(msg *proto.SignedGossipMessage) *proto.SignedGossipMessage {
		time.Sleep(time.Second * 5)
		return msg
	}
	acceptChan = handshaker("localhost:9618", comm, t, mutator, mutualTLS)
	time.Sleep(time.Second)
	assert.Equal(t, 0, len(acceptChan))
}

func TestBasic(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(2000, naiveSec)
	comm2, _ := newCommInstance(3000, naiveSec)
	comm1.(*commImpl).SetDialOpts()
	comm2.(*commImpl).SetDialOpts()
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	m2 := comm2.Accept(acceptAll)
	out := make(chan uint64, 2)
	reader := func(ch <-chan proto.ReceivedMessage) {
		m := <-ch
		out <- m.GetGossipMessage().Nonce
	}
	go reader(m1)
	go reader(m2)
	comm1.Send(createGossipMsg(), remotePeer(3000))
	time.Sleep(time.Second)
	comm2.Send(createGossipMsg(), remotePeer(2000))
	waitForMessages(t, out, 2, "Didn't receive 2 messages")
}

func TestConnectUnexpectedPeer(t *testing.T) {
	t.Parallel()
	// Scenarios: In both scenarios, comm1 connects to comm2 or comm3.
	// and expects to see a PKI-ID which is equal to comm4's PKI-ID.
	// The connection attempt would succeed or fail based on whether comm2 or comm3
	// are in the same org as comm4
	comm1Port := 1548
	comm2Port := 1549
	comm3Port := 1550
	comm4Port := 1551

	identityByPort := func(port int) api.PeerIdentityType {
		return api.PeerIdentityType(fmt.Sprintf("localhost:%d", port))
	}

	customNaiveSec := &naiveSecProvider{}
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm1Port)).Return(api.OrgIdentityType("O"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm2Port)).Return(api.OrgIdentityType("A"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm3Port)).Return(api.OrgIdentityType("B"))
	customNaiveSec.On("OrgByPeerIdentity", identityByPort(comm4Port)).Return(api.OrgIdentityType("A"))

	comm1, _ := newCommInstance(comm1Port, customNaiveSec)
	comm2, _ := newCommInstance(comm2Port, naiveSec)
	comm3, _ := newCommInstance(comm3Port, naiveSec)
	comm4, _ := newCommInstance(comm4Port, naiveSec)

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
			assert.Fail(t, "Didn't receive a message within a timely manner")
			util.PrintStackTrace()
		}
	})

	t.Run("Unexpected organization", func(t *testing.T) {
		unexpectedRemotePeer := remotePeer(comm3Port)
		unexpectedRemotePeer.PKIID = remotePeer(comm4Port).PKIID
		comm1.Send(createGossipMsg(), unexpectedRemotePeer)
		select {
		case <-messagesForComm3:
			assert.Fail(t, "Message shouldn't have been received")
		case <-time.After(time.Second * 5):
		}
	})
}

func TestProdConstructor(t *testing.T) {
	t.Parallel()
	srv, lsnr, dialOpts, certs := createGRPCLayer(20000)
	defer srv.Stop()
	defer lsnr.Close()
	id := []byte("localhost:20000")
	comm1, _ := NewCommInstance(srv, certs, identity.NewIdentityMapper(naiveSec, id, noopPurgeIdentity, naiveSec), id, dialOpts, naiveSec)
	go srv.Serve(lsnr)

	srv, lsnr, dialOpts, certs = createGRPCLayer(30000)
	defer srv.Stop()
	defer lsnr.Close()
	id = []byte("localhost:30000")
	comm2, _ := NewCommInstance(srv, certs, identity.NewIdentityMapper(naiveSec, id, noopPurgeIdentity, naiveSec), id, dialOpts, naiveSec)
	go srv.Serve(lsnr)
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	m2 := comm2.Accept(acceptAll)
	out := make(chan uint64, 2)
	reader := func(ch <-chan proto.ReceivedMessage) {
		m := <-ch
		out <- m.GetGossipMessage().Nonce
	}
	go reader(m1)
	go reader(m2)
	comm1.Send(createGossipMsg(), remotePeer(30000))
	time.Sleep(time.Second)
	comm2.Send(createGossipMsg(), remotePeer(20000))
	waitForMessages(t, out, 2, "Didn't receive 2 messages")
}

func TestGetConnectionInfo(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(6000, naiveSec)
	comm2, _ := newCommInstance(7000, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()
	m1 := comm1.Accept(acceptAll)
	comm2.Send(createGossipMsg(), remotePeer(6000))
	select {
	case <-time.After(time.Second * 10):
		t.Fatal("Didn't receive a message in time")
	case msg := <-m1:
		assert.Equal(t, comm2.GetPKIid(), msg.GetConnectionInfo().ID)
		assert.NotNil(t, msg.GetSourceEnvelope())
	}
}

func TestCloseConn(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(1611, naiveSec)
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
	conn, err := grpc.DialContext(ctx, "localhost:1611", grpc.WithTransportCredentials(ta), grpc.WithBlock())
	assert.NoError(t, err, "%v", err)
	cl := proto.NewGossipClient(conn)
	stream, err := cl.GossipStream(context.Background())
	assert.NoError(t, err, "%v", err)
	c := &commImpl{}
	tlsCertHash := certHashFromRawCert(tlsCfg.Certificates[0].Certificate[0])
	connMsg, _ := c.createConnectionMsg(common.PKIidType("pkiID"), tlsCertHash, api.PeerIdentityType("pkiID"), func(msg []byte) ([]byte, error) {
		mac := hmac.New(sha256.New, hmacKey)
		mac.Write(msg)
		return mac.Sum(nil), nil
	})
	assert.NoError(t, stream.Send(connMsg.Envelope))
	stream.Send(createGossipMsg().Envelope)
	select {
	case <-acceptChan:
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't receive a message within a timely period")
	}
	comm1.CloseConn(&RemotePeer{PKIID: common.PKIidType("pkiID")})
	time.Sleep(time.Second * 10)
	gotErr := false
	msg2Send := createGossipMsg()
	msg2Send.GetDataMsg().Payload = &proto.Payload{
		Data: make([]byte, 1024*1024),
	}
	msg2Send.NoopSign()
	for i := 0; i < defRecvBuffSize; i++ {
		err := stream.Send(msg2Send.Envelope)
		if err != nil {
			gotErr = true
			break
		}
	}
	assert.True(t, gotErr, "Should have failed because connection is closed")
}

func TestParallelSend(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(5411, naiveSec)
	comm2, _ := newCommInstance(5412, naiveSec)
	defer comm1.Stop()
	defer comm2.Stop()

	messages2Send := util.GetIntOrDefault("peer.gossip.recvBuffSize", defRecvBuffSize)

	wg := sync.WaitGroup{}
	go func() {
		for i := 0; i < messages2Send; i++ {
			wg.Add(1)
			emptyMsg := createGossipMsg()
			go func() {
				defer wg.Done()
				comm1.Send(emptyMsg, remotePeer(5412))
			}()
		}
		wg.Wait()
	}()

	c := 0
	waiting := true
	ticker := time.NewTicker(time.Duration(5) * time.Second)
	ch := comm2.Accept(acceptAll)
	for waiting {
		select {
		case <-ch:
			c++
			if c == messages2Send {
				waiting = false
			}
		case <-ticker.C:
			waiting = false
		}
	}
	assert.Equal(t, messages2Send, c)
}

type nonResponsivePeer struct {
	net.Listener
	*grpc.Server
	port int
}

func newNonResponsivePeer() *nonResponsivePeer {
	rand.Seed(time.Now().UnixNano())
	port := 50000 + rand.Intn(1000)
	s, l, _, _ := createGRPCLayer(port)
	nrp := &nonResponsivePeer{
		Listener: l,
		Server:   s,
		port:     port,
	}
	proto.RegisterGossipServer(s, nrp)
	go s.Serve(l)
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
	bp.Listener.Close()
}

func TestNonResponsivePing(t *testing.T) {
	t.Parallel()
	port := 50000 - rand.Intn(1000)
	c, _ := newCommInstance(port, naiveSec)
	defer c.Stop()
	nonRespPeer := newNonResponsivePeer()
	defer nonRespPeer.stop()
	s := make(chan struct{})
	go func() {
		c.Probe(remotePeer(nonRespPeer.port))
		s <- struct{}{}
	}()
	select {
	case <-time.After(time.Second * 10):
		assert.Fail(t, "Request wasn't cancelled on time")
	case <-s:
	}

}

func TestResponses(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(8611, naiveSec)
	comm2, _ := newCommInstance(8612, naiveSec)

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
	comm2.Send(msg, remotePeer(8611))

	select {
	case <-ticker.C:
		assert.Fail(t, "Haven't got response from comm1 within a timely manner")
		break
	case resp := <-responsesFromComm1:
		ticker.Stop()
		assert.Equal(t, expectedNOnce, resp.GetGossipMessage().Nonce)
		break
	}
}

func TestAccept(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(7611, naiveSec)
	comm2, _ := newCommInstance(7612, naiveSec)

	evenNONCESelector := func(m interface{}) bool {
		return m.(proto.ReceivedMessage).GetGossipMessage().Nonce%2 == 0
	}

	oddNONCESelector := func(m interface{}) bool {
		return m.(proto.ReceivedMessage).GetGossipMessage().Nonce%2 != 0
	}

	evenNONCES := comm1.Accept(evenNONCESelector)
	oddNONCES := comm1.Accept(oddNONCESelector)

	var evenResults []uint64
	var oddResults []uint64

	out := make(chan uint64, util.GetIntOrDefault("peer.gossip.recvBuffSize", defRecvBuffSize))
	sem := make(chan struct{}, 0)

	readIntoSlice := func(a *[]uint64, ch <-chan proto.ReceivedMessage) {
		for m := range ch {
			*a = append(*a, m.GetGossipMessage().Nonce)
			out <- m.GetGossipMessage().Nonce
		}
		sem <- struct{}{}
	}

	go readIntoSlice(&evenResults, evenNONCES)
	go readIntoSlice(&oddResults, oddNONCES)

	for i := 0; i < util.GetIntOrDefault("peer.gossip.recvBuffSize", defRecvBuffSize); i++ {
		comm2.Send(createGossipMsg(), remotePeer(7611))
	}

	waitForMessages(t, out, util.GetIntOrDefault("peer.gossip.recvBuffSize", defRecvBuffSize), "Didn't receive all messages sent")

	comm1.Stop()
	comm2.Stop()

	<-sem
	<-sem

	assert.NotEmpty(t, evenResults)
	assert.NotEmpty(t, oddResults)

	remainderPredicate := func(a []uint64, rem uint64) {
		for _, n := range a {
			assert.Equal(t, n%2, rem)
		}
	}

	remainderPredicate(evenResults, 0)
	remainderPredicate(oddResults, 1)
}

func TestReConnections(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(3611, naiveSec)
	comm2, _ := newCommInstance(3612, naiveSec)

	reader := func(out chan uint64, in <-chan proto.ReceivedMessage) {
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
	comm1.Send(createGossipMsg(), remotePeer(3612))
	waitForMessages(t, out2, 1, "Comm2 didn't receive a message from comm1 in a timely manner")
	time.Sleep(time.Second)
	// comm2 sends to comm1
	comm2.Send(createGossipMsg(), remotePeer(3611))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")

	comm1.Stop()
	comm1, _ = newCommInstance(3611, naiveSec)
	time.Sleep(time.Second)
	out1 = make(chan uint64, 1)
	go reader(out1, comm1.Accept(acceptAll))
	comm2.Send(createGossipMsg(), remotePeer(3611))
	waitForMessages(t, out1, 1, "Comm1 didn't receive a message from comm2 in a timely manner")
}

func TestProbe(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(6611, naiveSec)
	defer comm1.Stop()
	comm2, _ := newCommInstance(6612, naiveSec)
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm1.Probe(remotePeer(6612)))
	_, err := comm1.Handshake(remotePeer(6612))
	assert.NoError(t, err)
	assert.Error(t, comm1.Probe(remotePeer(9012)))
	_, err = comm1.Handshake(remotePeer(9012))
	assert.Error(t, err)
	comm2.Stop()
	time.Sleep(time.Second)
	assert.Error(t, comm1.Probe(remotePeer(6612)))
	_, err = comm1.Handshake(remotePeer(6612))
	assert.Error(t, err)
	comm2, _ = newCommInstance(6612, naiveSec)
	defer comm2.Stop()
	time.Sleep(time.Duration(1) * time.Second)
	assert.NoError(t, comm2.Probe(remotePeer(6611)))
	_, err = comm2.Handshake(remotePeer(6611))
	assert.NoError(t, err)
	assert.NoError(t, comm1.Probe(remotePeer(6612)))
	_, err = comm1.Handshake(remotePeer(6612))
	assert.NoError(t, err)
	// Now try a deep probe with an expected PKI-ID that doesn't match
	wrongRemotePeer := remotePeer(6612)
	if wrongRemotePeer.PKIID[0] == 0 {
		wrongRemotePeer.PKIID[0] = 1
	} else {
		wrongRemotePeer.PKIID[0] = 0
	}
	_, err = comm1.Handshake(wrongRemotePeer)
	assert.Error(t, err)
	// Try a deep probe with a nil PKI-ID
	id, err := comm1.Handshake(&RemotePeer{Endpoint: "localhost:6612"})
	assert.NoError(t, err)
	assert.Equal(t, api.PeerIdentityType("localhost:6612"), id)
}

func TestPresumedDead(t *testing.T) {
	t.Parallel()
	comm1, _ := newCommInstance(4611, naiveSec)
	comm2, _ := newCommInstance(4612, naiveSec)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Wait()
		comm1.Send(createGossipMsg(), remotePeer(4612))
	}()

	ticker := time.NewTicker(time.Duration(10) * time.Second)
	acceptCh := comm2.Accept(acceptAll)
	wg.Done()
	select {
	case <-acceptCh:
		ticker.Stop()
	case <-ticker.C:
		assert.Fail(t, "Didn't get first message")
	}

	comm2.Stop()
	go func() {
		for i := 0; i < 5; i++ {
			comm1.Send(createGossipMsg(), remotePeer(4612))
			time.Sleep(time.Millisecond * 200)
		}
	}()

	ticker = time.NewTicker(time.Second * time.Duration(3))
	select {
	case <-ticker.C:
		assert.Fail(t, "Didn't get a presumed dead message within a timely manner")
		break
	case <-comm1.PresumedDead():
		ticker.Stop()
		break
	}
}

func createGossipMsg() *proto.SignedGossipMessage {
	msg, _ := (&proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: uint64(rand.Int()),
		Content: &proto.GossipMessage_DataMsg{
			DataMsg: &proto.DataMessage{},
		},
	}).NoopSign()
	return msg
}

func remotePeer(port int) *RemotePeer {
	endpoint := fmt.Sprintf("localhost:%d", port)
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
	assert.Equal(t, count, c, errMsg)
}

func TestMain(m *testing.M) {
	SetDialTimeout(time.Duration(300) * time.Millisecond)

	ret := m.Run()
	os.Exit(ret)
}

func TestConcurrentCloseSend(t *testing.T) {
	t.Parallel()
	var stopping int32

	comm1, _ := newCommInstance(1111, naiveSec)
	comm2, _ := newCommInstance(2222, naiveSec)
	m := comm2.Accept(acceptAll)
	comm1.Send(createGossipMsg(), remotePeer(2222))
	<-m
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)

		comm1.Send(createGossipMsg(), remotePeer(2222))
		close(ready)

		for atomic.LoadInt32(&stopping) == int32(0) {
			comm1.Send(createGossipMsg(), remotePeer(2222))
		}
	}()
	<-ready
	comm2.Stop()
	atomic.StoreInt32(&stopping, int32(1))
	<-done
}
