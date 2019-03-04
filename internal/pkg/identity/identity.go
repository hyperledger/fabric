/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package identity provides a set of interfaces for identity-related operations.

package identity

// Signer is an interface which wraps the Sign method.
//
// Sign signs message bytes and returns the signature or an error on failure.
type Signer interface {
	Sign(message []byte) ([]byte, error)
}

// Serializer is an interface which wraps the Serialize function.
//
// Serialize converts an identity to bytes.  It returns an error on failure.
type Serializer interface {
	Serialize() ([]byte, error)
}

// SignerSerializer groups the Sign and Serialize methods.
type SignerSerializer interface {
	Signer
	Serializer
}
