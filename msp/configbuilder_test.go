/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/stretchr/testify/assert"
)

func TestSetupBCCSPKeystoreConfig(t *testing.T) {
	keystoreDir := "/tmp"

	// Case 1 : Check with empty FactoryOpts
	rtnConfig := SetupBCCSPKeystoreConfig(nil, keystoreDir)
	assert.NotNil(t, rtnConfig)
	assert.Equal(t, rtnConfig.ProviderName, "SW")
	assert.NotNil(t, rtnConfig.SwOpts)
	assert.NotNil(t, rtnConfig.SwOpts.FileKeystore)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2 : Check with 'SW' as default provider
	// Case 2-1 : without SwOpts
	bccspConfig := &factory.FactoryOpts{
		ProviderName: "SW",
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.NotNil(t, rtnConfig.SwOpts)
	assert.NotNil(t, rtnConfig.SwOpts.FileKeystore)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-2 : without SwOpts.FileKeystore
	bccspConfig.SwOpts = &factory.SwOpts{
		HashFamily: "SHA2",
		SecLevel:   256,
		Ephemeral:  true,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.NotNil(t, rtnConfig.SwOpts.FileKeystore)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-3 : without SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SwOpts = &factory.SwOpts{
		HashFamily:   "SHA2",
		SecLevel:     256,
		FileKeystore: &factory.FileKeystoreOpts{},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-4 : with empty SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SwOpts = &factory.SwOpts{
		HashFamily:   "SHA2",
		SecLevel:     256,
		FileKeystore: &factory.FileKeystoreOpts{KeyStorePath: ""},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)

	// Case 3 : Check with 'PKCS11' as default provider
	// Case 3-1 : without SwOpts
	bccspConfig.ProviderName = "PKCS11"
	bccspConfig.SwOpts = nil
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.Nil(t, rtnConfig.SwOpts)

	// Case 3-2 : without SwOpts.FileKeystore
	bccspConfig.SwOpts = &factory.SwOpts{
		HashFamily: "SHA2",
		SecLevel:   256,
		Ephemeral:  true,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	assert.NotNil(t, rtnConfig.SwOpts.FileKeystore)
	assert.Equal(t, rtnConfig.SwOpts.FileKeystore.KeyStorePath, keystoreDir)
}

func TestGetLocalMspConfig(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	_, err := GetLocalMspConfig(mspDir, nil, "SampleOrg")
	assert.NoError(t, err)
}

func TestGetLocalMspConfigFails(t *testing.T) {
	_, err := GetLocalMspConfig("/tmp/", nil, "SampleOrg")
	assert.Error(t, err)
}

func TestGetPemMaterialFromDirWithFile(t *testing.T) {
	tempFile, err := ioutil.TempFile("", "fabric-msp-test")
	assert.NoError(t, err)
	err = tempFile.Close()
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	_, err = getPemMaterialFromDir(tempFile.Name())
	assert.Error(t, err)
}

func TestGetPemMaterialFromDirWithSymlinks(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	tempDir, err := ioutil.TempDir("", "fabric-msp-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dirSymlinkName := filepath.Join(tempDir, "..data")
	err = os.Symlink(filepath.Join(mspDir, "signcerts"), dirSymlinkName)
	assert.NoError(t, err)

	fileSymlinkTarget := filepath.Join("..data", "peer.pem")
	fileSymlinkName := filepath.Join(tempDir, "peer.pem")
	err = os.Symlink(fileSymlinkTarget, fileSymlinkName)
	assert.NoError(t, err)

	pemdataSymlink, err := getPemMaterialFromDir(tempDir)
	assert.NoError(t, err)
	expected, err := getPemMaterialFromDir(filepath.Join(mspDir, "signcerts"))
	assert.NoError(t, err)
	assert.Equal(t, pemdataSymlink, expected)
}

func TestReadFileUtils(t *testing.T) {
	// test that reading a file with an empty path doesn't crash
	_, err := readPemFile("")
	assert.Error(t, err)

	// test that reading an existing file which is not a PEM file doesn't crash
	_, err = readPemFile("/dev/null")
	assert.Error(t, err)
}
