/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"sync"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

const (
	sendQueueSize    = 4096
	inboundQueueSize = 8192
)

// rpcSender is the subset of cluster.RPC the plugin needs.
type rpcSender interface {
	SendConsensus(dest uint64, msg *ab.ConsensusRequest) error
	SendSubmit(dest uint64, request *ab.SubmitRequest, report func(error)) error
}

// engineNetwork implements engine.NetworkInterface over Fabric's authenticated
// cluster transport (orderer/common/cluster).
//
//   - Outbound: one ordered queue + goroutine per destination, so a slow or
//     unreachable consenter can never block the engine while it holds its locks.
//   - Inbound: a single ordered worker, so the engine sees messages in arrival
//     order and a slow handler cannot stall the gRPC stream that delivered it.
type engineNetwork struct {
	selfID uint64
	logger *flogging.FabricLogger
	rpc    rpcSender

	mu       sync.Mutex
	handler  engine.MessageHandler
	queues   map[uint64]chan []byte
	stopped  bool
	stopC    chan struct{}
	wg       sync.WaitGroup
	inbound  chan engine.Message
	startRun sync.Once
}

func newEngineNetwork(selfID uint64, rpc rpcSender, logger *flogging.FabricLogger) *engineNetwork {
	return &engineNetwork{
		selfID:  selfID,
		logger:  logger,
		rpc:     rpc,
		queues:  make(map[uint64]chan []byte),
		stopC:   make(chan struct{}),
		inbound: make(chan engine.Message, inboundQueueSize),
	}
}

func (n *engineNetwork) RegisterHandler(h engine.MessageHandler) {
	n.mu.Lock()
	n.handler = h
	n.mu.Unlock()

	n.startRun.Do(func() {
		n.wg.Add(1)
		go n.runInbound()
	})
}

func (n *engineNetwork) runInbound() {
	defer n.wg.Done()
	for {
		select {
		case <-n.stopC:
			return
		case msg := <-n.inbound:
			n.mu.Lock()
			h := n.handler
			n.mu.Unlock()
			if h == nil {
				continue
			}
			if err := h.HandleMessage(msg.From, msg); err != nil {
				n.logger.Warnf("Engine rejected message type %d from %s: %v", msg.Type, msg.From, err)
			}
		}
	}
}

func (n *engineNetwork) receive(sender uint64, w *engineWire) {
	msg, err := w.toEngineMessage(sender, n.selfID)
	if err != nil {
		n.logger.Warnf("Dropping malformed engine message from %d: %v", sender, err)
		return
	}
	select {
	case n.inbound <- msg:
	case <-n.stopC:
	default:
		n.logger.Warnf("Inbound queue full, dropping engine message type %d from %d", msg.Type, sender)
	}
}

func (n *engineNetwork) Send(target engine.NodeID, msg engine.Message) error {
	dest, err := consenterID(target)
	if err != nil {
		return err
	}
	raw, err := encodeEngineMessage(msg)
	if err != nil {
		return err
	}
	return n.sendWire(dest, raw)
}

func (n *engineNetwork) Broadcast(targets []engine.NodeID, msg engine.Message) error {
	raw, err := encodeEngineMessage(msg)
	if err != nil {
		return err
	}
	var firstErr error
	for _, t := range targets {
		dest, err := consenterID(t)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := n.sendWire(dest, raw); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendTransaction forwards a client request (a marshalled common.Envelope) to
// another consenter, as Fabric's cluster Submit operation.
func (n *engineNetwork) SendTransaction(target engine.NodeID, request []byte) error {
	dest, err := consenterID(target)
	if err != nil {
		return err
	}
	if dest == n.selfID {
		return errors.New("refusing to forward a transaction to ourselves")
	}
	env := &cb.Envelope{}
	if err := proto.Unmarshal(request, env); err != nil {
		return errors.Wrap(err, "request is not a common.Envelope")
	}
	report := func(err error) {
		if err != nil {
			n.logger.Warnf("Failed forwarding transaction to consenter %d: %v", dest, err)
		}
	}
	return n.rpc.SendSubmit(dest, &ab.SubmitRequest{Payload: env}, report)
}

func (n *engineNetwork) sendWire(dest uint64, raw []byte) error {
	if dest == n.selfID {
		return nil
	}
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return errors.New("network stopped")
	}
	q, ok := n.queues[dest]
	if !ok {
		q = make(chan []byte, sendQueueSize)
		n.queues[dest] = q
		n.wg.Add(1)
		go n.runSender(dest, q)
	}
	n.mu.Unlock()

	select {
	case q <- raw:
		return nil
	default:
		return errors.Errorf("send queue to consenter %d is full", dest)
	}
}

func (n *engineNetwork) runSender(dest uint64, q chan []byte) {
	defer n.wg.Done()
	for {
		select {
		case <-n.stopC:
			return
		case raw := <-q:
			if err := n.rpc.SendConsensus(dest, &ab.ConsensusRequest{Payload: raw}); err != nil {
				n.logger.Warnf("Failed sending to consenter %d: %v", dest, err)
			}
		}
	}
}

func (n *engineNetwork) Stop() {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return
	}
	n.stopped = true
	close(n.stopC)
	n.mu.Unlock()
	n.wg.Wait()
}
