/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package gossip

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type peerMock struct {
	pkiID                common.PKIidType
	selfCertHash         []byte
	gRGCserv             *grpc.Server
	lsnr                 net.Listener
	finishedSignal       sync.WaitGroup
	expectedMsgs2Receive uint32
	msgReceivedCount     uint32
	msgAssertions        []msgInspection
	t                    *testing.T
}

func (p *peerMock) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	sessionCounter := 0
	for {
		envelope, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		gMsg, err := envelope.ToGossipMessage()
		if err != nil {
			panic(err)
		}
		if sessionCounter == 0 {
			connEstablishMsg := p.connEstablishMsg(p.pkiID, p.selfCertHash, api.PeerIdentityType(p.pkiID))
			stream.Send(connEstablishMsg.Envelope)
		}
		for _, assertion := range p.msgAssertions {
			assertion(p.t, sessionCounter, &receivedMsg{stream: stream, SignedGossipMessage: gMsg})
		}
		p.t.Log("sessionCounter:", sessionCounter, string(p.pkiID), "got msg:", gMsg)
		sessionCounter++
		atomic.AddUint32(&p.msgReceivedCount, uint32(1))
		if atomic.LoadUint32(&p.msgReceivedCount) == p.expectedMsgs2Receive {
			p.finishedSignal.Done()
		}
	}
}

func (p *peerMock) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func newPeerMock(port int, expectedMsgs2Receive int, t *testing.T, msgAssertions ...msgInspection) *peerMock {
	listenAddress := fmt.Sprintf(":%d", port)
	ll, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Printf("Error listening on %v, %v", listenAddress, err)
	}
	s, selfCertHash := newGRPCServerWithTLS()
	p := &peerMock{
		lsnr:                 ll,
		gRGCserv:             s,
		msgAssertions:        msgAssertions,
		t:                    t,
		pkiID:                common.PKIidType(fmt.Sprintf("localhost:%d", port)),
		selfCertHash:         selfCertHash,
		expectedMsgs2Receive: uint32(expectedMsgs2Receive),
	}
	p.finishedSignal.Add(1)
	proto.RegisterGossipServer(s, p)
	go s.Serve(ll)
	return p
}

func newGRPCServerWithTLS() (*grpc.Server, []byte) {
	cert := comm.GenerateCertificatesOrPanic()
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
	return s, util.ComputeSHA256(cert.Certificate[0])
}

func (p *peerMock) connEstablishMsg(pkiID common.PKIidType, hash []byte, cert api.PeerIdentityType) *proto.SignedGossipMessage {
	m := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Conn{
			Conn: &proto.ConnEstablish{
				TlsCertHash: hash,
				Identity:    cert,
				PkiId:       pkiID,
			},
		},
	}
	gMsg := &proto.SignedGossipMessage{
		GossipMessage: m,
	}
	gMsg.Sign((&configurableCryptoService{}).Sign)
	return gMsg
}

func (p *peerMock) stop() {
	p.lsnr.Close()
	p.gRGCserv.Stop()
}

type receivedMsg struct {
	*proto.SignedGossipMessage
	stream proto.Gossip_GossipStreamServer
}

func (msg *receivedMsg) respond(message *proto.SignedGossipMessage) {
	msg.stream.Send(message.Envelope)
}

func memResp(nonce uint64, endpoint string) *proto.SignedGossipMessage {
	fakePeerAliveMsg := &proto.SignedGossipMessage{
		GossipMessage: &proto.GossipMessage{
			Tag: proto.GossipMessage_EMPTY,
			Content: &proto.GossipMessage_AliveMsg{
				AliveMsg: &proto.AliveMessage{
					Membership: &proto.Member{
						Endpoint: endpoint,
						PkiId:    []byte(endpoint),
					},
					Identity: []byte(endpoint),
					Timestamp: &proto.PeerTime{
						IncNum: uint64(time.Now().UnixNano()),
						SeqNum: 0,
					},
				},
			},
		},
	}

	m, _ := fakePeerAliveMsg.Sign((&configurableCryptoService{}).Sign)
	sMsg, _ := (&proto.SignedGossipMessage{
		GossipMessage: &proto.GossipMessage{
			Tag:   proto.GossipMessage_EMPTY,
			Nonce: nonce,
			Content: &proto.GossipMessage_MemRes{
				MemRes: &proto.MembershipResponse{
					Alive: []*proto.Envelope{m},
					Dead:  []*proto.Envelope{},
				},
			},
		},
	}).NoopSign()
	return sMsg
}

type msgInspection func(t *testing.T, index int, m *receivedMsg)

