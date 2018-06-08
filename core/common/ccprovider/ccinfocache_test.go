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

package ccprovider

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func getDepSpec(name string, path string, version string, initArgs [][]byte) (*peer.ChaincodeDeploymentSpec, error) {
	spec := &peer.ChaincodeSpec{Type: 1, ChaincodeId: &peer.ChaincodeID{Name: name, Path: path, Version: version}, Input: &peer.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	err := util.WriteBytesToPackage("src/garbage.go", []byte(name+path+version), tw)
	if err != nil {
		return nil, err
	}

	tw.Close()
	gz.Close()

	return &peer.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes.Bytes()}, nil
}

func buildPackage(name string, path string, version string, initArgs [][]byte) (CCPackage, error) {
	depSpec, err := getDepSpec(name, path, version, initArgs)
	if err != nil {
		return nil, err
	}

	buf, err := proto.Marshal(depSpec)
	if err != nil {
		return nil, err
	}
	cccdspack := &CDSPackage{}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		return nil, err
	}

	return cccdspack, nil
}

type mockCCInfoFSStorageMgrImpl struct {
	CCMap map[string]CCPackage
}

func (m *mockCCInfoFSStorageMgrImpl) GetChaincode(ccname string, ccversion string) (CCPackage, error) {
	return m.CCMap[ccname+ccversion], nil
}

// here we test the cache implementation itself
func TestCCInfoCache(t *testing.T) {
	ccname := "foo"
	ccver := "1.0"
	ccpath := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ccinfoFs := &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	cccache := NewCCInfoCache(ccinfoFs)

	// test the get side

	// the cc data is not yet in the cache
	_, err := cccache.GetChaincodeData(ccname, ccver)
	assert.Error(t, err)

	// put it in the file system
	pack, err := buildPackage(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)
	ccinfoFs.CCMap[ccname+ccver] = pack

	// expect it to be in the cache now
	cd1, err := cccache.GetChaincodeData(ccname, ccver)
	assert.NoError(t, err)

	// it should still be in the cache
	cd2, err := cccache.GetChaincodeData(ccname, ccver)
	assert.NoError(t, err)

	// they are not null
	assert.NotNil(t, cd1)
	assert.NotNil(t, cd2)

	// test the put side now..
	ccver = "2.0"
	// put it in the file system
	pack, err = buildPackage(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)
	ccinfoFs.CCMap[ccname+ccver] = pack

	// create a dep spec to put
	_, err = getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// expect it to be cached
	cd1, err = cccache.GetChaincodeData(ccname, ccver)
	assert.NoError(t, err)

	// it should still be in the cache
	cd2, err = cccache.GetChaincodeData(ccname, ccver)
	assert.NoError(t, err)

	// they are not null
	assert.NotNil(t, cd1)
	assert.NotNil(t, cd2)
}

func TestPutChaincode(t *testing.T) {
	ccname := ""
	ccver := "1.0"
	ccpath := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	ccinfoFs := &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	NewCCInfoCache(ccinfoFs)

	// Error case 1: ccname is empty
	// create a dep spec to put
	_, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// Error case 2: ccver is empty
	ccname = "foo"
	ccver = ""
	_, err = getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// Error case 3: ccfs.PutChainCode returns an error
	ccinfoFs = &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	NewCCInfoCache(ccinfoFs)

	ccname = "foo"
	ccver = "1.0"
	_, err = getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)
}

// here we test the peer's built-in cache after enabling it
func TestCCInfoFSPeerInstance(t *testing.T) {
	ccname := "bar"
	ccver := "1.0"
	ccpath := "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"

	// the cc data is not yet in the cache
	_, err := GetChaincodeFromFS(ccname, ccver)
	assert.Error(t, err)

	// create a dep spec to put
	ds, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// put it
	err = PutChaincodeIntoFS(ds)
	assert.NoError(t, err)

	// Get all installed chaincodes, it should not return 0 chaincodes
	resp, err := GetInstalledChaincodes()
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotZero(t, len(resp.Chaincodes), "GetInstalledChaincodes should not have returned 0 chaincodes")

	//get chaincode data
	_, err = GetChaincodeData(ccname, ccver)
	assert.NoError(t, err)
}

func TestGetInstalledChaincodesErrorPaths(t *testing.T) {
	// Get the existing chaincode install path value and set it
	// back after we are done with the test
	cip := chaincodeInstallPath
	defer SetChaincodesPath(cip)

	// Create a temp dir and remove it at the end
	dir, err := ioutil.TempDir(os.TempDir(), "chaincodes")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Set the above created directory as the chaincode install path
	SetChaincodesPath(dir)
	err = ioutil.WriteFile(filepath.Join(dir, "idontexist.1.0"), []byte("test"), 0777)
	assert.NoError(t, err)
	resp, err := GetInstalledChaincodes()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(resp.Chaincodes),
		"Expected 0 chaincodes but GetInstalledChaincodes returned %s chaincodes", len(resp.Chaincodes))
}

func TestChaincodePackageExists(t *testing.T) {
	_, err := ChaincodePackageExists("foo1", "1.0")
	assert.Error(t, err)
}

func TestSetChaincodesPath(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "setchaincodes")
	if err != nil {
		assert.Fail(t, err.Error(), "Unable to create temp dir")
	}
	defer os.RemoveAll(dir)
	t.Logf("created temp dir %s", dir)

	// Get the existing chaincode install path value and set it
	// back after we are done with the test
	cip := chaincodeInstallPath
	defer SetChaincodesPath(cip)

	f, err := ioutil.TempFile(dir, "chaincodes")
	assert.NoError(t, err)
	assert.Panics(t, func() {
		SetChaincodesPath(f.Name())
	}, "SetChaincodesPath should have paniced if a file is passed to it")

	// Following code works on mac but does not work in CI
	// // Make the directory read only
	// err = os.Chmod(dir, 0444)
	// assert.NoError(t, err)
	// cdir := filepath.Join(dir, "chaincodesdir")
	// assert.Panics(t, func() {
	// 	SetChaincodesPath(cdir)
	// }, "SetChaincodesPath should have paniced if it is not able to stat the dir")

	// // Make the directory read and execute
	// err = os.Chmod(dir, 0555)
	// assert.NoError(t, err)
	// assert.Panics(t, func() {
	// 	SetChaincodesPath(cdir)
	// }, "SetChaincodesPath should have paniced if it is not able to create the dir")
}

var ccinfocachetestpath = "/tmp/ccinfocachetest"

func TestMain(m *testing.M) {
	os.RemoveAll(ccinfocachetestpath)

	SetChaincodesPath(ccinfocachetestpath)
	rc := m.Run()
	os.RemoveAll(ccinfocachetestpath)
	os.Exit(rc)
}
