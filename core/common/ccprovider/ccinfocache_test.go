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
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func getDepSpec(name string, path string, version string, initArgs [][]byte) (*peer.ChaincodeDeploymentSpec, error) {
	spec := &peer.ChaincodeSpec{Type: 1, ChaincodeId: &peer.ChaincodeID{Name: name, Path: path, Version: version}, Input: &peer.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	payload := []byte(name + path + version)
	err := tw.WriteHeader(&tar.Header{
		Name: "src/garbage.go",
		Size: int64(len(payload)),
		Mode: 0o100644,
	})
	if err != nil {
		return nil, err
	}

	_, err = tw.Write(payload)
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

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}
	cccdspack := &CDSPackage{GetHasher: cryptoProvider}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		return nil, err
	}

	return cccdspack, nil
}

type mockCCInfoFSStorageMgrImpl struct {
	CCMap map[string]CCPackage
}

func (m *mockCCInfoFSStorageMgrImpl) GetChaincode(ccNameVersion string) (CCPackage, error) {
	return m.CCMap[ccNameVersion], nil
}

// here we test the cache implementation itself
func TestCCInfoCache(t *testing.T) {
	ccinfoFs := &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	cccache := NewCCInfoCache(ccinfoFs)

	// test the get side

	// the cc data is not yet in the cache
	_, err := cccache.GetChaincodeData("foo:1.0")
	require.Error(t, err)

	// put it in the file system
	pack, err := buildPackage("foo", "mychaincode", "1.0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)
	ccinfoFs.CCMap["foo:1.0"] = pack

	// expect it to be in the cache now
	cd1, err := cccache.GetChaincodeData("foo:1.0")
	require.NoError(t, err)

	// it should still be in the cache
	cd2, err := cccache.GetChaincodeData("foo:1.0")
	require.NoError(t, err)

	// they are not null
	require.NotNil(t, cd1)
	require.NotNil(t, cd2)

	// put it in the file system
	pack, err = buildPackage("foo", "mychaincode", "2.0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)
	ccinfoFs.CCMap["foo:2.0"] = pack

	// create a dep spec to put
	_, err = getDepSpec("foo", "mychaincode", "2.0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)

	// expect it to be cached
	cd1, err = cccache.GetChaincodeData("foo:2.0")
	require.NoError(t, err)

	// it should still be in the cache
	cd2, err = cccache.GetChaincodeData("foo:2.0")
	require.NoError(t, err)

	// they are not null
	require.NotNil(t, cd1)
	require.NotNil(t, cd2)
}

func TestPutChaincode(t *testing.T) {
	ccname := ""
	ccver := "1.0"
	ccpath := "mychaincode"

	ccinfoFs := &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	NewCCInfoCache(ccinfoFs)

	// Error case 1: ccname is empty
	// create a dep spec to put
	_, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)

	// Error case 2: ccver is empty
	ccname = "foo"
	ccver = ""
	_, err = getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)

	// Error case 3: ccfs.PutChainCode returns an error
	ccinfoFs = &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	NewCCInfoCache(ccinfoFs)

	ccname = "foo"
	ccver = "1.0"
	_, err = getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)
}

// here we test the peer's built-in cache after enabling it
func TestCCInfoFSPeerInstance(t *testing.T) {
	ccname := "bar"
	ccver := "1.0"
	ccpath := "mychaincode"

	// the cc data is not yet in the cache
	_, err := GetChaincodeFromFS("bar:1.0")
	require.Error(t, err)

	// create a dep spec to put
	ds, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	require.NoError(t, err)

	// put it
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	ccinfoFSImpl := &CCInfoFSImpl{GetHasher: cryptoProvider}
	_, err = ccinfoFSImpl.PutChaincode(ds)
	require.NoError(t, err)

	// Get all installed chaincodes, it should not return 0 chaincodes
	resp, err := GetInstalledChaincodes()
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotZero(t, len(resp.Chaincodes), "GetInstalledChaincodes should not have returned 0 chaincodes")

	// get chaincode data
	_, err = GetChaincodeData("bar:1.0")
	require.NoError(t, err)
}

func TestGetInstalledChaincodesErrorPaths(t *testing.T) {
	// Get the existing chaincode install path value and set it
	// back after we are done with the test
	cip := chaincodeInstallPath
	defer SetChaincodesPath(cip)

	// Create a temp dir and remove it at the end
	dir := t.TempDir()

	// Set the above created directory as the chaincode install path
	SetChaincodesPath(dir)
	err := ioutil.WriteFile(filepath.Join(dir, "idontexist.1.0"), []byte("test"), 0o777)
	require.NoError(t, err)
	resp, err := GetInstalledChaincodes()
	require.NoError(t, err)
	require.Equal(t, 0, len(resp.Chaincodes),
		"Expected 0 chaincodes but GetInstalledChaincodes returned %s chaincodes", len(resp.Chaincodes))
}

func TestChaincodePackageExists(t *testing.T) {
	_, err := ChaincodePackageExists("foo1", "1.0")
	require.Error(t, err)
}

func TestSetChaincodesPath(t *testing.T) {
	dir := t.TempDir()
	t.Logf("created temp dir %s", dir)

	// Get the existing chaincode install path value and set it
	// back after we are done with the test
	cip := chaincodeInstallPath
	defer SetChaincodesPath(cip)

	f, err := ioutil.TempFile(dir, "chaincodes")
	require.NoError(t, err)
	require.Panics(t, func() {
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
