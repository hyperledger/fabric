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
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/op/go-logging"
)

const (
	presumedDeadChanSize = 100
	acceptChanSize       = 100
)

type gossipServiceImpl struct {
	presumedDead chan common.PKIidType
	disc         discovery.Discovery
	comm         comm.Comm
	*comm.ChannelDeMultiplexer
	logger      *util.Logger
	stopSignal  *sync.WaitGroup
	conf        *Config
	toDieChan   chan struct{}
	stopFlag    int32
	msgStore    messageStore
	emitter     batchingEmitter
	pushPull    *algo.PullEngine
	goRoutines  []uint64
	discAdapter *discoveryAdapter
}

// NewGossipService creates a new gossip instance
func NewGossipService(conf *Config, c comm.Comm, crypto discovery.CryptoService) Gossip {
	g := &gossipServiceImpl{
		presumedDead:         make(chan common.PKIidType, presumedDeadChanSize),
		disc:                 nil,
		comm:                 c,
		conf:                 conf,
		ChannelDeMultiplexer: comm.NewChannelDemultiplexer(),
		logger:               util.GetLogger(util.LOGGING_GOSSIP_MODULE, conf.ID),
		toDieChan:            make(chan struct{}, 1),
		stopFlag:             int32(0),
		stopSignal:           &sync.WaitGroup{},
		goRoutines:           make([]uint64, 0),
	}

	g.emitter = newBatchingEmitter(conf.PropagateIterations,
		conf.MaxPropagationBurstSize, conf.MaxPropagationBurstLatency,
		g.sendGossipBatch)

	g.discAdapter = g.newDiscoveryAdapter()

	g.disc = discovery.NewDiscoveryService(conf.BootstrapPeers, discovery.NetworkMember{
		Endpoint: conf.SelfEndpoint, PKIid: g.comm.GetPKIid(), Metadata: []byte{},
	}, g.discAdapter, crypto)

	g.pushPull = algo.NewPullEngine(g, conf.PullInterval)

	g.msgStore = newMessageStore(proto.NewGossipMessageComparator(g.conf.MaxMessageCountToStore), func(m interface{}) {
		if dataMsg, isDataMsg := m.(*proto.DataMessage); isDataMsg {
			g.pushPull.Remove(dataMsg.Payload.SeqNum)
		}
	})

	g.logger.SetLevel(logging.WARNING)

	go g.start()

	return g
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

func (g *gossipServiceImpl) SelectPeers() []string {
	if g.disc == nil {
		return []string{}
	}
	peers := selectEndpoints(g.conf.PullPeerNum, g.disc.GetMembership())
	g.logger.Debug("Selected", len(peers), "peers")
	return peers
}

func (g *gossipServiceImpl) Hello(dest string, nonce uint64) {
	helloMsg := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_Hello{
			Hello: &proto.GossipHello{
				Nonce: nonce,
			},
		},
	}

	g.logger.Debug("Sending hello to", dest)
	g.comm.Send(helloMsg, g.peersWithEndpoints(dest)...)

}

func (g *gossipServiceImpl) SendDigest(digest []uint64, nonce uint64, context interface{}) {
	digMsg := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_DataDig{
			DataDig: &proto.DataDigest{
				Nonce:  nonce,
				SeqMap: digest,
			},
		},
	}
	g.logger.Debug("Sending digest", digMsg.GetDataDig().SeqMap)
	context.(comm.ReceivedMessage).Respond(digMsg)
}

func (g *gossipServiceImpl) SendReq(dest string, items []uint64, nonce uint64) {
	req := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_DataReq{
			DataReq: &proto.DataRequest{
				Nonce:  nonce,
				SeqMap: items,
			},
		},
	}
	g.logger.Debug("Sending", req, "to", dest)
	g.comm.Send(req, g.peersWithEndpoints(dest)...)
}

func (g *gossipServiceImpl) SendRes(requestedItems []uint64, context interface{}, nonce uint64) {
	itemMap := make(map[uint64]*proto.DataMessage)
	for _, msg := range g.msgStore.get() {
		if dataMsg := msg.(*proto.GossipMessage).GetDataMsg(); dataMsg != nil {
			itemMap[dataMsg.Payload.SeqNum] = dataMsg
		}
	}

	dataMsgs := []*proto.DataMessage{}

	for _, item := range requestedItems {
		if dataMsg, exists := itemMap[item]; exists {
			dataMsgs = append(dataMsgs, dataMsg)
		}
	}

	returnedUpdate := &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: 0,
		Content: &proto.GossipMessage_DataUpdate{
			DataUpdate: &proto.DataUpdate{
				Nonce: nonce,
				Data:  dataMsgs,
			},
		},
	}

	g.logger.Debug("Sending response", returnedUpdate.GetDataUpdate().Data)
	context.(comm.ReceivedMessage).Respond(returnedUpdate)
}

