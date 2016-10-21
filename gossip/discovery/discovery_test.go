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
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

type dummyCommModule struct {
	id           string
	presumeDead  chan string
	detectedDead chan string
	streams      map[string]proto.Gossip_GossipStreamClient
	conns        map[string]*grpc.ClientConn
	lock         *sync.RWMutex
	incMsgs      chan GossipMsg
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
	DiscoveryService
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
	for _, conn := range comm.streams {
		conn.Send(msg)
	}
}

func (comm *dummyCommModule) SendToPeer(peer *NetworkMember, msg *proto.GossipMessage) {
	comm.lock.RLock()
	_, exists := comm.streams[peer.Id]
	comm.lock.RUnlock()

	if !exists {
		if comm.Ping(peer) == false {
			fmt.Printf("Ping to %v failed\n", peer.Endpoint)
			return
		}
	}
	comm.lock.Lock()
	comm.streams[peer.Id].Send(msg)
	comm.lock.Unlock()
}

func (comm *dummyCommModule) Ping(peer *NetworkMember) bool {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	_, alreadyExists := comm.streams[peer.Id]
	if !alreadyExists {
		newConn, err := grpc.Dial(peer.Endpoint, grpc.WithInsecure(), grpc.WithTimeout(time.Duration(500)*time.Millisecond))
		if err != nil {
			//fmt.Printf("Error dialing: to %v: %v\n",peer.Endpoint, err)
			return false
		}
		if stream, err := proto.NewGossipClient(newConn).GossipStream(context.Background()); err == nil {
			comm.conns[peer.Id] = newConn
			comm.streams[peer.Id] = stream
			return true
		} else {
			//fmt.Printf("Error creating stream to %v: %v\n",peer.Endpoint,  err)
			return false
		}
	}
	conn := comm.conns[peer.Id]
	if _, err := proto.NewGossipClient(conn).Ping(context.Background(), &proto.Empty{}); err != nil {
		//fmt.Printf("Error pinging %v: %v\n",peer.Endpoint, err)
		return false
	}
	return true
}

func (comm *dummyCommModule) Accept() <-chan GossipMsg {
	return comm.incMsgs
}

func (comm *dummyCommModule) PresumedDead() <-chan string {
	return comm.presumeDead
}

func (comm *dummyCommModule) CloseConn(id string) {
	comm.lock.Lock()
	defer comm.lock.Unlock()

	if _, exists := comm.streams[id]; !exists {
		return
	}

	comm.streams[id].CloseSend()
	comm.conns[id].Close()

	delete(comm.streams, id)
	delete(comm.conns, id)
}

func init() {
	aliveTimeInterval = time.Duration(time.Millisecond * 10)
	aliveExpirationTimeout = 100 * aliveTimeInterval
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
		g.comm.incMsgs <- &gossipMsg{gMsg}

		if aliveMsg := gMsg.GetAliveMsg(); aliveMsg != nil {
			g.tryForwardMessage(gMsg)
		}
	}
}

func (g *gossipInstance) tryForwardMessage(msg *proto.GossipMessage) {
	g.comm.lock.Lock()

	aliveMsg := msg.GetAliveMsg()

	forward := false
	id := aliveMsg.Membership.Id
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
	g.DiscoveryService.Stop()
}

func (g *gossipInstance) Ping(context.Context, *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

func createDiscoveryInstance(port int, id string, bootstrapPeers []*NetworkMember) *gossipInstance {
	comm := &dummyCommModule{
		conns:        make(map[string]*grpc.ClientConn),
		streams:      make(map[string]proto.Gossip_GossipStreamClient),
		incMsgs:      make(chan GossipMsg, 1000),
		presumeDead:  make(chan string, 10000),
		id:           id,
		detectedDead: make(chan string, 10000),
		lock:         &sync.RWMutex{},
		lastSeqs:     make(map[string]uint64),
	}

	self := NetworkMember{
		Metadata: []byte{},
		Id:       id,
		Endpoint: fmt.Sprintf("localhost:%d", port),
	}

	listenAddress := fmt.Sprintf("%s:%d", "", port)
	ll, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Printf("Error listening on %v, %v", listenAddress, err)
	}
	s := grpc.NewServer()

	discSvc := NewDiscoveryService(bootstrapPeers, self, comm, comm)
	gossInst := &gossipInstance{comm: comm, gRGCserv: s, DiscoveryService: discSvc, lsnr: ll}

	proto.RegisterGossipServer(s, gossInst)
	go s.Serve(ll)

	return gossInst
}

