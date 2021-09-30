/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"crypto/ecdsa"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Revocation encapsulates the idemix algorithms for revocation
type Revocation struct {
	Translator idemix.Translator
	Idemix     *idemix.Idemix
}

// NewKey generate a new revocation key-pair.
func (r *Revocation) NewKey() (*ecdsa.PrivateKey, error) {
	return r.Idemix.GenerateLongTermRevocationKey()
}

func (r *Revocation) NewKeyFromBytes(raw []byte) (*ecdsa.PrivateKey, error) {
	return r.Idemix.LongTermRevocationKeyFromBytes(raw)
}

// Sign generates a new CRI with the respect to the passed unrevoked handles, epoch, and revocation algorithm.
func (r *Revocation) Sign(key *ecdsa.PrivateKey, unrevokedHandles [][]byte, epoch int, alg bccsp.RevocationAlgorithm) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	handles := make([]*math.Zr, len(unrevokedHandles))
	for i := 0; i < len(unrevokedHandles); i++ {
		handles[i] = r.Idemix.Curve.NewZrFromBytes(unrevokedHandles[i])
	}
	cri, err := r.Idemix.CreateCRI(key, handles, epoch, idemix.RevocationAlgorithm(alg), newRandOrPanic(r.Idemix.Curve), r.Translator)
	if err != nil {
		return nil, errors.WithMessage(err, "failed creating CRI")
	}

	return proto.Marshal(cri)
}

// Verify checks that the passed serialised CRI (criRaw) is valid with the respect to the passed revocation public key,
// epoch, and revocation algorithm.
func (r *Revocation) Verify(pk *ecdsa.PublicKey, criRaw []byte, epoch int, alg bccsp.RevocationAlgorithm) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	cri := &idemix.CredentialRevocationInformation{}
	err = proto.Unmarshal(criRaw, cri)
	if err != nil {
		return err
	}

	return r.Idemix.VerifyEpochPK(
		pk,
		cri.EpochPk,
		cri.EpochPkSig,
		int(cri.Epoch),
		idemix.RevocationAlgorithm(cri.RevocationAlg),
	)
}
