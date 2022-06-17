/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider_test

import (
	"crypto/sha256"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestInstalledCCs(t *testing.T) {
	tmpDir, hashes := setupDirectoryStructure(t)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

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
					Hash:    hashes["example02.1.0"],
				},
				{
					Name:    "example04",
					Version: "1",
					Hash:    hashes["example04.1"],
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
			extractCCFromPath: func(_ string, _ string, _ ccprovider.GetHasher) (ccprovider.CCPackage, error) {
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
			c := &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider}
			res, err := c.ListInstalledChaincodes(path.Join(tmpDir, test.directory), test.ls, test.extractCCFromPath)
			require.Equal(t, test.expected, res)
			if test.errorContains == "" {
				require.NoError(t, err)
			} else {
				require.Contains(t, err.Error(), test.errorContains)
			}
		})
	}
}

func TestSetGetChaincodeInstallPath(t *testing.T) {
	tempDir := t.TempDir()
	ccprovider.SetChaincodesPath(tempDir)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	c := &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider}
	installPath := c.GetChaincodeInstallPath()
	defer ccprovider.SetChaincodesPath(installPath)

	path := filepath.Join(tempDir, "blahblah")
	ccprovider.SetChaincodesPath(path)
	require.DirExistsf(t, path, "expect %s to be created")

	require.Equal(t, path, c.GetChaincodeInstallPath())
}

func setupDirectoryStructure(t *testing.T) (string, map[string][]byte) {
	files := []string{
		"example02.1.0", // Version contains the delimiter '.' is a valid case
		"example03",     // No version specified
		"example04.1",   // Version doesn't contain the '.' delimiter
	}
	hashes := map[string][]byte{}
	tmp := t.TempDir()
	dir := path.Join(tmp, "empty")
	require.NoError(t, os.Mkdir(dir, 0o755))
	dir = path.Join(tmp, "nonempty")
	require.NoError(t, os.Mkdir(dir, 0o755))
	dir = path.Join(tmp, "nopermission")
	require.NoError(t, os.Mkdir(dir, 0o755))
	dir = path.Join(tmp, "nopermissionforfiles")
	require.NoError(t, os.Mkdir(dir, 0o755))
	noPermissionFile := path.Join(tmp, "nopermissionforfiles", "nopermission.1")
	_, err := os.Create(noPermissionFile)
	require.NoError(t, err)
	dir = path.Join(tmp, "nonempty")
	require.NoError(t, os.Mkdir(path.Join(tmp, "nonempty", "directory"), 0o755))
	for _, f := range files {
		var name, ver string
		parts := strings.SplitN(f, ".", 2)
		name = parts[0]
		if len(parts) > 1 {
			ver = parts[1]
		}
		file, err := os.Create(path.Join(dir, f))
		require.NoError(t, err)
		cds := &peer.ChaincodeDeploymentSpec{
			ChaincodeSpec: &peer.ChaincodeSpec{
				ChaincodeId: &peer.ChaincodeID{Name: name, Version: ver},
			},
		}

		codehash := sha256.New()
		codehash.Write(cds.CodePackage)

		metahash := sha256.New()
		metahash.Write([]byte(name))
		metahash.Write([]byte(ver))

		hash := sha256.New()
		hash.Write(codehash.Sum(nil))
		hash.Write(metahash.Sum(nil))

		hashes[f] = hash.Sum(nil)

		b, _ := proto.Marshal(cds)
		file.Write(b)
		file.Close()
	}

	return tmp, hashes
}
