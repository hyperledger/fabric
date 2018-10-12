/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

// nonRevokedProver is the Verifier of the ZK proof system that handles revocation.
type nonRevocationVerifier interface {
	// recomputeFSContribution recomputes the contribution of the non-revocation proof to the ZKP challenge
	recomputeFSContribution(proof *NonRevocationProof, chal *FP256BN.BIG, epochPK *FP256BN.ECP2, proofSRh *FP256BN.BIG) ([]byte, error)
}

// nopNonRevocationVerifier is an empty nonRevocationVerifier that produces an empty contribution
type nopNonRevocationVerifier struct{}

func (verifier *nopNonRevocationVerifier) recomputeFSContribution(proof *NonRevocationProof, chal *FP256BN.BIG, epochPK *FP256BN.ECP2, proofSRh *FP256BN.BIG) ([]byte, error) {
	return nil, nil
}

// getNonRevocationVerifier returns the nonRevocationVerifier bound to the passed revocation algorithm
func getNonRevocationVerifier(algorithm RevocationAlgorithm) (nonRevocationVerifier, error) {
	switch algorithm {
	case ALG_NO_REVOCATION:
		return &nopNonRevocationVerifier{}, nil
	default:
		// unknown revocation algorithm
		return nil, errors.Errorf("unknown revocation algorithm %d", algorithm)
	}
}
