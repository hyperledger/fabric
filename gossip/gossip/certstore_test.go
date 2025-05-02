/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	proto "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	util.SetupTestLogging()
}

var cs = &naiveCryptoService{
	revokedPkiIDS: make(map[string]struct{}),
}

type pullerMock struct {
	mock.Mock
	pull.Mediator
}

type sentMsg struct {
	msg *protoext.SignedGossipMessage
	mock.Mock
}

// GetSourceEnvelope Returns the SignedGossipMessage the ReceivedMessage was
// constructed with
func (s *sentMsg) GetSourceEnvelope() *proto.Envelope {
	return nil
}

// Ack returns to the sender an acknowledgement for the message
func (s *sentMsg) Ack(err error) {
}

func (s *sentMsg) Respond(msg *proto.GossipMessage) {
	s.Called(msg)
}

func (s *sentMsg) GetGossipMessage() *protoext.SignedGossipMessage {
	return s.msg
}

func (s *sentMsg) GetConnectionInfo() *protoext.ConnectionInfo {
	return nil
}

type senderMock struct {
	mock.Mock
	sync.Mutex
}

func (s *senderMock) Send(msg *protoext.SignedGossipMessage, peers ...*comm.RemotePeer) {
	s.Lock()
	defer s.Unlock()
	s.Called(msg, peers)
}

type membershipSvcMock struct {
	mock.Mock
}

func (m *membershipSvcMock) GetMembership() []discovery.NetworkMember {
	args := m.Called()
	return args.Get(0).([]discovery.NetworkMember)
}

func TestCertStoreBadSignature(t *testing.T) {
	badSignature := func(nonce uint64) protoext.ReceivedMessage {
		return createUpdateMessage(nonce, createBadlySignedUpdateMessage())
	}
	pm, cs, _ := createObjects(badSignature, nil)
	defer pm.Stop()
	defer cs.stop()
	testCertificateUpdate(t, false, cs)
}

func TestCertStoreMismatchedIdentity(t *testing.T) {
	mismatchedIdentity := func(nonce uint64) protoext.ReceivedMessage {
		return createUpdateMessage(nonce, createMismatchedUpdateMessage())
	}

	pm, cs, _ := createObjects(mismatchedIdentity, nil)
	defer pm.Stop()
	defer cs.stop()
	testCertificateUpdate(t, false, cs)
}

func TestCertStoreShouldSucceed(t *testing.T) {
	totallyFineIdentity := func(nonce uint64) protoext.ReceivedMessage {
		return createUpdateMessage(nonce, createValidUpdateMessage())
	}

	pm, cs, _ := createObjects(totallyFineIdentity, nil)
	defer pm.Stop()
	defer cs.stop()
	testCertificateUpdate(t, true, cs)
}

func TestCertRevocation(t *testing.T) {
	defer func() {
		cs.revokedPkiIDS = map[string]struct{}{}
	}()

	totallyFineIdentity := func(nonce uint64) protoext.ReceivedMessage {
		return createUpdateMessage(nonce, createValidUpdateMessage())
	}

	askedForIdentity := make(chan struct{}, 1)

	pm, cStore, sender := createObjects(totallyFineIdentity, func(message *protoext.SignedGossipMessage) {
		askedForIdentity <- struct{}{}
	})
	defer cStore.stop()
	defer pm.Stop()
	testCertificateUpdate(t, true, cStore)
	// Should have asked for an identity for the first time
	require.Len(t, askedForIdentity, 1)
	// Drain channel
	<-askedForIdentity
	// Now it's 0
	require.Len(t, askedForIdentity, 0)

	sentHello := false
	l := sync.Mutex{}
	sender.Lock()
	sender.Mock = mock.Mock{}
	sender.On("Send", mock.Anything, mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*protoext.SignedGossipMessage)
		l.Lock()
		defer l.Unlock()

		if hello := msg.GetHello(); hello != nil && !sentHello {
			sentHello = true
			dig := &proto.GossipMessage{
				Tag: proto.GossipMessage_EMPTY,
				Content: &proto.GossipMessage_DataDig{
					DataDig: &proto.DataDigest{
						Nonce:   hello.Nonce,
						MsgType: proto.PullMsgType_IDENTITY_MSG,
						Digests: [][]byte{[]byte("B")},
					},
				},
			}
			sMsg, _ := protoext.NoopSign(dig)
			go cStore.handleMessage(&sentMsg{msg: sMsg})
		}

		if dataReq := msg.GetDataReq(); dataReq != nil {
			askedForIdentity <- struct{}{}
		}
	})
	sender.Unlock()
	testCertificateUpdate(t, true, cStore)
	// Shouldn't have asked, because already got identity
	select {
	case <-time.After(time.Second * 5):
	case <-askedForIdentity:
		require.Fail(t, "Shouldn't have asked for an identity, because we already have it")
	}
	require.Len(t, askedForIdentity, 0)
	// Revoke the identity
	cs.revoke(common.PKIidType("B"))
	cStore.suspectPeers(func(id api.PeerIdentityType) bool {
		return string(id) == "B"
	})

	l.Lock()
	sentHello = false
	l.Unlock()

	select {
	case <-time.After(time.Second * 5):
		require.Fail(t, "Didn't ask for identity, but should have. Looks like identity hasn't expired")
	case <-askedForIdentity:
	}
}

