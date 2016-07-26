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

package util

import (
	"sync"

	"github.com/op/go-logging"

	pb "github.com/hyperledger/fabric/protos"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("consensus/util")
}

// Message encapsulates an OpenchainMessage with sender information
type Message struct {
	Msg    *pb.Message
	Sender *pb.PeerID
}

// MessageFan contains the reference to the peer's MessageHandlerCoordinator
type MessageFan struct {
	ins  map[*pb.PeerID]<-chan *Message
	out  chan *Message
	lock sync.Mutex
}

// NewMessageFan will return an initialized MessageFan
func NewMessageFan() *MessageFan {
	return &MessageFan{
		ins: make(map[*pb.PeerID]<-chan *Message),
		out: make(chan *Message),
	}
}

// RegisterChannel is intended to be invoked by Handler to add a channel to be fan-ed in
func (fan *MessageFan) RegisterChannel(sender *pb.PeerID, channel <-chan *Message) {
	fan.lock.Lock()
	defer fan.lock.Unlock()

	if _, ok := fan.ins[sender]; ok {
		logger.Warningf("Received duplicate connection from %v, switching to new connection", sender)
	} else {
		logger.Infof("Registering connection from %v", sender)
	}

	fan.ins[sender] = channel

	go func() {
		for msg := range channel {
			fan.out <- msg
		}

		logger.Infof("Connection from peer %v terminated", sender)

		fan.lock.Lock()
		defer fan.lock.Unlock()

		delete(fan.ins, sender)
	}()
}

// GetOutChannel returns a read only channel which the registered channels fan into
func (fan *MessageFan) GetOutChannel() <-chan *Message {
	return fan.out
}
