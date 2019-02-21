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

	"github.com/hyperledger/fabric/internal/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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

	cmd := installCmd(mockCF, nil)
	addFlags(cmd)

	return cmd, mockCF
}

func cleanupInstallTest(fsPath string) {
	os.RemoveAll(fsPath)
}

func TestInstallBadVersion(t *testing.T) {
	fsPath, err := ioutil.TempDir("", "installbadversion")
	assert.NoError(t, err)

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "mychaincode", "-p", "github.com/hyperledger/fabric/internal/peer/chaincode/testdata/src/chaincodes/noop"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for version not specified")
	}
}

func TestInstallNonExistentCC(t *testing.T) {
	fsPath, err := ioutil.TempDir("", "install-nonexistentcc")
	assert.NoError(t, err)

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

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
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage(t, []string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, mockCF := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

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
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := ioutil.WriteFile(ccpackfile, []byte("really bad CC package"), 0700)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error installing bad package")
	}
}

func installCC(t *testing.T) error {
	defer viper.Reset()

	fsPath, err := ioutil.TempDir("", "installLegacyEx02")
	assert.NoError(t, err)
	cmd, _ := initInstallTest(t, fsPath, nil, nil)
	defer cleanupInstallTest(fsPath)

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

func newInstallerForTest(t *testing.T, ec pb.EndorserClient) (installer *Installer, cleanup func()) {
	fsPath, err := ioutil.TempDir("", "installerForTest")
	assert.NoError(t, err)
	_, mockCF := initInstallTest(t, fsPath, ec, nil)

	i := &Installer{
		EndorserClients: mockCF.EndorserClients,
		Signer:          mockCF.Signer,
	}

	cleanupFunc := func() {
		cleanupInstallTest(fsPath)
	}

	return i, cleanupFunc
}
