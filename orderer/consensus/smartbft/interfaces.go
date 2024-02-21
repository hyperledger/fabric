/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
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
