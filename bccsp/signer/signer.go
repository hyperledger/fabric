/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signer

import (
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"io"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// bccspCryptoSigner is the BCCSP-based implementation of a crypto.Signer
type bccspCryptoSigner struct {
	csp          bccsp.BCCSP
	classicalKey bccsp.Key
	quantumKey   bccsp.Key
	// TODO(amelia): Should this be an interface?
	classicalPk interface{}
	quantumPk   interface{}
}

type hybridSignature struct {
	ClassicalSign asn1.BitString
	QuantumSign   asn1.BitString
}

// New returns a new BCCSP-based crypto.Signer
// for the given BCCSP instance and key.
func New(csp bccsp.BCCSP, classicalKey bccsp.Key, quantumKey bccsp.Key) (crypto.Signer, error) {
	// Validate arguments
	if csp == nil {
		return nil, errors.New("bccsp instance must be different from nil.")
	}
	if classicalKey == nil {
		return nil, errors.New("classical key must be different from nil.")
	}
	if classicalKey.Symmetric() || (quantumKey != nil && quantumKey.Symmetric()) {
		return nil, errors.New("key must be asymmetric.")
	}

	// Marshall the classical public key as a crypto.PublicKey
	classicalPub, err := classicalKey.PublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting classical public key")
	}

	classicalRaw, err := classicalPub.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling public key")
	}

	classicalPk, err := x509.ParsePKIXPublicKey(classicalRaw)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling der to public key")
	}

	// Marshall the quantum public key as a crypto.PublicKey
	if quantumKey != nil {
		quantumPub, err := quantumKey.PublicKey()
		if err != nil {
			return nil, errors.Wrap(err, "failed getting quantum public key")
		}
		quantumRaw, err := quantumPub.Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "failed marshalling public key")
		}
		quantumPk, err := x509.ParsePKIXPublicKey(quantumRaw)
		if err != nil {
			return nil, errors.Wrap(err, "failed marshalling der to public key")
		}
		return &bccspCryptoSigner{
			csp,
			classicalKey,
			quantumKey,
			classicalPk,
			quantumPk,
		}, nil
	}
	return &bccspCryptoSigner{
		csp,
		classicalKey,
		nil,
		classicalPk,
		nil,
	}, nil
}

// Public returns the public key corresponding to the opaque,
// private key.
func (s *bccspCryptoSigner) Public() crypto.PublicKey {
	return s.classicalPk
}

// Sign signs digest with the private key, possibly using entropy from rand.
// For an (EC)DSA key, it should be a DER-serialised, ASN.1 signature
// structure.
//
// Hash implements the SignerOpts interface and, in most cases, one can
// simply pass in the hash function used as opts. Sign may also attempt
// to type assert opts to other types in order to obtain algorithm
// specific values. See the documentation in each package for details.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest) and the hash function (as opts) to Sign.
func (s *bccspCryptoSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	// If there is a quantum key associated with the signer,
	// Follow the strong-nested hybrid signature proposed in
	// https://eprint.iacr.org/2017/460.pdf
	// Note that if there is no quantum key, this function returns
	// an unmodified classical signature.
	var qSign []byte
	var err error
	if s.quantumKey != nil {
		qSign, err = s.csp.Sign(s.quantumKey, digest, opts)
		if err != nil {
			return nil, err
		}
		digest = append(digest, qSign...)
		// Must use SHA384 for post-quantum.
		hashopt, err := bccsp.GetHashOpt(bccsp.SHA384)
		if err != nil {
			return nil, err
		}
		digest, err = s.csp.Hash(digest, hashopt)
		if err != nil {
			return nil, err
		}
	}

	cSign, err := s.csp.Sign(s.classicalKey, digest, opts)
	if err != nil {
		return nil, err
	}

	if s.quantumKey != nil {
		signature := hybridSignature{
			ClassicalSign: asn1.BitString{
				Bytes:     cSign,
				BitLength: 8 * len(cSign),
			},
			QuantumSign: asn1.BitString{
				Bytes:     qSign,
				BitLength: 8 * len(qSign),
			},
		}
		ret, err := asn1.Marshal(signature)
		if err != nil {
			return nil, err
		}
		return ret, nil
	} else {
		return cSign, nil
	}
}
