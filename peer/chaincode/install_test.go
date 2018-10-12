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

	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func initInstallTest(fsPath string, t *testing.T) (*cobra.Command, *ChaincodeCmdFactory) {
	viper.Set("peer.fileSystemPath", fsPath)
	cleanupInstallTest(fsPath)

	//if mkdir fails everything will fail... but it should not
	if err := os.Mkdir(fsPath, 0755); err != nil {
		t.Fatalf("could not create install env")
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChaincodeCmdFactory{
		Signer: signer,
	}

	cmd := installCmd(mockCF)
	addFlags(cmd)

	return cmd, mockCF
}

func cleanupInstallTest(fsPath string) {
	os.RemoveAll(fsPath)
}

// TestBadVersion tests generation of install command
func TestBadVersion(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd, _ := initInstallTest(fsPath, t)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for version not specified")
	}
}

// TestNonExistentCC non existent chaincode should fail as expected
func TestNonExistentCC(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd, _ := initInstallTest(fsPath, t)
	defer cleanupInstallTest(fsPath)

	args := []string{"-n", "badexample02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/bad_example02", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Expected error executing install command for bad chaincode")
	}

	if _, err := os.Stat(fsPath + "/chaincodes/badexample02.testversion"); err == nil {
		t.Fatal("chaincode example02.testversion should not exist")
	}
}

// TestInstallFromPackage installs using package
func TestInstallFromPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := createSignedCDSPackage([]string{"-n", "somecc", "-p", "some/go/package", "-v", "0", ccpackfile}, false)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, mockCF := initInstallTest(fsPath, t)
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

// TestInstallFromBadPackage tests bad package failure
func TestInstallFromBadPackage(t *testing.T) {
	pdir := newTempDir()
	defer os.RemoveAll(pdir)

	ccpackfile := pdir + "/ccpack.file"
	err := ioutil.WriteFile(ccpackfile, []byte("really bad CC package"), 0700)
	if err != nil {
		t.Fatalf("could not create package :%v", err)
	}

	fsPath := "/tmp/installtest"

	cmd, mockCF := initInstallTest(fsPath, t)
	defer cleanupInstallTest(fsPath)

	//this should not reach the endorser which will respond with success
	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockCF.EndorserClients = []pb.EndorserClient{mockEndorserClient}

	args := []string{ccpackfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error installing bad package")
	}
}
func installEx02(t *testing.T) error {
	defer viper.Reset()
	viper.Set("chaincode.mode", "dev")

	fsPath := "/tmp/installtest"
	cmd, mockCF := initInstallTest(fsPath, t)
	defer cleanupInstallTest(fsPath)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockCF.EndorserClients = []pb.EndorserClient{mockEndorserClient}

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd", "-v", "anotherversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("Run chaincode upgrade cmd error:%v", err)
	}

	return nil
}

func TestInstall(t *testing.T) {
	if err := installEx02(t); err != nil {
		t.Fatalf("Install failed with error: %v", err)
	}
}
