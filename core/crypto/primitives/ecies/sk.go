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

package ecies

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"io"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type secretKeyImpl struct {
	priv   *ecdsa.PrivateKey
	pub    primitives.PublicKey
	params *Params
	rand   io.Reader
}

func (sk *secretKeyImpl) IsPublic() bool {
	return false
}

func (sk *secretKeyImpl) GetRand() io.Reader {
	return sk.rand
}

func (sk *secretKeyImpl) GetPublicKey() primitives.PublicKey {
	if sk.pub == nil {
		sk.pub = &publicKeyImpl{&sk.priv.PublicKey, sk.rand, sk.params}
	}
	return sk.pub
}

type secretKeySerializerImpl struct{}

func (sks *secretKeySerializerImpl) ToBytes(key interface{}) ([]byte, error) {
	if key == nil {
		return nil, primitives.ErrInvalidNilKeyParameter
	}

	switch sk := key.(type) {
	case *secretKeyImpl:
		return x509.MarshalECPrivateKey(sk.priv)
	default:
		return nil, primitives.ErrInvalidKeyParameter
	}
}

func (sks *secretKeySerializerImpl) FromBytes(bytes []byte) (interface{}, error) {
	key, err := x509.ParseECPrivateKey(bytes)
	if err != nil {
		return nil, err
	}

	// TODO: add params here
	return &secretKeyImpl{key, nil, nil, rand.Reader}, nil
}
