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

type publicKeyImpl struct {
	pub    *ecdsa.PublicKey
	rand   io.Reader
	params *Params
}

func (pk *publicKeyImpl) GetRand() io.Reader {
	return pk.rand
}

func (pk *publicKeyImpl) IsPublic() bool {
	return true
}

type publicKeySerializerImpl struct{}

func (pks *publicKeySerializerImpl) ToBytes(key interface{}) ([]byte, error) {
	if key == nil {
		return nil, primitives.ErrInvalidNilKeyParameter
	}

	switch pk := key.(type) {
	case *publicKeyImpl:
		return x509.MarshalPKIXPublicKey(pk.pub)
	default:
		return nil, primitives.ErrInvalidPublicKeyType
	}
}

func (pks *publicKeySerializerImpl) FromBytes(bytes []byte) (interface{}, error) {
	key, err := x509.ParsePKIXPublicKey(bytes)
	if err != nil {
		return nil, err
	}

	// TODO: add params here
	return &publicKeyImpl{key.(*ecdsa.PublicKey), rand.Reader, nil}, nil
}
