/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

//go:generate counterfeiter -o client/mock/identity.go -fake-name Identity . Identity

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)
}

//go:generate counterfeiter -o client/mock/signing_identity.go -fake-name SigningIdentity . SigningIdentity

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
type SigningIdentity interface {
	Identity //extends Identity

	Sign(msg []byte) ([]byte, error)

	GetPublicVersion() Identity
}
