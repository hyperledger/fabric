/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package registry

import (
	"github.com/hyperledger/fabric/internal/pkg/gateway"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
)

type Registry struct {
	localEndorser gateway.Endorser
	logger        *flogging.FabricLogger
}

// Construct a New registry representing the gateway's view of the network.
func New(localEndorser gateway.Endorser) (*Registry, error) {
	logger := flogging.MustGetLogger("gateway.registry")

	reg := &Registry{
		localEndorser: localEndorser,
		logger:        logger,
	}

	return reg, nil
}

// Endorsers returns a set of endorsers that satisfies the endorsement plan for the given chaincode on a channel.
func (reg *Registry) Endorsers(channel string, chaincode string) []gateway.Endorser {
	// placeholder code for now - just return host peer - allows verification that Evaluate() works!
	// this will be replaced by a set of endorsers selected from the discovery endorsement plan
	// for the given channel/chaincode
	return []gateway.Endorser{reg.localEndorser}
}

// Orderers returns a set of orderers that can order a transaction for the given channel.
func (reg *Registry) Orderers(channel string) []orderer.AtomicBroadcast_BroadcastClient {
	// placeholder code for now - just return empty array - Submit() won't work at the moment.
	// this will be replaced by the set of orderers for a channel determined from the discovery service
	return make([]orderer.AtomicBroadcast_BroadcastClient, 0)
}
