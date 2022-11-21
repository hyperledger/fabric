/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	pg "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/gossip/msgstore"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	presumedDeadChanSize = 100
	acceptChanSize       = 100
)

type channelRoutingFilterFactory func(channel.GossipChannel) filter.RoutingFilter

// Node is a member of a gossip network
type Node struct {
	selfIdentity          api.PeerIdentityType
	includeIdentityPeriod time.Time
	certStore             *certStore
	idMapper              identity.Mapper
	presumedDead          chan common.PKIidType
	disc                  discovery.Discovery
	comm                  comm.Comm
	selfOrg               api.OrgIdentityType
	*comm.ChannelDeMultiplexer
	logger            util.Logger
	stopSignal        *sync.WaitGroup
	conf              *Config
	toDieChan         chan struct{}
	stopFlag          int32
	emitter           batchingEmitter
	discAdapter       *discoveryAdapter
	secAdvisor        api.SecurityAdvisor
	chanState         *channelState
	disSecAdap        *discoverySecurityAdapter
	mcs               api.MessageCryptoService
	stateInfoMsgStore msgstore.MessageStore
	certPuller        pull.Mediator
	gossipMetrics     *metrics.GossipMetrics
}

// New creates a gossip instance attached to a gRPC server
func New(conf *Config, s *grpc.Server, sa api.SecurityAdvisor,
	mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType,
	secureDialOpts api.PeerSecureDialOpts, gossipMetrics *metrics.GossipMetrics,
	anchorPeerTracker discovery.AnchorPeerTracker) *Node {
	var err error

	lgr := util.GetLogger(util.GossipLogger, conf.ID)

	g := &Node{
		selfOrg:               sa.OrgByPeerIdentity(selfIdentity),
		secAdvisor:            sa,
		selfIdentity:          selfIdentity,
		presumedDead:          make(chan common.PKIidType, presumedDeadChanSize),
		disc:                  nil,
		mcs:                   mcs,
		conf:                  conf,
		ChannelDeMultiplexer:  comm.NewChannelDemultiplexer(),
		logger:                lgr,
		toDieChan:             make(chan struct{}),
		stopFlag:              int32(0),
		stopSignal:            &sync.WaitGroup{},
		includeIdentityPeriod: time.Now().Add(conf.PublishCertPeriod),
		gossipMetrics:         gossipMetrics,
	}
	g.stateInfoMsgStore = g.newStateInfoMsgStore()

	g.idMapper = identity.NewIdentityMapper(mcs, selfIdentity, func(pkiID common.PKIidType, identity api.PeerIdentityType) {
		g.comm.CloseConn(&comm.RemotePeer{PKIID: pkiID})
		g.certPuller.Remove(string(pkiID))
	}, sa)

	commConfig := comm.CommConfig{
		DialTimeout:  conf.DialTimeout,
		ConnTimeout:  conf.ConnTimeout,
		RecvBuffSize: conf.RecvBuffSize,
		SendBuffSize: conf.SendBuffSize,
	}
	g.comm, err = comm.NewCommInstance(s, conf.TLSCerts, g.idMapper, selfIdentity, secureDialOpts, sa,
		gossipMetrics.CommMetrics, commConfig)

	if err != nil {
		lgr.Error("Failed instantiating communication layer:", err)
		return nil
	}

	g.chanState = newChannelState(g)
	g.emitter = newBatchingEmitter(conf.PropagateIterations,
		conf.MaxPropagationBurstSize, conf.MaxPropagationBurstLatency,
		g.sendGossipBatch)

	g.discAdapter = g.newDiscoveryAdapter()
	g.disSecAdap = g.newDiscoverySecurityAdapter()

	discoveryConfig := discovery.DiscoveryConfig{
		AliveTimeInterval:            conf.AliveTimeInterval,
		AliveExpirationTimeout:       conf.AliveExpirationTimeout,
		AliveExpirationCheckInterval: conf.AliveExpirationCheckInterval,
		ReconnectInterval:            conf.ReconnectInterval,
		MaxConnectionAttempts:        conf.MaxConnectionAttempts,
		MsgExpirationFactor:          conf.MsgExpirationFactor,
		BootstrapPeers:               conf.BootstrapPeers,
	}
	self := g.selfNetworkMember()
	logger := util.GetLogger(util.DiscoveryLogger, self.InternalEndpoint)
	g.disc = discovery.NewDiscoveryService(self, g.discAdapter, g.disSecAdap, g.disclosurePolicy,
		discoveryConfig, anchorPeerTracker, logger)
	g.logger.Infof("Creating gossip service with self membership of %s", g.selfNetworkMember())

	g.certPuller = g.createCertStorePuller()
	g.certStore = newCertStore(g.certPuller, g.idMapper, selfIdentity, mcs)

	if g.conf.ExternalEndpoint == "" {
		g.logger.Warning("External endpoint is empty, peer will not be accessible outside of its organization")
	}
	// Adding delta for handlePresumedDead and
	// acceptMessages goRoutines to block on Wait
	g.stopSignal.Add(2)
	go g.start()
	go g.connect2BootstrapPeers()

	return g
}

func (g *Node) newStateInfoMsgStore() msgstore.MessageStore {
	pol := protoext.NewGossipMessageComparator(0)
	return msgstore.NewMessageStoreExpirable(pol,
		msgstore.Noop,
		g.conf.PublishStateInfoInterval*100,
		nil,
		nil,
		msgstore.Noop)
}

func (g *Node) selfNetworkMember() discovery.NetworkMember {
	self := discovery.NetworkMember{
		Endpoint:         g.conf.ExternalEndpoint,
		PKIid:            g.comm.GetPKIid(),
		Metadata:         []byte{},
		InternalEndpoint: g.conf.InternalEndpoint,
	}
	if g.disc != nil {
		self.Metadata = g.disc.Self().Metadata
	}
	return self
}

func newChannelState(g *Node) *channelState {
	return &channelState{
		stopping: int32(0),
		channels: make(map[string]channel.GossipChannel),
		g:        g,
	}
}

func (g *Node) toDie() bool {
	return atomic.LoadInt32(&g.stopFlag) == int32(1)
}

