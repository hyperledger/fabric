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

package pbft

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/hyperledger/fabric/protos"
)

type endpoint interface {
	stop()
	deliver([]byte, *pb.PeerID)
	getHandle() *pb.PeerID
	getID() uint64
	isBusy() bool
}

type taggedMsg struct {
	src int
	dst int
	msg []byte
}

type testnet struct {
	debug     bool
	N         int
	closed    chan struct{}
	endpoints []endpoint
	msgs      chan taggedMsg
	filterFn  func(int, int, []byte) []byte
}

type testEndpoint struct {
	id  uint64
	net *testnet
}

func makeTestEndpoint(id uint64, net *testnet) *testEndpoint {
	ep := &testEndpoint{}
	ep.id = id
	ep.net = net
	return ep
}

func (ep *testEndpoint) getID() uint64 {
	return ep.id
}

func (ep *testEndpoint) getHandle() *pb.PeerID {
	return &pb.PeerID{Name: fmt.Sprintf("vp%d", ep.id)}
}

func (ep *testEndpoint) GetNetworkInfo() (self *pb.PeerEndpoint, network []*pb.PeerEndpoint, err error) {
	oSelf, oNetwork, _ := ep.GetNetworkHandles()
	self = &pb.PeerEndpoint{
		ID:   oSelf,
		Type: pb.PeerEndpoint_VALIDATOR,
	}

	network = make([]*pb.PeerEndpoint, len(oNetwork))
	for i, id := range oNetwork {
		network[i] = &pb.PeerEndpoint{
			ID:   id,
			Type: pb.PeerEndpoint_VALIDATOR,
		}
	}
	return
}

func (ep *testEndpoint) GetNetworkHandles() (self *pb.PeerID, network []*pb.PeerID, err error) {
	if nil == ep.net {
		err = fmt.Errorf("Network not initialized")
		return
	}
	self = ep.getHandle()
	network = make([]*pb.PeerID, len(ep.net.endpoints))
	for i, oep := range ep.net.endpoints {
		if nil != oep {
			// In case this is invoked before all endpoints are initialized, this emulates a real network as well
			network[i] = oep.getHandle()
		}
	}
	return
}

// Broadcast delivers to all endpoints.  In contrast to the stack
// Broadcast, this will also deliver back to the replica.  We keep
// this behavior, because it exposes subtle bugs in the
// implementation.
func (ep *testEndpoint) Broadcast(msg *pb.Message, peerType pb.PeerEndpoint_Type) error {
	ep.net.broadcastFilter(ep, msg.Payload)
	return nil
}

func (ep *testEndpoint) Unicast(msg *pb.Message, receiverHandle *pb.PeerID) error {
	receiverID, err := getValidatorID(receiverHandle)
	if err != nil {
		return fmt.Errorf("Couldn't unicast message to %s: %v", receiverHandle.Name, err)
	}
	internalQueueMessage(ep.net.msgs, taggedMsg{int(ep.id), int(receiverID), msg.Payload})
	return nil
}

func internalQueueMessage(queue chan<- taggedMsg, tm taggedMsg) {
	select {
	case queue <- tm:
	default:
		fmt.Println("TEST NET: Message cannot be queued without blocking, consider increasing the queue size")
		queue <- tm
	}
}

func (net *testnet) debugMsg(msg string, args ...interface{}) {
	if net.debug {
		fmt.Printf(msg, args...)
	}
}

func (net *testnet) broadcastFilter(ep *testEndpoint, payload []byte) {
	select {
	case <-net.closed:
		fmt.Println("WARNING! Attempted to send a request to a closed network, ignoring")
		return
	default:
	}
	if net.filterFn != nil {
		payload = net.filterFn(int(ep.id), -1, payload)
		net.debugMsg("TEST: filtered message\n")
	}
	if payload != nil {
		net.debugMsg("TEST: attempting to queue message %p\n", payload)
		internalQueueMessage(net.msgs, taggedMsg{int(ep.id), -1, payload})
		net.debugMsg("TEST: message queued successfully %p\n", payload)
	} else {
		net.debugMsg("TEST: suppressing message with payload %p\n", payload)
	}
}

