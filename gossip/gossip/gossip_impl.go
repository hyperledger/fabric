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
	"bytes"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/gossip/msgstore"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

const (
	presumedDeadChanSize = 100
	acceptChanSize       = 100
)

type channelRoutingFilterFactory func(channel.GossipChannel) filter.RoutingFilter

type gossipServiceImpl struct {
	selfIdentity          api.PeerIdentityType
	includeIdentityPeriod time.Time
	certStore             *certStore
	idMapper              identity.Mapper
	presumedDead          chan common.PKIidType
	disc                  discovery.Discovery
	comm                  comm.Comm
	incTime               time.Time
	selfOrg               api.OrgIdentityType
	*comm.ChannelDeMultiplexer
	logger            *logging.Logger
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
	aliveMsgStore     msgstore.MessageStore
	stateInfoMsgStore msgstore.MessageStore
}

// NewGossipService creates a gossip instance attached to a gRPC server
func NewGossipService(conf *Config, s *grpc.Server, secAdvisor api.SecurityAdvisor, mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType, dialOpts ...grpc.DialOption) Gossip {
	var c comm.Comm
	var err error
	idMapper := identity.NewIdentityMapper(mcs)
	lgr := util.GetLogger(util.LoggingGossipModule, conf.ID)
	if s == nil {
		c, err = createCommWithServer(conf.BindPort, idMapper, selfIdentity)
	} else {
		c, err = createCommWithoutServer(s, conf.TLSServerCert, idMapper, selfIdentity, dialOpts...)
	}

	if err != nil {
		lgr.Error("Failed instntiating communication layer:", err)
		return nil
	}

	g := &gossipServiceImpl{
		stateInfoMsgStore:     channel.NewStateInfoMessageStore(),
		selfOrg:               secAdvisor.OrgByPeerIdentity(selfIdentity),
		secAdvisor:            secAdvisor,
		selfIdentity:          selfIdentity,
		presumedDead:          make(chan common.PKIidType, presumedDeadChanSize),
		idMapper:              idMapper,
		disc:                  nil,
		mcs:                   mcs,
		comm:                  c,
		conf:                  conf,
		ChannelDeMultiplexer:  comm.NewChannelDemultiplexer(),
		logger:                lgr,
		toDieChan:             make(chan struct{}, 1),
		stopFlag:              int32(0),
		stopSignal:            &sync.WaitGroup{},
		includeIdentityPeriod: time.Now().Add(conf.PublishCertPeriod),
	}

	g.aliveMsgStore = msgstore.NewMessageStore(proto.NewGossipMessageComparator(0), func(m interface{}) {})

	g.chanState = newChannelState(g)
	g.emitter = newBatchingEmitter(conf.PropagateIterations,
		conf.MaxPropagationBurstSize, conf.MaxPropagationBurstLatency,
		g.sendGossipBatch)

	g.discAdapter = g.newDiscoveryAdapter(selfIdentity)
	g.disSecAdap = newDiscoverySecurityAdapter(idMapper, mcs, c, g.logger)
	g.disc = discovery.NewDiscoveryService(conf.BootstrapPeers, discovery.NetworkMember{
		Endpoint: conf.SelfEndpoint, PKIid: g.comm.GetPKIid(), Metadata: []byte{},
	}, g.discAdapter, g.disSecAdap)

	g.certStore = newCertStore(g.createCertStorePuller(), idMapper, selfIdentity, mcs)

	go g.start()

	return g
}

func newChannelState(g *gossipServiceImpl) *channelState {
	return &channelState{
		stopping: int32(0),
		channels: make(map[string]channel.GossipChannel),
		g:        g,
	}
}

func createCommWithoutServer(s *grpc.Server, cert *tls.Certificate, idStore identity.Mapper, identity api.PeerIdentityType, dialOpts ...grpc.DialOption) (comm.Comm, error) {
	return comm.NewCommInstance(s, cert, idStore, identity, dialOpts...)
}

