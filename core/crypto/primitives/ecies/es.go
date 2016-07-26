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
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
)

type encryptionSchemeImpl struct {
	isForEncryption bool

	// Parameters
	params primitives.AsymmetricCipherParameters
	pub    *publicKeyImpl
	priv   *secretKeyImpl
}

func (es *encryptionSchemeImpl) Init(params primitives.AsymmetricCipherParameters) error {
	if params == nil {
		return primitives.ErrInvalidNilKeyParameter
	}
	es.isForEncryption = params.IsPublic()
	es.params = params

	if es.isForEncryption {
		switch pk := params.(type) {
		case *publicKeyImpl:
			es.pub = pk
		default:
			return primitives.ErrInvalidPublicKeyType
		}
	} else {
		switch sk := params.(type) {
		case *secretKeyImpl:
			es.priv = sk
		default:
			return primitives.ErrInvalidKeyParameter
		}
	}

	return nil
}

func (es *encryptionSchemeImpl) Process(msg []byte) ([]byte, error) {
	if len(msg) == 0 {
		return nil, utils.ErrNilArgument
	}

	if es.isForEncryption {
		// Encrypt
		return eciesEncrypt(es.params.GetRand(), es.pub.pub, nil, nil, msg)
	}

	// Decrypt
	return eciesDecrypt(es.priv.priv, nil, nil, msg)
}
