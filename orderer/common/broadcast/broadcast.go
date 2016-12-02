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

package broadcast

import (
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/multichain"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
)

var logger = logging.MustGetLogger("orderer/common/broadcast")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// Handler defines an interface which handles broadcasts
type Handler interface {
	// Handle starts a service thread for a given gRPC connection and services the broadcast connection
	Handle(srv ab.AtomicBroadcast_BroadcastServer) error
}

type handlerImpl struct {
	queueSize int
	ml        multichain.Manager
	exitChan  chan struct{}
}

// NewHandlerImpl constructs a new implementation of the Handler interface
func NewHandlerImpl(queueSize int, ml multichain.Manager) Handler {
	return &handlerImpl{
		queueSize: queueSize,
		ml:        ml,
		exitChan:  make(chan struct{}),
	}
}

// Handle starts a service thread for a given gRPC connection and services the broadcast connection
func (bh *handlerImpl) Handle(srv ab.AtomicBroadcast_BroadcastServer) error {
	b := newBroadcaster(bh)
	defer close(b.queue)
	go b.drainQueue()
	return b.queueEnvelopes(srv)
}

type msgAndChainSupport struct {
	msg          *cb.Envelope
	chainSupport multichain.ChainSupport
}

type broadcaster struct {
	bs    *handlerImpl
	queue chan *msgAndChainSupport
}

func newBroadcaster(bs *handlerImpl) *broadcaster {
	b := &broadcaster{
		bs:    bs,
		queue: make(chan *msgAndChainSupport, bs.queueSize),
	}
	return b
}

func (b *broadcaster) drainQueue() {
	for {
		select {
		case msgAndChainSupport, ok := <-b.queue:
			if ok {
				if !msgAndChainSupport.chainSupport.Chain().Enqueue(msgAndChainSupport.msg) {
					return
				}
			} else {
				return
			}
		case <-b.bs.exitChan:
			return
		}
	}
}

func (b *broadcaster) queueEnvelopes(srv ab.AtomicBroadcast_BroadcastServer) error {

	for {
		msg, err := srv.Recv()
		if err != nil {
			return err
		}

		payload := &cb.Payload{}
		err = proto.Unmarshal(msg.Payload, payload)
		if payload.Header == nil || payload.Header.ChainHeader == nil || payload.Header.ChainHeader.ChainID == "" {
			logger.Debugf("Received malformed message, dropping connection")
			return srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
		}

		chainSupport, ok := b.bs.ml.GetChain(payload.Header.ChainHeader.ChainID)
		if !ok {
			// XXX Hook in chain creation logic here
			panic("Unimplemented")
		}

		action, _ := chainSupport.Filters().Apply(msg)

		switch action {
		case broadcastfilter.Reconfigure:
			fallthrough
		case broadcastfilter.Accept:
			select {
			case b.queue <- &msgAndChainSupport{msg: msg, chainSupport: chainSupport}:
				err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SUCCESS})
			default:
				err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE})
			}
		case broadcastfilter.Forward:
			fallthrough
		case broadcastfilter.Reject:
			err = srv.Send(&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST})
		default:
			logger.Fatalf("Unknown filter action :%v", action)
		}

		if err != nil {
			return err
		}
	}
}