// NewGossipServiceWithServer creates a new gossip instance with a gRPC server
func NewGossipServiceWithServer(conf *Config, secAdvisor api.SecurityAdvisor, mcs api.MessageCryptoService, identity api.PeerIdentityType) Gossip {
	return NewGossipService(conf, nil, secAdvisor, mcs, identity)
}

func createCommWithServer(port int, idStore identity.Mapper, identity api.PeerIdentityType) (comm.Comm, error) {
	return comm.NewCommInstanceWithServer(port, idStore, identity)
}

func (g *gossipServiceImpl) toDie() bool {
	return atomic.LoadInt32(&g.stopFlag) == int32(1)
}

func (g *gossipServiceImpl) JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
	// joinMsg is supposed to have been already verified
	g.chanState.joinChannel(joinMsg, chainID)

	selfPkiID := g.mcs.GetPKIidOfCert(g.selfIdentity)
	for _, ap := range joinMsg.AnchorPeers() {
		if ap.Host == "" {
			g.logger.Warning("Got empty hostname, skipping connecting to anchor peer", ap)
		}
		if ap.Port == 0 {
			g.logger.Warning("Got invalid port (0), skipping connecting to anchor peer", ap)
		}
		pkiID := g.mcs.GetPKIidOfCert(ap.Cert)
		// Skip connecting to self
		if bytes.Equal([]byte(pkiID), []byte(selfPkiID)) {
			g.logger.Info("Anchor peer with same PKI-ID, skipping connecting to myself")
			continue
		}
		endpoint := fmt.Sprintf("%s:%d", ap.Host, ap.Port)
		g.disc.Connect(discovery.NetworkMember{Endpoint: endpoint, PKIid: pkiID})
	}
}

func (g *gossipServiceImpl) handlePresumedDead() {
	defer g.logger.Debug("Exiting")
	g.stopSignal.Add(1)
	defer g.stopSignal.Done()
	for {
		select {
		case s := <-g.toDieChan:
			g.toDieChan <- s
			return
		case deadEndpoint := <-g.comm.PresumedDead():
			g.presumedDead <- deadEndpoint
			break
		}
	}
}

func (g *gossipServiceImpl) syncDiscovery() {
	g.logger.Debugf("Entering discovery sync with interal %ds", g.conf.PullInterval)
	defer g.logger.Debug("Exiting discovery sync loop")
	for !g.toDie() {
		//g.logger.Debug("Intiating discovery sync")
		g.disc.InitiateSync(g.conf.PullPeerNum)
		//g.logger.Debug("Sleeping", g.conf.PullInterval)
		time.Sleep(g.conf.PullInterval)
	}
}

func (g *gossipServiceImpl) start() {
	go g.syncDiscovery()
	go g.handlePresumedDead()

	msgSelector := func(msg interface{}) bool {
		gMsg, isGossipMsg := msg.(comm.ReceivedMessage)
		if !isGossipMsg {
			return false
		}

		isConn := gMsg.GetGossipMessage().GetConn() != nil
		isEmpty := gMsg.GetGossipMessage().GetEmpty() != nil

		return !(isConn || isEmpty)
	}

	incMsgs := g.comm.Accept(msgSelector)

	go g.acceptMessages(incMsgs)

	g.logger.Info("Gossip instance", g.conf.ID, "started")
}

func (g *gossipServiceImpl) acceptMessages(incMsgs <-chan comm.ReceivedMessage) {
	defer g.logger.Debug("Exiting")
	g.stopSignal.Add(1)
	defer g.stopSignal.Done()
	for {
		select {
		case s := <-g.toDieChan:
			g.toDieChan <- s
			return
		case msg := <-incMsgs:
			g.handleMessage(msg)
			break
		}
	}
}

