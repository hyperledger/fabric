/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/handlers"
	math "github.com/IBM/mathlib"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NymSignatureScheme encapsulates the idemix algorithms to sign and verify using an idemix
// pseudonym.
type NymSignatureScheme struct {
	Translator idemix.Translator
	Idemix     *idemix.Idemix
}

// Sign produces a signature over the passed digest. It takes in input, the user secret key (sk),
// the pseudonym public key (Nym) and secret key (RNym), and the issuer public key (ipk).
func (n *NymSignatureScheme) Sign(sk *math.Zr, Nym *math.G1, RNym *math.Zr, ipk handlers.IssuerPublicKey, digest []byte) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig, err := n.Idemix.NewNymSignature(
		sk,
		Nym,
		RNym,
		iipk.PK,
		digest,
		newRandOrPanic(n.Idemix.Curve),
		n.Translator)
	if err != nil {
		return nil, errors.WithMessage(err, "failed creating new nym signature")
	}

	return proto.Marshal(sig)
}

// Verify checks that the passed signatures is valid with the respect to the passed digest, issuer public key,
// and pseudonym public key.
func (n *NymSignatureScheme) Verify(ipk handlers.IssuerPublicKey, Nym *math.G1, signature, digest []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig := &idemix.NymSignature{}
	err = proto.Unmarshal(signature, sig)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling signature")
	}

	return sig.Ver(Nym, iipk.PK, digest, n.Idemix.Curve, n.Translator)
}
