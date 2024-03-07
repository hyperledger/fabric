/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderers

import (
	"github.com/hyperledger/fabric-lib-go/common/flogging"
)

type ConnectionSourcer interface {
	RandomEndpoint() (*Endpoint, error)
	ShuffledEndpoints() []*Endpoint
	Update(globalAddrs []string, orgs map[string]OrdererOrg)
}

type ConnectionSourceCreator interface {
	// CreateConnectionSource creates a ConnectionSourcer implementation.
	// In a peer, selfEndpoint == "";
	// In an orderer selfEndpoint carries the (delivery service) endpoint of the orderer.
	CreateConnectionSource(logger *flogging.FabricLogger, selfEndpoint string) ConnectionSourcer
}

type ConnectionSourceFactory struct {
	Overrides map[string]*Endpoint
}

func (f *ConnectionSourceFactory) CreateConnectionSource(logger *flogging.FabricLogger, selfEndpoint string) ConnectionSourcer {
	return NewConnectionSource(logger, f.Overrides, selfEndpoint)
}