func (g *gossipServiceImpl) handleMessage(m comm.ReceivedMessage) {
	if g.toDie() {
		return
	}

	if m == nil || m.GetGossipMessage() == nil {
		return
	}

	msg := m.GetGossipMessage()

	g.logger.Debug("Entering,", m.GetPKIID(), "sent us", msg)
	defer g.logger.Debug("Exiting")

	if !g.validateMsg(m) {
		g.logger.Warning("Message", msg, "isn't valid")
		return
	}

	if msg.IsAliveMsg() {
		am := msg.GetAliveMsg()
		storedIdentity, _ := g.idMapper.Get(common.PKIidType(am.Membership.PkiID))
		// If peer's certificate is included inside AliveMessage, and we don't have a mapping between
		// its PKI-ID and certificate, create a mapping for it now.
		if identity := am.Identity; identity != nil && storedIdentity == nil {
			err := g.idMapper.Put(common.PKIidType(am.Membership.PkiID), api.PeerIdentityType(identity))
			if err != nil {
				g.logger.Warning("Failed adding identity of", am, "into identity store:", err)
				return
			}
			g.logger.Info("Learned identity of", am.Membership.PkiID)
		}

		added := g.aliveMsgStore.Add(msg)
		if !added {
			return
		}
		g.emitter.Add(msg)
	}

	if msg.IsChannelRestricted() {
		if gc := g.chanState.getGossipChannelByChainID(msg.Channel); gc == nil {
			// If we're not in the channel but we should forward to peers of our org
			if g.isInMyorg(discovery.NetworkMember{PKIid: m.GetPKIID()}) && msg.IsStateInfoMsg() {
				if g.stateInfoMsgStore.Add(msg) {
					g.emitter.Add(msg)
				}
			}
			if !g.toDie() {
				g.logger.Warning("No such channel", msg.Channel, "discarding message", msg)
			}
		} else {
			gc.HandleMessage(m)
		}
		return
	}

	if selectOnlyDiscoveryMessages(m) {
		g.forwardDiscoveryMsg(m)
	}

	if msg.IsPullMsg() && msg.GetPullMsgType() == proto.PullMsgType_IdentityMsg {
		g.certStore.handleMessage(m)
	}
}

func (g *gossipServiceImpl) forwardDiscoveryMsg(msg comm.ReceivedMessage) {
	defer func() { // can be closed while shutting down
		recover()
	}()
	g.discAdapter.incChan <- msg.GetGossipMessage()
}

// validateMsg checks the signature of the message if exists,
// and also checks that the tag matches the message type
func (g *gossipServiceImpl) validateMsg(msg comm.ReceivedMessage) bool {
	if err := msg.GetGossipMessage().IsTagLegal(); err != nil {
		g.logger.Warning("Tag of", msg.GetGossipMessage(), "isn't legal:", err)
		return false
	}

	if msg.GetGossipMessage().IsAliveMsg() {
		if !g.disSecAdap.ValidateAliveMsg(msg.GetGossipMessage()) {
			return false
		}
	}

	if msg.GetGossipMessage().IsDataMsg() {
		blockMsg := msg.GetGossipMessage().GetDataMsg()
		if blockMsg.Payload == nil {
			g.logger.Warning("Empty block! Discarding it")
			return false
		}

		// If we're configured to skip block validation, don't verify it
		if g.conf.SkipBlockVerification {
			return true
		}

		if err := g.mcs.VerifyBlock(msg.GetGossipMessage().Channel, blockMsg); err != nil {
			g.logger.Warning("Could not verify block", blockMsg.Payload.SeqNum, ":", err)
			return false
		}
	}

	if msg.GetGossipMessage().IsStateInfoMsg() {
		if err := g.validateStateInfoMsg(msg.GetGossipMessage()); err != nil {
			g.logger.Warning("StateInfo message", msg, "is found invalid:", err)
			return false
		}
	}
	return true
}

func (g *gossipServiceImpl) forwardToDiscoveryLayer(msg comm.ReceivedMessage) {
	defer func() { // can be closed while shutting down
		recover()
	}()
	g.discAdapter.incChan <- msg.GetGossipMessage()
}

