/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"fmt"
	"io"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = flogging.MustGetLogger("eventhub_producer")

// EventsServer implementation of the Peer service
type EventsServer struct {
	eventProcessor *eventProcessor
}

// EventsServerConfig contains the setup config for the events server
type EventsServerConfig struct {
	BufferSize       uint
	Timeout          time.Duration
	SendTimeout      time.Duration
	TimeWindow       time.Duration
	BindingInspector comm.BindingInspector
}

// singleton - if we want to create multiple servers, we need to subsume
// events.gEventConsumers into EventsServer
var globalEventsServer *EventsServer

// NewEventsServer returns an EventsServer
func NewEventsServer(config *EventsServerConfig) *EventsServer {
	if globalEventsServer != nil {
		panic("Cannot create multiple event hub servers")
	}
	eventProcessor := initializeEvents(config)
	globalEventsServer := &EventsServer{eventProcessor: eventProcessor}
	return globalEventsServer
}

func (e *EventsServer) Chat(stream pb.Events_ChatServer) error {
	handler := newHandler(stream, e.eventProcessor)
	defer e.eventProcessor.cleanupHandler(handler)
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			logger.Debug("Received EOF, ending Chat")
			return nil
		}
		if err != nil {
			err := fmt.Errorf("error during Chat, stopping handler: %s", err)
			logger.Error(err.Error())
			return err
		}
		err = handler.HandleMessage(in)
		if err != nil {
			logger.Errorf("Error handling message: %s", err)
			return err
		}
	}
}
