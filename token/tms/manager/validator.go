/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/pkg/errors"
)

// AllIssuingValidator allows all members of a channel to issue new tokens.
type AllIssuingValidator struct {
	Deserializer identity.Deserializer
}

// Validate returns no error if the passed creator can issue tokens of the passed type,, an error otherwise.
func (p *AllIssuingValidator) Validate(creator identity.PublicInfo, tokenType string) error {
	// Deserialize identity
	identity, err := p.Deserializer.DeserializeIdentity(creator.Public())
	if err != nil {
		return errors.Wrapf(err, "identity [0x%x] cannot be deserialised", creator.Public())
	}

	// Check identity validity - in this simple policy, all valid identities are issuers.
	if err := identity.Validate(); err != nil {
		return errors.Wrapf(err, "identity [0x%x] cannot be validated", creator.Public())
	}

	return nil
}

// FabricTokenOwnerValidator checks that an owner is valid identity in a given channel
type FabricTokenOwnerValidator struct {
	Deserializer identity.Deserializer
}

func (v *FabricTokenOwnerValidator) Validate(owner *token.TokenOwner) error {
	if owner == nil {
		return errors.New("identity cannot be nil")
	}

	switch owner.Type {
	case token.TokenOwner_MSP_IDENTIFIER:
		// Deserialize identity
		id, err := v.Deserializer.DeserializeIdentity(owner.Raw)
		if err != nil {
			return errors.Wrapf(err, "identity [0x%x] cannot be deserialised", owner)
		}

		// Check identity validity - in this simple policy, all valid identities are issuers.
		if err := id.Validate(); err != nil {
			return errors.Wrapf(err, "identity [0x%x] cannot be validated", owner)
		}
	default:
		return errors.Errorf("identity's type '%s' not recognized", owner.Type)
	}

	return nil
}
