/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto"
	"crypto/ecdsa"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"

	"github.com/miekg/pkcs11"
)

// P11ECDSAKey test implementation of crypto.Signer.
type P11ECDSAKey struct {
	ctx              *pkcs11.Ctx
	session          pkcs11.SessionHandle
	publicKey        *ecdsa.PublicKey
	privateKeyHandle pkcs11.ObjectHandle
}

// Public returns the corresponding public key for the
// private key.
func (k *P11ECDSAKey) Public() crypto.PublicKey {
	return k.publicKey
}

// Sign implements crypto.Signer Sign(). Signs the digest the with the private key and returns a byte signature.
func (k *P11ECDSAKey) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	if len(digest) != opts.HashFunc().Size() {
		return nil, fmt.Errorf("digest length does not equal hash function length")
	}

	mech := []*pkcs11.Mechanism{
		pkcs11.NewMechanism(pkcs11.CKM_ECDSA, nil),
	}

	err = k.ctx.SignInit(k.session, mech, k.privateKeyHandle)
	if err != nil {
		return nil, fmt.Errorf("sign init failed: %s", err)
	}

	signature, err = k.ctx.Sign(k.session, digest)
	if err != nil {
		return nil, fmt.Errorf("sign failed: %s", err)
	}

	type ECDSASignature struct{ R, S *big.Int }

	R := new(big.Int)
	S := new(big.Int)
	R.SetBytes(signature[0 : len(signature)/2])
	S.SetBytes(signature[len(signature)/2:])

	return asn1.Marshal(ECDSASignature{R: R, S: S})
}