// JoinChan makes gossip participate in the given channel, or update it.
func (g *Node) JoinChan(joinMsg api.JoinChannelMessage, channelID common.ChannelID) {
	// joinMsg is supposed to have been already verified
	g.chanState.joinChannel(joinMsg, channelID, g.gossipMetrics.MembershipMetrics)

	g.logger.Info("Joining gossip network of channel", channelID, "with", len(joinMsg.Members()), "organizations")
	for _, org := range joinMsg.Members() {
		g.learnAnchorPeers(string(channelID), org, joinMsg.AnchorPeersOf(org))
	}
}

// LeaveChan makes gossip stop participating in the given channel
func (g *Node) LeaveChan(channelID common.ChannelID) {
	gc := g.chanState.getGossipChannelByChainID(channelID)
	if gc == nil {
		g.logger.Debug("No such channel", channelID)
		return
	}
	gc.LeaveChannel()
}

// SuspectPeers makes the gossip instance validate identities of suspected peers, and close
// any connections to peers with identities that are found invalid
func (g *Node) SuspectPeers(isSuspected api.PeerSuspector) {
	g.certStore.suspectPeers(isSuspected)
}

func (g *Node) learnAnchorPeers(channel string, orgOfAnchorPeers api.OrgIdentityType, anchorPeers []api.AnchorPeer) {
	if len(anchorPeers) == 0 {
		g.logger.Infof("No configured anchor peers of %s for channel %s to learn about", string(orgOfAnchorPeers), channel)
		return
	}
	g.logger.Infof("Learning about the configured anchor peers of %s for channel %s: %v", string(orgOfAnchorPeers), channel, anchorPeers)
	for _, ap := range anchorPeers {
		if ap.Host == "" {
			g.logger.Warningf("Got empty hostname for channel %s, skipping connecting to anchor peer %v", channel, ap)
			continue
		}
		if ap.Port == 0 {
			g.logger.Warningf("Got invalid port (0) for channel %s, skipping connecting to anchor peer %v", channel, ap)
			continue
		}
		endpoint := net.JoinHostPort(ap.Host, fmt.Sprintf("%d", ap.Port))
		// Skip connecting to self
		if g.selfNetworkMember().Endpoint == endpoint || g.selfNetworkMember().InternalEndpoint == endpoint {
			g.logger.Infof("Anchor peer for channel %s with same endpoint, skipping connecting to myself", channel)
			continue
		}

		inOurOrg := bytes.Equal(g.selfOrg, orgOfAnchorPeers)
		if !inOurOrg && g.selfNetworkMember().Endpoint == "" {
			g.logger.Infof("Anchor peer %s:%d isn't in our org(%v) and we have no external endpoint, skipping", ap.Host, ap.Port, string(orgOfAnchorPeers))
			continue
		}
		identifier := func() (*discovery.PeerIdentification, error) {
			remotePeerIdentity, err := g.comm.Handshake(&comm.RemotePeer{Endpoint: endpoint})
			if err != nil {
				g.logger.Warningf("Deep probe of %s for channel %s failed: %s", endpoint, channel, err)
				return nil, err
			}
			isAnchorPeerInMyOrg := bytes.Equal(g.selfOrg, g.secAdvisor.OrgByPeerIdentity(remotePeerIdentity))
			if bytes.Equal(orgOfAnchorPeers, g.selfOrg) && !isAnchorPeerInMyOrg {
				err := errors.Errorf("Anchor peer %s for channel %s isn't in our org, but is claimed to be", endpoint, channel)
				g.logger.Warningf("%s", err)
				return nil, err
			}
			pkiID := g.mcs.GetPKIidOfCert(remotePeerIdentity)
			if len(pkiID) == 0 {
				return nil, errors.Errorf("Wasn't able to extract PKI-ID of remote peer with identity of %v", remotePeerIdentity)
			}
			return &discovery.PeerIdentification{
				ID:      pkiID,
				SelfOrg: isAnchorPeerInMyOrg,
			}, nil
		}

		g.disc.Connect(discovery.NetworkMember{
			InternalEndpoint: endpoint, Endpoint: endpoint,
		}, identifier)
	}
}

func (g *Node) handlePresumedDead() {
	defer g.logger.Debug("Exiting")
	defer g.stopSignal.Done()
	for {
		select {
		case <-g.toDieChan:
			return
		case deadEndpoint := <-g.comm.PresumedDead():
			g.presumedDead <- deadEndpoint
		}
	}
}

func (g *Node) syncDiscovery() {
	g.logger.Debug("Entering discovery sync with interval", g.conf.PullInterval)
	defer g.logger.Debug("Exiting discovery sync loop")
	for !g.toDie() {
		g.disc.InitiateSync(g.conf.PullPeerNum)
		time.Sleep(g.conf.PullInterval)
	}
}

func (g *Node) start() {
	go g.syncDiscovery()
	go g.handlePresumedDead()

	msgSelector := func(msg interface{}) bool {
		gMsg, isGossipMsg := msg.(protoext.ReceivedMessage)
		if !isGossipMsg {
			return false
		}

		isConn := gMsg.GetGossipMessage().GetConn() != nil
		isEmpty := gMsg.GetGossipMessage().GetEmpty() != nil
		isPrivateData := protoext.IsPrivateDataMsg(gMsg.GetGossipMessage().GossipMessage)

		return !(isConn || isEmpty || isPrivateData)
	}

	incMsgs := g.comm.Accept(msgSelector)

	go g.acceptMessages(incMsgs)

	g.logger.Info("Gossip instance", g.conf.ID, "started")
}

func (g *Node) acceptMessages(incMsgs <-chan protoext.ReceivedMessage) {
	defer g.logger.Debug("Exiting")
	defer g.stopSignal.Done()
	for {
		select {
		case <-g.toDieChan:
			return
		case msg := <-incMsgs:
			g.handleMessage(msg)
		}
	}
}