func (net *testnet) deliverFilter(msg taggedMsg) {
	net.debugMsg("TEST: deliver\n")
	senderHandle := net.endpoints[msg.src].getHandle()
	if msg.dst == -1 {
		net.debugMsg("TEST: Sending broadcast %v\n", net.endpoints)
		wg := &sync.WaitGroup{}
		wg.Add(len(net.endpoints))
		for id, ep := range net.endpoints {
			net.debugMsg("TEST: Looping broadcast %d\n", ep.getID())
			lid := id
			lep := ep
			go func() {
				defer wg.Done()
				if msg.src == lid {
					if net.debug {
						net.debugMsg("TEST: Skipping local delivery %d %d\n", lid, msg.src)
					}
					// do not deliver to local replica
					return
				}
				payload := msg.msg
				net.debugMsg("TEST: Filtering %d\n", lid)
				if net.filterFn != nil {
					payload = net.filterFn(msg.src, lid, payload)
				}
				net.debugMsg("TEST: Delivering %d\n", lid)
				if payload != nil {
					net.debugMsg("TEST: Sending message %d\n", lid)
					lep.deliver(payload, senderHandle)
					net.debugMsg("TEST: Sent message %d\n", lid)
				} else {
					net.debugMsg("TEST: Message to %d was skipped\n", lid)
				}
			}()
		}
		wg.Wait()
	} else {
		payload := msg.msg
		net.debugMsg("TEST: Filtering %d\n", msg.dst)
		if net.filterFn != nil {
			payload = net.filterFn(msg.src, msg.dst, payload)
		}
		if payload != nil {
			net.debugMsg("TEST: Sending unicast\n")
			net.endpoints[msg.dst].deliver(msg.msg, senderHandle)
		}
	}
}

func (net *testnet) processMessageFromChannel(msg taggedMsg, ok bool) bool {
	if !ok {
		net.debugMsg("TEST: message channel closed, exiting\n")
		return false
	}
	net.debugMsg("TEST: new message, delivering\n")
	net.deliverFilter(msg)
	return true
}

func (net *testnet) process() error {
	retry := true
	countdown := time.After(60 * time.Second)
	for {
		net.debugMsg("TEST: process looping\n")
		select {
		case msg, ok := <-net.msgs:
			retry = true
			net.debugMsg("TEST: processing message without testing for idle\n")
			if !net.processMessageFromChannel(msg, ok) {
				return nil
			}
		case <-net.closed:
			return nil
		case <-countdown:
			panic("Test network took more than 60 seconds to resolve requests, this usually indicates a hang")
		default:
			if !retry {
				return nil
			}

			var busy []int
			for i, ep := range net.endpoints {
				if ep.isBusy() {
					busy = append(busy, i)
				}
			}
			if len(busy) == 0 {
				retry = false
				continue
			}

			net.debugMsg("TEST: some replicas are busy, waiting: %v\n", busy)
			select {
			case msg, ok := <-net.msgs:
				retry = true
				if !net.processMessageFromChannel(msg, ok) {
					return nil
				}
				continue
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
	}
}

func (net *testnet) processContinually() {
	for {
		select {
		case msg, ok := <-net.msgs:
			if !net.processMessageFromChannel(msg, ok) {
				return
			}
		case <-net.closed:
			return
		}
	}
}

func makeTestnet(N int, initFn func(id uint64, network *testnet) endpoint) *testnet {
	net := &testnet{}
	net.msgs = make(chan taggedMsg, 100)
	net.closed = make(chan struct{})
	net.endpoints = make([]endpoint, N)

	for i := range net.endpoints {
		net.endpoints[i] = initFn(uint64(i), net)
	}

	return net
}

func (net *testnet) clearMessages() {
	for {
		select {
		case <-net.msgs:
		default:
			return
		}
	}
}

func (net *testnet) stop() {
	close(net.closed)
	for _, ep := range net.endpoints {
		ep.stop()
	}
}
