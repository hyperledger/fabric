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
package csp_test

import (
	"crypto/ecdsa"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/tools/cryptogen/csp"
	"github.com/stretchr/testify/assert"
)

var testDir = filepath.Join(os.TempDir(), "csp-test")

func TestGeneratePrivateKey(t *testing.T) {

	priv, signer, err := csp.GeneratePrivateKey(testDir)
	assert.NoError(t, err, "Failed to generate private key")
	assert.NotNil(t, priv, "Should have returned a bccsp.Key")
	assert.Equal(t, true, priv.Private(), "Failed to return private key")
	assert.NotNil(t, signer, "Should have returned a crypto.Signer")
	pkFile := filepath.Join(testDir, hex.EncodeToString(priv.SKI())+"_sk")
	t.Log(pkFile)
	assert.Equal(t, true, checkForFile(pkFile),
		"Expected to find private key file")
	cleanup(testDir)

}

func TestGetECPublicKey(t *testing.T) {

	priv, _, err := csp.GeneratePrivateKey(testDir)
	assert.NoError(t, err, "Failed to generate private key")

	ecPubKey, err := csp.GetECPublicKey(priv)
	assert.NoError(t, err, "Failed to get public key from private key")
	assert.IsType(t, &ecdsa.PublicKey{}, ecPubKey,
		"Failed to return an ecdsa.PublicKey")
	cleanup(testDir)
}

func cleanup(dir string) {
	os.RemoveAll(dir)
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
