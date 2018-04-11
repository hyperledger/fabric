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

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/stretchr/testify/assert"
)

func TestGetLocalMspConfig(t *testing.T) {
	mspDir, err := configtest.GetDevMspDir()
	assert.NoError(t, err)
	_, err = GetLocalMspConfig(mspDir, nil, "SampleOrg")
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
	mspDir, err := configtest.GetDevMspDir()
	assert.NoError(t, err)

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