func (g *Node) handleMessage(m protoext.ReceivedMessage) {
	if g.toDie() {
		return
	}

	if m == nil || m.GetGossipMessage() == nil {
		return
	}

	msg := m.GetGossipMessage()

	g.logger.Debug("Entering,", m.GetConnectionInfo(), "sent us", msg)
	defer g.logger.Debug("Exiting")

	if !g.validateMsg(m) {
		g.logger.Warning("Message", msg, "isn't valid")
		return
	}

	if protoext.IsChannelRestricted(msg.GossipMessage) {
		if gc := g.chanState.lookupChannelForMsg(m); gc == nil {
			// If we're not in the channel, we should still forward to peers of our org
			// in case it's a StateInfo message
			if g.IsInMyOrg(discovery.NetworkMember{PKIid: m.GetConnectionInfo().ID}) && protoext.IsStateInfoMsg(msg.GossipMessage) {
				if g.stateInfoMsgStore.Add(msg) {
					g.emitter.Add(&emittedGossipMessage{
						SignedGossipMessage: msg,
						filter:              m.GetConnectionInfo().ID.IsNotSameFilter,
					})
				}
			}
			if !g.toDie() {
				g.logger.Debug("No such channel", msg.Channel, "discarding message", msg)
			}
		} else {
			if protoext.IsLeadershipMsg(m.GetGossipMessage().GossipMessage) {
				if err := g.validateLeadershipMessage(m.GetGossipMessage()); err != nil {
					g.logger.Warningf("Failed validating LeaderElection message: %+v", errors.WithStack(err))
					return
				}
			}
			gc.HandleMessage(m)
		}
		return
	}

	if selectOnlyDiscoveryMessages(m) {
		// It's a membership request, check its self information
		// matches the sender
		if m.GetGossipMessage().GetMemReq() != nil {
			sMsg, err := protoext.EnvelopeToGossipMessage(m.GetGossipMessage().GetMemReq().SelfInformation)
			if err != nil {
				g.logger.Warningf("Got membership request with invalid selfInfo: %+v", errors.WithStack(err))
				return
			}
			if !protoext.IsAliveMsg(sMsg.GossipMessage) {
				g.logger.Warning("Got membership request with selfInfo that isn't an AliveMessage")
				return
			}
			if !bytes.Equal(sMsg.GetAliveMsg().Membership.PkiId, m.GetConnectionInfo().ID) {
				g.logger.Warning("Got membership request with selfInfo that doesn't match the handshake")
				return
			}
		}
		g.forwardDiscoveryMsg(m)
	}

	if protoext.IsPullMsg(msg.GossipMessage) && protoext.GetPullMsgType(msg.GossipMessage) == pg.PullMsgType_IDENTITY_MSG {
		g.certStore.handleMessage(m)
	}
}

func (g *Node) forwardDiscoveryMsg(msg protoext.ReceivedMessage) {
	g.discAdapter.incChan <- msg
}

// validateMsg checks the signature of the message if exists,
// and also checks that the tag matches the message type
func (g *Node) validateMsg(msg protoext.ReceivedMessage) bool {
	if err := protoext.IsTagLegal(msg.GetGossipMessage().GossipMessage); err != nil {
		g.logger.Warningf("Tag of %v isn't legal: %v", msg.GetGossipMessage(), errors.WithStack(err))
		return false
	}

	if protoext.IsStateInfoMsg(msg.GetGossipMessage().GossipMessage) {
		if err := g.validateStateInfoMsg(msg.GetGossipMessage()); err != nil {
			g.logger.Warningf("StateInfo message %v is found invalid: %v", msg, err)
			return false
		}
	}
	return true
}

func (g *Node) sendGossipBatch(a []interface{}) {
	msgs2Gossip := make([]*emittedGossipMessage, len(a))
	for i, e := range a {
		msgs2Gossip[i] = e.(*emittedGossipMessage)
	}
	g.gossipBatch(msgs2Gossip)
}

// gossipBatch - This is the method that actually decides to which peers to gossip the message
// batch we possess.
// For efficiency, we first isolate all the messages that have the same routing policy
// and send them together, and only after that move to the next group of messages.
// i.e: we send all blocks of channel C to the same group of peers,
// and send all StateInfo messages to the same group of peers, etc.
// When we send blocks, we send only to peers that advertised themselves in the channel.
// When we send StateInfo messages, we send to peers in the channel.
// When we send messages that are marked to be sent only within the org, we send all of these messages
// to the same set of peers.
// The rest of the messages that have no restrictions on their destinations can be sent
// to any group of peers.
func (g *Node) gossipBatch(msgs []*emittedGossipMessage) {
	if g.disc == nil {
		g.logger.Error("Discovery has not been initialized yet, aborting!")
		return
	}

	var blocks []*emittedGossipMessage
	var stateInfoMsgs []*emittedGossipMessage
	var orgMsgs []*emittedGossipMessage
	var leadershipMsgs []*emittedGossipMessage

	isABlock := func(o interface{}) bool {
		return protoext.IsDataMsg(o.(*emittedGossipMessage).GossipMessage)
	}
	isAStateInfoMsg := func(o interface{}) bool {
		return protoext.IsStateInfoMsg(o.(*emittedGossipMessage).GossipMessage)
	}
	aliveMsgsWithNoEndpointAndInOurOrg := func(o interface{}) bool {
		msg := o.(*emittedGossipMessage)
		if !protoext.IsAliveMsg(msg.GossipMessage) {
			return false
		}
		member := msg.GetAliveMsg().Membership
		return member.Endpoint == "" && g.IsInMyOrg(discovery.NetworkMember{PKIid: member.PkiId})
	}
	isOrgRestricted := func(o interface{}) bool {
		return aliveMsgsWithNoEndpointAndInOurOrg(o) || protoext.IsOrgRestricted(o.(*emittedGossipMessage).GossipMessage)
	}
	isLeadershipMsg := func(o interface{}) bool {
		return protoext.IsLeadershipMsg(o.(*emittedGossipMessage).GossipMessage)
	}

	// Gossip blocks
	blocks, msgs = partitionMessages(isABlock, msgs)
	g.gossipInChan(blocks, func(gc channel.GossipChannel) filter.RoutingFilter {
		return filter.CombineRoutingFilters(gc.EligibleForChannel, gc.IsMemberInChan, g.IsInMyOrg)
	})

	// Gossip Leadership messages
	leadershipMsgs, msgs = partitionMessages(isLeadershipMsg, msgs)
	g.gossipInChan(leadershipMsgs, func(gc channel.GossipChannel) filter.RoutingFilter {
		return filter.CombineRoutingFilters(gc.EligibleForChannel, gc.IsMemberInChan, g.IsInMyOrg)
	})

	// Gossip StateInfo messages
	stateInfoMsgs, msgs = partitionMessages(isAStateInfoMsg, msgs)
	for _, stateInfMsg := range stateInfoMsgs {
		peerSelector := g.IsInMyOrg
		gc := g.chanState.lookupChannelForGossipMsg(stateInfMsg.GossipMessage)
		if gc != nil && g.hasExternalEndpoint(stateInfMsg.GossipMessage.GetStateInfo().PkiId) {
			peerSelector = gc.IsMemberInChan
		}

		peerSelector = filter.CombineRoutingFilters(peerSelector, func(member discovery.NetworkMember) bool {
			return stateInfMsg.filter(member.PKIid)
		})

		peers2Send := filter.SelectPeers(g.conf.PropagatePeerNum, g.disc.GetMembership(), peerSelector)
		g.comm.Send(stateInfMsg.SignedGossipMessage, peers2Send...)
	}

	// Gossip messages restricted to our org
	orgMsgs, msgs = partitionMessages(isOrgRestricted, msgs)
	peers2Send := filter.SelectPeers(g.conf.PropagatePeerNum, g.disc.GetMembership(), g.IsInMyOrg)
	for _, msg := range orgMsgs {
		g.comm.Send(msg.SignedGossipMessage, g.removeSelfLoop(msg, peers2Send)...)
	}

	// Finally, gossip the remaining messages
	for _, msg := range msgs {
		if !protoext.IsAliveMsg(msg.GossipMessage) {
			g.logger.Error("Unknown message type", msg)
			continue
		}
		selectByOriginOrg := g.peersByOriginOrgPolicy(discovery.NetworkMember{PKIid: msg.GetAliveMsg().Membership.PkiId})
		selector := filter.CombineRoutingFilters(selectByOriginOrg, func(member discovery.NetworkMember) bool {
			return msg.filter(member.PKIid)
		})
		peers2Send := filter.SelectPeers(g.conf.PropagatePeerNum, g.disc.GetMembership(), selector)
		g.sendAndFilterSecrets(msg.SignedGossipMessage, peers2Send...)
	}
}

