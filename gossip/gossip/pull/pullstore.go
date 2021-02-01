/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pull

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
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
type MessageHook func(itemIDs []string, items []*protoext.SignedGossipMessage, msg protoext.ReceivedMessage)

// Sender sends messages to remote peers
type Sender interface {
	// Send sends a message to a list of remote peers
	Send(msg *protoext.SignedGossipMessage, peers ...*comm.RemotePeer)
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
	Channel           common.ChannelID
	PeerCountToSelect int // Number of peers to initiate pull with
	Tag               gossip.GossipMessage_Tag
	MsgType           gossip.PullMsgType
	PullEngineConfig  algo.PullEngineConfig
}

// IngressDigestFilter filters out entities in digests that are received from remote peers
type IngressDigestFilter func(digestMsg *gossip.DataDigest) *gossip.DataDigest

// EgressDigestFilter filters digests to be sent to a remote peer, that
// sent a hello with the following message
type EgressDigestFilter func(helloMsg protoext.ReceivedMessage) func(digestItem string) bool

// byContext converts this EgressDigFilter to an algo.DigestFilter
func (df EgressDigestFilter) byContext() algo.DigestFilter {
	return func(context interface{}) func(digestItem string) bool {
		return func(digestItem string) bool {
			return df(context.(protoext.ReceivedMessage))(digestItem)
		}
	}
}

// MsgConsumer invokes code given a SignedGossipMessage
type MsgConsumer func(message *protoext.SignedGossipMessage)

// IdentifierExtractor extracts from a SignedGossipMessage an identifier
type IdentifierExtractor func(*protoext.SignedGossipMessage) string

// PullAdapter defines methods of the pullStore to interact
// with various modules of gossip
type PullAdapter struct {
	Sndr             Sender
	MemSvc           MembershipService
	IdExtractor      IdentifierExtractor
	MsgCons          MsgConsumer
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
	Add(*protoext.SignedGossipMessage)

	// Remove removes a GossipMessage from the Mediator with a matching digest,
	// if such a message exits
	Remove(digest string)

	// HandleMessage handles a message from some remote peer
	HandleMessage(msg protoext.ReceivedMessage)
}

// pullMediatorImpl is an implementation of Mediator
type pullMediatorImpl struct {
	sync.RWMutex
	*PullAdapter
	msgType2Hook map[MsgType][]MessageHook
	config       Config
	logger       util.Logger
	itemID2Msg   map[string]*protoext.SignedGossipMessage
	engine       *algo.PullEngine
}

// NewPullMediator returns a new Mediator
func NewPullMediator(config Config, adapter *PullAdapter) Mediator {
	egressDigFilter := adapter.EgressDigFilter

	acceptAllFilter := func(_ protoext.ReceivedMessage) func(string) bool {
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
		logger:       util.GetLogger(util.PullLogger, config.ID),
		itemID2Msg:   make(map[string]*protoext.SignedGossipMessage),
	}

	p.engine = algo.NewPullEngineWithFilter(p, config.PullInterval, egressDigFilter.byContext(), config.PullEngineConfig)

	if adapter.IngressDigFilter == nil {
		// Create accept all filter
		adapter.IngressDigFilter = func(digestMsg *gossip.DataDigest) *gossip.DataDigest {
			return digestMsg
		}
	}
	return p
}