func (g *gossipServiceImpl) sendGossipBatch(a []interface{}) {
	msgs2Gossip := make([]*proto.GossipMessage, len(a))
	for i, e := range a {
		msgs2Gossip[i] = e.(*proto.GossipMessage)
	}
	g.gossipBatch(msgs2Gossip)
}

// gossipBatch - This is the method that actually decides to which peers to gossip the message
// batch we possess.
// For efficiency, we first isolate all the messages that have the same routing policy
// and send them together, and only after that move to the next group of messages.
// i.e: we send all blocks of channel C to the same group of peers,
// and send all StateInfo messages to the same group of peers, etc. etc.
// When we send blocks, we send only to peers that advertised themselves in the channel.
// When we send StateInfo messages, we send to peers in the channel.
// When we send messages that are marked to be sent only within the org, we send all of these messages
// to the same set of peers.
// The rest of the messages that have no restrictions on their destinations can be sent
// to any group of peers.
func (g *gossipServiceImpl) gossipBatch(msgs []*proto.GossipMessage) {
	if g.disc == nil {
		g.logger.Error("Discovery has not been initialized yet, aborting!")
		return
	}

	var blocks []*proto.GossipMessage
	var stateInfoMsgs []*proto.GossipMessage
	var orgMsgs []*proto.GossipMessage
	var leadershipMsgs []*proto.GossipMessage

	isABlock := func(o interface{}) bool {
		return o.(*proto.GossipMessage).IsDataMsg()
	}
	isAStateInfoMsg := func(o interface{}) bool {
		return o.(*proto.GossipMessage).IsStateInfoMsg()
	}
	isOrgRestricted := func(o interface{}) bool {
		return o.(*proto.GossipMessage).IsOrgRestricted()
	}
	isLeadershipMsg := func(o interface{}) bool {
		return o.(*proto.GossipMessage).IsLeadershipMsg()
	}

	// Gossip blocks
	blocks, msgs = partitionMessages(isABlock, msgs)
	g.gossipInChan(blocks, func(gc channel.GossipChannel) filter.RoutingFilter {
		return filter.CombineRoutingFilters(gc.IsSubscribed, gc.IsMemberInChan, g.isInMyorg)
	})

	// Gossip StateInfo messages
	stateInfoMsgs, msgs = partitionMessages(isAStateInfoMsg, msgs)
	g.gossipInChan(stateInfoMsgs, func(gc channel.GossipChannel) filter.RoutingFilter {
		return gc.IsMemberInChan
	})

	//Gossip Leadership messages
	leadershipMsgs, msgs = partitionMessages(isLeadershipMsg, msgs)
	g.gossipInChan(leadershipMsgs, func(gc channel.GossipChannel) filter.RoutingFilter {
		return filter.CombineRoutingFilters(gc.IsSubscribed, gc.IsMemberInChan, g.isInMyorg)
	})

	// Gossip messages restricted to our org
	orgMsgs, msgs = partitionMessages(isOrgRestricted, msgs)
	peers2Send := filter.SelectPeers(g.conf.PropagatePeerNum, g.disc.GetMembership(), g.isInMyorg)
	for _, msg := range orgMsgs {
		g.comm.Send(msg, peers2Send...)
	}

	// Finally, gossip the remaining messages
	peers2Send = filter.SelectPeers(g.conf.PropagatePeerNum, g.disc.GetMembership())
	for _, msg := range msgs {
		g.comm.Send(msg, peers2Send...)
	}
}