func (g *Node) sendAndFilterSecrets(msg *protoext.SignedGossipMessage, peers ...*comm.RemotePeer) {
	for _, peer := range peers {
		// Prevent forwarding alive messages of external organizations
		// to peers that have no external endpoints
		aliveMsgFromDiffOrg := protoext.IsAliveMsg(msg.GossipMessage) && !g.IsInMyOrg(discovery.NetworkMember{PKIid: msg.GetAliveMsg().Membership.PkiId})
		if aliveMsgFromDiffOrg && !g.hasExternalEndpoint(peer.PKIID) {
			continue
		}

		// Use cloned message to filter secrets to avoid data races when same message is sent multiple times
		clonedMsg := &protoext.SignedGossipMessage{}
		clonedMsg.GossipMessage = msg.GossipMessage
		clonedMsg.Envelope = msg.Envelope

		// Don't gossip secrets
		if !g.IsInMyOrg(discovery.NetworkMember{PKIid: peer.PKIID}) {
			clonedMsg.Envelope = proto.Clone(msg.Envelope).(*pg.Envelope) // clone the envelope
			clonedMsg.Envelope.SecretEnvelope = nil
		}
		g.comm.Send(clonedMsg, peer)
	}
}

// gossipInChan gossips a given GossipMessage slice according to a channel's routing policy.
func (g *Node) gossipInChan(messages []*emittedGossipMessage, chanRoutingFactory channelRoutingFilterFactory) {
	if len(messages) == 0 {
		return
	}
	totalChannels := extractChannels(messages)
	var channel common.ChannelID
	var messagesOfChannel []*emittedGossipMessage
	for len(totalChannels) > 0 {
		// Take first channel
		channel, totalChannels = totalChannels[0], totalChannels[1:]
		// Extract all messages of that channel
		grabMsgs := func(o interface{}) bool {
			return bytes.Equal(o.(*emittedGossipMessage).Channel, channel)
		}
		messagesOfChannel, messages = partitionMessages(grabMsgs, messages)
		if len(messagesOfChannel) == 0 {
			continue
		}
		// Grab channel object for that channel
		gc := g.chanState.getGossipChannelByChainID(channel)
		if gc == nil {
			g.logger.Warning("Channel", channel, "wasn't found")
			continue
		}
		// Select the peers to send the messages to
		// For leadership messages we will select all peers that pass routing factory - e.g. all peers in channel and org
		membership := g.disc.GetMembership()
		var peers2Send []*comm.RemotePeer
		if protoext.IsLeadershipMsg(messagesOfChannel[0].GossipMessage) {
			peers2Send = filter.SelectPeers(len(membership), membership, chanRoutingFactory(gc))
		} else {
			peers2Send = filter.SelectPeers(g.conf.PropagatePeerNum, membership, chanRoutingFactory(gc))
		}

		// Send the messages to the remote peers
		for _, msg := range messagesOfChannel {
			filteredPeers := g.removeSelfLoop(msg, peers2Send)
			g.comm.Send(msg.SignedGossipMessage, filteredPeers...)
		}
	}
}

// removeSelfLoop deletes from the list of peers peer which has sent the message
func (g *Node) removeSelfLoop(msg *emittedGossipMessage, peers []*comm.RemotePeer) []*comm.RemotePeer {
	var result []*comm.RemotePeer
	for _, peer := range peers {
		if msg.filter(peer.PKIID) {
			result = append(result, peer)
		}
	}
	return result
}

// IdentityInfo returns information known peer identities
func (g *Node) IdentityInfo() api.PeerIdentitySet {
	return g.idMapper.IdentityInfo()
}

// SendByCriteria sends a given message to all peers that match the given SendCriteria
func (g *Node) SendByCriteria(msg *protoext.SignedGossipMessage, criteria SendCriteria) error {
	if criteria.MaxPeers == 0 {
		return nil
	}
	if criteria.Timeout == 0 {
		return errors.New("Timeout should be specified")
	}

	if criteria.IsEligible == nil {
		criteria.IsEligible = filter.SelectAllPolicy
	}

	membership := g.disc.GetMembership()

	if len(criteria.Channel) > 0 {
		gc := g.chanState.getGossipChannelByChainID(criteria.Channel)
		if gc == nil {
			return fmt.Errorf("requested to Send for channel %s, but no such channel exists", criteria.Channel)
		}
		membership = gc.GetPeers()
	}

	peers2send := filter.SelectPeers(criteria.MaxPeers, membership, criteria.IsEligible)
	if len(peers2send) < criteria.MinAck {
		return fmt.Errorf("requested to send to at least %d peers, but know only of %d suitable peers", criteria.MinAck, len(peers2send))
	}

	results := g.comm.SendWithAck(msg, criteria.Timeout, criteria.MinAck, peers2send...)

	for _, res := range results {
		if res.Error() == "" {
			continue
		}
		g.logger.Warning("Failed sending to", res.Endpoint, "error:", res.Error())
	}

	if results.AckCount() < criteria.MinAck {
		return errors.New(results.String())
	}
	return nil
}