func (g *gossipServiceImpl) handleMessage(msg comm.ReceivedMessage) {
	if g.toDie() {
		return
	}
	g.logger.Info("Entering,", msg)
	defer g.logger.Info("Exiting")
	if msg == nil {
		return
	}

	if selectOnlyDiscoveryMessages(msg) {
		g.forwardDiscoveryMsg(msg)
	}

	// TODO: add only validated alive messages!
	if msg.GetGossipMessage().GetAliveMsg() != nil || msg.GetGossipMessage().GetDataMsg() != nil {
		added := g.msgStore.add(msg.GetGossipMessage())
		if !added {
			g.logger.Debug("Didn't add", msg, "to store")
			return
		}

		g.emitter.Add(msg.GetGossipMessage())

		if dataMsg := msg.GetGossipMessage().GetDataMsg(); dataMsg != nil {
			g.DeMultiplex(msg.GetGossipMessage())
			g.pushPull.Add(dataMsg.Payload.SeqNum)
		}

		return
	}

	if msg.GetGossipMessage().GetDataReq() != nil || msg.GetGossipMessage().GetDataUpdate() != nil ||
		msg.GetGossipMessage().GetHello() != nil || msg.GetGossipMessage().GetDataDig() != nil {
		g.handlePushPullMsg(msg)
	}
}

func (g *gossipServiceImpl) forwardDiscoveryMsg(msg comm.ReceivedMessage) {
	defer func() { // can be closed while shutting down
		recover()
	}()
	g.discAdapter.incChan <- msg.GetGossipMessage()
}

func (g *gossipServiceImpl) handlePushPullMsg(msg comm.ReceivedMessage) {
	g.logger.Debug(msg)
	if helloMsg := msg.GetGossipMessage().GetHello(); helloMsg != nil {
		g.pushPull.OnHello(helloMsg.Nonce, msg)
	}
	if digest := msg.GetGossipMessage().GetDataDig(); digest != nil {
		g.pushPull.OnDigest(digest.SeqMap, digest.Nonce, msg)
	}
	if req := msg.GetGossipMessage().GetDataReq(); req != nil {
		g.pushPull.OnReq(req.SeqMap, req.Nonce, msg)
	}
	if res := msg.GetGossipMessage().GetDataUpdate(); res != nil {
		items := make([]uint64, len(res.Data))
		for i, data := range res.Data {
			dataMsg := &proto.GossipMessage{
				Tag: proto.GossipMessage_EMPTY,
				Content: &proto.GossipMessage_DataMsg{
					DataMsg: data,
				},
				Nonce: msg.GetGossipMessage().Nonce,
			}
			added := g.msgStore.add(dataMsg)
			// if we can't add the message to the msgStore,
			// no point in disseminating it to others...
			if !added {
				continue
			}
			g.DeMultiplex(dataMsg)
			items[i] = data.Payload.SeqNum
		}
		g.pushPull.OnRes(items, res.Nonce)
	}
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
	peers2Send := selectEndpoints(g.conf.PropagatePeerNum, g.disc.GetMembership())
	for _, msg := range msgs {
		g.comm.Send(msg, g.peersWithEndpoints(peers2Send...)...)
	}
}

func selectEndpoints(k int, peerPool []discovery.NetworkMember) []string {
	if len(peerPool) < k {
		k = len(peerPool)
	}

	indices := util.GetRandomIndices(k, len(peerPool)-1)
	endpoints := make([]string, len(indices))
	for i, j := range indices {
		endpoints[i] = peerPool[j].Endpoint
	}
	return endpoints
}

func (g *gossipServiceImpl) Gossip(msg *proto.GossipMessage) {
	g.logger.Info(msg)
	if dataMsg := msg.GetDataMsg(); dataMsg != nil {
		g.msgStore.add(msg)
		g.pushPull.Add(dataMsg.Payload.SeqNum)
	}
	g.emitter.Add(msg)
}

func (g *gossipServiceImpl) GetPeers() []discovery.NetworkMember {
	s := []discovery.NetworkMember{}
	for _, member := range g.disc.GetMembership() {
		s = append(s, member)
	}
	return s
}

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
	g.pushPull.Stop()
	g.toDieChan <- struct{}{}
	g.emitter.Stop()
	g.ChannelDeMultiplexer.Close()
	g.stopSignal.Wait()
	comWG.Wait()
}

func (g *gossipServiceImpl) UpdateMetadata(md []byte) {
	g.disc.UpdateMetadata(md)
}

func (g *gossipServiceImpl) Accept(acceptor common.MessageAcceptor) <-chan *proto.GossipMessage {
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
	return outCh
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

func (g *gossipServiceImpl) newDiscoveryAdapter() *discoveryAdapter {
	return &discoveryAdapter{
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
	stopping     int32
	c            comm.Comm
	presumedDead chan common.PKIidType
	incChan      chan *proto.GossipMessage
	gossipFunc   func(*proto.GossipMessage)
}

func (da *discoveryAdapter) close() {
	atomic.StoreInt32((&da.stopping), int32(1))
	close(da.incChan)
}

func (da *discoveryAdapter) toDie() bool {
	return atomic.LoadInt32(&da.stopping) == int32(1)
}

func (da *discoveryAdapter) Gossip(msg *proto.GossipMessage) {
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

func equalPKIIds(a, b common.PKIidType) bool {
	return bytes.Equal(a, b)
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
