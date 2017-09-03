/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package entities

import (
	"encoding/json"
	"fmt"
)

// SignedMessage is a simple struct that contains space
// for a payload and a signature over it, and convenience
// functions to sign, verify, marshal and unmarshal
type SignedMessage struct {
	// ID contains a description of the entity signing this message
	ID []byte `json:"id"`

	// Payload contains the message that is signed
	Payload []byte `json:"payload"`

	// Sig contains a signature over ID and Payload
	Sig []byte `json:"sig"`
}

// Sign signs the SignedMessage and stores the signature in the Sig field
func (m *SignedMessage) Sign(signer Signer) error {
	if signer == nil {
		return fmt.Errorf("nil signer")
	}

	m.Sig = nil
	bytes, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("sign error: json.Marshal returned %s", err)
	}
	sig, err := signer.Sign(bytes)
	if err != nil {
		return fmt.Errorf("sign error: signer.Sign returned %s", err)
	}
	m.Sig = sig

	return nil
}

// Verify verifies the signature over Payload stored in Sig
func (m *SignedMessage) Verify(verifier Signer) (bool, error) {
	if verifier == nil {
		return false, fmt.Errorf("nil verifier")
	}

	sig := m.Sig
	m.Sig = nil
	defer func() {
		m.Sig = sig
	}()

	bytes, err := json.Marshal(m)
	if err != nil {
		return false, fmt.Errorf("sign error: json.Marshal returned %s", err)
	}

	return verifier.Verify(sig, bytes)
}

// ToBytes serializes the intance to bytes
func (m *SignedMessage) ToBytes() ([]byte, error) {
	return json.Marshal(m)
}

// FromBytes populates the instance from the supplied byte array
func (m *SignedMessage) FromBytes(d []byte) error {
	return json.Unmarshal(d, m)
}
