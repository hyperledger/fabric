/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemixca

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"crypto/x509"

	"encoding/pem"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/idemix"
	m "github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var testDir = filepath.Join(os.TempDir(), "idemixca-test")
var configName = "testconfig"

func TestIdemixCa(t *testing.T) {
	cleanup()

	isk, ipkBytes, err := GenerateIssuerKey()
	assert.NoError(t, err)

	revocationkey, err := idemix.GenerateLongTermRevocationKey()
	assert.NoError(t, err)

	ipk := &idemix.IssuerPublicKey{}
	err = proto.Unmarshal(ipkBytes, ipk)
	assert.NoError(t, err)

	encodedRevocationPK, err := x509.MarshalPKIXPublicKey(revocationkey.Public())
	assert.NoError(t, err)
	pemEncodedRevocationPK := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encodedRevocationPK})

	writeVerifierToFile(ipkBytes, pemEncodedRevocationPK)

	key := &idemix.IssuerKey{Isk: isk, Ipk: ipk}

	conf, err := GenerateSignerConfig(m.GetRoleMaskFromIdemixRole(m.MEMBER), "OU1", "enrollmentid1", 1, key, revocationkey)
	assert.NoError(t, err)
	cleanupSigner()
	assert.NoError(t, writeSignerToFile(conf))
	assert.NoError(t, setupMSP())

	conf, err = GenerateSignerConfig(m.GetRoleMaskFromIdemixRole(m.ADMIN), "OU1", "enrollmentid2", 1234, key, revocationkey)
	assert.NoError(t, err)
	cleanupSigner()
	assert.NoError(t, writeSignerToFile(conf))
	assert.NoError(t, setupMSP())

	// Without the verifier dir present, setup should give an error
	cleanupVerifier()
	assert.Error(t, setupMSP())

	_, err = GenerateSignerConfig(m.GetRoleMaskFromIdemixRole(m.ADMIN), "", "enrollmentid", 1, key, revocationkey)
	assert.EqualError(t, err, "the OU attribute value is empty")

	_, err = GenerateSignerConfig(m.GetRoleMaskFromIdemixRole(m.ADMIN), "OU1", "", 1, key, revocationkey)
	assert.EqualError(t, err, "the enrollment id value is empty")
}

func cleanup() error {
	// clean up any previous files
	err := os.RemoveAll(testDir)
	if err != nil {
		return nil
	}
	return os.Mkdir(testDir, os.ModePerm)
}

func cleanupSigner() {
	os.RemoveAll(filepath.Join(testDir, m.IdemixConfigDirUser))
}

func cleanupVerifier() {
	os.RemoveAll(filepath.Join(testDir, m.IdemixConfigDirMsp))
}

func writeVerifierToFile(ipkBytes []byte, revpkBytes []byte) error {
	err := os.Mkdir(filepath.Join(testDir, m.IdemixConfigDirMsp), os.ModePerm)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(testDir, m.IdemixConfigDirMsp, m.IdemixConfigFileIssuerPublicKey), ipkBytes, 0644)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(testDir, m.IdemixConfigDirMsp, m.IdemixConfigFileRevocationPublicKey), revpkBytes, 0644)
}

func writeSignerToFile(signerBytes []byte) error {
	err := os.Mkdir(filepath.Join(testDir, m.IdemixConfigDirUser), os.ModePerm)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(testDir, m.IdemixConfigDirUser, m.IdemixConfigFileSigner), signerBytes, 0644)
}

// setupMSP tests whether we can successfully setup an idemix msp
// with the generated config bytes
func setupMSP() error {
	// setup an idemix msp from the test directory
	msp, err := m.New(&m.IdemixNewOpts{NewBaseOpts: m.NewBaseOpts{Version: m.MSPv1_1}})
	if err != nil {
		return errors.Wrap(err, "Getting MSP failed")
	}
	mspConfig, err := m.GetIdemixMspConfig(testDir, "TestName")

	if err != nil {
		return err
	}

	return msp.Setup(mspConfig)
}