// gossipInChan gossips a given GossipMessage slice according to a channel's routing policy.
func (g *gossipServiceImpl) gossipInChan(messages []*proto.GossipMessage, chanRoutingFactory channelRoutingFilterFactory) {
	if len(messages) == 0 {
		return
	}
	totalChannels := extractChannels(messages)
	var channel common.ChainID
	var messagesOfChannel []*proto.GossipMessage
	for len(totalChannels) > 0 {
		// Take first channel
		channel, totalChannels = totalChannels[0], totalChannels[1:]
		// Extract all messages of that channel
		grabMsgs := func(o interface{}) bool {
			return bytes.Equal(o.(*proto.GossipMessage).Channel, channel)
		}
		messagesOfChannel, messages = partitionMessages(grabMsgs, messages)
		// Grab channel object for that channel
		gc := g.chanState.getGossipChannelByChainID(channel)
		if gc == nil {
			g.logger.Warning("Channel", channel, "wasn't found")
			continue
		}
		// Select the peers to send the messages to
		// For leadership messages we will select all peers that pass routing factory - e.g. all peers in channel and org
		membership := g.disc.GetMembership()
		allPeersInCh := filter.SelectPeers(len(membership), membership, chanRoutingFactory(gc))
		peers2Send := filter.SelectPeers(g.conf.PropagatePeerNum, membership, chanRoutingFactory(gc))
		// Send the messages to the remote peers
		for _, msg := range messagesOfChannel {
			if msg.IsLeadershipMsg() {
				g.comm.Send(msg, allPeersInCh...)
			} else {
				g.comm.Send(msg, peers2Send...)
			}
		}
	}
}

// Gossip sends a message to other peers to the network
func (g *gossipServiceImpl) Gossip(msg *proto.GossipMessage) {
	// Educate developers to Gossip messages with the right tags.
	// See IsTagLegal() for wanted behavior.
	if err := msg.IsTagLegal(); err != nil {
		panic(err)
	}

	if msg.IsChannelRestricted() {
		gc := g.chanState.getGossipChannelByChainID(msg.Channel)
		if gc == nil {
			g.logger.Warning("Failed obtaining gossipChannel of", msg.Channel, "aborting")
			return
		}
		if msg.IsDataMsg() {
			gc.AddToMsgStore(msg)
		}
	}

	if g.conf.PropagateIterations == 0 {
		return
	}
	g.emitter.Add(msg)
}

// Send sends a message to remote peers
func (g *gossipServiceImpl) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	g.comm.Send(msg, peers...)
}

// GetPeers returns a mapping of endpoint --> []discovery.NetworkMember
func (g *gossipServiceImpl) Peers() []discovery.NetworkMember {
	s := []discovery.NetworkMember{}
	for _, member := range g.disc.GetMembership() {
		s = append(s, member)
	}
	return s

}

// PeersOfChannel returns the NetworkMembers considered alive
// and also subscribed to the channel given
func (g *gossipServiceImpl) PeersOfChannel(channel common.ChainID) []discovery.NetworkMember {
	gc := g.chanState.getGossipChannelByChainID(channel)
	if gc == nil {
		g.logger.Warning("No such channel", channel)
		return nil
	}

	return gc.GetPeers()
}

// Stop stops the gossip component
func (g *gossipServiceImpl) Stop() {
	if g.toDie() {
		return
	}
	atomic.StoreInt32((&g.stopFlag), int32(1))
	g.logger.Info("Stopping gossip")
	comWG := sync.WaitGroup{}
	comWG.Add(1)
	go func() {
		defer comWG.Done()
		g.comm.Stop()
	}()
	g.chanState.stop()
	g.discAdapter.close()
	g.disc.Stop()
	g.certStore.stop()
	g.toDieChan <- struct{}{}
	g.emitter.Stop()
	g.ChannelDeMultiplexer.Close()
	g.stopSignal.Wait()
	comWG.Wait()
}

func (g *gossipServiceImpl) UpdateMetadata(md []byte) {
	g.disc.UpdateMetadata(md)
}

// UpdateChannelMetadata updates the self metadata the peer
// publishes to other peers about its channel-related state
func (g *gossipServiceImpl) UpdateChannelMetadata(md []byte, chainID common.ChainID) {
	gc := g.chanState.getGossipChannelByChainID(chainID)
	if gc == nil {
		g.logger.Warning("No such channel", chainID)
		return
	}
	stateInfMsg, err := g.createStateInfoMsg(md, chainID)
	if err != nil {
		g.logger.Error("Failed creating StateInfo message")
		return
	}
	gc.UpdateStateInfo(stateInfMsg)
}

// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
// If passThrough is false, the messages are processed by the gossip layer beforehand.
// If passThrough is true, the gossip layer doesn't intervene and the messages
// can be used to send a reply back to the sender
func (g *gossipServiceImpl) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan comm.ReceivedMessage) {
	if passThrough {
		return nil, g.comm.Accept(acceptor)
	}
	inCh := g.AddChannel(acceptor)
	outCh := make(chan *proto.GossipMessage, acceptChanSize)
	go func() {
		for {
			select {
			case s := <-g.toDieChan:
				g.toDieChan <- s
				return
			case m := (<-inCh):
				if m == nil {
					return
				}
				outCh <- m.(*proto.GossipMessage)
				break
			}
		}
	}()
	return outCh, nil
}

func selectOnlyDiscoveryMessages(m interface{}) bool {
	msg, isGossipMsg := m.(comm.ReceivedMessage)
	if !isGossipMsg {
		return false
	}
	alive := msg.GetGossipMessage().GetAliveMsg()
	memRes := msg.GetGossipMessage().GetMemRes()
	memReq := msg.GetGossipMessage().GetMemReq()

	selected := alive != nil || memReq != nil || memRes != nil

	return selected
}

func (g *gossipServiceImpl) newDiscoveryAdapter(identity api.PeerIdentityType) *discoveryAdapter {
	return &discoveryAdapter{
		identity:              identity,
		includeIdentityPeriod: g.includeIdentityPeriod,
		c:        g.comm,
		stopping: int32(0),
		gossipFunc: func(msg *proto.GossipMessage) {
			g.Gossip(msg)
		},
		incChan:      make(chan *proto.GossipMessage),
		presumedDead: g.presumedDead,
	}
}

// discoveryAdapter is used to supply the discovery module with needed abilities
// that the comm interface in the discovery module declares
type discoveryAdapter struct {
	includeIdentityPeriod time.Time
	identity              api.PeerIdentityType
	mcs                   api.MessageCryptoService
	stopping              int32
	c                     comm.Comm
	presumedDead          chan common.PKIidType
	incChan               chan *proto.GossipMessage
	gossipFunc            func(*proto.GossipMessage)
}

func (da *discoveryAdapter) close() {
	atomic.StoreInt32((&da.stopping), int32(1))
	close(da.incChan)
}

func (da *discoveryAdapter) toDie() bool {
	return atomic.LoadInt32(&da.stopping) == int32(1)
}

func (da *discoveryAdapter) Gossip(msg *proto.GossipMessage) {
	if da.toDie() {
		return
	}
	if msg.IsAliveMsg() && time.Now().Before(da.includeIdentityPeriod) {
		msg.GetAliveMsg().Identity = da.identity
	}
	da.gossipFunc(msg)
}

func (da *discoveryAdapter) SendToPeer(peer *discovery.NetworkMember, msg *proto.GossipMessage) {
	if da.toDie() {
		return
	}
	da.c.Send(msg, &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.Endpoint})
}

func (da *discoveryAdapter) Ping(peer *discovery.NetworkMember) bool {
	return da.c.Probe(&comm.RemotePeer{Endpoint: peer.Endpoint, PKIID: peer.PKIid}) == nil
}

func (da *discoveryAdapter) Accept() <-chan *proto.GossipMessage {
	return da.incChan
}

func (da *discoveryAdapter) PresumedDead() <-chan common.PKIidType {
	return da.presumedDead
}

func (da *discoveryAdapter) CloseConn(peer *discovery.NetworkMember) {
	da.c.CloseConn(&comm.RemotePeer{Endpoint: peer.Endpoint, PKIID: peer.PKIid})
}

type discoverySecurityAdapter struct {
	idMapper identity.Mapper
	mcs      api.MessageCryptoService
	c        comm.Comm
	logger   *logging.Logger
}