func TestAnchorPeer(t *testing.T) {
	t.Parallel()
	// Actors:
	// OrgA: {
	// 	p:   a real gossip instance
	//	ap1: anchor peer of type *peerMock
	//	pm1: a *peerMock
	// }
	// OrgB: {
	//	ap2: anchor peer of type *peerMock
	//	pm2: a *peerMock
	// }
	// Scenario:
	// 	Spawn the peer (p) that will be used to connect to the 2 anchor peers.
	//	After 5 seconds, spawn the anchor peers.
	// 	See that the MembershipRequest messages that are sent to the anchor peers
	// 	contain internal endpoints only to the anchor peers that are in orgA.
	//	Each anchor peer tells about a peer in its own organization (pm1 or pm2).
	//	Wait until 'p' sends 3 messages (handshake, handshake + memReq) to each anchor peer
	//	and 1 message (for handshake) to each of pm1 and pm2 to prove that the membership response
	//	was successfully sent from the anchor peers to p.

	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	portPrefix := 13610
	orgA := "orgA"
	orgB := "orgB"
	cs.putInOrg(portPrefix, orgA)   // Real peer
	cs.putInOrg(portPrefix+1, orgA) // Anchor peer mock
	cs.putInOrg(portPrefix+2, orgB) // Anchor peer mock
	cs.putInOrg(portPrefix+3, orgA) // peer mock I
	cs.putInOrg(portPrefix+4, orgB) // peer mock II

	// Create the assertions
	handshake := func(t *testing.T, index int, m *receivedMsg) {
		if index != 0 {
			return
		}
		assert.NotNil(t, m.GetConn())
	}

	memReqWithInternalEndpoint := func(t *testing.T, index int, m *receivedMsg) {
		if m.GetMemReq() == nil {
			return
		}
		assert.True(t, index > 0)
		req := m.GetMemReq()
		am, err := req.SelfInformation.ToGossipMessage()
		assert.NoError(t, err)
		assert.NotEmpty(t, am.GetSecretEnvelope().InternalEndpoint())
		m.respond(memResp(m.Nonce, fmt.Sprintf("localhost:%d", portPrefix+3)))
	}

	memReqWithoutInternalEndpoint := func(t *testing.T, index int, m *receivedMsg) {
		if m.GetMemReq() == nil {
			return
		}
		assert.True(t, index > 0)
		req := m.GetMemReq()
		am, err := req.SelfInformation.ToGossipMessage()
		assert.NoError(t, err)
		assert.Nil(t, am.GetSecretEnvelope())
		m.respond(memResp(m.Nonce, fmt.Sprintf("localhost:%d", portPrefix+4)))
	}

	// Create a peer mock
	pm1 := newPeerMock(portPrefix+3, 1, t, handshake)
	defer pm1.stop()
	pm2 := newPeerMock(portPrefix+4, 1, t, handshake)
	defer pm2.stop()
	jcm := &joinChanMsg{
		members2AnchorPeers: map[string][]api.AnchorPeer{
			orgA: {
				{Host: "localhost", Port: portPrefix + 1},
			},
			orgB: {
				{Host: "localhost", Port: portPrefix + 2},
			},
		},
	}
	channel := common.ChainID("TEST")
	endpoint := fmt.Sprintf("localhost:%d", portPrefix)
	// Create the gossip instance (the peer that connects to anchor peers)
	p := newGossipInstanceWithExternalEndpoint(portPrefix, 0, cs, endpoint)
	defer p.Stop()
	p.JoinChan(jcm, channel)
	p.UpdateChannelMetadata([]byte("bla"), channel)

	time.Sleep(time.Second * 5)

	// Create the anchor peers
	ap1 := newPeerMock(portPrefix+1, 3, t, handshake, memReqWithInternalEndpoint)
	defer ap1.stop()
	ap2 := newPeerMock(portPrefix+2, 3, t, handshake, memReqWithoutInternalEndpoint)
	defer ap2.stop()

	// Wait until received all expected messages from gossip instance
	ap1.finishedSignal.Wait()
	ap2.finishedSignal.Wait()
	pm1.finishedSignal.Wait()
	pm2.finishedSignal.Wait()
}

func TestBootstrapPeerMisConfiguration(t *testing.T) {
	t.Parallel()
	// Scenario:
	// The peer 'p' is a peer in orgA
	// Peers bs1 and bs2 are bootstrap peers.
	// bs1 is in orgB, so p shouldn't connect to it.
	// bs2 is in orgA, so p should connect to it.
	// We test by intercepting *all* messages that bs1 and bs2 get from p, that:
	// 1) At least 3 connection attempts were sent from p to bs1
	// 2) A membership request was sent from p to bs2

	cs := &configurableCryptoService{m: make(map[string]api.OrgIdentityType)}
	portPrefix := 43478
	orgA := "orgA"
	orgB := "orgB"
	cs.putInOrg(portPrefix, orgA)
	cs.putInOrg(portPrefix+1, orgB)
	cs.putInOrg(portPrefix+2, orgA)

	onlyHandshakes := func(t *testing.T, index int, m *receivedMsg) {
		// Ensure all messages sent are connection establishment messages
		// that are probing attempts
		assert.NotNil(t, m.GetConn())
		// If the logic we test in this test- fails,
		// the first message would be a membership request,
		// so this assertion would capture it and print a corresponding failure
		assert.Nil(t, m.GetMemReq())
	}
	// Initialize a peer mock that would wait for 3 messages sent to it
	bs1 := newPeerMock(portPrefix+1, 3, t, onlyHandshakes)
	defer bs1.stop()

	membershipRequestsSent := make(chan struct{}, 100)
	detectMembershipRequest := func(t *testing.T, index int, m *receivedMsg) {
		if m.GetMemReq() != nil {
			membershipRequestsSent <- struct{}{}
		}
	}

	bs2 := newPeerMock(portPrefix+2, 0, t, detectMembershipRequest)
	defer bs2.stop()

	p := newGossipInstanceWithExternalEndpoint(portPrefix, 0, cs, fmt.Sprintf("localhost:%d", portPrefix), 1, 2)
	defer p.Stop()

	// Wait for 3 handshake attempts from the bootstrap peer from orgB,
	// to prove that the peer did try to probe the bootstrap peer from orgB
	got3Handshakes := make(chan struct{})
	go func() {
		bs1.finishedSignal.Wait()
		got3Handshakes <- struct{}{}
	}()

	select {
	case <-got3Handshakes:
	case <-time.After(time.Second * 15):
		assert.Fail(t, "Didn't detect 3 handshake attempts to the bootstrap peer from orgB")
	}

	select {
	case <-membershipRequestsSent:
	case <-time.After(time.Second * 15):
		assert.Fail(t, "Bootstrap peer didn't receive a membership request from the peer within a timely manner")
	}
}
