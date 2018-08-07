/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

// Constants go here.
const (
	HelloMsgType MsgType = iota
	DigestMsgType
	RequestMsgType
	ResponseMsgType
)

// MsgType defines the type of a message that is sent to the PullStore
type MsgType int

// MessageHook defines a function that will run after a certain pull message is received
type MessageHook func(itemIDs []string, items []*proto.SignedGossipMessage, msg proto.ReceivedMessage)

// Sender sends messages to remote peers
type Sender interface {
	// Send sends a message to a list of remote peers
	Send(msg *proto.SignedGossipMessage, peers ...*comm.RemotePeer)
}

// MembershipService obtains membership information of alive peers
type MembershipService interface {
	// GetMembership returns the membership of
	GetMembership() []discovery.NetworkMember
}

// Config defines the configuration of the pull mediator
type Config struct {
	ID                string
	PullInterval      time.Duration // Duration between pull invocations
	Channel           common.ChainID
	PeerCountToSelect int // Number of peers to initiate pull with
	Tag               proto.GossipMessage_Tag
	MsgType           proto.PullMsgType
}

// IngressDigestFilter filters out entities in digests that are received from remote peers
type IngressDigestFilter func(digestMsg *proto.DataDigest) *proto.DataDigest

// EgressDigestFilter filters digests to be sent to a remote peer, that
// sent a hello with the following message
type EgressDigestFilter func(helloMsg proto.ReceivedMessage) func(digestItem string) bool

// byContext converts this EgressDigFilter to an algo.DigestFilter
func (df EgressDigestFilter) byContext() algo.DigestFilter {
	return func(context interface{}) func(digestItem string) bool {
		return func(digestItem string) bool {
			return df(context.(proto.ReceivedMessage))(digestItem)
		}
	}
}

// PullAdapter defines methods of the pullStore to interact
// with various modules of gossip
type PullAdapter struct {
	Sndr             Sender
	MemSvc           MembershipService
	IdExtractor      proto.IdentifierExtractor
	MsgCons          proto.MsgConsumer
	EgressDigFilter  EgressDigestFilter
	IngressDigFilter IngressDigestFilter
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
	RegisterMsgHook(MsgType, MessageHook)

	// Add adds a GossipMessage to the Mediator
	Add(*proto.SignedGossipMessage)

	// Remove removes a GossipMessage from the Mediator with a matching digest,
	// if such a message exits
	Remove(digest string)

	// HandleMessage handles a message from some remote peer
	HandleMessage(msg proto.ReceivedMessage)
}

// pullMediatorImpl is an implementation of Mediator
type pullMediatorImpl struct {
	sync.RWMutex
	*PullAdapter
	msgType2Hook map[MsgType][]MessageHook
	config       Config
	logger       util.Logger
	itemID2Msg   map[string]*proto.SignedGossipMessage
	engine       *algo.PullEngine
}

// NewPullMediator returns a new Mediator
func NewPullMediator(config Config, adapter *PullAdapter) Mediator {
	egressDigFilter := adapter.EgressDigFilter

	acceptAllFilter := func(_ proto.ReceivedMessage) func(string) bool {
		return func(_ string) bool {
			return true
		}
	}

	if egressDigFilter == nil {
		egressDigFilter = acceptAllFilter
	}

	p := &pullMediatorImpl{
		PullAdapter:  adapter,
		msgType2Hook: make(map[MsgType][]MessageHook),
		config:       config,
		logger:       util.GetLogger(util.LoggingPullModule, config.ID),
		itemID2Msg:   make(map[string]*proto.SignedGossipMessage),
	}

	p.engine = algo.NewPullEngineWithFilter(p, config.PullInterval, egressDigFilter.byContext())

	if adapter.IngressDigFilter == nil {
		// Create accept all filter
		adapter.IngressDigFilter = func(digestMsg *proto.DataDigest) *proto.DataDigest {
			return digestMsg
		}
	}
	return p

}

