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

package channel

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/gossip/msgstore"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

// Config is a configuration item
// of the channel store
type Config struct {
	ID                       string
	PublishStateInfoInterval time.Duration
	MaxBlockCountToStore     int
	PullPeerNum              int
	PullInterval             time.Duration
	RequestStateInfoInterval time.Duration
}

// GossipChannel defines an object that deals with all channel-related messages
type GossipChannel interface {

	// GetPeers returns a list of peers with metadata as published by them
	GetPeers() []discovery.NetworkMember

	// IsMemberInChan checks whether the given member is eligible to be in the channel
	IsMemberInChan(member discovery.NetworkMember) bool

	// UpdateStateInfo updates this channel's StateInfo message
	// that is periodically published
	UpdateStateInfo(msg *proto.SignedGossipMessage)

	// IsOrgInChannel returns whether the given organization is in the channel
	IsOrgInChannel(membersOrg api.OrgIdentityType) bool

	// EligibleForChannel returns whether the given member should get blocks
	// for this channel
	EligibleForChannel(member discovery.NetworkMember) bool

	// HandleMessage processes a message sent by a remote peer
	HandleMessage(proto.ReceivedMessage)

	// AddToMsgStore adds a given GossipMessage to the message store
	AddToMsgStore(msg *proto.SignedGossipMessage)

	// ConfigureChannel (re)configures the list of organizations
	// that are eligible to be in the channel
	ConfigureChannel(joinMsg api.JoinChannelMessage)

	// Stop stops the channel's activity
	Stop()
}

// Adapter enables the gossipChannel
// to communicate with gossipServiceImpl.
type Adapter interface {
	// GetConf returns the configuration that this GossipChannel will posses
	GetConf() Config

	// Gossip gossips a message in the channel
	Gossip(message *proto.SignedGossipMessage)

	// DeMultiplex de-multiplexes an item to subscribers
	DeMultiplex(interface{})

	// GetMembership returns the known alive peers and their information
	GetMembership() []discovery.NetworkMember

	// Send sends a message to a list of peers
	Send(msg *proto.SignedGossipMessage, peers ...*comm.RemotePeer)

	// ValidateStateInfoMessage returns an error if a message
	// hasn't been signed correctly, nil otherwise.
	ValidateStateInfoMessage(message *proto.SignedGossipMessage) error

	// GetOrgOfPeer returns the organization ID of a given peer PKI-ID
	GetOrgOfPeer(pkiID common.PKIidType) api.OrgIdentityType

	// GetIdentityByPKIID returns an identity of a peer with a certain
	// pkiID, or nil if not found
	GetIdentityByPKIID(pkiID common.PKIidType) api.PeerIdentityType
}

type gossipChannel struct {
	Adapter
	sync.RWMutex
	shouldGossipStateInfo     int32
	mcs                       api.MessageCryptoService
	stopChan                  chan struct{}
	stateInfoMsg              *proto.SignedGossipMessage
	orgs                      []api.OrgIdentityType
	joinMsg                   api.JoinChannelMessage
	blockMsgStore             msgstore.MessageStore
	stateInfoMsgStore         *stateInfoCache
	leaderMsgStore            msgstore.MessageStore
	chainID                   common.ChainID
	blocksPuller              pull.Mediator
	logger                    *logging.Logger
	stateInfoPublishScheduler *time.Ticker
	stateInfoRequestScheduler *time.Ticker
	memFilter                 *membershipFilter
}

type membershipFilter struct {
	adapter Adapter
	*gossipChannel
}

// GetMembership returns the known alive peers and their information
func (mf *membershipFilter) GetMembership() []discovery.NetworkMember {
	var members []discovery.NetworkMember
	for _, mem := range mf.adapter.GetMembership() {
		if mf.EligibleForChannel(mem) {
			members = append(members, mem)
		}
	}
	return members
}

