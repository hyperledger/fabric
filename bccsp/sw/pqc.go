/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package sw

import (
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/open-quantum-safe/liboqs-go/oqs"
)

func sign(algo string, sk []byte, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	signer := oqs.Signature{}
	defer signer.Clean()
	if err := signer.Init(algo, sk); err != nil {
		return nil, fmt.Errorf("Failed PQC Init %s key [%s]", algo, err)
	}

	return signer.Sign(digest)
}

func verify(algo string, pk []byte, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	verifier := oqs.Signature{}
	defer verifier.Clean() // clean up even in case of panic

	if err := verifier.Init(algo, nil); err != nil {
		return false, fmt.Errorf("Failed PQC Init %s key [%s]", algo, err)
	}

	isValid, err := verifier.Verify(digest, signature, pk)
	if err != nil {
		return false, fmt.Errorf("Failed PQC Private Verify %s key [%s]", algo, err)
	}
	return isValid, nil
}

type pqcSigner struct{}

func (s *pqcSigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	return sign(string(k.(*pqcPrivateKey).privKey.Sig.Algorithm), k.(*pqcPrivateKey).privKey.Sk, digest, opts)
}

type pqcPrivateKeyVerifier struct{}

func (v *pqcPrivateKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	return verify(string(k.(*pqcPrivateKey).privKey.Sig.Algorithm), k.(*pqcPrivateKey).privKey.Pk, signature, digest, opts)
}

type pqcPublicKeyKeyVerifier struct {
	level string
}

func (v *pqcPublicKeyKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	return verify(string(k.(*pqcPublicKey).pubKey.Sig.Algorithm), k.(*pqcPublicKey).pubKey.Pk, signature, digest, opts)
}
