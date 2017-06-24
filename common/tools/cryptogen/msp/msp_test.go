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
package msp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/tools/cryptogen/ca"
	"github.com/hyperledger/fabric/common/tools/cryptogen/msp"
	fabricmsp "github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

const (
	testCAOrg  = "example.com"
	testCAName = "ca" + "." + testCAOrg
	testName   = "peer0"
)

var testDir = filepath.Join(os.TempDir(), "msp-test")

func TestGenerateLocalMSP(t *testing.T) {

	cleanup(testDir)

	err := msp.GenerateLocalMSP(testDir, testName, nil, &ca.CA{}, &ca.CA{})
	assert.Error(t, err, "Empty CA should have failed")

	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")

	// generate signing CA
	signCA, err := ca.NewCA(caDir, testCAOrg, testCAName)
	assert.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca.NewCA(tlsCADir, testCAOrg, testCAName)
	assert.NoError(t, err, "Error generating CA")
	// generate local MSP
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA)
	assert.NoError(t, err, "Failed to generate local MSP")

	// check to see that the right files were generated/saved
	files := []string{
		filepath.Join(mspDir, "admincerts", testName+"-cert.pem"),
		filepath.Join(mspDir, "cacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "tlscacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "keystore"),
		filepath.Join(mspDir, "signcerts", testName+"-cert.pem"),
	}

	for _, file := range files {
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}

	// finally check to see if we can load this as a local MSP config
	testMSPConfig, err := fabricmsp.GetLocalMspConfig(mspDir, nil, testName)
	assert.NoError(t, err, "Error parsing local MSP config")
	testMSP, err := fabricmsp.NewBccspMsp()
	assert.NoError(t, err, "Error creating new BCCSP MSP")
	err = testMSP.Setup(testMSPConfig)
	assert.NoError(t, err, "Error setting up local MSP")

	tlsCA.Name = "test/fail"
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
	cleanup(testDir)

}

func TestGenerateVerifyingMSP(t *testing.T) {

	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")
	// generate signing CA
	signCA, err := ca.NewCA(caDir, testCAOrg, testCAName)
	assert.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca.NewCA(tlsCADir, testCAOrg, testCAName)
	assert.NoError(t, err, "Error generating CA")

	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA)
	assert.NoError(t, err, "Failed to generate verifying MSP")

	// check to see that the right files were generated/saved
	files := []string{
		filepath.Join(mspDir, "admincerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "cacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "tlscacerts", testCAName+"-cert.pem"),
	}

	for _, file := range files {
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}
	// finally check to see if we can load this as a verifying MSP config
	testMSPConfig, err := fabricmsp.GetVerifyingMspConfig(mspDir, testName)
	assert.NoError(t, err, "Error parsing verifying MSP config")
	testMSP, err := fabricmsp.NewBccspMsp()
	assert.NoError(t, err, "Error creating new BCCSP MSP")
	err = testMSP.Setup(testMSPConfig)
	assert.NoError(t, err, "Error setting up verifying MSP")

	tlsCA.Name = "test/fail"
	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
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
