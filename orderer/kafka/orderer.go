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

package kafka

import (
	"github.com/hyperledger/fabric/orderer/localconfig"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// Orderer allows the caller to submit to and receive messages from the orderer
type Orderer interface {
	Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error
	Deliver(stream ab.AtomicBroadcast_DeliverServer) error
	Teardown() error
}

// Closeable allows the shut down of the calling resource
type Closeable interface {
	Close() error
}

type serverImpl struct {
	broadcaster Broadcaster
	deliverer   Deliverer
}

// New creates a new orderer
func New(conf *config.TopLevel) Orderer {
	return &serverImpl{
		broadcaster: newBroadcaster(conf),
		deliverer:   newDeliverer(conf),
	}
}

// Broadcast submits messages for ordering
func (s *serverImpl) Broadcast(stream ab.AtomicBroadcast_BroadcastServer) error {
	return s.broadcaster.Broadcast(stream)
}

// Deliver returns a stream of ordered messages
func (s *serverImpl) Deliver(stream ab.AtomicBroadcast_DeliverServer) error {
	return s.deliverer.Deliver(stream)
}

// Teardown shuts down the orderer
func (s *serverImpl) Teardown() error {
	s.deliverer.Close()
	return s.broadcaster.Close()
}
