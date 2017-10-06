/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auth

import (
	"github.com/hyperledger/fabric/protos/peer"
)

// Filter defines an authentication filter that intercepts
// ProcessProposal methods
type Filter interface {
	peer.EndorserServer
	// Init initializes the Filter with the next EndorserServer
	Init(next peer.EndorserServer)
}

// ChainFilters chains the given auth filters in the order provided.
// the last filter always forwards to the endorser
func ChainFilters(endorser peer.EndorserServer, filters ...Filter) peer.EndorserServer {
	if len(filters) == 0 {
		return endorser
	}

	// Each filter forwards to the next
	for i := 0; i < len(filters)-1; i++ {
		filters[i].Init(filters[i+1])
	}

	// Last filter forwards to the endorser
	filters[len(filters)-1].Init(endorser)

	return filters[0]
}