func bootPeer(port int) *NetworkMember {
	return &NetworkMember{Id: fmt.Sprintf("d%d", port), Endpoint: fmt.Sprintf("localhost:%d", port)}
}

func TestUpdate(t *testing.T) {
	nodeNum := 5
	bootPeers := []*NetworkMember{bootPeer(6611), bootPeer(6612)}
	instances := make([]*gossipInstance, 0)

	inst := createDiscoveryInstance(6611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(6612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(6610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	time.Sleep(time.Duration(2) * time.Second)
	assert.Equal(t, nodeNum-1, len(instances[nodeNum-1].GetMembership()))

	instances[0].UpdateMetadata([]byte("bla bla"))
	instances[nodeNum-1].UpdateEndpoint("localhost:5511")
	time.Sleep(time.Duration(2) * time.Second)

	for _, member := range instances[nodeNum-1].GetMembership() {
		if member.Id == instances[0].comm.id {
			assert.Equal(t, "bla bla", string(member.Metadata))
		}
	}

	for _, member := range instances[0].GetMembership() {
		if member.Id == instances[nodeNum-1].comm.id {
			assert.Equal(t, "localhost:5511", string(member.Endpoint))
		}
	}

	stopAction := &sync.WaitGroup{}
	for _, inst := range instances {
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}
	stopAction.Wait()
}

func TestExpiration(t *testing.T) {
	nodeNum := 5
	bootPeers := []*NetworkMember{bootPeer(2611), bootPeer(2612)}
	instances := make([]*gossipInstance, 0)

	inst := createDiscoveryInstance(2611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(2612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(2610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	time.Sleep(time.Duration(2) * time.Second)
	assert.Equal(t, nodeNum-1, len(instances[nodeNum-1].GetMembership()))

	instances[nodeNum-1].Stop() // stop instance 4
	instances[nodeNum-2].Stop() // stop instance 3

	time.Sleep(time.Duration(5) * time.Second)
	assert.Equal(t, nodeNum-3, len(instances[0].GetMembership()))

	go printStackTrace()

	for i, inst := range instances {
		if i == 4 || i == 3 {
			continue
		}
		inst.Stop()
	}
}

func TestGetFullMembership(t *testing.T) {
	nodeNum := 10
	bootPeers := []*NetworkMember{bootPeer(5611), bootPeer(5612)}
	instances := make([]*gossipInstance, 0)

	inst := createDiscoveryInstance(5611, "d1", bootPeers)
	instances = append(instances, inst)

	inst = createDiscoveryInstance(5612, "d2", bootPeers)
	instances = append(instances, inst)

	for i := 3; i <= nodeNum; i++ {
		id := fmt.Sprintf("d%d", i)
		inst = createDiscoveryInstance(5610+i, id, bootPeers)
		instances = append(instances, inst)
	}

	time.Sleep(time.Duration(2) * time.Second)
	assert.Equal(t, nodeNum-1, len(instances[nodeNum-1].GetMembership()))

	stopAction := &sync.WaitGroup{}
	for _, inst := range instances {
		stopAction.Add(1)
		go func(inst *gossipInstance) {
			defer stopAction.Done()
			inst.Stop()
		}(inst)
	}
	stopAction.Wait()
}

func TestGossipDiscoveryStopping(t *testing.T) {
	inst := createDiscoveryInstance(9611, "d1", []*NetworkMember{bootPeer(9611)})

	diedChan := make(chan struct{})
	go func(inst *gossipInstance) {
		inst.Stop()
		diedChan <- struct{}{}
	}(inst)

	timer := time.Tick(time.Duration(2000) * time.Millisecond)

	select {
	case <-timer:
		t.Fatal("Didn't stop within a timely manner")
		t.Fail()
	case <-diedChan:
	}
}

func printStackTrace() {
	func() {
		time.Sleep(time.Duration(10) * time.Second)
		buf := make([]byte, 1<<16)
		runtime.Stack(buf, true)
		fmt.Printf("%s", buf)
	}()
}
