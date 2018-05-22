/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

type nonRevocationVerifier interface {
	recomputeFSContribution(proof *NonRevocationProof, chal *FP256BN.BIG, epochPK *FP256BN.ECP2, proofSRh *FP256BN.BIG) ([]byte, error)
}
type nopNonRevocationVerifier struct{}

func (verifier *nopNonRevocationVerifier) recomputeFSContribution(proof *NonRevocationProof, chal *FP256BN.BIG, epochPK *FP256BN.ECP2, proofSRh *FP256BN.BIG) ([]byte, error) {
	return nil, nil
}

func getNonRevocationVerifier(algorithm RevocationAlgorithm) (nonRevocationVerifier, error) {
	switch algorithm {
	case ALG_NO_REVOCATION:
		return &nopNonRevocationVerifier{}, nil
	default:
		// unknown revocation algorithm
		return nil, errors.Errorf("unknown revocation algorithm %d", algorithm)
	}
}
