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

package crypto

import (
	"math/big"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

func (node *nodeImpl) sign(signKey interface{}, msg []byte) ([]byte, error) {
	return primitives.ECDSASign(signKey, msg)
}

func (node *nodeImpl) signWithEnrollmentKey(msg []byte) ([]byte, error) {
	return primitives.ECDSASign(node.enrollPrivKey, msg)
}

func (node *nodeImpl) ecdsaSignWithEnrollmentKey(msg []byte) (*big.Int, *big.Int, error) {
	return primitives.ECDSASignDirect(node.enrollPrivKey, msg)
}

func (node *nodeImpl) verify(verKey interface{}, msg, signature []byte) (bool, error) {
	return primitives.ECDSAVerify(verKey, msg, signature)
}

func (node *nodeImpl) verifyWithEnrollmentCert(msg, signature []byte) (bool, error) {
	return primitives.ECDSAVerify(node.enrollCert.PublicKey, msg, signature)
}
