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

package gossip

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	shortenedWaitTime := time.Millisecond * 300
	algo.SetDigestWaitTime(shortenedWaitTime / 2)
	algo.SetRequestWaitTime(shortenedWaitTime)
	algo.SetResponseWaitTime(shortenedWaitTime)
}

type pullerMock struct {
	mock.Mock
	pull.Mediator
}

type sentMsg struct {
	msg *proto.GossipMessage
	mock.Mock
}

func (s *sentMsg) Respond(msg *proto.GossipMessage) {
	s.Called(msg)
}

func (s *sentMsg) GetGossipMessage() *proto.GossipMessage {
	return s.msg
}

func (s *sentMsg) GetPKIID() common.PKIidType {
	return nil
}

type senderMock struct {
	mock.Mock
}

func (s *senderMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
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
	badSignature := func(nonce uint64) comm.ReceivedMessage {
		return createUpdateMessage(nonce, createBadlySignedUpdateMessage())
	}

	testCertificateUpdate(t, badSignature, false)
}

func TestCertStoreMismatchedIdentity(t *testing.T) {
	mismatchedIdentity := func(nonce uint64) comm.ReceivedMessage {
		return createUpdateMessage(nonce, createMismatchedUpdateMessage())
	}

	testCertificateUpdate(t, mismatchedIdentity, false)
}

func TestCertStoreShouldSucceed(t *testing.T) {
	totallyFineIdentity := func(nonce uint64) comm.ReceivedMessage {
		return createUpdateMessage(nonce, createValidUpdateMessage())
	}

	testCertificateUpdate(t, totallyFineIdentity, true)
}

func testCertificateUpdate(t *testing.T, updateFactory func(uint64) comm.ReceivedMessage, shouldSucceed bool) {
	config := pull.PullConfig{
		MsgType:           proto.PullMsgType_IdentityMsg,
		PeerCountToSelect: 1,
		PullInterval:      time.Millisecond * 500,
		Tag:               proto.GossipMessage_EMPTY,
		Channel:           nil,
		ID:                "id1",
	}
	sender := &senderMock{}
	memberSvc := &membershipSvcMock{}
	memberSvc.On("GetMembership").Return([]discovery.NetworkMember{{PKIid: []byte("bla bla"), Endpoint: "localhost:5611"}})

	pullMediator := pull.NewPullMediator(config,
		sender,
		memberSvc,
		func(msg *proto.GossipMessage) string { return string(msg.GetPeerIdentity().PkiID) },
		func(msg *proto.GossipMessage) {})
	certStore := newCertStore(&pullerMock{
		Mediator: pullMediator,
	}, identity.NewIdentityMapper(&naiveCryptoService{}), api.PeerIdentityType("SELF"), &naiveCryptoService{})

	defer pullMediator.Stop()

	wg := sync.WaitGroup{}
	wg.Add(1)
	sentHello := false
	sentDataReq := false
	l := sync.Mutex{}
	sender.On("Send", mock.Anything, mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
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

	hello := &sentMsg{
		msg: &proto.GossipMessage{
			Channel: []byte(""),
			Tag:     proto.GossipMessage_EMPTY,
			Content: &proto.GossipMessage_Hello{
				Hello: &proto.GossipHello{
					Nonce:    0,
					Metadata: nil,
					MsgType:  proto.PullMsgType_IdentityMsg,
				},
			},
		},
	}
	responseChan := make(chan *proto.GossipMessage, 1)
	hello.On("Respond", mock.Anything).Run(func(arg mock.Arguments) {
		msg := arg.Get(0).(*proto.GossipMessage)
		assert.NotNil(t, msg.GetDataDig())
		responseChan <- msg
	})
	certStore.handleMessage(hello)
	select {
	case msg := <-responseChan:
		if shouldSucceed {
			assert.Len(t, msg.GetDataDig().Digests, 2, "Valid identity hasn't entered the certStore")
		} else {
			assert.Len(t, msg.GetDataDig().Digests, 1, "Mismatched identity has been injected into certStore")
		}
	case <-time.After(time.Second):
		t.Fatalf("Didn't respond with a digest message in a timely manner")
	}
}

func createMismatchedUpdateMessage() *proto.GossipMessage {
	identity := &proto.PeerIdentity{
		// This PKI-ID is different than the cert, and the mapping between
		// certificate to PKI-ID in this test is simply the identity function.
		PkiID: []byte("A"),
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
			PeerIdentity: identity,
		},
	}

	m.Sign(signer)

	return m
}

func createBadlySignedUpdateMessage() *proto.GossipMessage {
	identity := &proto.PeerIdentity{
		PkiID: []byte("C"),
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
			PeerIdentity: identity,
		},
	}
	m.Sign(signer)
	// This would simulate a bad sig
	if m.Signature[0] == 0 {
		m.Signature[0] = 1
	} else {
		m.Signature[0] = 0
	}

	return m
}

func createValidUpdateMessage() *proto.GossipMessage {
	identity := &proto.PeerIdentity{
		PkiID: []byte("B"),
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
			PeerIdentity: identity,
		},
	}
	m.Sign(signer)
	return m
}

func createUpdateMessage(nonce uint64, idMsg *proto.GossipMessage) comm.ReceivedMessage {
	update := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				MsgType: proto.PullMsgType_IdentityMsg,
				Nonce:   nonce,
				Data:    []*proto.GossipMessage{idMsg},
			},
		},
	}
	return &sentMsg{msg: update}
}

func createDigest(nonce uint64) comm.ReceivedMessage {
	digest := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_DataDig{
			DataDig: &proto.DataDigest{
				Nonce:   nonce,
				MsgType: proto.PullMsgType_IdentityMsg,
				Digests: []string{"A", "C"},
			},
		},
	}
	return &sentMsg{msg: digest}
}
