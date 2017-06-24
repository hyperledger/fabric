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
	"io"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = flogging.MustGetLogger("eventhub_producer")

// EventsServer implementation of the Peer service
type EventsServer struct {
}

//singleton - if we want to create multiple servers, we need to subsume events.gEventConsumers into EventsServer
var globalEventsServer *EventsServer

// NewEventsServer returns a EventsServer
func NewEventsServer(bufferSize uint, timeout time.Duration) *EventsServer {
	if globalEventsServer != nil {
		panic("Cannot create multiple event hub servers")
	}
	globalEventsServer = new(EventsServer)
	initializeEvents(bufferSize, timeout)
	//initializeCCEventProcessor(bufferSize, timeout)
	return globalEventsServer
}

// Chat implementation of the Chat bidi streaming RPC function
func (p *EventsServer) Chat(stream pb.Events_ChatServer) error {
	handler, err := newEventHandler(stream)
	if err != nil {
		return fmt.Errorf("error creating handler during handleChat initiation: %s", err)
	}
	defer handler.Stop()
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			logger.Debug("Received EOF, ending Chat")
			return nil
		}
		if err != nil {
			e := fmt.Errorf("error during Chat, stopping handler: %s", err)
			logger.Error(e.Error())
			return e
		}
		err = handler.HandleMessage(in)
		if err != nil {
			logger.Errorf("Error handling message: %s", err)
			return err
		}
	}
}
