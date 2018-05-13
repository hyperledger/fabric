/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/cclifecycle/mocks"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
		ls                cc.DirEnumerator
		extractCCFromPath cc.ChaincodeExtractor
	}{
		{
			name:              "None empty directory",
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
			name:              "None present directory",
			ls:                ioutil.ReadDir,
			extractCCFromPath: ccprovider.LoadPackage,
			expected:          nil,
			directory:         "notexistent",
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
				return nil, errors.New("permission denied")
			},
			extractCCFromPath: ccprovider.LoadPackage,
			expected:          nil,
			directory:         "nopermission",
			errorContains:     "permission denied",
		},
		{
			name: "No permission on chaincode files",
			ls:   ioutil.ReadDir,
			extractCCFromPath: func(_ string, _ string, _ string) (ccprovider.CCPackage, error) {
				return nil, errors.New("permission denied")
			},
			expected:      nil,
			directory:     "nopermissionforfiles",
			errorContains: "permission denied",
		},
	}
	_ = testCases

	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			res, err := cc.InstalledCCs(path.Join(tmpDir, test.directory), test.ls, test.extractCCFromPath)
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

func TestChaincodeInspection(t *testing.T) {
	acceptAll := func(cc chaincode.Metadata) bool {
		return true
	}

	policy := &common.SignaturePolicyEnvelope{}
	policy.Rule = &common.SignaturePolicy{
		Type: &common.SignaturePolicy_NOutOf_{
			NOutOf: &common.SignaturePolicy_NOutOf{
				N: int32(2),
			},
		},
	}
	policyBytes, _ := proto.Marshal(policy)

	cc1 := &ccprovider.ChaincodeData{Name: "cc1", Version: "1.0", Policy: policyBytes, Id: []byte{42}}
	cc1Bytes, _ := proto.Marshal(cc1)
	acceptSpecificCCId := func(cc chaincode.Metadata) bool {
		// cc1.Id above is 42 and not 24
		return bytes.Equal(cc.Id, []byte{24})
	}

	var corruptBytes []byte
	corruptBytes = append(corruptBytes, cc1Bytes...)
	corruptBytes = append(corruptBytes, 0)

	cc2 := &ccprovider.ChaincodeData{Name: "cc2", Version: "1.1"}
	cc2Bytes, _ := proto.Marshal(cc2)

	tests := []struct {
		name              string
		expected          chaincode.MetadataSet
		returnedCCBytes   []byte
		queryErr          error
		queriedChaincodes []string
		filter            func(cc chaincode.Metadata) bool
	}{
		{
			name:              "No chaincodes installed",
			expected:          nil,
			filter:            acceptAll,
			queriedChaincodes: []string{},
		},
		{
			name:              "Failure on querying the ledger",
			queryErr:          errors.New("failed querying ledger"),
			filter:            acceptAll,
			queriedChaincodes: []string{"cc1", "cc2"},
			expected:          nil,
		},
		{
			name:              "The data in LSCC is corrupt",
			returnedCCBytes:   corruptBytes,
			filter:            acceptAll,
			queriedChaincodes: []string{"cc1"},
			expected:          nil,
		},
		{
			name:              "The chaincode is listed as a different chaincode in the ledger",
			returnedCCBytes:   cc2Bytes,
			filter:            acceptAll,
			queriedChaincodes: []string{"cc1"},
			expected:          nil,
		},
		{
			name:              "Chaincode doesn't exist",
			returnedCCBytes:   nil,
			filter:            acceptAll,
			queriedChaincodes: []string{"cc1"},
			expected:          nil,
		},
		{
			name:              "Select a specific chaincode",
			returnedCCBytes:   cc1Bytes,
			queriedChaincodes: []string{"cc1", "cc2"},
			filter:            acceptSpecificCCId,
		},
		{
			name:              "Green path",
			returnedCCBytes:   cc1Bytes,
			filter:            acceptAll,
			queriedChaincodes: []string{"cc1", "cc2"},
			expected: chaincode.MetadataSet([]chaincode.Metadata{
				{
					Id:      []byte{42},
					Name:    "cc1",
					Version: "1.0",
					Policy:  policyBytes,
				},
				{
					Name:    "cc2",
					Version: "1.1",
				},
			}),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			query := &mocks.Query{}
			query.On("Done")
			query.On("GetState", mock.Anything, mock.Anything).Return(test.returnedCCBytes, test.queryErr).Once()
			query.On("GetState", mock.Anything, mock.Anything).Return(cc2Bytes, nil).Once()
			ccInfo, err := cc.DeployedChaincodes(query, test.filter, false, test.queriedChaincodes...)
			if test.queryErr != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, ccInfo)
		})
	}
}
