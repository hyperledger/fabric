/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestInstalledCCs(t *testing.T) {
	tmpDir := setupDirectoryStructure(t)
	defer func() {
		os.RemoveAll(tmpDir)
	}()
	testCases := []struct {
		name              string
		directory         string
		expected          []chaincode.InstalledChaincode
		errorContains     string
		ls                ccprovider.DirEnumerator
		extractCCFromPath ccprovider.ChaincodeExtractor
	}{
		{
			name:              "Non-empty directory",
			ls:                ioutil.ReadDir,
			extractCCFromPath: ccprovider.LoadPackage,
			expected: []chaincode.InstalledChaincode{
				{
					Name:    "example02",
					Version: "1.0",
					Id:      []byte{45, 186, 93, 188, 51, 158, 115, 22, 174, 162, 104, 63, 175, 131, 156, 27, 123, 30, 226, 49, 61, 183, 146, 17, 37, 136, 17, 141, 240, 102, 170, 53},
				},
				{
					Name:    "example04",
					Version: "1",
					Id:      []byte{45, 186, 93, 188, 51, 158, 115, 22, 174, 162, 104, 63, 175, 131, 156, 27, 123, 30, 226, 49, 61, 183, 146, 17, 37, 136, 17, 141, 240, 102, 170, 53},
				},
			},
			directory: "nonempty",
		},
		{
			name:              "Nonexistent directory",
			ls:                ioutil.ReadDir,
			extractCCFromPath: ccprovider.LoadPackage,
			expected:          nil,
			directory:         "nonexistent",
		},
		{
			name:              "Empty directory",
			ls:                ioutil.ReadDir,
			extractCCFromPath: ccprovider.LoadPackage,
			expected:          nil,
			directory:         "empty",
		},
		{
			name: "No permission to open directory",
			ls: func(_ string) ([]os.FileInfo, error) {
				return nil, errors.New("orange")
			},
			extractCCFromPath: ccprovider.LoadPackage,
			expected:          nil,
			directory:         "nopermission",
			errorContains:     "orange",
		},
		{
			name: "No permission on chaincode files",
			ls:   ioutil.ReadDir,
			extractCCFromPath: func(_ string, _ string, _ string) (ccprovider.CCPackage, error) {
				return nil, errors.New("banana")
			},
			expected:      nil,
			directory:     "nopermissionforfiles",
			errorContains: "banana",
		},
	}
	_ = testCases

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			c := &ccprovider.CCInfoFSImpl{}
			res, err := c.ListInstalledChaincodes(path.Join(tmpDir, test.directory), test.ls, test.extractCCFromPath)
			assert.Equal(t, test.expected, res)
			if test.errorContains == "" {
				assert.NoError(t, err)
			} else {
				assert.Contains(t, err.Error(), test.errorContains)
			}
		})
	}
}

func setupDirectoryStructure(t *testing.T) string {
	files := []string{
		"example02.1.0", // Version contains the delimiter '.' is a valid case
		"example03",     // No version specified
		"example04.1",   // Version doesn't contain the '.' delimiter
	}
	rand.Seed(time.Now().UnixNano())
	tmp := path.Join(os.TempDir(), fmt.Sprintf("%d", rand.Int()))
	assert.NoError(t, os.Mkdir(tmp, 0755))
	dir := path.Join(tmp, "empty")
	assert.NoError(t, os.Mkdir(dir, 0755))
	dir = path.Join(tmp, "nonempty")
	assert.NoError(t, os.Mkdir(dir, 0755))
	dir = path.Join(tmp, "nopermission")
	assert.NoError(t, os.Mkdir(dir, 0755))
	dir = path.Join(tmp, "nopermissionforfiles")
	assert.NoError(t, os.Mkdir(dir, 0755))
	noPermissionFile := path.Join(tmp, "nopermissionforfiles", "nopermission.1")
	_, err := os.Create(noPermissionFile)
	assert.NoError(t, err)
	dir = path.Join(tmp, "nonempty")
	assert.NoError(t, os.Mkdir(path.Join(tmp, "nonempty", "directory"), 0755))
	for _, f := range files {
		file, err := os.Create(path.Join(dir, f))
		assert.NoError(t, err)
		cds := &peer.ChaincodeDeploymentSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{},
			},
		}
		b, _ := proto.Marshal(cds)
		file.Write(b)
		file.Close()
	}

	return tmp
}
