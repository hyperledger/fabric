// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package pkcs11

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/stretchr/testify/assert"
)

func TestECDSAPKIXPublicKeyImportOptsKeyImporter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAPKIXPublicKeyImportOptsKeyImporter")
	}
	ki := currentBCCSP

	_, err := ki.KeyImport("Hello World", &bccsp.ECDSAPKIXPublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "[ECDSAPKIXPublicKeyImportOpts] Invalid raw material. Expected byte array")

	_, err = ki.KeyImport("Hello World", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil")

	_, err = ki.KeyImport(nil, &bccsp.ECDSAPKIXPublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw. Cannot be nil")

	_, err = ki.KeyImport([]byte(nil), &bccsp.ECDSAPKIXPublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "[ECDSAPKIXPublicKeyImportOpts] Invalid raw. It must not be nil")

	_, err = ki.KeyImport([]byte{0}, &bccsp.ECDSAPKIXPublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed converting PKIX to ECDSA public key")

	k, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)
	raw, err := utils.PublicKeyToDER(&k.PublicKey)
	assert.NoError(t, err)
	_, err = ki.KeyImport(raw, &bccsp.ECDSAPKIXPublicKeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed casting to ECDSA public key. Invalid raw material")
}
