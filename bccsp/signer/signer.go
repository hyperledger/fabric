/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package signer

import (
	"crypto"
	"errors"
	"fmt"
	"io"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/utils"
)

// CryptoSigner is the BCCSP-based implementation of a crypto.Signer
type CryptoSigner struct {
	csp bccsp.BCCSP
	key bccsp.Key
	pk  interface{}
}

// Init initializes this CryptoSigner.
func (s *CryptoSigner) Init(csp bccsp.BCCSP, key bccsp.Key) error {
	// Validate arguments
	if csp == nil {
		return errors.New("Invalid BCCSP. Nil.")
	}
	if key == nil {
		return errors.New("Invalid Key. Nil.")
	}
	if key.Symmetric() {
		return errors.New("Invalid Key. Symmetric.")
	}

	// Marshall the bccsp public key as a crypto.PublicKey
	pub, err := key.PublicKey()
	if err != nil {
		return fmt.Errorf("Failed getting public key [%s]", err)
	}

	raw, err := pub.Bytes()
	if err != nil {
		return fmt.Errorf("Failed marshalling public key [%s]", err)
	}

	pk, err := utils.DERToPublicKey(raw)
	if err != nil {
		return fmt.Errorf("Failed marshalling public key [%s]", err)
	}

	// Init fields
	s.csp = csp
	s.key = key
	s.pk = pk

	return nil

}

// Public returns the public key corresponding to the opaque,
// private key.
func (s *CryptoSigner) Public() crypto.PublicKey {
	return s.pk
}

// Sign signs digest with the private key, possibly using entropy from
// rand. For an RSA key, the resulting signature should be either a
// PKCS#1 v1.5 or PSS signature (as indicated by opts). For an (EC)DSA
// key, it should be a DER-serialised, ASN.1 signature structure.
//
// Hash implements the SignerOpts interface and, in most cases, one can
// simply pass in the hash function used as opts. Sign may also attempt
// to type assert opts to other types in order to obtain algorithm
// specific values. See the documentation in each package for details.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest) and the hash function (as opts) to Sign.
func (s *CryptoSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	if opts == nil {
		return s.csp.Sign(s.key, digest, nil)
	}

	so, ok := opts.(bccsp.SignerOpts)
	if !ok {
		return nil, errors.New("Invalid opts type. Expecting bccsp.SignerOpts")
	}

	return s.csp.Sign(s.key, digest, so)
}
