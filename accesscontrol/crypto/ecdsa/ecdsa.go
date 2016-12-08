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

package ecdsa

import (
	"crypto/ecdsa"
	"encoding/asn1"
	"math/big"

	"github.com/hyperledger/fabric/core/chaincode/shim/crypto"
)

type x509ECDSASignatureVerifierImpl struct {
}

// ECDSASignature represents an ECDSA signature
type ECDSASignature struct {
	R, S *big.Int
}

func (sv *x509ECDSASignatureVerifierImpl) Verify(certificate, signature, message []byte) (bool, error) {
	// Interpret vk as an x509 certificate
	cert, err := derToX509Certificate(certificate)
	if err != nil {
		return false, err
	}

	// TODO: verify certificate

	// Interpret signature as an ECDSA signature
	vk := cert.PublicKey.(*ecdsa.PublicKey)

	return sv.verifyImpl(vk, signature, message)
}

func (sv *x509ECDSASignatureVerifierImpl) verifyImpl(vk *ecdsa.PublicKey, signature, message []byte) (bool, error) {
	ecdsaSignature := new(ECDSASignature)
	_, err := asn1.Unmarshal(signature, ecdsaSignature)
	if err != nil {
		return false, err
	}

	h, err := computeHash(message, vk.Params().BitSize)
	if err != nil {
		return false, err
	}

	return ecdsa.Verify(vk, h, ecdsaSignature.R, ecdsaSignature.S), nil
}

func NewX509ECDSASignatureVerifier() crypto.SignatureVerifier {
	return &x509ECDSASignatureVerifierImpl{}
}
