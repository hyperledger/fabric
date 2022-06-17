/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"fmt"
	"os"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func setupccdir(t *testing.T) string {
	tempDir := t.TempDir()
	SetChaincodesPath(tempDir)
	return tempDir
}

func processCDS(cds *pb.ChaincodeDeploymentSpec, tofs bool) (*CDSPackage, []byte, *ChaincodeData, error) {
	b := protoutil.MarshalOrPanic(cds)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error creating bootBCCSP: %s", err)
	}
	ccpack := &CDSPackage{GetHasher: cryptoProvider}
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
	_ = setupccdir(t)

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
	ccdir := setupccdir(t)

	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{
		Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
		Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}},
	}, CodePackage: []byte("code")}

	ccpack, b, _, err := processCDS(cds, true)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	// validate with invalid name
	if err = ccpack.ValidateCC(&ChaincodeData{Name: "invalname", Version: "0"}); err == nil {
		t.Fatalf("expected error validating package")
		return
	}
	// remove the buffer
	ccpack.buf = nil
	if err = ccpack.PutChaincodeToFS(); err == nil {
		t.Fatalf("expected error putting package on the FS")
		return
	}

	// put back  the buffer but remove the depspec
	ccpack.buf = b
	savDepSpec := ccpack.depSpec
	ccpack.depSpec = nil
	if err = ccpack.PutChaincodeToFS(); err == nil {
		t.Fatalf("expected error putting package on the FS")
		return
	}

	// put back dep spec
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

	b := protoutil.MarshalOrPanic(cds)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	ccpack, err := GetCCPackage(b, cryptoProvider)
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

// switch the chaincodes on the FS and validate
func TestCDSSwitchChaincodes(t *testing.T) {
	_ = setupccdir(t)

	// someone modified the code on the FS with "badcode"
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}, CodePackage: []byte("badcode")}

	// write the bad code to the fs
	badccpack, _, _, err := processCDS(cds, true)
	if err != nil {
		t.Fatalf("error putting CDS to FS %s", err)
		return
	}

	// mimic the good code ChaincodeData from the instantiate...
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
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	ccpack := &CDSPackage{GetHasher: cryptoProvider}
	err = ccpack.PutChaincodeToFS()
	require.Error(t, err)
	require.Contains(t, err.Error(), "uninitialized package", "Unexpected error returned")

	ccpack.buf = []byte("hello")
	err = ccpack.PutChaincodeToFS()
	require.Error(t, err)
	require.Contains(t, err.Error(), "id cannot be nil if buf is not nil", "Unexpected error returned")

	ccpack.id = []byte("cc123")
	err = ccpack.PutChaincodeToFS()
	require.Error(t, err)
	require.Contains(t, err.Error(), "depspec cannot be nil if buf is not nil", "Unexpected error returned")

	ccpack.depSpec = &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{
		Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
		Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}},
	}, CodePackage: []byte("code")}
	err = ccpack.PutChaincodeToFS()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil data", "Unexpected error returned")

	ccpack.data = &CDSData{}
	err = ccpack.PutChaincodeToFS()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil data bytes", "Unexpected error returned")
}

func TestValidateCCErrorPaths(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cpack := &CDSPackage{GetHasher: cryptoProvider}
	ccdata := &ChaincodeData{}
	err = cpack.ValidateCC(ccdata)
	require.Error(t, err)
	require.Contains(t, err.Error(), "uninitialized package", "Unexpected error returned")

	cpack.depSpec = &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte("code"),
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        1,
			ChaincodeId: &pb.ChaincodeID{Name: "testcc", Version: "0"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("")}},
		},
	}
	err = cpack.ValidateCC(ccdata)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil data", "Unexpected error returned")

	// invalid encoded name
	cpack = &CDSPackage{GetHasher: cryptoProvider}
	ccdata = &ChaincodeData{Name: "\027"}
	cpack.depSpec = &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte("code"),
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: ccdata.Name, Version: "0"},
		},
	}
	cpack.data = &CDSData{}
	err = cpack.ValidateCC(ccdata)
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid chaincode name: "\x17"`)

	// mismatched names
	cpack = &CDSPackage{GetHasher: cryptoProvider}
	ccdata = &ChaincodeData{Name: "Tom"}
	cpack.depSpec = &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte("code"),
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: "Jerry", Version: "0"},
		},
	}
	cpack.data = &CDSData{}
	err = cpack.ValidateCC(ccdata)
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid chaincode data name:"Tom"  (name:"Jerry" version:"0" )`)

	// mismatched versions
	cpack = &CDSPackage{GetHasher: cryptoProvider}
	ccdata = &ChaincodeData{Name: "Tom", Version: "1"}
	cpack.depSpec = &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte("code"),
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: ccdata.Name, Version: "0"},
		},
	}
	cpack.data = &CDSData{}
	err = cpack.ValidateCC(ccdata)
	require.Error(t, err)
	require.Contains(t, err.Error(), `invalid chaincode data name:"Tom" version:"1"  (name:"Tom" version:"0" )`)
}
