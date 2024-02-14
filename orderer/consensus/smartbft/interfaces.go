/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sync/atomic"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
)

//go:generate mockery --dir . --name ConsenterSupport --case underscore --with-expecter=true --output mocks

// ConsenterSupport provides the resources available to a Consenter implementation.
type ConsenterSupport interface {
	consensus.ConsenterSupport
}

//go:generate mockery --dir . --name Communicator --case underscore --with-expecter=true --output mocks

// Communicator defines communication for a consenter
type Communicator interface {
	cluster.Communicator
}

//go:generate mockery --dir . --name PolicyManager --case underscore --with-expecter=true --output mocks

// PolicyManager is a read only subset of the policy ManagerImpl
type PolicyManager interface {
	policies.Manager
}

//go:generate mockery --dir . --name Policy --case underscore --with-expecter=true --output mocks

// Policy is used to determine if a signature is valid
type Policy interface {
	policies.Policy
}

//go:generate mockery --dir . --name EgressComm --case underscore --with-expecter=true --output mocks

type EgressCommFactory func(runtimeConfig *atomic.Value, channelId string, comm Communicator) EgressComm

// Comm enables the communications between the nodes.
type EgressComm interface {
	// SendConsensus sends the consensus protocol related message m to the node with id targetID.
	SendConsensus(targetID uint64, m *protos.Message)
	// SendTransaction sends the given client's request to the node with id targetID.
	SendTransaction(targetID uint64, request []byte)
	// Nodes returns a set of ids of participating nodes.
	// In case you need to change or keep this slice, create a copy.
	Nodes() []uint64
}