// Gossip sends a message to other peers to the network
func (g *Node) Gossip(msg *pg.GossipMessage) {
	// Educate developers to Gossip messages with the right tags.
	// See IsTagLegal() for wanted behavior.
	if err := protoext.IsTagLegal(msg); err != nil {
		panic(errors.WithStack(err))
	}

	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: msg,
	}

	var err error
	if protoext.IsDataMsg(sMsg.GossipMessage) {
		sMsg, err = protoext.NoopSign(sMsg.GossipMessage)
	} else {
		_, err = sMsg.Sign(func(msg []byte) ([]byte, error) {
			return g.mcs.Sign(msg)
		})
	}

	if err != nil {
		g.logger.Warningf("Failed signing message: %+v", errors.WithStack(err))
		return
	}

	if protoext.IsChannelRestricted(msg) {
		gc := g.chanState.getGossipChannelByChainID(msg.Channel)
		if gc == nil {
			g.logger.Warning("Failed obtaining gossipChannel of", msg.Channel, "aborting")
			return
		}
		if protoext.IsDataMsg(msg) {
			gc.AddToMsgStore(sMsg)
		}
	}

	if g.conf.PropagateIterations == 0 {
		return
	}
	g.emitter.Add(&emittedGossipMessage{
		SignedGossipMessage: sMsg,
		filter: func(_ common.PKIidType) bool {
			return true
		},
	})
}

// Send sends a message to remote peers
func (g *Node) Send(msg *pg.GossipMessage, peers ...*comm.RemotePeer) {
	m, err := protoext.NoopSign(msg)
	if err != nil {
		g.logger.Warningf("Failed creating SignedGossipMessage: %+v", errors.WithStack(err))
		return
	}
	g.comm.Send(m, peers...)
}

// Peers returns the current alive NetworkMembers
func (g *Node) Peers() []discovery.NetworkMember {
	return g.disc.GetMembership()
}

// PeersOfChannel returns the NetworkMembers considered alive
// and also subscribed to the channel given
func (g *Node) PeersOfChannel(channel common.ChannelID) []discovery.NetworkMember {
	gc := g.chanState.getGossipChannelByChainID(channel)
	if gc == nil {
		g.logger.Debug("No such channel", channel)
		return nil
	}

	return gc.GetPeers()
}

// SelfMembershipInfo returns the peer's membership information
func (g *Node) SelfMembershipInfo() discovery.NetworkMember {
	return g.disc.Self()
}

// SelfChannelInfo returns the peer's latest StateInfo message of a given channel
func (g *Node) SelfChannelInfo(chain common.ChannelID) *protoext.SignedGossipMessage {
	ch := g.chanState.getGossipChannelByChainID(chain)
	if ch == nil {
		return nil
	}
	return ch.Self()
}

// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
// only peer identities that match the given criteria, and that they published their channel participation
func (g *Node) PeerFilter(channel common.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	gc := g.chanState.getGossipChannelByChainID(channel)
	if gc == nil {
		return nil, errors.Errorf("Channel %s doesn't exist", channel)
	}
	return gc.PeerFilter(messagePredicate), nil
}

// Stop stops the gossip component
func (g *Node) Stop() {
	if g.toDie() {
		return
	}
	atomic.StoreInt32(&g.stopFlag, int32(1))
	g.logger.Info("Stopping gossip")
	close(g.toDieChan)
	g.stopSignal.Wait()
	g.chanState.stop()
	g.discAdapter.close()
	g.disc.Stop()
	g.certStore.stop()
	g.emitter.Stop()
	g.ChannelDeMultiplexer.Close()
	g.stateInfoMsgStore.Stop()
	g.comm.Stop()
}

// UpdateMetadata updates gossip membership metadata.
func (g *Node) UpdateMetadata(md []byte) {
	g.disc.UpdateMetadata(md)
}

// UpdateLedgerHeight updates the ledger height the peer
// publishes to other peers in the channel
func (g *Node) UpdateLedgerHeight(height uint64, channelID common.ChannelID) {
	gc := g.chanState.getGossipChannelByChainID(channelID)
	if gc == nil {
		g.logger.Warning("No such channel", channelID)
		return
	}
	gc.UpdateLedgerHeight(height)
}

// UpdateChaincodes updates the chaincodes the peer publishes
// to other peers in the channel
func (g *Node) UpdateChaincodes(chaincodes []*pg.Chaincode, channelID common.ChannelID) {
	gc := g.chanState.getGossipChannelByChainID(channelID)
	if gc == nil {
		g.logger.Warning("No such channel", channelID)
		return
	}
	gc.UpdateChaincodes(chaincodes)
}

// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
// If passThrough is false, the messages are processed by the gossip layer beforehand.
// If passThrough is true, the gossip layer doesn't intervene and the messages
// can be used to send a reply back to the sender
func (g *Node) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *pg.GossipMessage, <-chan protoext.ReceivedMessage) {
	if passThrough {
		return nil, g.comm.Accept(acceptor)
	}
	acceptByType := func(o interface{}) bool {
		if o, isGossipMsg := o.(*pg.GossipMessage); isGossipMsg {
			return acceptor(o)
		}
		if o, isSignedMsg := o.(*protoext.SignedGossipMessage); isSignedMsg {
			sMsg := o
			return acceptor(sMsg.GossipMessage)
		}
		g.logger.Warning("Message type:", reflect.TypeOf(o), "cannot be evaluated")
		return false
	}
	inCh := g.AddChannel(acceptByType)
	outCh := make(chan *pg.GossipMessage, acceptChanSize)
	go func() {
		defer close(outCh)
		for {
			select {
			case <-g.toDieChan:
				return
			case m, channelOpen := <-inCh:
				if !channelOpen {
					return
				}
				select {
				case <-g.toDieChan:
					return
				case outCh <- m.(*protoext.SignedGossipMessage).GossipMessage:
				}
			}
		}
	}()
	return outCh, nil
}

