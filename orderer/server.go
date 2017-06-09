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

package main

import (
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/configupdate"
	"github.com/hyperledger/fabric/orderer/multichain"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"runtime/debug"
)

type configUpdateSupport struct {
	multichain.Manager
}

func (cus configUpdateSupport) GetChain(chainID string) (configupdate.Support, bool) {
	return cus.Manager.GetChain(chainID)
}

type broadcastSupport struct {
	multichain.Manager
	broadcast.ConfigUpdateProcessor
}

func (bs broadcastSupport) GetChain(chainID string) (broadcast.Support, bool) {
	return bs.Manager.GetChain(chainID)
}

type deliverSupport struct {
	multichain.Manager
}

func (bs deliverSupport) GetChain(chainID string) (deliver.Support, bool) {
	return bs.Manager.GetChain(chainID)
}

type server struct {
	bh broadcast.Handler
	dh deliver.Handler
}

// NewServer creates an ab.AtomicBroadcastServer based on the broadcast target and ledger Reader
func NewServer(ml multichain.Manager, signer crypto.LocalSigner) ab.AtomicBroadcastServer {
	s := &server{
		dh: deliver.NewHandlerImpl(deliverSupport{Manager: ml}),
		bh: broadcast.NewHandlerImpl(broadcastSupport{
			Manager:               ml,
			ConfigUpdateProcessor: configupdate.New(ml.SystemChannelID(), configUpdateSupport{Manager: ml}, signer),
		}),
	}
	return s
}

// Broadcast receives a stream of messages from a client for ordering
func (s *server) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	logger.Debugf("Starting new Broadcast handler")
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("Broadcast client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing Broadcast stream")
	}()
	return s.bh.Handle(srv)
}

// Deliver sends a stream of blocks to a client after ordering
func (s *server) Deliver(srv ab.AtomicBroadcast_DeliverServer) error {
	logger.Debugf("Starting new Deliver handler")
	defer func() {
		if r := recover(); r != nil {
			logger.Criticalf("Deliver client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing Deliver stream")
	}()
	return s.dh.Handle(srv)
}
