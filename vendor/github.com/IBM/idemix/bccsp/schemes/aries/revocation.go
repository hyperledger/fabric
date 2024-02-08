/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aries

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"

	weakbb "github.com/IBM/idemix/bccsp/schemes/weak-bb"
	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type RevocationAuthority struct {
	Rng   io.Reader
	Curve *math.Curve
}

// NewKey generates a long term signing key that will be used for revocation
func (r *RevocationAuthority) NewKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// NewKeyFromBytes generates a long term signing key that will be used for revocation from the passed bytes
func (r *RevocationAuthority) NewKeyFromBytes(raw []byte) (*ecdsa.PrivateKey, error) {
	priv := &ecdsa.PrivateKey{}
	priv.D = new(big.Int).SetBytes(raw)
	priv.PublicKey.Curve = elliptic.P384()
	priv.PublicKey.X, priv.PublicKey.Y = elliptic.P384().ScalarBaseMult(priv.D.Bytes())

	return priv, nil
}

// Sign creates the Credential Revocation Information for a certain time period (epoch).
// Users can use the CRI to prove that they are not revoked.
func (r *RevocationAuthority) Sign(key *ecdsa.PrivateKey, _ [][]byte, epoch int, alg types.RevocationAlgorithm) ([]byte, error) {
	if key == nil {
		return nil, errors.Errorf("CreateCRI received nil input")
	}

	cri := &CredentialRevocationInformation{}
	cri.RevocationAlg = int32(alg)
	cri.Epoch = int64(epoch)

	if alg == types.AlgNoRevocation {
		// put a dummy PK in the proto
		cri.EpochPk = r.Curve.GenG2.Bytes()
	} else {
		// create epoch key
		_, epochPk := weakbb.WbbKeyGen(r.Curve, r.Rng)
		cri.EpochPk = epochPk.Bytes()
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

	if alg == types.AlgNoRevocation {
		return proto.Marshal(cri)
	} else {
		return nil, errors.Errorf("the specified revocation algorithm is not supported.")
	}
}

// Verify verifies that the revocation PK for a certain epoch is valid,
// by checking that it was signed with the long term revocation key.
// Note that even if we use no revocation (i.e., alg = ALG_NO_REVOCATION), we need
// to verify the signature to make sure the issuer indeed signed that no revocation
// is used in this epoch.
func (r *RevocationAuthority) Verify(pk *ecdsa.PublicKey, criRaw []byte, epoch int, alg types.RevocationAlgorithm) error {
	if pk == nil {
		return fmt.Errorf("CreateCRI received nil input")
	}

	cri := &CredentialRevocationInformation{}
	err := proto.Unmarshal(criRaw, cri)
	if err != nil {
		return err
	}

	epochPkSig := cri.EpochPkSig
	cri.EpochPkSig = nil
	cri.Epoch = int64(epoch)
	cri.RevocationAlg = int32(alg)

	bytesToVer, err := proto.Marshal(cri)
	if err != nil {
		return err
	}

	digest := sha256.Sum256(bytesToVer)

	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(epochPkSig, &sig); err != nil {
		return errors.Wrap(err, "failed unmashalling signature")
	}

	if !ecdsa.Verify(pk, digest[:], sig.R, sig.S) {
		return errors.Errorf("EpochPKSig invalid")
	}

	return nil
}