func selectOnlyDiscoveryMessages(m interface{}) bool {
	msg, isGossipMsg := m.(protoext.ReceivedMessage)
	if !isGossipMsg {
		return false
	}
	alive := msg.GetGossipMessage().GetAliveMsg()
	memRes := msg.GetGossipMessage().GetMemRes()
	memReq := msg.GetGossipMessage().GetMemReq()

	selected := alive != nil || memReq != nil || memRes != nil

	return selected
}

func (g *Node) newDiscoveryAdapter() *discoveryAdapter {
	return &discoveryAdapter{
		c:        g.comm,
		stopping: int32(0),
		gossipFunc: func(msg *protoext.SignedGossipMessage) {
			if g.conf.PropagateIterations == 0 {
				return
			}
			g.emitter.Add(&emittedGossipMessage{
				SignedGossipMessage: msg,
				filter: func(_ common.PKIidType) bool {
					return true
				},
			})
		},
		forwardFunc: func(message protoext.ReceivedMessage) {
			if g.conf.PropagateIterations == 0 {
				return
			}
			g.emitter.Add(&emittedGossipMessage{
				SignedGossipMessage: message.GetGossipMessage(),
				filter:              message.GetConnectionInfo().ID.IsNotSameFilter,
			})
		},
		incChan:          make(chan protoext.ReceivedMessage),
		presumedDead:     g.presumedDead,
		disclosurePolicy: g.disclosurePolicy,
	}
}

// discoveryAdapter is used to supply the discovery module with needed abilities
// that the comm interface in the discovery module declares
type discoveryAdapter struct {
	stopping         int32
	c                comm.Comm
	presumedDead     chan common.PKIidType
	incChan          chan protoext.ReceivedMessage
	gossipFunc       func(message *protoext.SignedGossipMessage)
	forwardFunc      func(message protoext.ReceivedMessage)
	disclosurePolicy discovery.DisclosurePolicy
}

func (da *discoveryAdapter) close() {
	atomic.StoreInt32(&da.stopping, int32(1))
	close(da.incChan)
}

func (da *discoveryAdapter) toDie() bool {
	return atomic.LoadInt32(&da.stopping) == int32(1)
}

func (da *discoveryAdapter) Gossip(msg *protoext.SignedGossipMessage) {
	if da.toDie() {
		return
	}

	da.gossipFunc(msg)
}

func (da *discoveryAdapter) Forward(msg protoext.ReceivedMessage) {
	if da.toDie() {
		return
	}

	da.forwardFunc(msg)
}

func (da *discoveryAdapter) SendToPeer(peer *discovery.NetworkMember, msg *protoext.SignedGossipMessage) {
	if da.toDie() {
		return
	}
	// Check membership requests for peers that we know of their PKI-ID.
	// The only peers we don't know about their PKI-IDs are bootstrap peers.
	if memReq := msg.GetMemReq(); memReq != nil && len(peer.PKIid) != 0 {
		selfMsg, err := protoext.EnvelopeToGossipMessage(memReq.SelfInformation)
		if err != nil {
			// Shouldn't happen
			panic(errors.Wrapf(err, "Tried to send a membership request with a malformed AliveMessage"))
		}
		// Apply the EnvelopeFilter of the disclosure policy
		// on the alive message of the selfInfo field of the membership request
		_, omitConcealedFields := da.disclosurePolicy(peer)
		selfMsg.Envelope = omitConcealedFields(selfMsg)
		// Backup old known field
		oldKnown := memReq.Known
		// Override new SelfInfo message with updated envelope
		memReq = &pg.MembershipRequest{
			SelfInformation: selfMsg.Envelope,
			Known:           oldKnown,
		}
		msgCopy := proto.Clone(msg.GossipMessage).(*pg.GossipMessage)

		// Update original message
		msgCopy.Content = &pg.GossipMessage_MemReq{
			MemReq: memReq,
		}
		// Update the envelope of the outer message, no need to sign (point2point)
		msg, err = protoext.NoopSign(msgCopy)
		if err != nil {
			return
		}
		da.c.Send(msg, &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.PreferredEndpoint()})
		return
	}
	da.c.Send(msg, &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.PreferredEndpoint()})
}

func (da *discoveryAdapter) Ping(peer *discovery.NetworkMember) bool {
	err := da.c.Probe(&comm.RemotePeer{Endpoint: peer.PreferredEndpoint(), PKIID: peer.PKIid})
	return err == nil
}

func (da *discoveryAdapter) Accept() <-chan protoext.ReceivedMessage {
	return da.incChan
}

func (da *discoveryAdapter) PresumedDead() <-chan common.PKIidType {
	return da.presumedDead
}

func (da *discoveryAdapter) IdentitySwitch() <-chan common.PKIidType {
	return da.c.IdentitySwitch()
}

func (da *discoveryAdapter) CloseConn(peer *discovery.NetworkMember) {
	da.c.CloseConn(&comm.RemotePeer{PKIID: peer.PKIid})
}

type discoverySecurityAdapter struct {
	identity              api.PeerIdentityType
	includeIdentityPeriod time.Time
	idMapper              identity.Mapper
	sa                    api.SecurityAdvisor
	mcs                   api.MessageCryptoService
	c                     comm.Comm
	logger                util.Logger
}

func (g *Node) newDiscoverySecurityAdapter() *discoverySecurityAdapter {
	return &discoverySecurityAdapter{
		sa:                    g.secAdvisor,
		idMapper:              g.idMapper,
		mcs:                   g.mcs,
		c:                     g.comm,
		logger:                g.logger,
		includeIdentityPeriod: g.includeIdentityPeriod,
		identity:              g.selfIdentity,
	}
}