// NewGossipChannel creates a new GossipChannel
func NewGossipChannel(mcs api.MessageCryptoService, chainID common.ChainID, adapter Adapter, joinMsg api.JoinChannelMessage) GossipChannel {
	gc := &gossipChannel{
		mcs:                       mcs,
		Adapter:                   adapter,
		logger:                    util.GetLogger(util.LoggingChannelModule, adapter.GetConf().ID),
		stopChan:                  make(chan struct{}, 1),
		shouldGossipStateInfo:     int32(0),
		stateInfoPublishScheduler: time.NewTicker(adapter.GetConf().PublishStateInfoInterval),
		stateInfoRequestScheduler: time.NewTicker(adapter.GetConf().RequestStateInfoInterval),
		orgs:    []api.OrgIdentityType{},
		chainID: chainID,
	}

	gc.memFilter = &membershipFilter{adapter: gc.Adapter, gossipChannel: gc}

	comparator := proto.NewGossipMessageComparator(adapter.GetConf().MaxBlockCountToStore)
	gc.blockMsgStore = msgstore.NewMessageStore(comparator, func(m interface{}) {
		gc.blocksPuller.Remove(m.(*proto.SignedGossipMessage))
	})

	gc.stateInfoMsgStore = newStateInfoCache()
	gc.blocksPuller = gc.createBlockPuller()
	gc.leaderMsgStore = msgstore.NewMessageStore(proto.NewGossipMessageComparator(0), func(m interface{}) {})

	gc.ConfigureChannel(joinMsg)

	// Periodically publish state info
	go gc.periodicalInvocation(gc.publishStateInfo, gc.stateInfoPublishScheduler.C)
	// Periodically request state info
	go gc.periodicalInvocation(gc.requestStateInfo, gc.stateInfoRequestScheduler.C)
	return gc
}

// Stop stop the channel operations
func (gc *gossipChannel) Stop() {
	gc.stopChan <- struct{}{}
	gc.blocksPuller.Stop()
	gc.stateInfoPublishScheduler.Stop()
	gc.stateInfoRequestScheduler.Stop()
}

func (gc *gossipChannel) periodicalInvocation(fn func(), c <-chan time.Time) {
	for {
		select {
		case <-c:
			fn()
		case <-gc.stopChan:
			gc.stopChan <- struct{}{}
			return
		}
	}
}

// GetPeers returns a list of peers with metadata as published by them
func (gc *gossipChannel) GetPeers() []discovery.NetworkMember {
	members := []discovery.NetworkMember{}

	for _, member := range gc.GetMembership() {
		if !gc.EligibleForChannel(member) {
			continue
		}
		stateInf := gc.stateInfoMsgStore.MsgByID(member.PKIid)
		if stateInf == nil {
			continue
		}
		member.Metadata = stateInf.GetStateInfo().Metadata
		members = append(members, member)
	}
	return members
}

func (gc *gossipChannel) requestStateInfo() {
	req := gc.createStateInfoRequest().NoopSign()
	endpoints := filter.SelectPeers(gc.GetConf().PullPeerNum, gc.GetMembership(), gc.IsMemberInChan)
	gc.Send(req, endpoints...)
}

func (gc *gossipChannel) publishStateInfo() {
	if atomic.LoadInt32(&gc.shouldGossipStateInfo) == int32(0) {
		return
	}
	gc.RLock()
	stateInfoMsg := gc.stateInfoMsg
	gc.RUnlock()
	gc.Gossip(stateInfoMsg)
}

