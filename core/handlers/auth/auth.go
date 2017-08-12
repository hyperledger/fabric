/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auth

import (
	"github.com/hyperledger/fabric/protos/peer"
	"golang.org/x/net/context"
)

// Filter defines an authentication filter that intercepts
// ProcessProposal methods
type Filter interface {
	peer.EndorserServer
	// Init initializes the Filter with the next EndorserServer
	Init(next peer.EndorserServer)
}

// NewFilter creates a new Filter
func NewFilter() Filter {
	return &filter{}
}

type filter struct {
	next peer.EndorserServer
}

// Init initializes the Filter with the next EndorserServer
func (f *filter) Init(next peer.EndorserServer) {
	f.next = next
}

// ProcessProposal processes a signed proposal
func (f *filter) ProcessProposal(ctx context.Context, signedProp *peer.SignedProposal) (*peer.ProposalResponse, error) {
	return f.next.ProcessProposal(ctx, signedProp)
}
