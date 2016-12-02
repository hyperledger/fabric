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
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/common/policies"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/rawledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

type xxxMultichain struct {
	chainID      string
	chainSupport *xxxChainSupport
}

func (xxx *xxxMultichain) GetChain(id string) (multichain.ChainSupport, bool) {
	if id != xxx.chainID {
		return nil, false
	}
	return xxx.chainSupport, true
}

type xxxChainSupport struct {
	reader rawledger.Reader
}

func (xxx *xxxChainSupport) ConfigManager() configtx.Manager {
	panic("Unimplemented")
}

func (xxx *xxxChainSupport) PolicyManager() policies.Manager {
	panic("Unimplemented")
}

func (xxx *xxxChainSupport) Filters() *broadcastfilter.RuleSet {
	panic("Unimplemented")
}

func (xxx *xxxChainSupport) Reader() rawledger.Reader {
	return xxx.reader
}

func (xxx *xxxChainSupport) Chain() multichain.Chain {
	panic("Unimplemented")
}

type BackendAB struct {
	backend       *Backend
	deliverserver deliver.Handler
}

func NewBackendAB(backend *Backend) *BackendAB {

	// XXX All the code below is a hacky shim until sbft can be adapter to the new multichain interface
	it, _ := backend.ledger.Iterator(ab.SeekInfo_OLDEST, 0)
	block, status := it.Next()
	if status != cb.Status_SUCCESS {
		panic("Error getting a block from the ledger")
	}
	env := &cb.Envelope{}
	err := proto.Unmarshal(block.Data.Data[0], env)
	if err != nil {
		panic(err)
	}

	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	if err != nil {
		panic(err)
	}

	manager := &xxxMultichain{
		chainID:      payload.Header.ChainHeader.ChainID,
		chainSupport: &xxxChainSupport{reader: backend.ledger},
	}
	// XXX End hackiness

	bab := &BackendAB{
		backend:       backend,
		deliverserver: deliver.NewHandlerImpl(manager, 1000),
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
	return b.deliverserver.Handle(srv)
}
