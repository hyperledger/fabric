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
	"github.com/stretchr/testify/require"
)

func TestSetupBCCSPKeystoreConfig(t *testing.T) {
	keystoreDir := "/tmp"

	// Case 1 : Check with empty FactoryOpts
	rtnConfig := SetupBCCSPKeystoreConfig(nil, keystoreDir)
	require.NotNil(t, rtnConfig)
	require.Equal(t, rtnConfig.Default, "SW")
	require.NotNil(t, rtnConfig.SW)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2 : Check with 'SW' as default provider
	// Case 2-1 : without SwOpts
	bccspConfig := &factory.FactoryOpts{
		Default: "SW",
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-2 : without SwOpts.FileKeystore
	bccspConfig.SW = &factory.SwOpts{
		Hash:     "SHA2",
		Security: 256,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-3 : without SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SW = &factory.SwOpts{
		Hash:         "SHA2",
		Security:     256,
		FileKeystore: &factory.FileKeystoreOpts{},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-4 : with empty SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SW = &factory.SwOpts{
		Hash:         "SHA2",
		Security:     256,
		FileKeystore: &factory.FileKeystoreOpts{KeyStorePath: ""},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 3 : Check with 'PKCS11' as default provider
	// Case 3-1 : without SwOpts
	bccspConfig.Default = "PKCS11"
	bccspConfig.SW = nil
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Nil(t, rtnConfig.SW)

	// Case 3-2 : without SwOpts.FileKeystore
	bccspConfig.SW = &factory.SwOpts{
		Hash:     "SHA2",
		Security: 256,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)
}

func TestGetLocalMspConfig(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	_, err := GetLocalMspConfig(mspDir, nil, "SampleOrg")
	require.NoError(t, err)
}

func TestGetLocalMspConfigFails(t *testing.T) {
	_, err := GetLocalMspConfig("/tmp/", nil, "SampleOrg")
	require.Error(t, err)
}

func TestGetPemMaterialFromDirWithFile(t *testing.T) {
	tempFile, err := ioutil.TempFile("", "fabric-msp-test")
	require.NoError(t, err)
	err = tempFile.Close()
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	_, err = getPemMaterialFromDir(tempFile.Name())
	require.Error(t, err)
}

func TestGetPemMaterialFromDirWithSymlinks(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	tempDir := t.TempDir()

	dirSymlinkName := filepath.Join(tempDir, "..data")
	err := os.Symlink(filepath.Join(mspDir, "signcerts"), dirSymlinkName)
	require.NoError(t, err)

	fileSymlinkTarget := filepath.Join("..data", "peer.pem")
	fileSymlinkName := filepath.Join(tempDir, "peer.pem")
	err = os.Symlink(fileSymlinkTarget, fileSymlinkName)
	require.NoError(t, err)

	pemdataSymlink, err := getPemMaterialFromDir(tempDir)
	require.NoError(t, err)
	expected, err := getPemMaterialFromDir(filepath.Join(mspDir, "signcerts"))
	require.NoError(t, err)
	require.Equal(t, pemdataSymlink, expected)
}

func TestReadFileUtils(t *testing.T) {
	// test that reading a file with an empty path doesn't crash
	_, err := readPemFile("")
	require.Error(t, err)

	// test that reading an existing file which is not a PEM file doesn't crash
	_, err = readPemFile("/dev/null")
	require.Error(t, err)
}
