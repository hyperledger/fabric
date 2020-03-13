/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"github.com/hyperledger/fabric-protos-go/peer"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api"
)

// SigningIdentity signs messages and serializes its public identity to bytes
type SigningIdentity interface {
	// Serialize returns a byte representation of this identity which is used to verify
	// messages signed by this SigningIdentity
	Serialize() ([]byte, error)

	// Sign signs the given payload and returns a signature
	Sign([]byte) ([]byte, error)
}

// SigningIdentityFetcher fetches a signing identity based on the proposal
type SigningIdentityFetcher interface {
	endorsement.Dependency
	// SigningIdentityForRequest returns a signing identity for the given proposal
	SigningIdentityForRequest(*peer.SignedProposal) (SigningIdentity, error)
}
