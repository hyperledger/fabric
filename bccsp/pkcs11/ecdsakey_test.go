// +build pkcs11

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

package pkcs11

import (
	"crypto/x509"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/stretchr/testify/assert"
)

func TestX509PublicKeyImportOptsKeyImporter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestX509PublicKeyImportOptsKeyImporter")
	}
	ki := currentBCCSP

	_, err := ki.KeyImport("Hello World", &bccsp.X509PublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "[X509PublicKeyImportOpts] Invalid raw material. Expected *x509.Certificate.")

	_, err = ki.KeyImport(nil, &bccsp.X509PublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw. Cannot be nil.")

	cert := &x509.Certificate{}
	cert.PublicKey = "Hello world"
	_, err = ki.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Certificate's public key type not recognized. Supported keys: [ECDSA, RSA]")
}
