/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

//go:generate counterfeiter -o mock/signer_identity.go -fake-name SignerIdentity . SignerIdentity

type Signer interface {
	// Sign signs the given payload and returns a signature
	Sign([]byte) ([]byte, error)
}

// SignerIdentity signs messages and serializes its public identity to bytes
type SignerIdentity interface {
	Signer

	// Serialize returns a byte representation of this identity which is used to verify
	// messages signed by this SignerIdentity
	Serialize() ([]byte, error)
}