func (p *pullMediatorImpl) HandleMessage(m proto.ReceivedMessage) {
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
	items := []*proto.SignedGossipMessage{}
	var pullMsgType MsgType

	if helloMsg := msg.GetHello(); helloMsg != nil {
		pullMsgType = HelloMsgType
		p.engine.OnHello(helloMsg.Nonce, m)
	}
	if digest := msg.GetDataDig(); digest != nil {
		d := p.PullAdapter.IngressDigFilter(digest)
		itemIDs = util.BytesToStrings(d.Digests)
		pullMsgType = DigestMsgType
		p.engine.OnDigest(itemIDs, d.Nonce, m)
	}
	if req := msg.GetDataReq(); req != nil {
		itemIDs = util.BytesToStrings(req.Digests)
		pullMsgType = RequestMsgType
		p.engine.OnReq(itemIDs, req.Nonce, m)
	}
	if res := msg.GetDataUpdate(); res != nil {
		itemIDs = make([]string, len(res.Data))
		items = make([]*proto.SignedGossipMessage, len(res.Data))
		pullMsgType = ResponseMsgType
		for i, pulledMsg := range res.Data {
			msg, err := pulledMsg.ToGossipMessage()
			if err != nil {
				p.logger.Warningf("Data update contains an invalid message: %+v", errors.WithStack(err))
				return
			}
			p.MsgCons(msg)
			itemIDs[i] = p.IdExtractor(msg)
			items[i] = msg
			p.Lock()
			p.itemID2Msg[itemIDs[i]] = msg
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
func (p *pullMediatorImpl) RegisterMsgHook(pullMsgType MsgType, hook MessageHook) {
	p.Lock()
	defer p.Unlock()
	p.msgType2Hook[pullMsgType] = append(p.msgType2Hook[pullMsgType], hook)

}

// Add adds a GossipMessage to the store
func (p *pullMediatorImpl) Add(msg *proto.SignedGossipMessage) {
	p.Lock()
	defer p.Unlock()
	itemID := p.IdExtractor(msg)
	p.itemID2Msg[itemID] = msg
	p.engine.Add(itemID)
}

// Remove removes a GossipMessage from the Mediator with a matching digest,
// if such a message exits
func (p *pullMediatorImpl) Remove(digest string) {
	p.Lock()
	defer p.Unlock()
	delete(p.itemID2Msg, digest)
	p.engine.Remove(digest)
}

// SelectPeers returns a slice of peers which the engine will initiate the protocol with
func (p *pullMediatorImpl) SelectPeers() []string {
	remotePeers := SelectEndpoints(p.config.PeerCountToSelect, p.MemSvc.GetMembership())
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

	p.logger.Debug("Sending", p.config.MsgType, "hello to", dest)
	sMsg, err := helloMsg.NoopSign()
	if err != nil {
		p.logger.Errorf("Failed creating SignedGossipMessage: %+v", errors.WithStack(err))
		return
	}
	p.Sndr.Send(sMsg, p.peersWithEndpoints(dest)...)
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
				Digests: util.StringsToBytes(digest),
			},
		},
	}
	remotePeer := context.(proto.ReceivedMessage).GetConnectionInfo()
	if p.logger.IsEnabledFor(zapcore.DebugLevel) {
		p.logger.Debug("Sending", p.config.MsgType, "digest:", digMsg.GetDataDig().FormattedDigests(), "to", remotePeer)
	}

	context.(proto.ReceivedMessage).Respond(digMsg)
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
				Digests: util.StringsToBytes(items),
			},
		},
	}
	if p.logger.IsEnabledFor(zapcore.DebugLevel) {
		p.logger.Debug("Sending", req.GetDataReq().FormattedDigests(), "to", dest)
	}
	sMsg, err := req.NoopSign()
	if err != nil {
		p.logger.Warningf("Failed creating SignedGossipMessage: %+v", errors.WithStack(err))
		return
	}
	p.Sndr.Send(sMsg, p.peersWithEndpoints(dest)...)
}

// SendRes sends an array of items to a remote PullEngine identified by a context.
func (p *pullMediatorImpl) SendRes(items []string, context interface{}, nonce uint64) {
	items2return := []*proto.Envelope{}
	p.RLock()
	defer p.RUnlock()
	for _, item := range items {
		if msg, exists := p.itemID2Msg[item]; exists {
			items2return = append(items2return, msg.Envelope)
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
	remotePeer := context.(proto.ReceivedMessage).GetConnectionInfo()
	p.logger.Debug("Sending", len(returnedUpdate.GetDataUpdate().Data), p.config.MsgType, "items to", remotePeer)
	context.(proto.ReceivedMessage).Respond(returnedUpdate)
}

func (p *pullMediatorImpl) peersWithEndpoints(endpoints ...string) []*comm.RemotePeer {
	peers := []*comm.RemotePeer{}
	for _, member := range p.MemSvc.GetMembership() {
		for _, endpoint := range endpoints {
			if member.PreferredEndpoint() == endpoint {
				peers = append(peers, &comm.RemotePeer{Endpoint: member.PreferredEndpoint(), PKIID: member.PKIid})
			}
		}
	}
	return peers
}

func (p *pullMediatorImpl) hooksByMsgType(msgType MsgType) []MessageHook {
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
		endpoints[i] = &comm.RemotePeer{Endpoint: peerPool[j].PreferredEndpoint(), PKIID: peerPool[j].PKIid}
	}
	return endpoints
}
