/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/stretchr/testify/assert"
)

func TestInstalledMissingChaincodeName(t *testing.T) {
	defer resetFlags()

	mockCF := createCheckInstalledMockCF(t)

	cmd := checkinstalledCmd(mockCF)
	addFlags(cmd)

	args := []string{"--version", "1.0"}
	cmd.SetArgs(args)

	err := cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Equal(t, err.Error(), "Must supply chaincode name")
}

func TestInstalledMissingChaincodeVersion(t *testing.T) {
	defer resetFlags()

	mockCF := createCheckInstalledMockCF(t)

	cmd := checkinstalledCmd(mockCF)
	addFlags(cmd)

	args := []string{"--name", "name"}
	cmd.SetArgs(args)

	err := cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Equal(t, err.Error(), "Must supply chaincode version")
}

func TestInstalled(t *testing.T) {
	defer resetFlags()

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	mockCF := createCheckInstalledMockCF(t)

	cmd := checkinstalledCmd(mockCF)
	addFlags(cmd)

	args := []string{"--name", "mycc1", "--version", "1.0"}
	cmd.SetArgs(args)

	err := cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 0, exitCode, "os.Exit code is not 0")
}

func TestNotInstalled(t *testing.T) {
	defer resetFlags()

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	mockCF := createCheckInstalledMockCF(t)

	cmd := checkinstalledCmd(mockCF)
	addFlags(cmd)

	args := []string{"--name", "not_installed", "--version", "1.0"}
	cmd.SetArgs(args)

	err := cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 99, exitCode, "os.Exit code is not 99")
}

func createCheckInstalledMockCF(t *testing.T) *ChaincodeCmdFactory {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %s", err)
	}

	installedCqr := &pb.ChaincodeQueryResponse{
		Chaincodes: []*pb.ChaincodeInfo{
			{Name: "mycc1", Version: "1.0", Path: "codePath1", Input: "input", Escc: "escc", Vscc: "vscc", Id: []byte{1, 2, 3}},
			{Name: "mycc2", Version: "1.0", Path: "codePath2", Input: "input", Escc: "escc", Vscc: "vscc"},
		},
	}
	installedCqrBytes, err := proto.Marshal(installedCqr)
	if err != nil {
		t.Fatalf("Marshal error: %s", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200, Payload: installedCqrBytes},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	return mockCF
}
