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

package solo

import (
	"time"

	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/deliver"
	"github.com/hyperledger/fabric/orderer/rawledger"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

var logger = logging.MustGetLogger("orderer/solo")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type server struct {
	bh broadcast.Handler
	bs *broadcastServer
	ds deliver.Handler
}

// New creates a ab.AtomicBroadcastServer based on the solo orderer implementation
func New(queueSize, batchSize, maxWindowSize int, batchTimeout time.Duration, rl rawledger.ReadWriter, grpcServer *grpc.Server, filters *broadcastfilter.RuleSet, configManager configtx.Manager) ab.AtomicBroadcastServer {
	logger.Infof("Starting solo with queueSize=%d, batchSize=%d batchTimeout=%v and ledger=%T", queueSize, batchSize, batchTimeout, rl)
	bs := newBroadcastServer(batchSize, batchTimeout, rl, filters, configManager)
	ds := deliver.NewHandlerImpl(rl, maxWindowSize)
	bh := broadcast.NewHandlerImpl(queueSize, bs, filters, configManager)

	s := &server{
		bs: bs,
		ds: ds,
		bh: bh,
	}
	ab.RegisterAtomicBroadcastServer(grpcServer, s)
	return s
}

// Broadcast receives a stream of messages from a client for ordering
func (s *server) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	return s.bh.Handle(srv)
}

// Deliver sends a stream of blocks to a client after ordering
func (s *server) Deliver(srv ab.AtomicBroadcast_DeliverServer) error {
	logger.Debugf("Starting new Deliver loop")
	return s.ds.Handle(srv)
}
