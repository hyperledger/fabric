/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

type nonRevokedProver interface {
	getFSContribution(rh *FP256BN.BIG, rRh *FP256BN.BIG, cri *CredentialRevocationInformation, rng *amcl.RAND) ([]byte, error)
	getNonRevokedProof(chal *FP256BN.BIG) (*NonRevocationProof, error)
}
type nopNonRevokedProver struct{}

func (prover *nopNonRevokedProver) getFSContribution(rh *FP256BN.BIG, rRh *FP256BN.BIG, cri *CredentialRevocationInformation, rng *amcl.RAND) ([]byte, error) {
	return nil, nil
}
func (prover *nopNonRevokedProver) getNonRevokedProof(chal *FP256BN.BIG) (*NonRevocationProof, error) {
	ret := &NonRevocationProof{}
	ret.RevocationAlg = int32(ALG_NO_REVOCATION)
	return ret, nil
}

func getNonRevocationProver(algorithm RevocationAlgorithm) (nonRevokedProver, error) {
	switch algorithm {
	case ALG_NO_REVOCATION:
		return &nopNonRevokedProver{}, nil
	default:
		// unknown revocation algorithm
		return nil, errors.Errorf("unknown revocation algorithm %d", algorithm)
	}
}
