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
	"testing"

	"os"

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

func (m *mockCCInfoFSStorageMgrImpl) PutChaincode(depSpec *peer.ChaincodeDeploymentSpec) (CCPackage, error) {
	buf, err := proto.Marshal(depSpec)
	if err != nil {
		return nil, err
	}
	cccdspack := &CDSPackage{}
	if _, err := cccdspack.InitFromBuffer(buf); err != nil {
		return nil, err
	}

	m.CCMap[depSpec.ChaincodeSpec.ChaincodeId.Name+depSpec.ChaincodeSpec.ChaincodeId.Version] = cccdspack

	return cccdspack, nil
}

func (m *mockCCInfoFSStorageMgrImpl) GetChaincode(ccname string, ccversion string) (CCPackage, error) {
	return m.CCMap[ccname+ccversion], nil
}

// here we test the cache implementation itself
func TestCCInfoCache(t *testing.T) {
	ccname := "foo"
	ccver := "1.0"
	ccpath := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	ccinfoFs := &mockCCInfoFSStorageMgrImpl{CCMap: map[string]CCPackage{}}
	cccache := NewCCInfoCache(ccinfoFs)

	// test the get side

	// the cc data is not yet in the cache
	_, err := cccache.GetChaincode(ccname, ccver)
	assert.Error(t, err)

	// put it in the file system
	pack, err := buildPackage(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)
	ccinfoFs.CCMap[ccname+ccver] = pack

	// expect it to be in the cache now
	cd1, err := cccache.GetChaincode(ccname, ccver)
	assert.NoError(t, err)

	// it should still be in the cache
	cd2, err := cccache.GetChaincode(ccname, ccver)
	assert.NoError(t, err)

	// they are not null
	assert.NotNil(t, cd1)
	assert.NotNil(t, cd2)

	// test the put side now..
	ccver = "2.0"

	// create a dep spec to put
	ds, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// put it
	_, err = cccache.PutChaincode(ds)
	assert.NoError(t, err)

	// expect it to be in the cache
	cd1, err = cccache.GetChaincode(ccname, ccver)
	assert.NoError(t, err)

	// it should still be in the cache
	cd2, err = cccache.GetChaincode(ccname, ccver)
	assert.NoError(t, err)

	// they are not null
	assert.NotNil(t, cd1)
	assert.NotNil(t, cd2)
}

// here we test the peer's built-in cache after enabling it
func TestCCInfoCachePeerInstance(t *testing.T) {
	// enable the cache first: it's disabled by default
	EnableCCInfoCache()

	ccname := "foo"
	ccver := "1.0"
	ccpath := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	// the cc data is not yet in the cache
	_, err := GetChaincodeFromFS(ccname, ccver)
	assert.Error(t, err)

	// create a dep spec to put
	ds, err := getDepSpec(ccname, ccpath, ccver, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	assert.NoError(t, err)

	// put it
	err = PutChaincodeIntoFS(ds)
	assert.NoError(t, err)

	// expect it to be in the cache
	cd, err := GetChaincodeFromFS(ccname, ccver)
	assert.NoError(t, err)
	assert.NotNil(t, cd)
}

var ccinfocachetestpath = "/tmp/ccinfocachetest"

func TestMain(m *testing.M) {
	os.RemoveAll(ccinfocachetestpath)
	defer os.RemoveAll(ccinfocachetestpath)

	SetChaincodesPath(ccinfocachetestpath)
	os.Exit(m.Run())
}
