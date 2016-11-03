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

package discovery

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var timeout = time.Second * time.Duration(15)

type dummyCommModule struct {
	id           string
	presumeDead  chan PKIidType
	detectedDead chan string
	streams      map[string]proto.Gossip_GossipStreamClient
	conns        map[string]*grpc.ClientConn
	lock         *sync.RWMutex
	incMsgs      chan *proto.GossipMessage
	lastSeqs     map[string]uint64
}

type gossipMsg struct {
	*proto.GossipMessage
}

func (m *gossipMsg) GetGossipMessage() *proto.GossipMessage {
	return m.GossipMessage
}

type gossipInstance struct {
	comm *dummyCommModule
	Discovery
	gRGCserv *grpc.Server
	lsnr     net.Listener
}

func (comm *dummyCommModule) ValidateAliveMsg(am *proto.AliveMessage) bool {
	return true
}

func (comm *dummyCommModule) SignMessage(am *proto.AliveMessage) *proto.AliveMessage {
	return am
}

func (comm *dummyCommModule) Gossip(msg *proto.GossipMessage) {
	comm.lock.Lock()
	defer comm.lock.Unlock()
	for _, conn := range comm.streams {
		conn.Send(msg)
	}
}

func (comm *dummyCommModule) SendToPeer(peer *NetworkMember, msg *proto.GossipMessage) {
	comm.lock.RLock()
	_, exists := comm.streams[peer.Endpoint]
	comm.lock.RUnlock()

	if !exists {
		if comm.Ping(peer) == false {
			fmt.Printf("Ping to %v failed\n", peer.Endpoint)
			return
		}
	}
	comm.lock.Lock()
	comm.streams[peer.Endpoint].Send(msg)
	comm.lock.Unlock()
}

func (comm *dummyCommModule) Ping(peer *NetworkMember) bool {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	_, alreadyExists := comm.streams[peer.Endpoint]
	if !alreadyExists {
		newConn, err := grpc.Dial(peer.Endpoint, grpc.WithInsecure())
		if err != nil {
			return false
		}
		if stream, err := proto.NewGossipClient(newConn).GossipStream(context.Background()); err == nil {
			comm.conns[peer.Endpoint] = newConn
			comm.streams[peer.Endpoint] = stream
			return true
		}
		return false
	}
	conn := comm.conns[peer.Endpoint]
	if _, err := proto.NewGossipClient(conn).Ping(context.Background(), &proto.Empty{}); err != nil {
		return false
	}
	return true
}

func (comm *dummyCommModule) Accept() <-chan *proto.GossipMessage {
	return comm.incMsgs
}

func (comm *dummyCommModule) PresumedDead() <-chan PKIidType {
	return comm.presumeDead
}

func (comm *dummyCommModule) CloseConn(peer *NetworkMember) {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	if _, exists := comm.streams[peer.Endpoint]; !exists {
		return
	}

	comm.streams[peer.Endpoint].CloseSend()
	comm.conns[peer.Endpoint].Close()
}

func init() {
	aliveTimeInterval = time.Duration(time.Millisecond * 100)
	aliveExpirationTimeout = 10 * aliveTimeInterval
	aliveExpirationCheckInterval = aliveTimeInterval
	reconnectInterval = aliveExpirationTimeout
}

func (g *gossipInstance) GossipStream(stream proto.Gossip_GossipStreamServer) error {
	for {
		gMsg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		lgr := g.Discovery.(*gossipDiscoveryImpl).logger
		lgr.Debug(g.Discovery.Self().Endpoint, "Got message:", gMsg)
		g.comm.incMsgs <- gMsg

		if aliveMsg := gMsg.GetAliveMsg(); aliveMsg != nil {
			g.tryForwardMessage(gMsg)
		}
	}
}

func (g *gossipInstance) tryForwardMessage(msg *proto.GossipMessage) {
	g.comm.lock.Lock()

	aliveMsg := msg.GetAliveMsg()

	forward := false
	id := string(aliveMsg.Membership.PkiID)
	seqNum := aliveMsg.Timestamp.SeqNum
	if last, exists := g.comm.lastSeqs[id]; exists {
		if last < seqNum {
			g.comm.lastSeqs[id] = seqNum
			forward = true
		}
	} else {
		g.comm.lastSeqs[id] = seqNum
		forward = true
	}

	g.comm.lock.Unlock()

	if forward {
		g.comm.Gossip(msg)
	}
}

func (g *gossipInstance) Stop() {
	g.gRGCserv.Stop()
	g.lsnr.Close()
	for _, stream := range g.comm.streams {
		stream.CloseSend()
	}
	for _, conn := range g.comm.conns {
		conn.Close()
	}
	g.Discovery.Stop()
}

