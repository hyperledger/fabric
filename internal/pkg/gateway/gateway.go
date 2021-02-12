/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
)

//go:generate counterfeiter -o mocks/endorser.go --fake-name Endorser . Endorser

// An Endorser can process a signed proposal to produce a proposal response.
// The embedded gateway can either use the host peer server or an EndorserClient to a remote peer
// to process this.
type Endorser interface {
	ProcessProposal(ctx context.Context, in *peer.SignedProposal) (*peer.ProposalResponse, error)
}

//go:generate counterfeiter -o mocks/registry.go --fake-name Registry . Registry

// Registry represents the current network topology
type Registry interface {
	Endorsers(channel string, chaincode string) []Endorser
	Orderers(channel string) []orderer.AtomicBroadcast_BroadcastClient
}
