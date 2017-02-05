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
	"github.com/hyperledger/fabric/protos/gossip"
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
	UpdateStateInfo(msg *proto.GossipMessage)

	// IsOrgInChannel returns whether the given organization is in the channel
	IsOrgInChannel(membersOrg api.OrgIdentityType) bool

	// IsSubscribed returns whether the given member published
	// its participation in the channel
	IsSubscribed(member discovery.NetworkMember) bool

	// HandleMessage processes a message sent by a remote peer
	HandleMessage(comm.ReceivedMessage)

	// AddToMsgStore adds a given GossipMessage to the message store
	AddToMsgStore(msg *proto.GossipMessage)

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
	Gossip(*proto.GossipMessage)

	// DeMultiplex de-multiplexes an item to subscribers
	DeMultiplex(interface{})

	// GetMembership returns the known alive peers and their information
	GetMembership() []discovery.NetworkMember

	// Send sends a message to a list of peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// ValidateStateInfoMessage returns an error if a message
	// hasn't been signed correctly, nil otherwise.
	ValidateStateInfoMessage(*proto.GossipMessage) error

	// OrgByPeerIdentity returns the organization ID of a given peer identity
	OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType

	// GetOrgOfPeer returns the organization ID of a given peer PKI-ID
	GetOrgOfPeer(common.PKIidType) api.OrgIdentityType
}

type gossipChannel struct {
	Adapter
	sync.RWMutex
	shouldGossipStateInfo     int32
	mcs                       api.MessageCryptoService
	stopChan                  chan struct{}
	stateInfoMsg              *proto.GossipMessage
	orgs                      []api.OrgIdentityType
	joinMsg                   api.JoinChannelMessage
	blockMsgStore             msgstore.MessageStore
	stateInfoMsgStore         msgstore.MessageStore
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
		if mf.IsSubscribed(mem) {
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
		gc.blocksPuller.Remove(m.(*proto.GossipMessage))
	})

	gc.stateInfoMsgStore = NewStateInfoMessageStore()
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

	pkiID2NetMember := make(map[string]discovery.NetworkMember)
	for _, member := range gc.GetMembership() {
		pkiID2NetMember[string(member.PKIid)] = member
	}

	for _, o := range gc.stateInfoMsgStore.Get() {
		stateInf := o.(*proto.GossipMessage).GetStateInfo()
		pkiID := stateInf.PkiID
		if member, exists := pkiID2NetMember[string(pkiID)]; !exists {
			continue
		} else {
			member.Metadata = stateInf.Metadata
			members = append(members, member)
		}
	}
	return members
}

