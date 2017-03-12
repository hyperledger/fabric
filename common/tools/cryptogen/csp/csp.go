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
package csp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/bccsp/sw"
)

// GeneratePrivateKey creates a private key and stores it in keystorePath
func GeneratePrivateKey(keystorePath string) (bccsp.Key,
	crypto.Signer, error) {

	csp := factory.GetDefault()

	// generate a key
	priv, err := csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: true})
	if err != nil {
		return nil, nil, err
	}
	// write it to the keystore
	ks, err := sw.NewFileBasedKeyStore(nil, keystorePath, false)
	if err != nil {
		return nil, nil, err
	}
	err = ks.StoreKey(priv)
	if err != nil {
		return nil, nil, err
	}

	// create a crypto.Signer
	signer := &signer.CryptoSigner{}
	err = signer.Init(csp, priv)
	if err != nil {
		return nil, nil, err
	}

	return priv, signer, nil

}

func GetECPublicKey(priv bccsp.Key) (*ecdsa.PublicKey, error) {

	// get the public key
	pubKey, err := priv.PublicKey()
	if err != nil {
		return nil, err
	}
	// marshal to bytes
	pubKeyBytes, err := pubKey.Bytes()
	if err != nil {
		return nil, err
	}
	// unmarshal using pkix
	ecPubKey, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	return ecPubKey.(*ecdsa.PublicKey), nil
}
