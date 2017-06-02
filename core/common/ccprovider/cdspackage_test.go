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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func setupccdir() string {
	tempDir, err := ioutil.TempDir("/tmp", "ccprovidertest")
	if err != nil {
		panic(err)
	}
	SetChaincodesPath(tempDir)
	return tempDir
}

func processCDS(cds *pb.ChaincodeDeploymentSpec, tofs bool) (*CDSPackage, []byte, *ChaincodeData, error) {
	b := utils.MarshalOrPanic(cds)

	ccpack := &CDSPackage{}
	cd, err := ccpack.InitFromBuffer(b)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error owner creating package %s", err)
	}

	if tofs {
		if err = ccpack.PutChaincodeToFS(); err != nil {
			return nil, nil, nil, fmt.Errorf("error putting package on the FS %s", err)
		}
	}

	return ccpack, b, cd, nil
}

func TestPutCDSCC(t *testing.T) {
	ccdir := setupccdir()
	defer os.RemoveAll(ccdir)

	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("code")}

	ccpack, _, cd, err := processCDS(cds, true)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	if err = ccpack.ValidateCC(cd); err != nil {
		t.Fatalf("error validating package %s", err)
		return
	}
}

func TestPutCDSErrorPaths(t *testing.T) {
	ccdir := setupccdir()
	defer os.RemoveAll(ccdir)

	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
		Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("code")}

	ccpack, b, _, err := processCDS(cds, true)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	//validate with invalid name
	if err = ccpack.ValidateCC(&ChaincodeData{Name: "invalname", Version: "0"}); err == nil {
		t.Fatalf("expected error validating package")
		return
	}
	//remove the buffer
	ccpack.buf = nil
	if err = ccpack.PutChaincodeToFS(); err == nil {
		t.Fatalf("expected error putting package on the FS")
		return
	}

	//put back  the buffer but remove the depspec
	ccpack.buf = b
	savDepSpec := ccpack.depSpec
	ccpack.depSpec = nil
	if err = ccpack.PutChaincodeToFS(); err == nil {
		t.Fatalf("expected error putting package on the FS")
		return
	}

	//put back dep spec
	ccpack.depSpec = savDepSpec

	//...but remove the chaincode directory
	os.RemoveAll(ccdir)
	if err = ccpack.PutChaincodeToFS(); err == nil {
		t.Fatalf("expected error putting package on the FS")
		return
	}
}

func TestCDSGetCCPackage(t *testing.T) {
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("code")}

	b := utils.MarshalOrPanic(cds)

	ccpack, err := GetCCPackage(b)
	if err != nil {
		t.Fatalf("failed to get CDS CCPackage %s", err)
		return
	}

	cccdspack, ok := ccpack.(*CDSPackage)
	if !ok || cccdspack == nil {
		t.Fatalf("failed to get CDS CCPackage")
		return
	}

	cds2 := cccdspack.GetDepSpec()
	if cds2 == nil {
		t.Fatalf("nil dep spec in CDS CCPackage")
		return
	}

	if cds2.ChaincodeSpec.ChaincodeId.Name != cds.ChaincodeSpec.ChaincodeId.Name || cds2.ChaincodeSpec.ChaincodeId.Version != cds.ChaincodeSpec.ChaincodeId.Version {
		t.Fatalf("dep spec in CDS CCPackage does not match %v != %v", cds, cds2)
		return
	}
}

//switch the chaincodes on the FS and validate
func TestCDSSwitchChaincodes(t *testing.T) {
	ccdir := setupccdir()
	defer os.RemoveAll(ccdir)

	//someone modified the code on the FS with "badcode"
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("badcode")}

	//write the bad code to the fs
	badccpack, _, _, err := processCDS(cds, true)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	//mimic the good code ChaincodeData from the instantiate...
	cds.CodePackage = []byte("goodcode")

	//...and generate the CD for it (don't overwrite the bad code)
	_, _, goodcd, err := processCDS(cds, false)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	if err = badccpack.ValidateCC(goodcd); err == nil {
		t.Fatalf("expected goodcd to fail against bad package but succeeded!")
		return
	}
}

func TestPutChaincodeToFSErrorPaths(t *testing.T) {
	ccpack := &CDSPackage{}
	err := ccpack.PutChaincodeToFS()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uninitialized package", "Unexpected error returned")

	ccpack.buf = []byte("hello")
	err = ccpack.PutChaincodeToFS()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id cannot be nil if buf is not nil", "Unexpected error returned")

	ccpack.id = []byte("cc123")
	err = ccpack.PutChaincodeToFS()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "depspec cannot be nil if buf is not nil", "Unexpected error returned")

	ccpack.depSpec = &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
		Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("code")}
	err = ccpack.PutChaincodeToFS()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil data", "Unexpected error returned")

	ccpack.data = &CDSData{}
	err = ccpack.PutChaincodeToFS()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil data bytes", "Unexpected error returned")
}

func TestValidateCCErrorPaths(t *testing.T) {
	cpack := &CDSPackage{}
	ccdata := &ChaincodeData{}
	err := cpack.ValidateCC(ccdata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uninitialized package", "Unexpected error returned")

	cpack.depSpec = &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
		Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("code")}
	err = cpack.ValidateCC(ccdata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil data", "Unexpected error returned")
}
