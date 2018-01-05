/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
package peer

import (
	"runtime/debug"
	"time"

	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
)

const pkgLogID = "common/peer"

var logger *logging.Logger

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

type server struct {
	dh deliver.Handler
}

// Broadcast is not implemented/supported on a peer
func (s *server) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	return srv.Send(&ab.BroadcastResponse{
		Status: common.Status_NOT_IMPLEMENTED,
	})
}

// Deliver sends a stream of blocks to a client after commitment
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

// NewAtomicBroadcastServer creates an ab.AtomicBroadcastServer based on the
// ledger Reader. Broadcast is not implemented/supported on the peer.
func NewAtomicBroadcastServer(timeWindow time.Duration, mutualTLS bool, policyChecker deliver.PolicyChecker) ab.AtomicBroadcastServer {
	s := &server{
		dh: deliver.NewHandlerImpl(DeliverSupportManager{}, policyChecker, timeWindow, mutualTLS),
	}
	return s
}
