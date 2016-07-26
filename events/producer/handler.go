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

package producer

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos"
)

type handler struct {
	ChatStream pb.Events_ChatServer
	doneChan   chan bool
	registered bool
	// PM: this should be a list, add/del, iterate
	interestedEvents []*pb.Interest
}

func newEventHandler(stream pb.Events_ChatServer) (*handler, error) {
	d := &handler{
		ChatStream: stream,
	}
	d.doneChan = make(chan bool)
	return d, nil
}

func (d *handler) addInterest(interest *pb.Interest) {
	n := len(d.interestedEvents)
	if n == cap(d.interestedEvents) {
		// Slice is full; must grow.
		// We double its size and add 1, so if the size is zero we still grow.
		newSlice := make([]*pb.Interest, len(d.interestedEvents), 2*len(d.interestedEvents)+1)
		copy(newSlice, d.interestedEvents)
		d.interestedEvents = newSlice
	}
	d.interestedEvents = d.interestedEvents[0 : n+1]
	d.interestedEvents[n] = interest
}

// Stop stops this handler
func (d *handler) Stop() error {
	d.deregister()
	d.doneChan <- true
	d.registered = false
	return nil
}

func (d *handler) register(iMsg []*pb.Interest) error {
	//TODO add the handler to the map for the interested events
	//if successfully done, continue....
	for _, v := range iMsg {
		if err := registerHandler(v, d); err != nil {
			producerLogger.Errorf("could not register %s", v)
			continue
		}
		d.addInterest(v)
	}

	return nil
}

func (d *handler) deregister() {
	for _, v := range d.interestedEvents {
		if err := deRegisterHandler(v, d); err != nil {
			producerLogger.Errorf("could not deregister %s", v)
			continue
		}
		v = nil
	}
	// PM the following should release slice and its elements for GC?
	d.interestedEvents = nil
}

// HandleMessage handles the Openchain messages for the Peer.
func (d *handler) HandleMessage(msg *pb.Event) error {
	producerLogger.Debug("Handling Event")
	eventsObj := msg.GetRegister()
	if eventsObj == nil {
		return fmt.Errorf("Invalid object from consumer %v", msg.GetEvent())
	}

	if err := d.register(eventsObj.Events); err != nil {
		return fmt.Errorf("Could not register events %s", err)
	}

	//TODO return supported events.. for now just return the received msg
	if err := d.ChatStream.Send(msg); err != nil {
		return fmt.Errorf("Error sending response to %v:  %s", msg, err)
	}

	d.registered = true

	return nil
}

// SendMessage sends a message to the remote PEER through the stream
func (d *handler) SendMessage(msg *pb.Event) error {
	err := d.ChatStream.Send(msg)
	if err != nil {
		return fmt.Errorf("Error Sending message through ChatStream: %s", err)
	}
	return nil
}