func newDiscoverySecurityAdapter(idMapper identity.Mapper, mcs api.MessageCryptoService, c comm.Comm, logger *logging.Logger) *discoverySecurityAdapter {
	return &discoverySecurityAdapter{
		idMapper: idMapper,
		mcs:      mcs,
		c:        c,
		logger:   logger,
	}
}

// validateAliveMsg validates that an Alive message is authentic
func (sa *discoverySecurityAdapter) ValidateAliveMsg(m *proto.GossipMessage) bool {
	am := m.GetAliveMsg()
	if am == nil || am.Membership == nil || am.Membership.PkiID == nil || m.Signature == nil {
		sa.logger.Warning("Invalid alive message:", am)
		return false
	}

	var identity api.PeerIdentityType

	// If identity is included inside AliveMessage
	if am.Identity != nil {
		identity = api.PeerIdentityType(am.Identity)
		calculatedPKIID := sa.mcs.GetPKIidOfCert(identity)
		claimedPKIID := am.Membership.PkiID
		if !bytes.Equal(calculatedPKIID, claimedPKIID) {
			sa.logger.Warning("Calculated pkiID doesn't match identity:", calculatedPKIID, claimedPKIID)
			return false
		}
		err := sa.mcs.ValidateIdentity(api.PeerIdentityType(identity))
		if err != nil {
			sa.logger.Warning("Failed validating identity of", am, "reason:", err)
			return false
		}
	} else {
		identity, _ = sa.idMapper.Get(am.Membership.PkiID)
		if identity != nil {
			sa.logger.Debug("Fetched identity of", am.Membership.PkiID, "from identity store")
		}
	}

	if identity == nil {
		sa.logger.Warning("Don't have certificate for", am)
		return false
	}

	// At this point we got the certificate of the peer, proceed to verifying the AliveMessage
	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return sa.mcs.Verify(api.PeerIdentityType(peerIdentity), signature, message)
	}

	rawIdentity := m.GetAliveMsg().Identity
	m.GetAliveMsg().Identity = nil
	defer func() {
		m.GetAliveMsg().Identity = rawIdentity
	}()
	err := m.Verify(identity, verifier)
	if err != nil {
		sa.logger.Warning("Failed verifying:", am, ":", err)
		return false
	}
	return true
}

// SignMessage signs an AliveMessage and updates its signature field
func (sa *discoverySecurityAdapter) SignMessage(m *proto.GossipMessage) *proto.GossipMessage {
	am := m.GetAliveMsg()
	signer := func(msg []byte) ([]byte, error) {
		return sa.mcs.Sign(msg)
	}
	identity := am.Identity
	am.Identity = nil
	defer func() {
		am.Identity = identity
	}()
	err := m.Sign(signer)
	if err != nil {
		sa.logger.Error("Failed signing", am, ":", err)
		return nil
	}
	return m
}

func (g *gossipServiceImpl) peersWithEndpoints(endpoints ...string) []*comm.RemotePeer {
	peers := []*comm.RemotePeer{}
	for _, member := range g.disc.GetMembership() {
		for _, endpoint := range endpoints {
			if member.Endpoint == endpoint {
				peers = append(peers, &comm.RemotePeer{Endpoint: member.Endpoint, PKIID: member.PKIid})
			}
		}
	}
	return peers
}

