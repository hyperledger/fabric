/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localmsp

import (
	"github.com/hyperledger/fabric/internal/pkg/identity"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"

	"github.com/pkg/errors"
)

//TODO: This package will be removed once all references to NewSigner() have benn
// removed from the codebase.

// NewSigner creates a new Signer.  It assumes that the local msp has already been
// initialized.  See mspmgmt.LoadLocalMsp for further information.
func NewSigner() *Signer {
	signer, _ := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	return &Signer{
		id: signer,
	}
}

// Signer represents a signing identity.
type Signer struct {
	id identity.SignerSerializer
}

// Sign signs message bytes and returns the signature or an error on failure.
func (s *Signer) Sign(message []byte) ([]byte, error) {
	if s.id == nil {
		return nil, errors.New("sign failed: local MSP not initialized")
	}
	return s.id.Sign(message)
}

// Serialize converts an identity to bytes.  It returns an error on failure.
func (s *Signer) Serialize() ([]byte, error) {
	if s.id == nil {
		return nil, errors.New("serialize failed: local MSP not initialized")
	}
	return s.id.Serialize()
}
