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

package peer

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/looplab/fsm"
	"github.com/spf13/viper"

	pb "github.com/hyperledger/fabric/protos"
)

// Handler peer handler implementation.
type Handler struct {
	chatMutex       sync.Mutex
	ToPeerEndpoint  *pb.PeerEndpoint
	Coordinator     MessageHandlerCoordinator
	ChatStream      ChatStream
	doneChan        chan struct{}
	FSM             *fsm.FSM
	initiatedStream bool // Was the stream initiated within this Peer
	registered      bool
	syncBlocks      chan *pb.SyncBlocks
}

// NewPeerHandler returns a new Peer handler
// Is instance of HandlerFactory
func NewPeerHandler(coord MessageHandlerCoordinator, stream ChatStream, initiatedStream bool, nextHandler MessageHandler) (MessageHandler, error) {

	d := &Handler{
		ChatStream:      stream,
		initiatedStream: initiatedStream,
		Coordinator:     coord,
	}
	d.doneChan = make(chan struct{})

	d.FSM = fsm.NewFSM(
		"created",
		fsm.Events{
			{Name: pb.Message_DISC_HELLO.String(), Src: []string{"created"}, Dst: "established"},
			{Name: pb.Message_DISC_GET_PEERS.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_DISC_PEERS.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_BLOCK_ADDED.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_GET_BLOCKS.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_BLOCKS.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_STATE_GET_SNAPSHOT.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_STATE_SNAPSHOT.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_STATE_GET_DELTAS.String(), Src: []string{"established"}, Dst: "established"},
			{Name: pb.Message_SYNC_STATE_DELTAS.String(), Src: []string{"established"}, Dst: "established"},
		},
		fsm.Callbacks{
			"enter_state":                                    func(e *fsm.Event) { d.enterState(e) },
			"before_" + pb.Message_DISC_HELLO.String():       func(e *fsm.Event) { d.beforeHello(e) },
			"before_" + pb.Message_DISC_GET_PEERS.String():   func(e *fsm.Event) { d.beforeGetPeers(e) },
			"before_" + pb.Message_DISC_PEERS.String():       func(e *fsm.Event) { d.beforePeers(e) },
			"before_" + pb.Message_SYNC_BLOCK_ADDED.String(): func(e *fsm.Event) { d.beforeBlockAdded(e) },
		},
	)

	// If the stream was initiated from this Peer, send an Initial HELLO message
	if d.initiatedStream {
		// Send intiial Hello
		helloMessage, err := d.Coordinator.NewOpenchainDiscoveryHello()
		if err != nil {
			return nil, fmt.Errorf("Error getting new HelloMessage: %s", err)
		}
		if err := d.SendMessage(helloMessage); err != nil {
			return nil, fmt.Errorf("Error creating new Peer Handler, error returned sending %s: %s", pb.Message_DISC_HELLO, err)
		}
	}

	return d, nil
}

func (d *Handler) enterState(e *fsm.Event) {
	peerLogger.Debugf("The Peer's bi-directional stream to %s is %s, from event %s\n", d.ToPeerEndpoint, e.Dst, e.Event)
}

func (d *Handler) deregister() error {
	var err error
	if d.registered {
		err = d.Coordinator.DeregisterHandler(d)
		//doneChan is created and waiting for registered handlers only
		d.doneChan <- struct{}{}
		d.registered = false
	}
	return err
}

// To return the PeerEndpoint this Handler is connected to.
func (d *Handler) To() (pb.PeerEndpoint, error) {
	if d.ToPeerEndpoint == nil {
		return pb.PeerEndpoint{}, fmt.Errorf("No peer endpoint for handler")
	}
	return *(d.ToPeerEndpoint), nil
}

// Stop stops this handler, which will trigger the Deregister from the MessageHandlerCoordinator.
func (d *Handler) Stop() error {
	// Deregister the handler
	err := d.deregister()
	if err != nil {
		return fmt.Errorf("Error stopping MessageHandler: %s", err)
	}
	return nil
}

func (d *Handler) beforeHello(e *fsm.Event) {
	peerLogger.Debugf("Received %s, parsing out Peer identification", e.Event)
	// Parse out the PeerEndpoint information
	if _, ok := e.Args[0].(*pb.Message); !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	msg := e.Args[0].(*pb.Message)

	helloMessage := &pb.HelloMessage{}
	err := proto.Unmarshal(msg.Payload, helloMessage)
	if err != nil {
		e.Cancel(fmt.Errorf("Error unmarshalling HelloMessage: %s", err))
		return
	}
	// Store the PeerEndpoint
	d.ToPeerEndpoint = helloMessage.PeerEndpoint
	peerLogger.Debugf("Received %s from endpoint=%s", e.Event, helloMessage)

	// If security enabled, need to verify the signature on the hello message
	if SecurityEnabled() {
		if err := d.Coordinator.GetSecHelper().Verify(helloMessage.PeerEndpoint.PkiID, msg.Signature, msg.Payload); err != nil {
			e.Cancel(fmt.Errorf("Error Verifying signature for received HelloMessage: %s", err))
			return
		}
		peerLogger.Debugf("Verified signature for %s", e.Event)
	}

	if d.initiatedStream == false {
		// Did NOT intitiate the stream, need to send back HELLO
		peerLogger.Debugf("Received %s, sending back %s", e.Event, pb.Message_DISC_HELLO.String())
		// Send back out PeerID information in a Hello
		helloMessage, err := d.Coordinator.NewOpenchainDiscoveryHello()
		if err != nil {
			e.Cancel(fmt.Errorf("Error getting new HelloMessage: %s", err))
			return
		}
		if err := d.SendMessage(helloMessage); err != nil {
			e.Cancel(fmt.Errorf("Error sending response to %s:  %s", e.Event, err))
			return
		}
	}
	// Register
	err = d.Coordinator.RegisterHandler(d)
	if err != nil {
		e.Cancel(fmt.Errorf("Error registering Handler: %s", err))
	} else {
		// Registered successfully
		d.registered = true
		otherPeer := d.ToPeerEndpoint.Address
		if !d.Coordinator.GetDiscHelper().FindNode(otherPeer) {
			if ok := d.Coordinator.GetDiscHelper().AddNode(otherPeer); !ok {
				peerLogger.Warningf("Unable to add peer %v to discovery list", otherPeer)
			}
			err = d.Coordinator.StoreDiscoveryList()
			if err != nil {
				peerLogger.Error(err)
			}
		}
		go d.start()
	}
}