// validateAliveMsg validates that an Alive message is authentic
func (sa *discoverySecurityAdapter) ValidateAliveMsg(m *protoext.SignedGossipMessage) bool {
	am := m.GetAliveMsg()
	if am == nil || am.Membership == nil || am.Membership.PkiId == nil || !m.IsSigned() {
		sa.logger.Warning("Invalid alive message:", m)
		return false
	}

	var identity api.PeerIdentityType

	// If identity is included inside AliveMessage
	if am.Identity != nil {
		identity = api.PeerIdentityType(am.Identity)
		claimedPKIID := am.Membership.PkiId
		err := sa.idMapper.Put(claimedPKIID, identity)
		if err != nil {
			sa.logger.Debugf("Failed validating identity of %v reason: %+v", am, errors.WithStack(err))
			return false
		}
	} else {
		identity, _ = sa.idMapper.Get(am.Membership.PkiId)
		if identity != nil {
			sa.logger.Debug("Fetched identity of", protoext.MemberToString(am.Membership), "from identity store")
		}
	}

	if identity == nil {
		sa.logger.Debug("Don't have certificate for", am)
		return false
	}

	return sa.validateAliveMsgSignature(m, identity)
}

// SignMessage signs an AliveMessage and updates its signature field
func (sa *discoverySecurityAdapter) SignMessage(m *pg.GossipMessage, internalEndpoint string) *pg.Envelope {
	signer := func(msg []byte) ([]byte, error) {
		return sa.mcs.Sign(msg)
	}
	if protoext.IsAliveMsg(m) && time.Now().Before(sa.includeIdentityPeriod) {
		m.GetAliveMsg().Identity = sa.identity
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: m,
	}
	e, err := sMsg.Sign(signer)
	if err != nil {
		sa.logger.Warningf("Failed signing message: %+v", errors.WithStack(err))
		return nil
	}

	if internalEndpoint == "" {
		return e
	}
	protoext.SignSecret(e, signer, &pg.Secret{
		Content: &pg.Secret_InternalEndpoint{
			InternalEndpoint: internalEndpoint,
		},
	})
	return e
}

func (sa *discoverySecurityAdapter) validateAliveMsgSignature(m *protoext.SignedGossipMessage, identity api.PeerIdentityType) bool {
	am := m.GetAliveMsg()
	// At this point we got the certificate of the peer, proceed to verifying the AliveMessage
	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return sa.mcs.Verify(api.PeerIdentityType(peerIdentity), signature, message)
	}

	// We verify the signature on the message
	err := m.Verify(identity, verifier)
	if err != nil {
		sa.logger.Warningf("Failed verifying: %v: %+v", am, errors.WithStack(err))
		return false
	}

	return true
}

func (g *Node) createCertStorePuller() pull.Mediator {
	conf := pull.Config{
		MsgType:           pg.PullMsgType_IDENTITY_MSG,
		Channel:           []byte(""),
		ID:                g.conf.InternalEndpoint,
		PeerCountToSelect: g.conf.PullPeerNum,
		PullInterval:      g.conf.PullInterval,
		Tag:               pg.GossipMessage_EMPTY,
		PullEngineConfig: algo.PullEngineConfig{
			DigestWaitTime:   g.conf.DigestWaitTime,
			RequestWaitTime:  g.conf.RequestWaitTime,
			ResponseWaitTime: g.conf.ResponseWaitTime,
		},
	}
	pkiIDFromMsg := func(msg *protoext.SignedGossipMessage) string {
		identityMsg := msg.GetPeerIdentity()
		if identityMsg == nil || identityMsg.PkiId == nil {
			return ""
		}
		return string(identityMsg.PkiId)
	}
	certConsumer := func(msg *protoext.SignedGossipMessage) {
		idMsg := msg.GetPeerIdentity()
		if idMsg == nil || idMsg.Cert == nil || idMsg.PkiId == nil {
			g.logger.Warning("Invalid PeerIdentity:", idMsg)
			return
		}
		err := g.idMapper.Put(common.PKIidType(idMsg.PkiId), api.PeerIdentityType(idMsg.Cert))
		if err != nil {
			g.logger.Warningf("Failed associating PKI-ID with certificate: %+v", errors.WithStack(err))
		}
		g.logger.Debug("Learned of a new certificate:", idMsg.Cert)
	}
	adapter := &pull.PullAdapter{
		Sndr:            g.comm,
		MemSvc:          g.disc,
		IdExtractor:     pkiIDFromMsg,
		MsgCons:         certConsumer,
		EgressDigFilter: g.sameOrgOrOurOrgPullFilter,
	}
	return pull.NewPullMediator(conf, adapter)
}

func (g *Node) sameOrgOrOurOrgPullFilter(msg protoext.ReceivedMessage) func(string) bool {
	peersOrg := g.secAdvisor.OrgByPeerIdentity(msg.GetConnectionInfo().Identity)
	if len(peersOrg) == 0 {
		g.logger.Warning("Failed determining organization of", msg.GetConnectionInfo())
		return func(_ string) bool {
			return false
		}
	}

	// If the peer is from our org, gossip all identities
	if bytes.Equal(g.selfOrg, peersOrg) {
		return func(_ string) bool {
			return true
		}
	}
	// Else, the peer is from a different org
	return func(item string) bool {
		pkiID := common.PKIidType(item)
		msgsOrg := g.getOrgOfPeer(pkiID)
		if len(msgsOrg) == 0 {
			g.logger.Warning("Failed determining organization of", pkiID)
			return false
		}
		// Don't gossip identities of dead peers or of peers
		// without external endpoints, to peers of foreign organizations.
		if !g.hasExternalEndpoint(pkiID) {
			return false
		}
		// Peer from our org or identity from our org or identity from peer's org
		return bytes.Equal(msgsOrg, g.selfOrg) || bytes.Equal(msgsOrg, peersOrg)
	}
}

func (g *Node) connect2BootstrapPeers() {
	for _, endpoint := range g.conf.BootstrapPeers {
		endpoint := endpoint
		identifier := func() (*discovery.PeerIdentification, error) {
			remotePeerIdentity, err := g.comm.Handshake(&comm.RemotePeer{Endpoint: endpoint})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			sameOrg := bytes.Equal(g.selfOrg, g.secAdvisor.OrgByPeerIdentity(remotePeerIdentity))
			if !sameOrg {
				return nil, errors.Errorf("%s isn't in our organization, cannot be a bootstrap peer", endpoint)
			}
			pkiID := g.mcs.GetPKIidOfCert(remotePeerIdentity)
			if len(pkiID) == 0 {
				return nil, errors.Errorf("Wasn't able to extract PKI-ID of remote peer with identity of %v", remotePeerIdentity)
			}
			return &discovery.PeerIdentification{ID: pkiID, SelfOrg: sameOrg}, nil
		}
		g.disc.Connect(discovery.NetworkMember{
			InternalEndpoint: endpoint,
			Endpoint:         endpoint,
		}, identifier)
	}
}

