/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func initInstallTest(t *testing.T, fsPath string, ec pb.EndorserClient, mockResponse *pb.ProposalResponse) (*cobra.Command, *ChaincodeCmdFactory) {
	viper.Set("peer.fileSystemPath", fsPath)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	if mockResponse == nil {
		mockResponse = &pb.ProposalResponse{
			Response:    &pb.Response{Status: 200},
			Endorsement: &pb.Endorsement{},
		}
	}
	if ec == nil {
		ec = common.GetMockEndorserClient(mockResponse, nil)
	}

	mockCF := &ChaincodeCmdFactory{
		Signer:          signer,
		EndorserClients: []pb.EndorserClient{ec},
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	cmd := installCmd(mockCF, nil, cryptoProvider)
	addFlags(cmd)

	return cmd, mockCF
}

func TestInstallBadVersion(t *testing.T) {
	fsPath := t.TempDir()

	cmd, _ := initInstallTest(t, fsPath, nil, nil)

	args := []string{"-n", "mychaincode", "-p", "github.com/hyperledger/fabric/internal/peer/chaincode/testdata/src/chaincodes/noop"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for version not specified")
	}
}

func TestInstallNonExistentCC(t *testing.T) {
	fsPath := t.TempDir()

	cmd, _ := initInstallTest(t, fsPath, nil, nil)

	args := []string{"-n", "badmychaincode", "-p", "github.com/hyperledger/fabric/internal/peer/chaincode/testdata/src/chaincodes/bad_mychaincode", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for bad chaincode")
	}

	if _, err := os.Stat(fsPath + "/chaincodes/badmychaincode.testversion"); err == nil {
		t.Fatal("chaincode mychaincode.testversion should not exist")
	}
}

func TestInstallFromPackage(t *testing.T) {
	pdir := t.TempDir()

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := t.TempDir()

	cmd, mockCF := initInstallTest(t, fsPath, nil, nil)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockCF.EndorserClients = []pb.EndorserClient{mockEndorserClient}

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatal("error executing install command from package")
	}
}

func TestInstallFromBadPackage(t *testing.T) {
	pdir := t.TempDir()

	ccpackfile := pdir + "/ccpack.file"
	err := ioutil.WriteFile(ccpackfile, []byte("really bad CC package"), 0o700)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := t.TempDir()

	cmd, _ := initInstallTest(t, fsPath, nil, nil)

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error installing bad package")
	}
}

func installCC(t *testing.T) error {
	defer viper.Reset()

	fsPath := t.TempDir()
	cmd, _ := initInstallTest(t, fsPath, nil, nil)

	args := []string{"-n", "mychaincode", "-p", "github.com/hyperledger/fabric/internal/peer/chaincode/testdata/src/chaincodes/noop", "-v", "anotherversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("Run chaincode upgrade cmd error:%v", err)
	}

	return nil
}

func TestInstall(t *testing.T) {
	if err := installCC(t); err != nil {
		t.Fatalf("Install failed with error: %v", err)
	}
}
