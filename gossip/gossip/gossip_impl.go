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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	prot "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

const (
	presumedDeadChanSize = 100
	acceptChanSize       = 100
)

type gossipServiceImpl struct {
	selfIdentity          api.PeerIdentityType
	includeIdentityPeriod time.Time
	certStore             *certStore
	idMapper              identity.Mapper
	presumedDead          chan common.PKIidType
	disc                  discovery.Discovery
	comm                  comm.Comm
	*comm.ChannelDeMultiplexer
	logger       *util.Logger
	stopSignal   *sync.WaitGroup
	conf         *Config
	toDieChan    chan struct{}
	stopFlag     int32
	msgStore     messageStore
	emitter      batchingEmitter
	goRoutines   []uint64
	discAdapter  *discoveryAdapter
	disSecAdap   *discoverySecurityAdapter
	mcs          api.MessageCryptoService
	blocksPuller pull.Mediator
}

// NewGossipService creates a gossip instance attached to a gRPC server
func NewGossipService(conf *Config, s *grpc.Server, mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType, dialOpts ...grpc.DialOption) Gossip {
	var c comm.Comm
	var err error
	idMapper := identity.NewIdentityMapper(mcs)
	lgr := util.GetLogger(util.LOGGING_GOSSIP_MODULE, conf.ID)
	if s == nil {
		c, err = createCommWithServer(conf.BindPort, idMapper, selfIdentity)
	} else {
		c, err = createCommWithoutServer(s, idMapper, selfIdentity, dialOpts...)
	}

	if err != nil {
		lgr.Error("Failed instntiating communication layer:", err)
		return nil
	}

	g := &gossipServiceImpl{
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
		goRoutines:            make([]uint64, 0),
		includeIdentityPeriod: time.Now().Add(conf.PublishCertPeriod),
	}

	g.emitter = newBatchingEmitter(conf.PropagateIterations,
		conf.MaxPropagationBurstSize, conf.MaxPropagationBurstLatency,
		g.sendGossipBatch)

	g.discAdapter = g.newDiscoveryAdapter(selfIdentity)
	g.disSecAdap = newDiscoverySecurityAdapter(idMapper, mcs, c, g.logger)
	g.disc = discovery.NewDiscoveryService(conf.BootstrapPeers, discovery.NetworkMember{
		Endpoint: conf.SelfEndpoint, PKIid: g.comm.GetPKIid(), Metadata: []byte{},
	}, g.discAdapter, g.disSecAdap)

	g.msgStore = newMessageStore(proto.NewGossipMessageComparator(g.conf.MaxMessageCountToStore), func(m interface{}) {
		g.blocksPuller.Remove(m.(*proto.GossipMessage))
	})

	g.blocksPuller = g.createBlockPuller()

	g.certStore = newCertStore(g.createCertStorePuller(), idMapper, selfIdentity, mcs)

	g.logger.SetLevel(logging.WARNING)

	go g.start()

	return g
}

func (g *gossipServiceImpl) createBlockPuller() pull.Mediator {
	conf := pull.PullConfig{
		MsgType:           proto.PullMsgType_BlockMessage,
		Channel:           []byte(""),
		Id:                g.conf.SelfEndpoint,
		PeerCountToSelect: g.conf.PullPeerNum,
		PullInterval:      g.conf.PullInterval,
		Tag:               proto.GossipMessage_EMPTY,
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
			g.logger.Warning("Invalid DataMessage:", dataMsg)
			return
		}
		added := g.msgStore.add(msg)
		// if we can't add the message to the msgStore,
		// no point in disseminating it to others...
		if !added {
			return
		}
		g.DeMultiplex(msg)
	}
	return pull.NewPullMediator(conf, g.comm, g.disc, seqNumFromMsg, blockConsumer)
}

// NewGossipServiceWithServer creates a new gossip instance with a gRPC server
func NewGossipServiceWithServer(conf *Config, mcs api.MessageCryptoService, identity api.PeerIdentityType) Gossip {
	return NewGossipService(conf, nil, mcs, identity)
}

func createCommWithServer(port int, idStore identity.Mapper, identity api.PeerIdentityType) (comm.Comm, error) {
	return comm.NewCommInstanceWithServer(port, idStore, identity)
}

func createCommWithoutServer(s *grpc.Server, idStore identity.Mapper, identity api.PeerIdentityType, dialOpts ...grpc.DialOption) (comm.Comm, error) {
	return comm.NewCommInstance(s, idStore, identity, dialOpts...)
}

