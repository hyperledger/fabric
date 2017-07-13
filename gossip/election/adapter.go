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
	"sync"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

type msgImpl struct {
	msg *proto.GossipMessage
}

func (mi *msgImpl) SenderID() peerID {
	return mi.msg.GetLeadershipMsg().PkiId
}

func (mi *msgImpl) IsProposal() bool {
	return !mi.IsDeclaration()
}

func (mi *msgImpl) IsDeclaration() bool {
	return mi.msg.GetLeadershipMsg().IsDeclaration
}

type peerImpl struct {
	member discovery.NetworkMember
}

func (pi *peerImpl) ID() peerID {
	return peerID(pi.member.PKIid)
}

type gossip interface {
	// Peers returns the NetworkMembers considered alive
	Peers() []discovery.NetworkMember

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)
}

type adapterImpl struct {
	gossip    gossip
	selfPKIid common.PKIidType

	incTime uint64
	seqNum  uint64

	channel common.ChainID

	logger *logging.Logger

	doneCh   chan struct{}
	stopOnce *sync.Once
}

// NewAdapter creates new leader election adapter
func NewAdapter(gossip gossip, pkiid common.PKIidType, channel common.ChainID) LeaderElectionAdapter {
	return &adapterImpl{
		gossip:    gossip,
		selfPKIid: pkiid,

		incTime: uint64(time.Now().UnixNano()),
		seqNum:  uint64(0),

		channel: channel,

		logger: util.GetLogger(util.LoggingElectionModule, ""),

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
		return message.(*proto.GossipMessage).Tag == proto.GossipMessage_CHAN_AND_ORG &&
			message.(*proto.GossipMessage).IsLeadershipMsg() &&
			bytes.Equal(message.(*proto.GossipMessage).Channel, ai.channel)
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

	leadershipMsg := &proto.LeadershipMessage{
		PkiId:         ai.selfPKIid,
		IsDeclaration: isDeclaration,
		Timestamp: &proto.PeerTime{
			IncNum: ai.incTime,
			SeqNum: seqNum,
		},
	}

	msg := &proto.GossipMessage{
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_AND_ORG,
		Content: &proto.GossipMessage_LeadershipMsg{LeadershipMsg: leadershipMsg},
		Channel: ai.channel,
	}
	return &msgImpl{msg}
}

func (ai *adapterImpl) Peers() []Peer {
	peers := ai.gossip.Peers()

	var res []Peer
	for _, peer := range peers {
		res = append(res, &peerImpl{peer})
	}

	return res
}

func (ai *adapterImpl) Stop() {
	stopFunc := func() {
		close(ai.doneCh)
	}
	ai.stopOnce.Do(stopFunc)
}