func (p *pullMediatorImpl) HandleMessage(m protoext.ReceivedMessage) {
	if m.GetGossipMessage() == nil || !protoext.IsPullMsg(m.GetGossipMessage().GossipMessage) {
		return
	}
	msg := m.GetGossipMessage()
	msgType := protoext.GetPullMsgType(msg.GossipMessage)
	if msgType != p.config.MsgType {
		return
	}

	p.logger.Debug(msg)

	itemIDs := []string{}
	items := []*protoext.SignedGossipMessage{}
	var pullMsgType MsgType

	if helloMsg := msg.GetHello(); helloMsg != nil {
		pullMsgType = HelloMsgType
		p.engine.OnHello(helloMsg.Nonce, m)
	} else if digest := msg.GetDataDig(); digest != nil {
		d := p.PullAdapter.IngressDigFilter(digest)
		itemIDs = util.BytesToStrings(d.Digests)
		pullMsgType = DigestMsgType
		p.engine.OnDigest(itemIDs, d.Nonce, m)
	} else if req := msg.GetDataReq(); req != nil {
		itemIDs = util.BytesToStrings(req.Digests)
		pullMsgType = RequestMsgType
		p.engine.OnReq(itemIDs, req.Nonce, m)
	} else if res := msg.GetDataUpdate(); res != nil {
		itemIDs = make([]string, len(res.Data))
		items = make([]*protoext.SignedGossipMessage, len(res.Data))
		pullMsgType = ResponseMsgType
		for i, pulledMsg := range res.Data {
			msg, err := protoext.EnvelopeToGossipMessage(pulledMsg)
			if err != nil {
				p.logger.Warningf("Data update contains an invalid message: %+v", errors.WithStack(err))
				return
			}
			p.MsgCons(msg)
			itemIDs[i] = p.IdExtractor(msg)
			items[i] = msg
			p.Lock()
			p.itemID2Msg[itemIDs[i]] = msg
			p.logger.Debugf("Added %s to the in memory item map, total items: %d", itemIDs[i], len(p.itemID2Msg))
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
func (p *pullMediatorImpl) Add(msg *protoext.SignedGossipMessage) {
	p.Lock()
	defer p.Unlock()
	itemID := p.IdExtractor(msg)
	p.itemID2Msg[itemID] = msg
	p.engine.Add(itemID)
	p.logger.Debugf("Added %s, total items: %d", itemID, len(p.itemID2Msg))
}

// Remove removes a GossipMessage from the Mediator with a matching digest,
// if such a message exits
func (p *pullMediatorImpl) Remove(digest string) {
	p.Lock()
	defer p.Unlock()
	delete(p.itemID2Msg, digest)
	p.engine.Remove(digest)
	p.logger.Debugf("Removed %s, total items: %d", digest, len(p.itemID2Msg))
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
	helloMsg := &gossip.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Content: &gossip.GossipMessage_Hello{
			Hello: &gossip.GossipHello{
				Nonce:    nonce,
				Metadata: nil,
				MsgType:  p.config.MsgType,
			},
		},
	}

	p.logger.Debug("Sending", p.config.MsgType, "hello to", dest)
	sMsg, err := protoext.NoopSign(helloMsg)
	if err != nil {
		p.logger.Errorf("Failed creating SignedGossipMessage: %+v", errors.WithStack(err))
		return
	}
	p.Sndr.Send(sMsg, p.peersWithEndpoints(dest)...)
}

// SendDigest sends a digest to a remote PullEngine.
// The context parameter specifies the remote engine to send to.
func (p *pullMediatorImpl) SendDigest(digest []string, nonce uint64, context interface{}) {
	digMsg := &gossip.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &gossip.GossipMessage_DataDig{
			DataDig: &gossip.DataDigest{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Digests: util.StringsToBytes(digest),
			},
		},
	}
	remotePeer := context.(protoext.ReceivedMessage).GetConnectionInfo()
	if p.logger.IsEnabledFor(zapcore.DebugLevel) {
		p.logger.Debug("Sending", p.config.MsgType, "digest:", formattedDigests(digMsg.GetDataDig()), "to", remotePeer)
	}

	context.(protoext.ReceivedMessage).Respond(digMsg)
}

// SendReq sends an array of items to a certain remote PullEngine identified
// by a string
func (p *pullMediatorImpl) SendReq(dest string, items []string, nonce uint64) {
	req := &gossip.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &gossip.GossipMessage_DataReq{
			DataReq: &gossip.DataRequest{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Digests: util.StringsToBytes(items),
			},
		},
	}
	if p.logger.IsEnabledFor(zapcore.DebugLevel) {
		p.logger.Debug("Sending", formattedDigests(req.GetDataReq()), "to", dest)
	}
	sMsg, err := protoext.NoopSign(req)
	if err != nil {
		p.logger.Warningf("Failed creating SignedGossipMessage: %+v", errors.WithStack(err))
		return
	}
	p.Sndr.Send(sMsg, p.peersWithEndpoints(dest)...)
}

// typeDigster normalizes the common digest operations needed to format them
// for log messages.
type typedDigester interface {
	GetMsgType() gossip.PullMsgType
	GetDigests() [][]byte
}

func formattedDigests(dd typedDigester) []string {
	switch dd.GetMsgType() {
	case gossip.PullMsgType_IDENTITY_MSG:
		return digestsToHex(dd.GetDigests())
	default:
		return digestsAsStrings(dd.GetDigests())
	}
}

func digestsAsStrings(digests [][]byte) []string {
	a := make([]string, len(digests))
	for i, dig := range digests {
		a[i] = string(dig)
	}
	return a
}

func digestsToHex(digests [][]byte) []string {
	a := make([]string, len(digests))
	for i, dig := range digests {
		a[i] = hex.EncodeToString(dig)
	}
	return a
}

// SendRes sends an array of items to a remote PullEngine identified by a context.
func (p *pullMediatorImpl) SendRes(items []string, context interface{}, nonce uint64) {
	items2return := []*gossip.Envelope{}
	p.RLock()
	defer p.RUnlock()
	for _, item := range items {
		if msg, exists := p.itemID2Msg[item]; exists {
			items2return = append(items2return, msg.Envelope)
		}
	}
	returnedUpdate := &gossip.GossipMessage{
		Channel: p.config.Channel,
		Tag:     p.config.Tag,
		Nonce:   0,
		Content: &gossip.GossipMessage_DataUpdate{
			DataUpdate: &gossip.DataUpdate{
				MsgType: p.config.MsgType,
				Nonce:   nonce,
				Data:    items2return,
			},
		},
	}
	remotePeer := context.(protoext.ReceivedMessage).GetConnectionInfo()
	p.logger.Debug("Sending", len(returnedUpdate.GetDataUpdate().Data), p.config.MsgType, "items to", remotePeer)
	context.(protoext.ReceivedMessage).Respond(returnedUpdate)
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
	return append([]MessageHook(nil), p.msgType2Hook[msgType]...)
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
