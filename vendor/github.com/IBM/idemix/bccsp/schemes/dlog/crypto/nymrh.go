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

type NymRH []byte

func (nym NymRH) AuditNymRh(
	ipk *IssuerPublicKey,
	rhAttr *math.Zr,
	rhIndex int,
	RNymRh *math.Zr,
	curve *math.Curve,
	t Translator,
) error {
	// Validate inputs
	if ipk == nil {
		return errors.New("cannot verify idemix signature: received nil input")
	}

	if len(nym) == 0 {
		return errors.New("no RhNym provided")
	}

	if len(ipk.HAttrs) <= rhIndex {
		return errors.New("could not access H_a_rh in array")
	}

	H_a_rh, err := t.G1FromProto(ipk.HAttrs[rhIndex])
	if err != nil {
		return fmt.Errorf("could not deserialize H_a_rh: %w", err)
	}

	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return fmt.Errorf("could not deserialize HRand: %w", err)
	}

	RhNym, err := curve.NewG1FromBytes(nym)
	if err != nil {
		return fmt.Errorf("could not deserialize RhNym: %w", err)
	}

	Nym_rh := H_a_rh.Mul2(rhAttr, HRand, RNymRh)

	if !Nym_rh.Equals(RhNym) {
		return errors.New("rh nym does not match")
	}

	return nil
}
