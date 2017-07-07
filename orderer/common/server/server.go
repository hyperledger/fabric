/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/configupdate"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"runtime/debug"
)

type configUpdateSupport struct {
	multichannel.Manager
}

func (cus configUpdateSupport) GetChain(chainID string) (configupdate.Support, bool) {
	return cus.Manager.GetChain(chainID)
}

type broadcastSupport struct {
	multichannel.Manager
	broadcast.ConfigUpdateProcessor
}

func (bs broadcastSupport) GetChain(chainID string) (broadcast.Support, bool) {
	return bs.Manager.GetChain(chainID)
}

type deliverSupport struct {
	multichannel.Manager
}

func (bs deliverSupport) GetChain(chainID string) (deliver.Support, bool) {
	return bs.Manager.GetChain(chainID)
}

type server struct {
	bh broadcast.Handler
	dh deliver.Handler
}

// NewServer creates an ab.AtomicBroadcastServer based on the broadcast target and ledger Reader
func NewServer(ml multichannel.Manager, signer crypto.LocalSigner) ab.AtomicBroadcastServer {
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
