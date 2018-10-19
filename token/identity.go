/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

//go:generate mockery -dir . -name Identity -case underscore -output ./client/mocks/

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)
}

//go:generate mockery -dir . -name SigningIdentity -case underscore -output ./client/mocks/

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
type SigningIdentity interface {
	Identity //extends Identity

	Sign(msg []byte) ([]byte, error)

	GetPublicVersion() Identity
}