func (g *gossipServiceImpl) toDie() bool {
	return atomic.LoadInt32(&g.stopFlag) == int32(1)
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
	defer g.logger.Debug("Exiting discovery sync loop")
	for !g.toDie() {
		g.logger.Debug("Intiating discovery sync")
		g.disc.InitiateSync(g.conf.PullPeerNum)
		g.logger.Debug("Sleeping", g.conf.PullInterval)
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
	g.logger.Info("Entering,", m)
	defer g.logger.Info("Exiting")
	if m == nil || m.GetGossipMessage() == nil {
		return
	}

	msg := m.GetGossipMessage()

	if msg.IsAliveMsg() || msg.IsDataMsg() {
		if msg.IsAliveMsg() {
			if !g.disSecAdap.ValidateAliveMsg(msg.GetAliveMsg()) {
				g.logger.Warning("AliveMessage", m.GetGossipMessage(), "isn't authentic. Discarding it")
				return
			}

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
		}

		if msg.IsDataMsg() {
			blockMsg := msg.GetDataMsg()
			if blockMsg.Payload == nil {
				g.logger.Warning("Empty block! Discarding it")
				return
			}
			if err := g.mcs.VerifyBlock(blockMsg); err != nil {
				g.logger.Warning("Could not verify block", blockMsg.Payload.SeqNum, "Discarding it")
				return
			}
		}

		added := g.msgStore.add(msg)
		if added {
			g.emitter.Add(msg)
			if dataMsg := m.GetGossipMessage().GetDataMsg(); dataMsg != nil {
				g.blocksPuller.Add(msg)
				g.DeMultiplex(msg)
			}

		}
	}

	if selectOnlyDiscoveryMessages(m) {
		g.forwardDiscoveryMsg(m)
	}

	if msg.IsPullMsg() {
		switch msgType := msg.GetPullMsgType(); msgType {
		case proto.PullMsgType_BlockMessage:
			g.blocksPuller.HandleMessage(m)
		case proto.PullMsgType_IdentityMsg:
			g.certStore.handleMessage(m)
		default:
			g.logger.Warning("Got invalid pull message type:", msgType)
		}
	}
}

func (g *gossipServiceImpl) forwardDiscoveryMsg(msg comm.ReceivedMessage) {
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
	go g.gossipBatch(msgs2Gossip)
}

func (g *gossipServiceImpl) gossipBatch(msgs []*proto.GossipMessage) {
	if g.disc == nil {
		g.logger.Error("Discovery has not been initialized yet, aborting!")
		return
	}
	peers2Send := pull.SelectEndpoints(g.conf.PropagatePeerNum, g.disc.GetMembership())
	for _, msg := range msgs {
		g.comm.Send(msg, peers2Send...)
	}
}

// Gossip sends a message to other peers to the network
func (g *gossipServiceImpl) Gossip(msg *proto.GossipMessage) {
	g.logger.Info(msg)
	if dataMsg := msg.GetDataMsg(); dataMsg != nil {
		g.msgStore.add(msg)
		g.blocksPuller.Add(msg)
	}
	g.emitter.Add(msg)
}

// Send sends a message to remote peers
func (g *gossipServiceImpl) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	g.comm.Send(msg, peers...)
}

// GetPeers returns a mapping of endpoint --> []discovery.NetworkMember
func (g *gossipServiceImpl) GetPeers() []discovery.NetworkMember {
	s := []discovery.NetworkMember{}
	for _, member := range g.disc.GetMembership() {
		s = append(s, member)
	}
	return s
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
	g.discAdapter.close()
	g.disc.Stop()
	g.blocksPuller.Stop()
	g.certStore.stop()
	g.toDieChan <- struct{}{}
	g.emitter.Stop()
	g.ChannelDeMultiplexer.Close()
	g.stopSignal.Wait()
	comWG.Wait()
}

// UpdateMetadata updates the self metadata of the discovery layer
func (g *gossipServiceImpl) UpdateMetadata(md []byte) {
	g.disc.UpdateMetadata(md)
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
	logger   *util.Logger
}

func newDiscoverySecurityAdapter(idMapper identity.Mapper, mcs api.MessageCryptoService, c comm.Comm, logger *util.Logger) *discoverySecurityAdapter {
	return &discoverySecurityAdapter{
		idMapper: idMapper,
		mcs:      mcs,
		c:        c,
		logger:   logger,
	}
}

// validateAliveMsg validates that an Alive message is authentic
func (sa *discoverySecurityAdapter) ValidateAliveMsg(am *proto.AliveMessage) bool {
	if am == nil || am.Membership == nil || am.Membership.PkiID == nil || am.Signature == nil {
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

	sig := am.Signature
	am.Signature = nil
	amIdentity := am.Identity
	am.Identity = nil
	b, err := prot.Marshal(am)
	am.Signature = sig
	am.Identity = amIdentity
	if err != nil {
		sa.logger.Error("Failed marshalling", am, ":", err)
		return false
	}
	err = sa.mcs.Verify(identity, sig, b)
	if err != nil {
		sa.logger.Warning("Failed verifying:", am, ":", err)
		return false
	}
	return true
}

// SignMessage signs an AliveMessage and updates its signature field
func (sa *discoverySecurityAdapter) SignMessage(am *proto.AliveMessage) *proto.AliveMessage {
	am.Signature = nil
	identity := am.Identity
	am.Identity = nil
	b, err := prot.Marshal(am)
	if err != nil {
		sa.logger.Error("Failed marshalling", am, ":", err)
		return am
	}
	b, err = sa.mcs.Sign(b)
	if err != nil {
		sa.logger.Error("Failed signing", am, ":", err)
		return am
	}
	am.Signature = b
	am.Identity = identity
	return am
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
		Id:                g.conf.SelfEndpoint,
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

	}
	return pull.NewPullMediator(conf, g.comm, g.disc, pkiIDFromMsg, certConsumer)
}
