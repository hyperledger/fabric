// DERToPublicKey unmarshals a der to public key
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

package utils

import (
	"crypto/x509"
	"fmt"

	"github.com/pkg/errors"

	"github.com/hyperledger/fabric/bccsp/pqc"
)

func DERToPublicKey(raw []byte) (pub interface{}, err error) {
	if len(raw) == 0 {
		return nil, errors.New("Invalid DER. It must be different from nil.")
	}

	// Try parsing as an PQC key first
	key, err := pqc.ParsePKIXPublicKey(raw)
	if err == nil {
		return key, err
	}
	fmt.Println("pqc.ParsePKIXPublicKey error", err.Error())

	key2, err2 := x509.ParsePKIXPublicKey(raw)

	return key2, err2

}

// DERToX509Certificate converts der to x509
func DERToX509Certificate(asn1Data []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(asn1Data)
}
