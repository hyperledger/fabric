/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
)

//go:generate mockery --dir . --name consenterSupport --case underscore --with-expecter=true --exported --output mocks

// ConsenterSupport provides the resources available to a Consenter implementation.
type consenterSupport interface {
	consensus.ConsenterSupport
}

//go:generate mockery --dir . --name communicator --case underscore --with-expecter=true --exported --output mocks

// Communicator defines communication for a consenter
type communicator interface {
	cluster.Communicator
}

//go:generate mockery --dir . --name policyManager --case underscore --with-expecter=true --exported --output mocks

// PolicyManager is a read only subset of the policy ManagerImpl
type policyManager interface {
	policies.Manager
}

//go:generate mockery --dir . --name policy --case underscore --with-expecter=true --exported --output mocks

// Policy is used to determine if a signature is valid
type policy interface {
	policies.Policy
}