func (gc *gossipChannel) requestStateInfo() {
	req := gc.createStateInfoRequest()
	endpoints := filter.SelectPeers(gc.GetConf().PullPeerNum, gc.GetMembership(), gc.IsSubscribed)
	if len(endpoints) == 0 {
		endpoints = filter.SelectPeers(gc.GetConf().PullPeerNum, gc.GetMembership(), gc.IsMemberInChan)
	}
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
		MsgType:           proto.PullMsgType_BlockMessage,
		Channel:           []byte(gc.chainID),
		ID:                gc.GetConf().ID,
		PeerCountToSelect: gc.GetConf().PullPeerNum,
		PullInterval:      gc.GetConf().PullInterval,
		Tag:               proto.GossipMessage_CHAN_AND_ORG,
	}
	seqNumFromMsg := func(msg *proto.GossipMessage) string {
		dataMsg := msg.GetDataMsg()
		if dataMsg == nil || dataMsg.Payload == nil {
			return ""
		}
		return fmt.Sprintf("%d", dataMsg.Payload.SeqNum)
	}
	blockConsumer := func(msg *proto.GossipMessage) {
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

// IsSubscribed returns whether the given member published
// its participation in the channel
func (gc *gossipChannel) IsSubscribed(member discovery.NetworkMember) bool {
	if !gc.IsMemberInChan(member) {
		return false
	}
	for _, o := range gc.stateInfoMsgStore.Get() {
		m, isMsg := o.(*proto.GossipMessage)
		if isMsg && m.IsStateInfoMsg() && bytes.Equal(m.GetStateInfo().PkiID, member.PKIid) {
			return true
		}
	}
	return false
}

// AddToMsgStore adds a given GossipMessage to the message store
func (gc *gossipChannel) AddToMsgStore(msg *proto.GossipMessage) {
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

	if gc.joinMsg == nil {
		gc.joinMsg = joinMsg
	}

	if gc.joinMsg.SequenceNumber() > (joinMsg.SequenceNumber()) {
		gc.logger.Warning("Already have a more updated JoinChannel message(", gc.joinMsg.SequenceNumber(), ") than", gc.joinMsg.SequenceNumber())
		return
	}
	orgs := []api.OrgIdentityType{}
	existingOrgInJoinChanMsg := make(map[string]struct{})
	for _, anchorPeer := range joinMsg.AnchorPeers() {
		orgID := gc.OrgByPeerIdentity(anchorPeer.Cert)
		if orgID == nil {
			gc.logger.Warning("Cannot extract org identity from certificate, aborting.")
			return
		}
		if _, exists := existingOrgInJoinChanMsg[string(orgID)]; !exists {
			orgs = append(orgs, orgID)
			existingOrgInJoinChanMsg[string(orgID)] = struct{}{}
		}
	}
	gc.orgs = orgs
	gc.joinMsg = joinMsg
}

// HandleMessage processes a message sent by a remote peer
func (gc *gossipChannel) HandleMessage(msg comm.ReceivedMessage) {
	if !gc.verifyMsg(msg) {
		return
	}
	m := msg.GetGossipMessage()
	if !m.IsChannelRestricted() {
		gc.logger.Warning("Got message", msg.GetGossipMessage(), "but it's not a per-channel message, discarding it")
		return
	}
	orgID := gc.GetOrgOfPeer(msg.GetPKIID())
	if orgID == nil {
		gc.logger.Warning("Couldn't find org identity of peer", msg.GetPKIID())
		return
	}
	if !gc.IsOrgInChannel(orgID) {
		gc.logger.Warning("Point to point message came from", msg.GetPKIID(), "but it's not eligible for the channel", msg.GetGossipMessage().Channel)
		return
	}

	if m.IsStateInfoPullRequestMsg() {
		msg.Respond(gc.createStateInfoSnapshot())
		return
	}

	if m.IsStateInfoSnapshot() {
		gc.handleStateInfSnapshot(m, msg.GetPKIID())
		return
	}

	if m.IsDataMsg() || m.IsStateInfoMsg() {
		added := false

		if m.IsDataMsg() {
			if !gc.verifyBlock(m, msg.GetPKIID()) {
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
	if m.IsPullMsg() && m.GetPullMsgType() == proto.PullMsgType_BlockMessage {
		if m.IsDataUpdate() {
			for _, item := range m.GetDataUpdate().Data {
				if !bytes.Equal(item.Channel, []byte(gc.chainID)) {
					gc.logger.Warning("DataUpdate message contains item with channel", item.Channel, "but should be", gc.chainID)
					return
				}
				if !gc.verifyBlock(item, msg.GetPKIID()) {
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
	for _, stateInf := range m.GetStateSnapshot().Elements {
		if !stateInf.IsStateInfoMsg() {
			gc.logger.Warning("Element of StateInfoSnapshot isn't a StateInfoMessage:", stateInf, "message sent from", sender)
			return
		}

		orgID := gc.GetOrgOfPeer(stateInf.GetStateInfo().PkiID)
		if orgID == nil {
			gc.logger.Warning("Couldn't find org identity of peer", stateInf.GetStateInfo().PkiID, "message sent from", sender)
			return
		}

		if !gc.IsOrgInChannel(orgID) {
			gc.logger.Warning("Peer", stateInf.GetStateInfo().PkiID, "is not in an eligible org, can't process a stateInfo from it, sent from", sender)
			return
		}

		if !bytes.Equal(stateInf.Channel, []byte(gc.chainID)) {
			gc.logger.Warning("StateInfo message is of an invalid channel", stateInf, "sent from", sender)
			return
		}
		err := gc.ValidateStateInfoMessage(stateInf)
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
	err := gc.mcs.VerifyBlock(msg.Channel, msg.GetDataMsg().Payload)
	if err != nil {
		gc.logger.Warning("Received fabricated block from", sender, "in DataUpdate:", err)
		return false
	}
	return true
}

func (gc *gossipChannel) createStateInfoSnapshot() *proto.GossipMessage {
	rawElements := gc.stateInfoMsgStore.Get()
	elements := make([]*proto.GossipMessage, len(rawElements))
	for i, rawEl := range rawElements {
		elements[i] = rawEl.(*proto.GossipMessage)
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

func (gc *gossipChannel) verifyMsg(msg comm.ReceivedMessage) bool {
	if msg == nil {
		gc.logger.Warning("Messsage is nil")
		return false
	}
	m := msg.GetGossipMessage()
	if m == nil {
		gc.logger.Warning("Message content is empty")
		return false
	}

	if msg.GetPKIID() == nil {
		gc.logger.Warning("Message has nil PKI-ID")
		return false
	}

	if !bytes.Equal(m.Channel, []byte(gc.chainID)) {
		gc.logger.Warning("Message contains wrong channel(", m.Channel, "), expected", gc.chainID)
		return false
	}
	return true
}

func (gc *gossipChannel) createStateInfoRequest() *proto.GossipMessage {
	return &proto.GossipMessage{
		Channel: gc.chainID,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Nonce:   0,
		Content: &proto.GossipMessage_StateInfoPullReq{
			StateInfoPullReq: &proto.StateInfoPullRequest{},
		},
	}
}

// UpdateStateInfo updates this channel's StateInfo message
// that is periodically published
func (gc *gossipChannel) UpdateStateInfo(msg *proto.GossipMessage) {
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
