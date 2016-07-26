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
	"crypto/x509"

	"github.com/hyperledger/fabric/core/crypto/attributes"
	"github.com/hyperledger/fabric/core/crypto/utils"
)

type tCert interface {
	//GetCertificate returns the x509 certificate of the TCert.
	GetCertificate() *x509.Certificate

	//GetPreK0 returns the PreK0 of the TCert. This key is used to derivate attributes keys.
	GetPreK0() []byte

	//Sign signs a msg with the TCert secret key an returns the signature.
	Sign(msg []byte) ([]byte, error)

	//Verify verifies signature and message using the TCert public key.
	Verify(signature, msg []byte) error

	//GetKForAttribute derives the key for a specific attribute name.
	GetKForAttribute(attributeName string) ([]byte, error)
}

type tCertImpl struct {
	client *clientImpl
	cert   *x509.Certificate
	sk     interface{}
	preK0  []byte
}

//GetCertificate returns the x509 certificate of the TCert.
func (tCert *tCertImpl) GetCertificate() *x509.Certificate {
	return tCert.cert
}

//GetPreK0 returns the PreK0 of the TCert. This key is used to derivate attributes keys.
func (tCert *tCertImpl) GetPreK0() []byte {
	return tCert.preK0
}

//Sign signs a msg with the TCert secret key an returns the signature.
func (tCert *tCertImpl) Sign(msg []byte) ([]byte, error) {
	if tCert.sk == nil {
		return nil, utils.ErrNilArgument
	}

	return tCert.client.sign(tCert.sk, msg)
}

//Verify verifies signature and message using the TCert public key.
func (tCert *tCertImpl) Verify(signature, msg []byte) (err error) {
	ok, err := tCert.client.verify(tCert.cert.PublicKey, msg, signature)
	if err != nil {
		return
	}
	if !ok {
		return utils.ErrInvalidSignature
	}
	return
}

//GetKForAttribute derives the key for a specific attribute name.
func (tCert *tCertImpl) GetKForAttribute(attributeName string) ([]byte, error) {
	if tCert.preK0 == nil {
		return nil, utils.ErrNilArgument
	}

	return attributes.GetKForAttribute(attributeName, tCert.preK0, tCert.GetCertificate())
}
