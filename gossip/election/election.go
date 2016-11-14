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

package election

import (
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/proto"
)

// LeaderElectionAdapter is used by the leader election module
// to send and receive messages, as well as notify a leader change
type LeaderElectionAdapter interface {

	// Gossip gossips a message to other peers
	Gossip(msg *proto.GossipMessage)

	// Accept returns a channel that emits messages that fit
	// the given predicate
	Accept(common.MessageAcceptor) <-chan *proto.GossipMessage
}

// LeaderElectionService is the object that runs the leader election algorithm
type LeaderElectionService interface {
	// IsLeader returns whether this peer is a leader or not
	IsLeader() bool
}

// LeaderElectionService is the implementation of LeaderElectionService
type leaderElectionServiceImpl struct {
	adapter LeaderElectionAdapter
}
