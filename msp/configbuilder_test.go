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

package msp

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/core/config"
	"github.com/stretchr/testify/assert"
)

func TestGetLocalMspConfig(t *testing.T) {
	mspDir, err := config.GetDevMspDir()
	assert.NoError(t, err)
	_, err = GetLocalMspConfig(mspDir, nil, "DEFAULT")
	assert.NoError(t, err)
}

func TestGetLocalMspConfigFails(t *testing.T) {
	_, err := GetLocalMspConfig("/tmp/", nil, "DEFAULT")
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
	mspDir, err := config.GetDevMspDir()
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
