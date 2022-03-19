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
package sw

import (
	"crypto/sha256"
	"errors"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/pqc"
)

type pqcPrivateKey struct {
	privKey *pqc.SecretKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *pqcPrivateKey) Bytes() ([]byte, error) {
	return nil, errors.New("Not supported.")
}

// SKI returns the subject key identifier of this key.
func (k *pqcPrivateKey) SKI() []byte {
	if k.privKey == nil {
		return nil
	}

	hash := sha256.New()
	hash.Write(k.privKey.Pk)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *pqcPrivateKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *pqcPrivateKey) Private() bool {
	return true
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *pqcPrivateKey) PublicKey() (bccsp.Key, error) {
	return &pqcPublicKey{&k.privKey.PublicKey}, nil
}

type pqcPublicKey struct {
	pubKey *pqc.PublicKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *pqcPublicKey) Bytes() (raw []byte, err error) {
	if k.pubKey == nil {
		return nil, nil
	}
	return pqc.MarshalPKIXPublicKey(k.pubKey)
}

// SKI returns the subject key identifier of this key.
func (k *pqcPublicKey) SKI() []byte {
	if k.pubKey == nil {
		return nil
	}

	hash := sha256.New()
	hash.Write(k.pubKey.Pk)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *pqcPublicKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *pqcPublicKey) Private() bool {
	return false
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *pqcPublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}