func TestCertExpiration(t *testing.T) {
	// Scenario: In this test we make sure that a peer may not expire
	// its own identity.
	// This is important because the only way identities are gossiped
	// transitively is via the pull mechanism.
	// If a peer's own identity disappears from the pull mediator,
	// it will never be sent to peers transitively.
	// The test ensures that self identities don't expire
	// in the following manner:
	// It starts a peer and then sleeps twice the identity usage threshold,
	// in order to make sure that its own identity should be expired.
	// Then, it starts another peer, and listens to the messages sent
	// between both peers, and looks for a few identity digests of the first peer.
	// If such identity digest are detected, it means that the peer
	// didn't expire its own identity.

	// Backup original usageThreshold value
	idUsageThreshold := identity.GetIdentityUsageThreshold()
	identity.SetIdentityUsageThreshold(time.Second)
	// Restore original usageThreshold value
	defer identity.SetIdentityUsageThreshold(idUsageThreshold)

	port0, grpc0, certs0, secDialOpts0, _ := util.CreateGRPCLayer()
	port1, grpc1, certs1, secDialOpts1, _ := util.CreateGRPCLayer()
	g1 := newGossipInstanceWithGRPC(0, port0, grpc0, certs0, secDialOpts0, 0, port1)
	defer g1.Stop()
	time.Sleep(identity.GetIdentityUsageThreshold() * 2)
	g2 := newGossipInstanceWithGRPC(0, port1, grpc1, certs1, secDialOpts1, 0)
	defer g2.Stop()

	identities2Detect := 3
	// Make the channel bigger than needed so goroutines won't get stuck
	identitiesGotViaPull := make(chan struct{}, identities2Detect+100)
	acceptIdentityPullMsgs := func(o interface{}) bool {
		m := o.(protoext.ReceivedMessage).GetGossipMessage()
		if protoext.IsPullMsg(m.GossipMessage) && protoext.IsDigestMsg(m.GossipMessage) {
			for _, dig := range m.GetDataDig().Digests {
				if bytes.Equal(dig, fmt.Appendf(nil, "127.0.0.1:%d", port0)) {
					identitiesGotViaPull <- struct{}{}
				}
			}
		}
		return false
	}
	g1.Accept(acceptIdentityPullMsgs, true)
	for i := 0; i < identities2Detect; i++ {
		select {
		case <-identitiesGotViaPull:
		case <-time.After(time.Second * 15):
			require.Fail(t, "Didn't detect an identity gossiped via pull in a timely manner")
			return
		}
	}
}

func testCertificateUpdate(t *testing.T, shouldSucceed bool, certStore *certStore) {
	msg, _ := protoext.NoopSign(&proto.GossipMessage{
		Channel: []byte(""),
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce:    0,
				Metadata: nil,
				MsgType:  proto.PullMsgType_IDENTITY_MSG,
			},
		},
	})
	hello := &sentMsg{
		msg: msg,
	}
	responseChan := make(chan *proto.GossipMessage, 1)
	hello.On("Respond", mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		require.NotNil(t, msg.GetDataDig())
		responseChan <- msg
	})
	certStore.handleMessage(hello)
	select {
	case msg := <-responseChan:
		if shouldSucceed {
			require.Len(t, msg.GetDataDig().Digests, 2, "Valid identity hasn't entered the certStore")
		} else {
			require.Len(t, msg.GetDataDig().Digests, 1, "Mismatched identity has been injected into certStore")
		}
	case <-time.After(time.Second):
		t.Fatal("Didn't respond with a digest message in a timely manner")
	}
}

func createMismatchedUpdateMessage() *protoext.SignedGossipMessage {
	peeridentity := &proto.PeerIdentity{
		// This PKI-ID is different than the cert, and the mapping between
		// certificate to PKI-ID in this test is simply the identity function.
		PkiId: []byte("A"),
		Cert:  []byte("D"),
	}

	signer := func(msg []byte) ([]byte, error) {
		return (&naiveCryptoService{}).Sign(msg)
	}
	m := &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: peeridentity,
		},
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: m,
	}
	sMsg.Sign(signer)
	return sMsg
}

