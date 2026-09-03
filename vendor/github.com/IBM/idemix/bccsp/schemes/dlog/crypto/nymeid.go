/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"errors"
	fmt "fmt"

	math "github.com/IBM/mathlib"
)

type NymEID []byte

func (nym NymEID) AuditNymEid(
	ipk *IssuerPublicKey,
	eidAttr *math.Zr,
	eidIndex int,
	RNymEid *math.Zr,
	curve *math.Curve,
	t Translator,
) error {
	// Validate inputs
	if ipk == nil {
		return errors.New("cannot verify idemix signature: received nil input")
	}

	if len(nym) == 0 {
		return errors.New("no EidNym provided")
	}

	if len(ipk.HAttrs) <= eidIndex {
		return errors.New("could not access H_a_eid in array")
	}

	H_a_eid, err := t.G1FromProto(ipk.HAttrs[eidIndex])
	if err != nil {
		return fmt.Errorf("could not deserialize H_a_eid: %w", err)
	}

	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return fmt.Errorf("could not deserialize HRand: %w", err)
	}

	EidNym, err := curve.NewG1FromBytes(nym)
	if err != nil {
		return fmt.Errorf("could not deserialize EidNym: %w", err)
	}

	Nym_eid := H_a_eid.Mul2(eidAttr, HRand, RNymEid)

	if !Nym_eid.Equals(EidNym) {
		return errors.New("eid nym does not match")
	}

	return nil
}
