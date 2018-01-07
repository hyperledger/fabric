/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/broadcast"
	localconfig "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
)

type broadcastSupport struct {
	*multichannel.Registrar
}

func (bs broadcastSupport) BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, broadcast.ChannelSupport, error) {
	return bs.Registrar.BroadcastChannelSupport(msg)
}

type deliverSupport struct {
	*multichannel.Registrar
}

func (ds deliverSupport) GetChain(chainID string) (deliver.Support, bool) {
	return ds.Registrar.GetChain(chainID)
}

type server struct {
	bh    broadcast.Handler
	dh    deliver.Handler
	debug *localconfig.Debug
	*multichannel.Registrar
}

type deliverHandlerSupport struct {
	ab.AtomicBroadcast_DeliverServer
}

// CreateStatusReply generates status reply proto message
func (*deliverHandlerSupport) CreateStatusReply(status cb.Status) proto.Message {
	return &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: status},
	}
}

// CreateBlockReply generates deliver response with block message
func (*deliverHandlerSupport) CreateBlockReply(block *cb.Block) proto.Message {
	return &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Block{Block: block},
	}
}

// NewServer creates an ab.AtomicBroadcastServer based on the broadcast target and ledger Reader
func NewServer(r *multichannel.Registrar, _ crypto.LocalSigner, debug *localconfig.Debug, timeWindow time.Duration, mutualTLS bool) ab.AtomicBroadcastServer {
	s := &server{
		dh:        deliver.NewHandlerImpl(deliverSupport{Registrar: r}, timeWindow, mutualTLS),
		bh:        broadcast.NewHandlerImpl(broadcastSupport{Registrar: r}),
		debug:     debug,
		Registrar: r,
	}
	return s
}

type msgTracer struct {
	function string
	debug    *localconfig.Debug
}

func (mt *msgTracer) trace(traceDir string, msg *cb.Envelope, err error) {
	if err != nil {
		return
	}

	now := time.Now().UnixNano()
	path := fmt.Sprintf("%s%c%d_%p.%s", traceDir, os.PathSeparator, now, msg, mt.function)
	logger.Debugf("Writing %s request trace to %s", mt.function, path)
	go func() {
		pb, err := proto.Marshal(msg)
		if err != nil {
			logger.Debugf("Error marshaling trace msg for %s: %s", path, err)
			return
		}
		err = ioutil.WriteFile(path, pb, 0660)
		if err != nil {
			logger.Debugf("Error writing trace msg for %s: %s", path, err)
		}
	}()
}

type broadcastMsgTracer struct {
	ab.AtomicBroadcast_BroadcastServer
	msgTracer
}

func (bmt *broadcastMsgTracer) Recv() (*cb.Envelope, error) {
	msg, err := bmt.AtomicBroadcast_BroadcastServer.Recv()
	if traceDir := bmt.debug.BroadcastTraceDir; traceDir != "" {
		bmt.trace(bmt.debug.BroadcastTraceDir, msg, err)
	}
	return msg, err
}

type deliverMsgTracer struct {
	deliver.DeliverSupport
	msgTracer
}

func (dmt *deliverMsgTracer) Recv() (*cb.Envelope, error) {
	msg, err := dmt.DeliverSupport.Recv()
	if traceDir := dmt.debug.DeliverTraceDir; traceDir != "" {
		dmt.trace(traceDir, msg, err)
	}
	return msg, err
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
	return s.bh.Handle(&broadcastMsgTracer{
		AtomicBroadcast_BroadcastServer: srv,
		msgTracer: msgTracer{
			debug:    s.debug,
			function: "Broadcast",
		},
	})
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
	policyChecker := func(env *cb.Envelope, channelID string) error {
		chain, ok := s.GetChain(channelID)
		if !ok {
			return errors.Errorf("channel %s not found", channelID)
		}
		sf := msgprocessor.NewSigFilter(policies.ChannelReaders, chain)
		return sf.Apply(env)
	}
	server := &deliverMsgTracer{
		DeliverSupport: &deliverHandlerSupport{AtomicBroadcast_DeliverServer: srv},
		msgTracer: msgTracer{
			debug:    s.debug,
			function: "Deliver",
		},
	}
	return s.dh.Handle(deliver.NewDeliverServer(server, policyChecker, s.sendProducer(srv)))
}

func (s *server) sendProducer(srv ab.AtomicBroadcast_DeliverServer) func(msg proto.Message) error {
	return func(msg proto.Message) error {
		response, ok := msg.(*ab.DeliverResponse)
		if !ok {
			logger.Errorf("received wrong response type, expected response type ab.DeliverResponse")
			return errors.New("expected response type ab.DeliverResponse")
		}
		return srv.Send(response)
	}
}
