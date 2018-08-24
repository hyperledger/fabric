// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	assert.Contains(t, err.Error(), "[X509PublicKeyImportOpts] Invalid raw material. Expected *x509.Certificate")

	_, err = ki.KeyImport(nil, &bccsp.X509PublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw. Cannot be nil")

	cert := &x509.Certificate{}
	cert.PublicKey = "Hello world"
	_, err = ki.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Certificate's public key type not recognized. Supported keys: [ECDSA, RSA]")
}