func (gc *gossipChannel) createBlockPuller() pull.Mediator {
	conf := pull.PullConfig{
		MsgType:           proto.PullMsgType_BLOCK_MSG,
		Channel:           []byte(gc.chainID),
		ID:                gc.GetConf().ID,
		PeerCountToSelect: gc.GetConf().PullPeerNum,
		PullInterval:      gc.GetConf().PullInterval,
		Tag:               proto.GossipMessage_CHAN_AND_ORG,
	}
	seqNumFromMsg := func(msg *proto.SignedGossipMessage) string {
		dataMsg := msg.GetDataMsg()
		if dataMsg == nil || dataMsg.Payload == nil {
			gc.logger.Warning("Non-data block or with no payload")
			return ""
		}
		return fmt.Sprintf("%d", dataMsg.Payload.SeqNum)
	}
	blockConsumer := func(msg *proto.SignedGossipMessage) {
		dataMsg := msg.GetDataMsg()
		if dataMsg == nil || dataMsg.Payload == nil {
			gc.logger.Warning("Invalid DataMessage:", dataMsg)
			return
		}
		added := gc.blockMsgStore.Add(msg)
		// if we can't add the message to the msgStore,
		// no point in disseminating it to others...
		if !added {
			return
		}
		gc.DeMultiplex(msg)
	}
	return pull.NewPullMediator(conf, gc, gc.memFilter, seqNumFromMsg, blockConsumer)
}

// IsMemberInChan checks whether the given member is eligible to be in the channel
func (gc *gossipChannel) IsMemberInChan(member discovery.NetworkMember) bool {
	org := gc.GetOrgOfPeer(member.PKIid)
	if org == nil {
		return false
	}

	return gc.IsOrgInChannel(org)
}

// IsOrgInChannel returns whether the given organization is in the channel
func (gc *gossipChannel) IsOrgInChannel(membersOrg api.OrgIdentityType) bool {
	gc.RLock()
	defer gc.RUnlock()
	for _, orgOfChan := range gc.orgs {
		if bytes.Equal(orgOfChan, membersOrg) {
			return true
		}
	}
	return false
}

// EligibleForChannel returns whether the given member should get blocks
// for this channel
func (gc *gossipChannel) EligibleForChannel(member discovery.NetworkMember) bool {
	if !gc.IsMemberInChan(member) {
		return false
	}

	identity := gc.GetIdentityByPKIID(member.PKIid)
	msg := gc.stateInfoMsgStore.MsgByID(member.PKIid)
	if msg == nil || identity == nil {
		return false
	}

	return gc.mcs.VerifyByChannel(gc.chainID, identity, msg.Envelope.Signature, msg.Envelope.Payload) == nil
}

// AddToMsgStore adds a given GossipMessage to the message store
func (gc *gossipChannel) AddToMsgStore(msg *proto.SignedGossipMessage) {
	if msg.IsDataMsg() {
		gc.blockMsgStore.Add(msg)
		gc.blocksPuller.Add(msg)
	}

	if msg.IsStateInfoMsg() {
		gc.stateInfoMsgStore.Add(msg)
	}
}

// ConfigureChannel (re)configures the list of organizations
// that are eligible to be in the channel
func (gc *gossipChannel) ConfigureChannel(joinMsg api.JoinChannelMessage) {
	gc.Lock()
	defer gc.Unlock()

	if len(joinMsg.Members()) == 0 {
		gc.logger.Warning("Received join channel message with empty set of members")
		return
	}

	if gc.joinMsg == nil {
		gc.joinMsg = joinMsg
	}

	if gc.joinMsg.SequenceNumber() > (joinMsg.SequenceNumber()) {
		gc.logger.Warning("Already have a more updated JoinChannel message(", gc.joinMsg.SequenceNumber(), ") than", gc.joinMsg.SequenceNumber())
		return
	}

	gc.orgs = joinMsg.Members()
	gc.joinMsg = joinMsg
}

