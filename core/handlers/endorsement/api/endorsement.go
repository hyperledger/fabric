/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"github.com/hyperledger/fabric/protos/peer"
)

// Argument defines the argument for endorsement
type Argument interface {
	Dependency
	// Arg returns the bytes of the argument
	Arg() []byte
}

// Dependency marks a dependency passed to the Init() method
type Dependency interface {
}

// Plugin endorses a proposal response
type Plugin interface {
	// Endorse signs the given payload(ProposalResponsePayload bytes), and optionally mutates it.
	// Returns:
	// The Endorsement: A signature over the payload, and an identity that is used to verify the signature
	// The payload that was given as input (could be modified within this function)
	// Or error on failure
	Endorse(payload []byte, sp *peer.SignedProposal) (*peer.Endorsement, []byte, error)

	// Init injects dependencies into the instance of the Plugin
	Init(dependencies ...Dependency) error
}

// PluginFactory creates a new instance of a Plugin
type PluginFactory interface {
	New() Plugin
}