func createBadlySignedUpdateMessage() *protoext.SignedGossipMessage {
	peeridentity := &proto.PeerIdentity{
		PkiId: []byte("C"),
		Cert:  []byte("C"),
	}

	signer := func(msg []byte) ([]byte, error) {
		return (&naiveCryptoService{}).Sign(msg)
	}

	m := &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: peeridentity,
		},
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: m,
	}
	sMsg.Sign(signer)
	// This would simulate a bad sig
	if sMsg.Envelope.Signature[0] == 0 {
		sMsg.Envelope.Signature[0] = 1
	} else {
		sMsg.Envelope.Signature[0] = 0
	}
	return sMsg
}

func createValidUpdateMessage() *protoext.SignedGossipMessage {
	peeridentity := &proto.PeerIdentity{
		PkiId: []byte("B"),
		Cert:  []byte("B"),
	}

	signer := func(msg []byte) ([]byte, error) {
		return (&naiveCryptoService{}).Sign(msg)
	}
	m := &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: peeridentity,
		},
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: m,
	}
	sMsg.Sign(signer)
	return sMsg
}

func createUpdateMessage(nonce uint64, idMsg *protoext.SignedGossipMessage) protoext.ReceivedMessage {
	update := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				MsgType: proto.PullMsgType_IDENTITY_MSG,
				Nonce:   nonce,
				Data:    []*proto.Envelope{idMsg.Envelope},
			},
		},
	}
	sMsg, _ := protoext.NoopSign(update)
	return &sentMsg{msg: sMsg}
}

func createDigest(nonce uint64) protoext.ReceivedMessage {
	digest := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataDig{
			DataDig: &proto.DataDigest{
				Nonce:   nonce,
				MsgType: proto.PullMsgType_IDENTITY_MSG,
				Digests: [][]byte{[]byte("A"), []byte("C")},
			},
		},
	}
	sMsg, _ := protoext.NoopSign(digest)
	return &sentMsg{msg: sMsg}
}

func createObjects(updateFactory func(uint64) protoext.ReceivedMessage, msgCons pull.MsgConsumer) (pull.Mediator, *certStore, *senderMock) {
	if msgCons == nil {
		msgCons = func(_ *protoext.SignedGossipMessage) {}
	}
	shortenedWaitTime := time.Millisecond * 300
	config := pull.Config{
		MsgType:           proto.PullMsgType_IDENTITY_MSG,
		PeerCountToSelect: 1,
		PullInterval:      time.Second,
		Tag:               proto.GossipMessage_EMPTY,
		Channel:           nil,
		ID:                "id1",
		PullEngineConfig: algo.PullEngineConfig{
			DigestWaitTime:   shortenedWaitTime / 2,
			RequestWaitTime:  shortenedWaitTime,
			ResponseWaitTime: shortenedWaitTime,
		},
	}
	sender := &senderMock{}
	memberSvc := &membershipSvcMock{}
	memberSvc.On("GetMembership").Return([]discovery.NetworkMember{{PKIid: []byte("bla bla"), Endpoint: "127.0.0.1:5611"}})

	var certStore *certStore
	adapter := &pull.PullAdapter{
		Sndr: sender,
		MsgCons: func(msg *protoext.SignedGossipMessage) {
			certStore.idMapper.Put(msg.GetPeerIdentity().PkiId, msg.GetPeerIdentity().Cert)
			msgCons(msg)
		},
		IdExtractor: func(msg *protoext.SignedGossipMessage) string {
			return string(msg.GetPeerIdentity().PkiId)
		},
		MemSvc: memberSvc,
	}
	pullMediator := pull.NewPullMediator(config, adapter)
	selfIdentity := api.PeerIdentityType("SELF")
	certStore = newCertStore(&pullerMock{
		Mediator: pullMediator,
	}, identity.NewIdentityMapper(cs, selfIdentity, func(pkiID common.PKIidType, _ api.PeerIdentityType) {
		pullMediator.Remove(string(pkiID))
	}, cs), selfIdentity, cs)

	wg := sync.WaitGroup{}
	wg.Add(1)
	sentHello := false
	sentDataReq := false
	l := sync.Mutex{}
	sender.On("Send", mock.Anything, mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*protoext.SignedGossipMessage)
		l.Lock()
		defer l.Unlock()

		if hello := msg.GetHello(); hello != nil && !sentHello {
			sentHello = true
			go certStore.handleMessage(createDigest(hello.Nonce))
		}

		if dataReq := msg.GetDataReq(); dataReq != nil && !sentDataReq {
			sentDataReq = true
			certStore.handleMessage(updateFactory(dataReq.Nonce))
			wg.Done()
		}
	})
	wg.Wait()
	return pullMediator, certStore, sender
}