func (g *Node) hasExternalEndpoint(PKIID common.PKIidType) bool {
	if nm := g.disc.Lookup(PKIID); nm != nil {
		return nm.Endpoint != ""
	}
	return false
}

// IsInMyOrg checks whether a network member is in this peer's org
func (g *Node) IsInMyOrg(member discovery.NetworkMember) bool {
	if member.PKIid == nil {
		return false
	}
	if org := g.getOrgOfPeer(member.PKIid); org != nil {
		return bytes.Equal(g.selfOrg, org)
	}
	return false
}

func (g *Node) getOrgOfPeer(PKIID common.PKIidType) api.OrgIdentityType {
	cert, err := g.idMapper.Get(PKIID)
	if err != nil {
		return nil
	}

	return g.secAdvisor.OrgByPeerIdentity(cert)
}

func (g *Node) validateLeadershipMessage(msg *protoext.SignedGossipMessage) error {
	pkiID := msg.GetLeadershipMsg().PkiId
	if len(pkiID) == 0 {
		return errors.New("Empty PKI-ID")
	}
	identity, err := g.idMapper.Get(pkiID)
	if err != nil {
		return errors.Wrap(err, "Unable to fetch PKI-ID from id-mapper")
	}
	return msg.Verify(identity, func(peerIdentity []byte, signature, message []byte) error {
		return g.mcs.Verify(identity, signature, message)
	})
}

func (g *Node) validateStateInfoMsg(msg *protoext.SignedGossipMessage) error {
	verifier := func(identity []byte, signature, message []byte) error {
		pkiID := g.idMapper.GetPKIidOfCert(api.PeerIdentityType(identity))
		if pkiID == nil {
			return errors.New("PKI-ID not found in identity mapper")
		}
		return g.idMapper.Verify(pkiID, signature, message)
	}
	identity, err := g.idMapper.Get(msg.GetStateInfo().PkiId)
	if err != nil {
		return errors.WithStack(err)
	}
	return msg.Verify(identity, verifier)
}

func (g *Node) disclosurePolicy(remotePeer *discovery.NetworkMember) (discovery.Sieve, discovery.EnvelopeFilter) {
	remotePeerOrg := g.getOrgOfPeer(remotePeer.PKIid)

	if len(remotePeerOrg) == 0 {
		g.logger.Warning("Cannot determine organization of", remotePeer)
		return func(msg *protoext.SignedGossipMessage) bool {
				return false
			}, func(msg *protoext.SignedGossipMessage) *pg.Envelope {
				return msg.Envelope
			}
	}

	return func(msg *protoext.SignedGossipMessage) bool {
			if !protoext.IsAliveMsg(msg.GossipMessage) {
				g.logger.Panic("Programming error, this should be used only on alive messages")
			}
			org := g.getOrgOfPeer(msg.GetAliveMsg().Membership.PkiId)
			if len(org) == 0 {
				g.logger.Warning("Unable to determine org of message", msg.GossipMessage)
				// Don't disseminate messages who's origin org is unknown
				return false
			}

			// Target org and the message are from the same org
			fromSameForeignOrg := bytes.Equal(remotePeerOrg, org)
			// The message is from my org
			fromMyOrg := bytes.Equal(g.selfOrg, org)
			// Forward to target org only messages from our org, or from the target org itself.
			if !(fromSameForeignOrg || fromMyOrg) {
				return false
			}

			// Pass the alive message only if the alive message is in the same org as the remote peer
			// or the message has an external endpoint, and the remote peer also has one
			return bytes.Equal(org, remotePeerOrg) || msg.GetAliveMsg().Membership.Endpoint != "" && remotePeer.Endpoint != ""
		}, func(msg *protoext.SignedGossipMessage) *pg.Envelope {
			envelope := proto.Clone(msg.Envelope).(*pg.Envelope)
			if !bytes.Equal(g.selfOrg, remotePeerOrg) {
				envelope.SecretEnvelope = nil
			}
			return envelope
		}
}

func (g *Node) peersByOriginOrgPolicy(peer discovery.NetworkMember) filter.RoutingFilter {
	peersOrg := g.getOrgOfPeer(peer.PKIid)
	if len(peersOrg) == 0 {
		g.logger.Warning("Unable to determine organization of peer", peer)
		// Don't disseminate messages who's origin org is undetermined
		return filter.SelectNonePolicy
	}

	if bytes.Equal(g.selfOrg, peersOrg) {
		// Disseminate messages from our org to all known organizations.
		// IMPORTANT: Currently a peer cannot un-join a channel, so the only way
		// of making gossip stop talking to an organization is by having the MSP
		// refuse validating messages from it.
		return filter.SelectAllPolicy
	}

	// Else, select peers from the origin's organization,
	// and also peers from our own organization
	return func(member discovery.NetworkMember) bool {
		memberOrg := g.getOrgOfPeer(member.PKIid)
		if len(memberOrg) == 0 {
			return false
		}
		isFromMyOrg := bytes.Equal(g.selfOrg, memberOrg)
		return isFromMyOrg || bytes.Equal(memberOrg, peersOrg)
	}
}

// partitionMessages receives a predicate and a slice of gossip messages
// and returns a tuple of two slices: the messages that hold for the predicate
// and the rest
func partitionMessages(pred common.MessageAcceptor, a []*emittedGossipMessage) ([]*emittedGossipMessage, []*emittedGossipMessage) {
	s1 := []*emittedGossipMessage{}
	s2 := []*emittedGossipMessage{}
	for _, m := range a {
		if pred(m) {
			s1 = append(s1, m)
		} else {
			s2 = append(s2, m)
		}
	}
	return s1, s2
}

// extractChannels returns a slice with all channels
// of all given GossipMessages
func extractChannels(a []*emittedGossipMessage) []common.ChannelID {
	channels := []common.ChannelID{}
	for _, m := range a {
		if len(m.Channel) == 0 {
			continue
		}
		sameChan := func(a interface{}, b interface{}) bool {
			return bytes.Equal(a.(common.ChannelID), b.(common.ChannelID))
		}
		if util.IndexInSlice(channels, common.ChannelID(m.Channel), sameChan) == -1 {
			channels = append(channels, common.ChannelID(m.Channel))
		}
	}
	return channels
}
