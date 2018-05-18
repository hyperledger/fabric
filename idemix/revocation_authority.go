/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"crypto/ecdsa"

	"crypto/rand"
	"crypto/sha256"

	"crypto/elliptic"

	"math/big"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

type RevocationAlgorithm int32

const (
	ALG_NO_REVOCATION RevocationAlgorithm = iota
)

var ProofBytes = map[RevocationAlgorithm]int{
	ALG_NO_REVOCATION: 0,
}

// GenerateLongTermRevocationKey generates a long term signing key that will be used for revocation
func GenerateLongTermRevocationKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// CreateCRI creates the Credential Revocation Information for a certain time period (epoch).
// Users can use the CRI to prove that they are not revoked.
// Note that when not using revocation (i.e., alg = ALG_NO_REVOCATION), the entered unrevokedHandles are not used,
// and the resulting CRI can be used by any signer.
func CreateCRI(key *ecdsa.PrivateKey, unrevokedHandles []*FP256BN.BIG, epoch int, alg RevocationAlgorithm, rng *amcl.RAND) (*CredentialRevocationInformation, error) {
	if key == nil || rng == nil {
		return nil, errors.Errorf("CreateCRI received nil input")
	}
	cri := &CredentialRevocationInformation{}
	cri.RevocationAlg = int32(alg)
	cri.Epoch = int64(epoch)

	if alg == ALG_NO_REVOCATION {
		// put a dummy PK in the proto
		cri.EpochPK = Ecp2ToProto(GenG2)
	} else {
		// create epoch key
		_, epochPk := WBBKeyGen(rng)
		cri.EpochPK = Ecp2ToProto(epochPk)
	}

	// sign epoch + epoch key with long term key
	bytesToSign, err := proto.Marshal(cri)
	digest := sha256.New().Sum(bytesToSign)

	pkSigR, pkSigS, err := ecdsa.Sign(rand.Reader, key, digest)
	if err != nil {
		return nil, err
	}
	cri.EpochPKSig = append(pkSigR.Bytes(), pkSigS.Bytes()...)

	if alg == ALG_NO_REVOCATION {
		return cri, nil
	} else {
		return nil, errors.Errorf("the specified revocation algorithm is not supported.")
	}
}

// VerifyEpochPK verifies that the revocation PK for a certain epoch is valid,
// by checking that it was signed with the long term revocation key.
// Note that even if we use no revocation (i.e., alg = ALG_NO_REVOCATION), we need
// to verify the signature to make sure the issuer indeed signed that no revocation
// is used in this epoch.
func VerifyEpochPK(pk *ecdsa.PublicKey, epochPK *ECP2, epochPkSig []byte, epoch int, alg RevocationAlgorithm) error {
	if pk == nil || epochPK == nil {
		return errors.Errorf("EpochPK invalid: received nil input")
	}
	cri := &CredentialRevocationInformation{}
	cri.RevocationAlg = int32(alg)
	cri.EpochPK = epochPK
	cri.Epoch = int64(epoch)
	bytesToSign, err := proto.Marshal(cri)
	if err != nil {
		return err
	}
	digest := sha256.New().Sum(bytesToSign)
	sigR := &big.Int{}
	sigR.SetBytes(epochPkSig[0 : len(epochPkSig)/2])
	sigS := &big.Int{}
	sigS.SetBytes(epochPkSig[len(epochPkSig)/2:])

	if !ecdsa.Verify(pk, digest, sigR, sigS) {
		return errors.Errorf("EpochPKSig invalid")
	}

	return nil
}
