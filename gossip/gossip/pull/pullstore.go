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

package pull

import (
	"sync"
	"time"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

// Constants go here.
const (
	HelloMsgType PullMsgType = iota
	DigestMsgType
	RequestMsgType
	ResponseMsgType
)

// PullMsgType defines the type of a message that is sent to the PullStore
type PullMsgType int

// MessageHook defines a function that will run after a certain pull message is received
type MessageHook func(itemIDs []string, items []*proto.GossipMessage, msg comm.ReceivedMessage)

// Sender sends messages to remote peers
type Sender interface {
	// Send sends a message to a list of remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)
}

// MembershipService obtains membership information of alive peers
type MembershipService interface {
	// GetMembership returns the membership of
	GetMembership() []discovery.NetworkMember
}

// PullConfig defines the configuration of the pull mediator
type PullConfig struct {
	ID                string
	PullInterval      time.Duration // Duration between pull invocations
	PeerCountToSelect int           // Number of peers to initiate pull with
	Tag               proto.GossipMessage_Tag
	Channel           common.ChainID
	MsgType           proto.PullMsgType
}

// Mediator is a component wrap a PullEngine and provides the methods
// it needs to perform pull synchronization.
// The specialization of a pull mediator to a certain type of message is
// done by the configuration, a IdentifierExtractor, IdentifierExtractor
// given at construction, and also hooks that can be registered for each
// type of pullMsgType (hello, digest, req, res).
type Mediator interface {
	// Stop stop the Mediator
	Stop()

	// RegisterMsgHook registers a message hook to a specific type of pull message
	RegisterMsgHook(PullMsgType, MessageHook)

	// Add adds a GossipMessage to the Mediator
	Add(*proto.GossipMessage)

	// Remove removes a GossipMessage from the Mediator
	Remove(*proto.GossipMessage)

	// HandleMessage handles a message from some remote peer
	HandleMessage(msg comm.ReceivedMessage)
}

// pullMediatorImpl is an implementation of Mediator
type pullMediatorImpl struct {
	sync.RWMutex
	Sender
	msgType2Hook map[PullMsgType][]MessageHook
	idExtractor  proto.IdentifierExtractor
	msgCons      proto.MsgConsumer
	config       PullConfig
	logger       *logging.Logger
	itemID2Msg   map[string]*proto.GossipMessage
	memBvc       MembershipService
	engine       *algo.PullEngine
}

// NewPullMediator returns a new Mediator
func NewPullMediator(config PullConfig, sndr Sender, memSvc MembershipService, idExtractor proto.IdentifierExtractor, msgCons proto.MsgConsumer) Mediator {
	p := &pullMediatorImpl{
		msgCons:      msgCons,
		msgType2Hook: make(map[PullMsgType][]MessageHook),
		idExtractor:  idExtractor,
		config:       config,
		logger:       util.GetLogger(util.LoggingPullModule, config.ID),
		itemID2Msg:   make(map[string]*proto.GossipMessage),
		memBvc:       memSvc,
		Sender:       sndr,
	}
	p.engine = algo.NewPullEngine(p, config.PullInterval)
	return p
}

func (p *pullMediatorImpl) HandleMessage(m comm.ReceivedMessage) {
	if m.GetGossipMessage() == nil || !m.GetGossipMessage().IsPullMsg() {
		return
	}
	msg := m.GetGossipMessage()

	msgType := msg.GetPullMsgType()
	if msgType != p.config.MsgType {
		return
	}

	p.logger.Debug(msg)

	itemIDs := []string{}
	items := []*proto.GossipMessage{}
	var pullMsgType PullMsgType

	if helloMsg := msg.GetHello(); helloMsg != nil {
		pullMsgType = HelloMsgType
		p.engine.OnHello(helloMsg.Nonce, m)
	}
	if digest := msg.GetDataDig(); digest != nil {
		itemIDs = digest.Digests
		pullMsgType = DigestMsgType
		p.engine.OnDigest(digest.Digests, digest.Nonce, m)
	}
	if req := msg.GetDataReq(); req != nil {
		itemIDs = req.Digests
		pullMsgType = RequestMsgType
		p.engine.OnReq(req.Digests, req.Nonce, m)
	}
	if res := msg.GetDataUpdate(); res != nil {
		itemIDs = make([]string, len(res.Data))
		items = make([]*proto.GossipMessage, len(res.Data))
		pullMsgType = ResponseMsgType
		for i, pulledMsg := range res.Data {
			p.msgCons(pulledMsg)
			itemIDs[i] = p.idExtractor(pulledMsg)
			items[i] = pulledMsg
			p.Lock()
			p.itemID2Msg[itemIDs[i]] = pulledMsg
			p.Unlock()
		}
		p.engine.OnRes(itemIDs, res.Nonce)
	}

	// Invoke hooks for relevant message type
	for _, h := range p.hooksByMsgType(pullMsgType) {
		h(itemIDs, items, m)
	}
}

func (p *pullMediatorImpl) Stop() {
	p.engine.Stop()
}

// RegisterMsgHook registers a message hook to a specific type of pull message
func (p *pullMediatorImpl) RegisterMsgHook(pullMsgType PullMsgType, hook MessageHook) {
	p.Lock()
	defer p.Unlock()
	p.msgType2Hook[pullMsgType] = append(p.msgType2Hook[pullMsgType], hook)

}

// Add adds a GossipMessage to the store
func (p *pullMediatorImpl) Add(msg *proto.GossipMessage) {
	p.Lock()
	defer p.Unlock()
	itemID := p.idExtractor(msg)
	p.itemID2Msg[itemID] = msg
	p.engine.Add(itemID)
}

// Remove removes a GossipMessage from the store
func (p *pullMediatorImpl) Remove(msg *proto.GossipMessage) {
	p.Lock()
	defer p.Unlock()
	itemID := p.idExtractor(msg)
	delete(p.itemID2Msg, itemID)
	p.engine.Remove(itemID)
}

// SelectPeers returns a slice of peers which the engine will initiate the protocol with
func (p *pullMediatorImpl) SelectPeers() []string {
	remotePeers := SelectEndpoints(p.config.PeerCountToSelect, p.memBvc.GetMembership())
	endpoints := make([]string, len(remotePeers))
	for i, peer := range remotePeers {
		endpoints[i] = peer.Endpoint
	}
	return endpoints
}

// Hello sends a hello message to initiate the protocol
// and returns an NONCE that is expected to be returned
// in the digest message.
func (p *pullMediatorImpl) Hello(dest string, nonce uint64) {
	helloMsg := &proto.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce:    nonce,
				Metadata: nil,
				MsgType:  p.config.MsgType,
			},
		},
	}

	p.logger.Debug("Sending hello to", dest)
	p.Send(helloMsg, p.peersWithEndpoints(dest)...)
}

