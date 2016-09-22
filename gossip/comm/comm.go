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

package comm

import (
	"github.com/hyperledger/fabric/gossip/proto"
	"sync"
)

type CommModule interface {
	// Send sends a message to endpoints
	Send(msg *proto.GossipMessage, endpoints ...string)

	// Probe probes a remote node and returns nil if its responsive
	Probe(endpoint string) error

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// Each message from the channel can be used to send a reply back to the sender
	Accept(MessageAcceptor) <-chan *ReceivedMessage

	// PresumedDead returns a read-only channel for node endpoints that are suspected to be offline
	PresumedDead() <-chan string

	// CloseConn closes a connection to a certain endpoint
	CloseConn(endpoint string)

	// Stop stops the module
	Stop()
}


type MessageAcceptor func(*proto.GossipMessage) bool

type ReceivedMessage struct {
	*proto.GossipMessage
	lock      *sync.Mutex
	srvStream proto.Gossip_GossipStreamServer
	clStream  proto.Gossip_GossipStreamClient
}
