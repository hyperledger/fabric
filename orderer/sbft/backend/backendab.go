/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package backend

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

type BackendAB struct {
	backend       *Backend
	deliverserver *solo.DeliverServer
}

func NewBackendAB(backend *Backend) *BackendAB {
	bab := &BackendAB{
		backend:       backend,
		deliverserver: solo.NewDeliverServer(backend.ledger, 1000),
	}
	return bab
}

// Broadcast receives a stream of messages from a client for ordering
func (b *BackendAB) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	for {
		envelope, err := srv.Recv()
		if err != nil {
			return err
		}

		if envelope.Payload == nil {
			err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
			if err != nil {
				return err
			}
		}
		req, err := proto.Marshal(envelope)
		if err != nil {
			panic(err)
		}
		b.backend.enqueueRequest(req)
		err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SUCCESS})
		if err != nil {
			return err
		}
	}
}

// Deliver sends a stream of blocks to a client after ordering
func (b *BackendAB) Deliver(srv ab.AtomicBroadcast_DeliverServer) error {
	return b.deliverserver.HandleDeliver(srv)
}