// SendDigest sends a digest to a remote PullEngine.
// The context parameter specifies the remote engine to send to.
func (p *pullMediatorImpl) SendDigest(digest []string, nonce uint64, context interface{}) {
	digMsg := &proto.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &proto.GossipMessage_DataDig{
			DataDig: &proto.DataDigest{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Digests: digest,
			},
		},
	}
	p.logger.Debug("Sending digest", digMsg.GetDataDig().Digests)
	context.(comm.ReceivedMessage).Respond(digMsg)
}

// SendReq sends an array of items to a certain remote PullEngine identified
// by a string
func (p *pullMediatorImpl) SendReq(dest string, items []string, nonce uint64) {
	req := &proto.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &proto.GossipMessage_DataReq{
			DataReq: &proto.DataRequest{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Digests: items,
			},
		},
	}
	p.logger.Debug("Sending", req, "to", dest)
	p.Send(req, p.peersWithEndpoints(dest)...)
}

// SendRes sends an array of items to a remote PullEngine identified by a context.
func (p *pullMediatorImpl) SendRes(items []string, context interface{}, nonce uint64) {
	items2return := []*proto.GossipMessage{}
	p.RLock()
	defer p.RUnlock()
	for _, item := range items {
		if msg, exists := p.itemID2Msg[item]; exists {
			items2return = append(items2return, msg)
		}
	}

	returnedUpdate := &proto.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Data:    items2return,
			},
		},
	}
	p.logger.Debug("Sending", returnedUpdate, "to")
	context.(comm.ReceivedMessage).Respond(returnedUpdate)
}

func (p *pullMediatorImpl) peersWithEndpoints(endpoints ...string) []*comm.RemotePeer {
	peers := []*comm.RemotePeer{}
	for _, member := range p.memBvc.GetMembership() {
		for _, endpoint := range endpoints {
			if member.Endpoint == endpoint {
				peers = append(peers, &comm.RemotePeer{Endpoint: member.Endpoint, PKIID: member.PKIid})
			}
		}
	}
	return peers
}

func (p *pullMediatorImpl) hooksByMsgType(msgType PullMsgType) []MessageHook {
	p.RLock()
	defer p.RUnlock()
	returnedHooks := []MessageHook{}
	for _, h := range p.msgType2Hook[msgType] {
		returnedHooks = append(returnedHooks, h)
	}
	return returnedHooks
}

// SelectEndpoints select k peers from peerPool and returns them.
func SelectEndpoints(k int, peerPool []discovery.NetworkMember) []*comm.RemotePeer {
	if len(peerPool) < k {
		k = len(peerPool)
	}

	indices := util.GetRandomIndices(k, len(peerPool)-1)
	endpoints := make([]*comm.RemotePeer, len(indices))
	for i, j := range indices {
		endpoints[i] = &comm.RemotePeer{Endpoint: peerPool[j].Endpoint, PKIID: peerPool[j].PKIid}
	}
	return endpoints
}
