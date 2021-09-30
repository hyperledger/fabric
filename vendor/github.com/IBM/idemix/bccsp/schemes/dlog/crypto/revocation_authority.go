/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"io"
	"math/big"

	amcl "github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
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
func (i *Idemix) GenerateLongTermRevocationKey() (*ecdsa.PrivateKey, error) {
	return generateLongTermRevocationKey()
}

func generateLongTermRevocationKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// GenerateLongTermRevocationKey generates a long term signing key that will be used for revocation
func (i *Idemix) LongTermRevocationKeyFromBytes(raw []byte) (*ecdsa.PrivateKey, error) {
	return longTermRevocationKeyFromBytes(raw)
}

func longTermRevocationKeyFromBytes(raw []byte) (*ecdsa.PrivateKey, error) {
	priv := &ecdsa.PrivateKey{}
	priv.D = new(big.Int).SetBytes(raw)
	priv.PublicKey.Curve = elliptic.P384()
	priv.PublicKey.X, priv.PublicKey.Y = elliptic.P384().ScalarBaseMult(priv.D.Bytes())

	return priv, nil
}

// CreateCRI creates the Credential Revocation Information for a certain time period (epoch).
// Users can use the CRI to prove that they are not revoked.
// Note that when not using revocation (i.e., alg = ALG_NO_REVOCATION), the entered unrevokedHandles are not used,
// and the resulting CRI can be used by any signer.
func (i *Idemix) CreateCRI(key *ecdsa.PrivateKey, unrevokedHandles []*math.Zr, epoch int, alg RevocationAlgorithm, rng io.Reader, t Translator) (*CredentialRevocationInformation, error) {
	return createCRI(key, unrevokedHandles, epoch, alg, rng, i.Curve, t)
}

func createCRI(key *ecdsa.PrivateKey, unrevokedHandles []*math.Zr, epoch int, alg RevocationAlgorithm, rng io.Reader, curve *math.Curve, t Translator) (*CredentialRevocationInformation, error) {
	if key == nil || rng == nil {
		return nil, errors.Errorf("CreateCRI received nil input")
	}
	cri := &CredentialRevocationInformation{}
	cri.RevocationAlg = int32(alg)
	cri.Epoch = int64(epoch)

	if alg == ALG_NO_REVOCATION {
		// put a dummy PK in the proto
		cri.EpochPk = t.G2ToProto(curve.GenG2)
	} else {
		// create epoch key
		_, epochPk := wbbKeyGen(curve, rng)
		cri.EpochPk = t.G2ToProto(epochPk)
	}

	// sign epoch + epoch key with long term key
	bytesToSign, err := proto.Marshal(cri)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal CRI")
	}

	digest := sha256.Sum256(bytesToSign)

	cri.EpochPkSig, err = key.Sign(rand.Reader, digest[:], nil)
	if err != nil {
		return nil, err
	}

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
func (i *Idemix) VerifyEpochPK(pk *ecdsa.PublicKey, epochPK *amcl.ECP2, epochPkSig []byte, epoch int, alg RevocationAlgorithm) error {
	return verifyEpochPK(pk, epochPK, epochPkSig, epoch, alg)
}

func verifyEpochPK(pk *ecdsa.PublicKey, epochPK *amcl.ECP2, epochPkSig []byte, epoch int, alg RevocationAlgorithm) error {
	if pk == nil || epochPK == nil {
		return errors.Errorf("EpochPK invalid: received nil input")
	}
	cri := &CredentialRevocationInformation{}
	cri.RevocationAlg = int32(alg)
	cri.EpochPk = epochPK
	cri.Epoch = int64(epoch)
	bytesToSign, err := proto.Marshal(cri)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(bytesToSign)

	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(epochPkSig, &sig); err != nil {
		return errors.Wrap(err, "failed unmashalling signature")
	}

	if !ecdsa.Verify(pk, digest[:], sig.R, sig.S) {
		return errors.Errorf("EpochPKSig invalid")
	}

	return nil
}