// HandleMessage processes a message sent by a remote peer
func (gc *gossipChannel) HandleMessage(msg proto.ReceivedMessage) {
	if !gc.verifyMsg(msg) {
		gc.logger.Warning("Failed verifying message:", msg.GetGossipMessage().GossipMessage)
		return
	}
	m := msg.GetGossipMessage()
	if !m.IsChannelRestricted() {
		gc.logger.Warning("Got message", msg.GetGossipMessage(), "but it's not a per-channel message, discarding it")
		return
	}
	orgID := gc.GetOrgOfPeer(msg.GetConnectionInfo().ID)
	if orgID == nil {
		gc.logger.Warning("Couldn't find org identity of peer", msg.GetConnectionInfo().ID)
		return
	}
	if !gc.IsOrgInChannel(orgID) {
		gc.logger.Warning("Point to point message came from", msg.GetConnectionInfo().ID, "but it's not eligible for the channel", msg.GetGossipMessage().Channel)
		return
	}

	if m.IsStateInfoPullRequestMsg() {
		msg.Respond(gc.createStateInfoSnapshot())
		return
	}

	if m.IsStateInfoSnapshot() {
		gc.handleStateInfSnapshot(m.GossipMessage, msg.GetConnectionInfo().ID)
		return
	}

	if m.IsDataMsg() || m.IsStateInfoMsg() {
		added := false

		if m.IsDataMsg() {
			if m.GetDataMsg().Payload == nil {
				gc.logger.Warning("Payload is empty, got it from", msg.GetConnectionInfo().ID)
				return
			}
			if !gc.verifyBlock(m.GossipMessage, msg.GetConnectionInfo().ID) {
				gc.logger.Warning("Failed verifying block", m.GetDataMsg().Payload.SeqNum)
				return
			}
			added = gc.blockMsgStore.Add(msg.GetGossipMessage())
		} else { // StateInfoMsg verification should be handled in a layer above
			//  since we don't have access to the id mapper here
			added = gc.stateInfoMsgStore.Add(msg.GetGossipMessage())
		}

		if added {
			// Forward the message
			gc.Gossip(msg.GetGossipMessage())
			// DeMultiplex to local subscribers
			gc.DeMultiplex(m)

			if m.IsDataMsg() {
				gc.blocksPuller.Add(msg.GetGossipMessage())
			}
		}
		return
	}
	if m.IsPullMsg() && m.GetPullMsgType() == proto.PullMsgType_BLOCK_MSG {
		if !gc.EligibleForChannel(discovery.NetworkMember{PKIid: msg.GetConnectionInfo().ID}) {
			gc.logger.Warning(msg.GetConnectionInfo().ID, "isn't eligible for channel", gc.chainID)
			return
		}
		if m.IsDataUpdate() {
			for _, item := range m.GetDataUpdate().Data {
				gMsg, err := item.ToGossipMessage()
				if err != nil {
					gc.logger.Warning("Data update contains an invalid message:", err)
					return
				}
				if !bytes.Equal(gMsg.Channel, []byte(gc.chainID)) {
					gc.logger.Warning("DataUpdate message contains item with channel", gMsg.Channel, "but should be", gc.chainID)
					return
				}
				if !gc.verifyBlock(gMsg.GossipMessage, msg.GetConnectionInfo().ID) {
					return
				}
			}
		}
		gc.blocksPuller.HandleMessage(msg)
	}

	if m.IsLeadershipMsg() {
		// Handling leadership message
		added := gc.leaderMsgStore.Add(m)
		if added {
			gc.DeMultiplex(m)
		}
	}
}

func (gc *gossipChannel) handleStateInfSnapshot(m *proto.GossipMessage, sender common.PKIidType) {
	for _, envelope := range m.GetStateSnapshot().Elements {
		stateInf, err := envelope.ToGossipMessage()
		if err != nil {
			gc.logger.Warning("StateInfo snapshot contains an invalid message:", err)
			return
		}
		if !stateInf.IsStateInfoMsg() {
			gc.logger.Warning("Element of StateInfoSnapshot isn't a StateInfoMessage:", stateInf, "message sent from", sender)
			return
		}

		orgID := gc.GetOrgOfPeer(stateInf.GetStateInfo().PkiId)
		if orgID == nil {
			gc.logger.Warning("Couldn't find org identity of peer", stateInf.GetStateInfo().PkiId, "message sent from", sender)
			return
		}

		if !gc.IsOrgInChannel(orgID) {
			gc.logger.Warning("Peer", stateInf.GetStateInfo().PkiId, "is not in an eligible org, can't process a stateInfo from it, sent from", sender)
			return
		}

		if !bytes.Equal(stateInf.Channel, []byte(gc.chainID)) {
			gc.logger.Warning("StateInfo message is of an invalid channel", stateInf, "sent from", sender)
			return
		}
		err = gc.ValidateStateInfoMessage(stateInf)
		if err != nil {
			gc.logger.Warning("Failed validating state info message:", stateInf, ":", err, "sent from", sender)
			return
		}
		gc.stateInfoMsgStore.Add(stateInf)
	}
}

