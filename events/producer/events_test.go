/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"context"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/peer"
	ehpb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var peerAddress = "0.0.0.0:60303"

type client struct {
	conn   *grpc.ClientConn
	stream peer.Events_ChatClient
}

func newClient() *client {
	conn, err := comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
	if err != nil {
		panic(err)
	}

	stream, err := peer.NewEventsClient(conn).Chat(context.Background())
	if err != nil {
		panic(err)
	}

	cl := &client{
		conn:   conn,
		stream: stream,
	}
	go cl.processEvents()
	return cl
}

func (c *client) register(ies []*peer.Interest) error {
	emsg := &peer.Event{Event: &peer.Event_Register{Register: &peer.Register{Events: ies}}, Creator: signerSerialized}
	se, err := utils.GetSignedEvent(emsg, signer)
	if err != nil {
		return err
	}
	return c.stream.Send(se)
}

func (c *client) unregister(ies []*peer.Interest) error {
	emsg := &peer.Event{Event: &peer.Event_Unregister{Unregister: &peer.Unregister{Events: ies}}, Creator: signerSerialized}
	se, err := utils.GetSignedEvent(emsg, signer)
	if err != nil {
		return err
	}
	return c.stream.Send(se)
}

func (c *client) processEvents() error {
	defer c.stream.CloseSend()
	for {
		_, err := c.stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestEvents(t *testing.T) {
	test := func(duration time.Duration) {
		t.Log(duration)
		f := func() {
			Send(nil)
		}
		assert.Panics(t, f)
		Send(&peer.Event{})
		gEventProcessorBck := gEventProcessor
		gEventProcessor = nil
		e, err := createEvent()
		assert.NoError(t, err)
		Send(e)
		gEventProcessor = gEventProcessorBck
		Send(e)
	}
	prevTimeout := gEventProcessor.timeout
	for _, timeout := range []time.Duration{0, -1, 1} {
		gEventProcessor.timeout = timeout
		test(timeout)
	}
	gEventProcessor.timeout = prevTimeout
}

func TestDeRegister(t *testing.T) {
	f := func() {
		deRegisterHandler(nil, nil)
	}
	assert.Panics(t, f)
	assert.Error(t, deRegisterHandler(&peer.Interest{EventType: 100}, nil))
	assert.Error(t, deRegisterHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, nil))
}

func TestRegister(t *testing.T) {
	f := func() {
		registerHandler(nil, nil)
	}
	assert.Panics(t, f)

	// attempt to register handlers (invalid type or nil handlers)
	assert.Error(t, registerHandler(&peer.Interest{EventType: 100}, nil))
	assert.Error(t, registerHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, nil))
	assert.Error(t, registerHandler(&peer.Interest{EventType: peer.EventType_CHAINCODE}, nil))

	// attempt to register valid handler
	recvChan := make(chan *streamEvent)
	stream := &mockstream{c: recvChan}
	handler, err := newEventHandler(stream)
	assert.Nil(t, err, "error should have been nil")
	assert.NoError(t, registerHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, handler))
}

func TestProcessEvents(t *testing.T) {
	cl := newClient()
	interests := []*peer.Interest{
		{EventType: peer.EventType_BLOCK},
		{EventType: peer.EventType_CHAINCODE, RegInfo: &peer.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &peer.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event1"}}},
		{EventType: peer.EventType_CHAINCODE, RegInfo: &peer.Interest_ChaincodeRegInfo{ChaincodeRegInfo: &peer.ChaincodeReg{ChaincodeId: "0xffffffff", EventName: "event2"}}},
	}
	cl.register(interests)
	e, err := createEvent()
	assert.NoError(t, err)
	go Send(e)
	time.Sleep(time.Second * 2)
	cl.unregister(interests)
	time.Sleep(time.Second * 2)
}

func TestInitializeEvents_twice(t *testing.T) {
	initializeEventsTwice := func() {
		initializeEvents(
			uint(viper.GetInt("peer.events.buffersize")),
			viper.GetDuration("peer.events.timeout"))
	}
	assert.Panics(t, initializeEventsTwice)
}

func TestAddEventType_alreadyDefined(t *testing.T) {
	assert.Error(t, AddEventType(ehpb.EventType_CHAINCODE), "chaincode type already defined")
}