func (d *Handler) beforeGetPeers(e *fsm.Event) {
	peersMessage, err := d.Coordinator.GetPeers()
	if err != nil {
		e.Cancel(fmt.Errorf("Error Getting Peers: %s", err))
		return
	}
	data, err := proto.Marshal(peersMessage)
	if err != nil {
		e.Cancel(fmt.Errorf("Error Marshalling PeersMessage: %s", err))
		return
	}
	peerLogger.Debugf("Sending back %s", pb.Message_DISC_PEERS.String())
	if err := d.SendMessage(&pb.Message{Type: pb.Message_DISC_PEERS, Payload: data}); err != nil {
		e.Cancel(err)
	}
}

func (d *Handler) beforePeers(e *fsm.Event) {
	peerLogger.Debugf("Received %s, grabbing peers message", e.Event)
	// Parse out the PeerEndpoint information
	if _, ok := e.Args[0].(*pb.Message); !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	msg := e.Args[0].(*pb.Message)

	peersMessage := &pb.PeersMessage{}
	err := proto.Unmarshal(msg.Payload, peersMessage)
	if err != nil {
		e.Cancel(fmt.Errorf("Error unmarshalling PeersMessage: %s", err))
		return
	}

	peerLogger.Debugf("Received PeersMessage with Peers: %s", peersMessage)
	d.Coordinator.PeersDiscovered(peersMessage)

	// // Can be used to demonstrate Broadcast function
	// if viper.GetString("peer.id") == "jdoe" {
	// 	d.Coordinator.Broadcast(&pb.Message{Type: pb.Message_UNDEFINED})
	// }

}

func (d *Handler) beforeBlockAdded(e *fsm.Event) {
	peerLogger.Debugf("Received message: %s", e.Event)
	msg, ok := e.Args[0].(*pb.Message)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	// Add the block and any delta state to the ledger
	_ = msg
}

func (d *Handler) when(stateToCheck string) bool {
	return d.FSM.Is(stateToCheck)
}

// HandleMessage handles the Openchain messages for the Peer.
func (d *Handler) HandleMessage(msg *pb.Message) error {
	peerLogger.Debugf("Handling Message of type: %s ", msg.Type)
	if d.FSM.Cannot(msg.Type.String()) {
		return fmt.Errorf("Peer FSM cannot handle message (%s) with payload size (%d) while in state: %s", msg.Type.String(), len(msg.Payload), d.FSM.Current())
	}
	err := d.FSM.Event(msg.Type.String(), msg)
	if err != nil {
		if _, ok := err.(*fsm.NoTransitionError); !ok {
			// Only allow NoTransitionError's, all others are considered true error.
			return fmt.Errorf("Peer FSM failed while handling message (%s): current state: %s, error: %s", msg.Type.String(), d.FSM.Current(), err)
			//t.Error("expected only 'NoTransitionError'")
		}
	}
	return nil
}

// SendMessage sends a message to the remote PEER through the stream
func (d *Handler) SendMessage(msg *pb.Message) error {
	//make sure Sends are serialized. Also make sure everyone uses SendMessage
	//instead of calling Send directly on the grpc stream
	d.chatMutex.Lock()
	defer d.chatMutex.Unlock()
	peerLogger.Debugf("Sending message to stream of type: %s ", msg.Type)
	err := d.ChatStream.Send(msg)
	if err != nil {
		return fmt.Errorf("Error Sending message through ChatStream: %s", err)
	}
	return nil
}

// start starts the Peer server function
func (d *Handler) start() error {
	discPeriod := viper.GetDuration("peer.discovery.period")
	tickChan := time.NewTicker(discPeriod).C
	peerLogger.Debug("Starting Peer discovery service")
	for {
		select {
		case <-tickChan:
			if err := d.SendMessage(&pb.Message{Type: pb.Message_DISC_GET_PEERS}); err != nil {
				peerLogger.Errorf("Error sending %s during handler discovery tick: %s", pb.Message_DISC_GET_PEERS, err)
			}
		case <-d.doneChan:
			peerLogger.Debug("Stopping discovery service")
			return nil
		}
	}
}
