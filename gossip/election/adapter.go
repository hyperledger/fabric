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

package election

import (
	"bytes"
	"strconv"
	"sync"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

type msgImpl struct {
	msg *proto.GossipMessage
}

func (mi *msgImpl) SenderID() string {
	return string(mi.msg.GetLeadershipMsg().GetMembership().PkiID)
}

func (mi *msgImpl) IsProposal() bool {
	return !mi.IsDeclaration()
}

func (mi *msgImpl) IsDeclaration() bool {
	isDeclaration, _ := strconv.ParseBool(string(mi.msg.GetLeadershipMsg().GetMembership().Metadata))
	return isDeclaration
}

type peerImpl struct {
	member *discovery.NetworkMember
}

func (pi *peerImpl) ID() string {
	return string(pi.member.PKIid)
}

type gossip interface {
	// Peers returns the NetworkMembers considered alive
	Peers() []discovery.NetworkMember

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan comm.ReceivedMessage)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)
}

// MsgCrypto used to sign messages and verify received messages signatures
type MsgCrypto interface {
	// Sign signs a message, returns a signed message on success
	// or an error on failure
	Sign(msg []byte) ([]byte, error)

	// Verify verifies a signed message
	Verify(vkID, signature, message []byte) error

	// Get returns the identity of a given pkiID, or error if such an identity
	// isn't found
	Get(pkiID common.PKIidType) (api.PeerIdentityType, error)
}

type adapterImpl struct {
	gossip gossip
	self   *discovery.NetworkMember

	incTime uint64
	seqNum  uint64

	mcs MsgCrypto

	channel common.ChainID

	logger *logging.Logger

	doneCh   chan struct{}
	stopOnce *sync.Once
}

// NewAdapter creates new leader election adapter
func NewAdapter(gossip gossip, self *discovery.NetworkMember, mcs MsgCrypto, channel common.ChainID) LeaderElectionAdapter {
	return &adapterImpl{
		gossip: gossip,
		self:   self,

		incTime: uint64(time.Now().UnixNano()),
		seqNum:  uint64(0),

		mcs: mcs,

		channel: channel,

		logger: logging.MustGetLogger("LeaderElectionAdapter"),

		doneCh:   make(chan struct{}),
		stopOnce: &sync.Once{},
	}
}

func (ai *adapterImpl) Gossip(msg Msg) {
	ai.gossip.Gossip(msg.(*msgImpl).msg)
}

func (ai *adapterImpl) Accept() <-chan Msg {
	adapterCh, _ := ai.gossip.Accept(func(message interface{}) bool {
		// Get only leadership org and channel messages
		validMsg := message.(*proto.GossipMessage).Tag == proto.GossipMessage_CHAN_AND_ORG &&
			message.(*proto.GossipMessage).IsLeadershipMsg() &&
			bytes.Equal(message.(*proto.GossipMessage).Channel, ai.channel)
		if validMsg {
			leadershipMsg := message.(*proto.GossipMessage).GetLeadershipMsg()

			verifier := func(identity []byte, signature, message []byte) error {
				return ai.mcs.Verify(identity, signature, message)
			}
			identity, err := ai.mcs.Get(leadershipMsg.GetMembership().PkiID)
			if err != nil {
				ai.logger.Error("Failed verify, can't get identity", leadershipMsg, ":", err)
				return false
			}

			if err := message.(*proto.GossipMessage).Verify(identity, verifier); err != nil {
				ai.logger.Error("Failed verify", leadershipMsg, ":", err)
				return false
			}
			return true
		}
		return false
	}, false)

	msgCh := make(chan Msg)

	go func(inCh <-chan *proto.GossipMessage, outCh chan Msg, stopCh chan struct{}) {
		for {
			select {
			case <-stopCh:
				return
			case gossipMsg, ok := <-inCh:
				if ok {
					outCh <- &msgImpl{gossipMsg}
				} else {
					return
				}
			}
		}
	}(adapterCh, msgCh, ai.doneCh)
	return msgCh
}

func (ai *adapterImpl) CreateMessage(isDeclaration bool) Msg {
	ai.seqNum++
	seqNum := ai.seqNum

	metadata := []byte{}
	metadata = strconv.AppendBool(metadata, isDeclaration)

	leadershipMsg := &proto.LeadershipMessage{
		Membership: &proto.Member{
			PkiID:    ai.self.PKIid,
			Endpoint: ai.self.Endpoint,
			Metadata: metadata,
		},
		Timestamp: &proto.PeerTime{
			IncNumber: ai.incTime,
			SeqNum:    seqNum,
		},
	}

	msg := &proto.GossipMessage{
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_LeadershipMsg{LeadershipMsg: leadershipMsg},
		Channel: ai.channel,
	}

	signer := func(msg []byte) ([]byte, error) {
		return ai.mcs.Sign(msg)
	}

	msg.Sign(signer)
	return &msgImpl{msg}
}

func (ai *adapterImpl) Peers() []Peer {
	peers := ai.gossip.Peers()

	var res []Peer
	for _, peer := range peers {
		res = append(res, &peerImpl{&peer})
	}

	return res
}

func (ai *adapterImpl) Stop() {
	stopFunc := func() {
		close(ai.doneCh)
	}
	ai.stopOnce.Do(stopFunc)
}
