/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
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
		return errors.Errorf("cannot verify idemix signature: received nil input")
	}

	if len(nym) == 0 {
		return errors.Errorf("no RhNym provided")
	}

	if len(ipk.HAttrs) <= rhIndex {
		return errors.Errorf("could not access H_a_rh in array")
	}

	H_a_rh, err := t.G1FromProto(ipk.HAttrs[rhIndex])
	if err != nil {
		return errors.Wrap(err, "could not deserialize H_a_rh")
	}

	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return errors.Wrap(err, "could not deserialize HRand")
	}

	RhNym, err := curve.NewG1FromBytes(nym)
	if err != nil {
		return errors.Wrap(err, "could not deserialize RhNym")
	}

	Nym_rh := H_a_rh.Mul2(rhAttr, HRand, RNymRh)

	if !Nym_rh.Equals(RhNym) {
		return errors.New("rh nym does not match")
	}

	return nil
}