func (g *gossipInstance) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func createDiscoveryInstance(port int, id string, bootstrapPeers []string) *gossipInstance {
	comm := &dummyCommModule{
		conns:        make(map[string]*grpc.ClientConn),
		streams:      make(map[string]proto.Gossip_GossipStreamClient),
		incMsgs:      make(chan *proto.GossipMessage, 1000),
		presumeDead:  make(chan PKIidType, 10000),
		id:           id,
		detectedDead: make(chan string, 10000),
		lock:         &sync.RWMutex{},
		lastSeqs:     make(map[string]uint64),
	}

	endpoint := fmt.Sprintf("localhost:%d", port)
	self := NetworkMember{
		Metadata: []byte{},
		PKIid:    []byte(endpoint),
		Endpoint: endpoint,
	}

	listenAddress := fmt.Sprintf("%s:%d", "", port)
	ll, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Printf("Error listening on %v, %v", listenAddress, err)
	}
	s := grpc.NewServer()

	discSvc := NewDiscoveryService(bootstrapPeers, self, comm, comm)
	discSvc.(*gossipDiscoveryImpl).logger.SetLevel(logging.WARNING)
	gossInst := &gossipInstance{comm: comm, gRGCserv: s, Discovery: discSvc, lsnr: ll}

	proto.RegisterGossipServer(s, gossInst)
	go s.Serve(ll)

	return gossInst
}

func bootPeer(port int) string {
	return fmt.Sprintf("localhost:%d", port)
}

func TestUpdate(t *testing.T) {
	nodeNum := 5
	bootPeers := []string{bootPeer(6611), bootPeer(6612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(6611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(6612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(6610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	fullMembership := func() bool {
		return nodeNum-1 == len(instances[nodeNum-1].GetMembership())
	}

	waitUntilOrFail(t, fullMembership)

	instances[0].UpdateMetadata([]byte("bla bla"))
	instances[nodeNum-1].UpdateEndpoint("localhost:5511")

	checkMembership := func() bool {
		for _, member := range instances[nodeNum-1].GetMembership() {
			if string(member.PKIid) == instances[0].comm.id {
				if "bla bla" != string(member.Metadata) {
					return false
				}
			}
		}

		for _, member := range instances[0].GetMembership() {
			if string(member.PKIid) == instances[nodeNum-1].comm.id {
				if "localhost:5511" != string(member.Endpoint) {
					return false
				}
			}
		}
		return true
	}


	waitUntilOrFail(t, checkMembership)

	stopAction := &sync.WaitGroup{}
	for _, inst := range instances {
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}

	waitUntilOrFailBlocking(t, stopAction.Wait)
}

func TestExpiration(t *testing.T) {
	nodeNum := 5
	bootPeers := []string{bootPeer(2611), bootPeer(2612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(2611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(2612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(2610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	fullMembership := func() bool {
		return nodeNum-1 == len(instances[nodeNum-1].GetMembership())
	}

	waitUntilOrFail(t, fullMembership)

	waitUntilOrFailBlocking(t, instances[nodeNum-1].Stop)
	waitUntilOrFailBlocking(t, instances[nodeNum-2].Stop)

	time.Sleep(time.Duration(2) * time.Second)
	membershipReduced := func() bool {
		return nodeNum-3 == len(instances[0].GetMembership())
	}

	waitUntilOrFail(t, membershipReduced)

	stopAction := &sync.WaitGroup{}
	for i, inst := range instances {
		if i+2 == nodeNum {
			break
		}
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}

	waitUntilOrFailBlocking(t, stopAction.Wait)
}

func TestGetFullMembership(t *testing.T) {
	nodeNum := 15
	bootPeers := []string{bootPeer(5611), bootPeer(5612)}
	instances := []*gossipInstance{}

	inst := createDiscoveryInstance(5611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(5612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(5610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	fullMembership := func() bool {
		return nodeNum - 1 == len(instances[nodeNum-1].GetMembership())
	}
	waitUntilOrFail(t, fullMembership)

	stopAction := &sync.WaitGroup{}
	for _, inst := range instances {
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}

	waitUntilOrFailBlocking(t, stopAction.Wait)
}

func TestGossipDiscoveryStopping(t *testing.T) {
	inst := createDiscoveryInstance(9611, "d1", []string{bootPeer(9611)})
	time.Sleep(time.Second)
	waitUntilOrFailBlocking(t, inst.Stop)

}

func waitUntilOrFail(t *testing.T, pred func() bool) {
	start := time.Now()
	limit := start.UnixNano() + timeout.Nanoseconds()
	for time.Now().UnixNano() < limit {
		if pred() {
			return
		}
		time.Sleep(timeout / 10)
	}
	assert.Fail(t, "Timeout expired!")
}

func waitUntilOrFailBlocking(t *testing.T, f func()) {
	successChan := make(chan struct{}, 1)
	go func() {
		f()
		successChan <- struct{}{}
	}()
	select {
	case <-time.NewTimer(timeout).C:
		break
	case <-successChan:
		return
	}
	assert.Fail(t, "Timeout expired!")
}