func (g *gossipServiceImpl) createCertStorePuller() pull.Mediator {
	conf := pull.PullConfig{
		MsgType:           proto.PullMsgType_IdentityMsg,
		Channel:           []byte(""),
		ID:                g.conf.SelfEndpoint,
		PeerCountToSelect: g.conf.PullPeerNum,
		PullInterval:      g.conf.PullInterval,
		Tag:               proto.GossipMessage_EMPTY,
	}
	pkiIDFromMsg := func(msg *proto.GossipMessage) string {
		identityMsg := msg.GetPeerIdentity()
		if identityMsg == nil || identityMsg.PkiID == nil {
			return ""
		}
		return fmt.Sprintf("%s", string(identityMsg.PkiID))
	}
	certConsumer := func(msg *proto.GossipMessage) {
		idMsg := msg.GetPeerIdentity()
		if idMsg == nil || idMsg.Cert == nil || idMsg.PkiID == nil {
			g.logger.Warning("Invalid PeerIdentity:", idMsg)
			return
		}
		err := g.idMapper.Put(common.PKIidType(idMsg.PkiID), api.PeerIdentityType(idMsg.Cert))
		if err != nil {
			g.logger.Warning("Failed associating PKI-ID with certificate:", err)
		}
		g.logger.Info("Learned of a new certificate:", idMsg.Cert)

	}
	return pull.NewPullMediator(conf, g.comm, g.disc, pkiIDFromMsg, certConsumer)
}

func (g *gossipServiceImpl) createStateInfoMsg(metadata []byte, chainID common.ChainID) (*proto.GossipMessage, error) {
	stateInfMsg := &proto.StateInfo{
		Metadata: metadata,
		PkiID:    g.comm.GetPKIid(),
		Timestamp: &proto.PeerTime{
			IncNumber: uint64(g.incTime.UnixNano()),
			SeqNum:    uint64(time.Now().UnixNano()),
		},
	}

	m := &proto.GossipMessage{
		Channel: chainID,
		Nonce:   0,
		Tag:     proto.GossipMessage_CHAN_OR_ORG,
		Content: &proto.GossipMessage_StateInfo{
			StateInfo: stateInfMsg,
		},
	}

	signer := func(msg []byte) ([]byte, error) {
		return g.mcs.Sign(msg)
	}

	err := m.Sign(signer)
	if err != nil {
		g.logger.Error("Failed signing StateInfo message: ", err)
		return nil, err
	}
	return m, nil
}

func (g *gossipServiceImpl) isInMyorg(member discovery.NetworkMember) bool {
	if member.PKIid == nil {
		return false
	}
	if org := g.getOrgOfPeer(member.PKIid); org != nil {
		return bytes.Equal(g.selfOrg, org)
	}
	return false
}

func (g *gossipServiceImpl) getOrgOfPeer(PKIID common.PKIidType) api.OrgIdentityType {
	cert, err := g.idMapper.Get(PKIID)
	if err != nil {
		g.logger.Error("Failed getting certificate by PKIid:", PKIID, ":", err)
		return nil
	}

	return g.secAdvisor.OrgByPeerIdentity(cert)
}

func (g *gossipServiceImpl) validateStateInfoMsg(msg *proto.GossipMessage) error {
	verifier := func(identity []byte, signature, message []byte) error {
		pkiID := g.idMapper.GetPKIidOfCert(api.PeerIdentityType(identity))
		if pkiID == nil {
			return fmt.Errorf("PKI-ID not found in identity mapper")
		}
		return g.idMapper.Verify(pkiID, signature, message)
	}
	identity, err := g.idMapper.Get(msg.GetStateInfo().PkiID)
	if err != nil {
		return err
	}
	return msg.Verify(identity, verifier)
}

// partitionMessages receives a predicate and a slice of gossip messages
// and returns a tuple of two slices: the messages that hold for the predicate
// and the rest
func partitionMessages(pred common.MessageAcceptor, a []*proto.GossipMessage) ([]*proto.GossipMessage, []*proto.GossipMessage) {
	s1 := []*proto.GossipMessage{}
	s2 := []*proto.GossipMessage{}
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
func extractChannels(a []*proto.GossipMessage) []common.ChainID {
	channels := []common.ChainID{}
	for _, m := range a {
		if len(m.Channel) == 0 {
			continue
		}
		sameChan := func(a interface{}, b interface{}) bool {
			return bytes.Equal(a.(common.ChainID), b.(common.ChainID))
		}
		if util.IndexInSlice(channels, common.ChainID(m.Channel), sameChan) == -1 {
			channels = append(channels, common.ChainID(m.Channel))
		}
	}
	return channels
}