func (gc *gossipChannel) verifyBlock(msg *proto.GossipMessage, sender common.PKIidType) bool {
	if !msg.IsDataMsg() {
		gc.logger.Warning("Received from ", sender, "a DataUpdate message that contains a non-block GossipMessage:", msg)
		return false
	}
	if msg.GetDataMsg().Payload == nil {
		gc.logger.Warning("Received empty payload from", sender)
		return false
	}
	err := gc.mcs.VerifyBlock(msg.Channel, msg.GetDataMsg().Payload.Data)
	if err != nil {
		gc.logger.Warning("Received fabricated block from", sender, "in DataUpdate:", err)
		return false
	}
	return true
}

func (gc *gossipChannel) createStateInfoSnapshot() *proto.GossipMessage {
	rawElements := gc.stateInfoMsgStore.Get()
	elements := make([]*proto.Envelope, len(rawElements))
	for i, rawEl := range rawElements {
		elements[i] = rawEl.(*proto.SignedGossipMessage).Envelope
	}

	return &proto.GossipMessage{
		Channel: gc.chainID,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Nonce:   0,
		Content: &proto.GossipMessage_StateSnapshot{
			StateSnapshot: &proto.StateInfoSnapshot{
				Elements: elements,
			},
		},
	}
}

func (gc *gossipChannel) verifyMsg(msg proto.ReceivedMessage) bool {
	if msg == nil {
		gc.logger.Warning("Messsage is nil")
		return false
	}
	m := msg.GetGossipMessage()
	if m == nil {
		gc.logger.Warning("Message content is empty")
		return false
	}

	if msg.GetConnectionInfo().ID == nil {
		gc.logger.Warning("Message has nil PKI-ID")
		return false
	}

	if !bytes.Equal(m.Channel, []byte(gc.chainID)) {
		gc.logger.Warning("Message contains wrong channel(", m.Channel, "), expected", gc.chainID)
		return false
	}
	return true
}

func (gc *gossipChannel) createStateInfoRequest() *proto.SignedGossipMessage {
	return (&proto.GossipMessage{
		Channel: gc.chainID,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Nonce:   0,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{},
		},
	}).NoopSign()
}

// UpdateStateInfo updates this channel's StateInfo message
// that is periodically published
func (gc *gossipChannel) UpdateStateInfo(msg *proto.SignedGossipMessage) {
	if !msg.IsStateInfoMsg() {
		return
	}
	gc.stateInfoMsgStore.Add(msg)
	gc.Lock()
	defer gc.Unlock()
	gc.stateInfoMsg = msg
	atomic.StoreInt32(&gc.shouldGossipStateInfo, int32(1))
}

// NewStateInfoMessageStore returns a MessageStore
func NewStateInfoMessageStore() msgstore.MessageStore {
	return msgstore.NewMessageStore(proto.NewGossipMessageComparator(0), func(m interface{}) {})
}

func newStateInfoCache() *stateInfoCache {
	return &stateInfoCache{
		MembershipStore: util.NewMembershipStore(),
		MessageStore:    NewStateInfoMessageStore(),
	}
}

// stateInfoCache is actually a messageStore
// that also indexes messages that are added
// so that they could be extracted later
type stateInfoCache struct {
	*util.MembershipStore
	msgstore.MessageStore
}

// Add attempts to add the given message to the stateInfoCache,
// and if the message was added, also indexes it.
// Message must be a StateInfo message.
func (cache *stateInfoCache) Add(msg *proto.SignedGossipMessage) bool {
	added := cache.MessageStore.Add(msg)
	pkiID := msg.GetStateInfo().PkiId
	if added {
		cache.MembershipStore.Put(pkiID, msg)
	}
	return added
}
