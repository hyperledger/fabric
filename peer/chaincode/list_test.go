/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/stretchr/testify/assert"
)

func TestChaincodeListCmd(t *testing.T) {
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

	cmd := listCmd(mockCF)

	t.Run("get installed chaincodes - legacy lscc", func(t *testing.T) {
		resetFlags()

		args := []string{"--installed"}
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("Run chaincode list cmd to get installed chaincodes error:%v", err)
		}
	})

	t.Run("get installed chaincodes - _lifecycle", func(t *testing.T) {
		resetFlags()
		queryInstalledChaincodeResult := &lb.QueryInstalledChaincodesResult{
			InstalledChaincodes: []*lb.QueryInstalledChaincodesResult_InstalledChaincode{
				{Name: "test1", Version: "v1.0", Hash: []byte("hash1")},
				{Name: "testcc2", Version: "v2.0", Hash: []byte("hash2")},
			},
		}
		qicrBytes, err := proto.Marshal(queryInstalledChaincodeResult)
		if err != nil {
			t.Fatalf("Marshal error: %s", err)
		}

		mockResponse.Response = &pb.Response{Status: 200, Payload: qicrBytes}

		args := []string{"--installed", "--newLifecycle"}
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("Run chaincode list cmd to get installed chaincodes error:%v", err)
		}
	})

	t.Run("get instantiated chaincodes - no channel", func(t *testing.T) {
		resetFlags()

		args := []string{"--instantiated"}
		cmd.SetArgs(args)
		err = cmd.Execute()
		assert.Error(t, err, "Run chaincode list cmd to get instantiated chaincodes should fail if invoked without -C flag")
	})

	t.Run("get instantiated chaincodes - no channel", func(t *testing.T) {
		resetFlags()

		args := []string{"--instantiated"}
		cmd.SetArgs(args)
		err = cmd.Execute()
		assert.Error(t, err, "Run chaincode list cmd to get instantiated chaincodes should fail if invoked without -C flag")
	})

	t.Run("get instantiated chaincodes - success", func(t *testing.T) {
		resetFlags()
		instantiatedChaincodesCmd := listCmd(mockCF)
		args := []string{"--instantiated", "-C", "mychannel"}
		instantiatedChaincodesCmd.SetArgs(args)
		if err := instantiatedChaincodesCmd.Execute(); err != nil {
			t.Errorf("Run chaincode list cmd to get instantiated chaincodes error:%v", err)
		}
	})

	t.Run("both --installed and --instantiated set - no channel", func(t *testing.T) {
		resetFlags()

		// Wrong case: Set both "--installed" and "--instantiated"
		cmd = listCmd(mockCF)
		args := []string{"--installed", "--instantiated"}
		cmd.SetArgs(args)
		err = cmd.Execute()
		assert.Error(t, err, "Run chaincode list cmd to get instantiated/installed chaincodes should fail if invoked without -C flag")
	})

	t.Run("both --installed and --instantiated set - no channel", func(t *testing.T) {
		resetFlags()
		args := []string{"--installed", "--instantiated", "-C", "mychannel"}
		cmd.SetArgs(args)
		expectErr := fmt.Errorf("must explicitly specify \"--installed\" or \"--instantiated\"")
		if err := cmd.Execute(); err == nil || err.Error() != expectErr.Error() {
			t.Errorf("Expect error: %s", expectErr)
		}
	})

	t.Run("neither --installed nor --instantiated set", func(t *testing.T) {
		resetFlags()
		args := []string{"-C", "mychannel"}
		cmd.SetArgs(args)

		expectErr := fmt.Errorf("must explicitly specify \"--installed\" or \"--instantiated\"")
		if err := cmd.Execute(); err == nil || err.Error() != expectErr.Error() {
			t.Errorf("Expect error: %s", expectErr)
		}
	})
}

func TestChaincodeListFailure(t *testing.T) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %s", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 500, Message: "error message"},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	// reset channelID, it might have been set by previous test
	channelID = ""

	resetFlags()

	// Get instantiated chaincodes
	instantiatedChaincodesCmd := listCmd(mockCF)
	args := []string{"--instantiated", "-C", "mychannel"}
	instantiatedChaincodesCmd.SetArgs(args)
	err = instantiatedChaincodesCmd.Execute()
	assert.Error(t, err)
	assert.Regexp(t, "bad response: 500 - error message", err.Error())
}

func TestString(t *testing.T) {
	id := []byte{1, 2, 3, 4, 5}
	idBytes := hex.EncodeToString(id)
	b, _ := hex.DecodeString(idBytes)
	ccInf := &ccInfo{
		ChaincodeInfo: &pb.ChaincodeInfo{
			Name:    "ccName",
			Id:      b,
			Version: "1.0",
			Escc:    "escc",
			Input:   "input",
			Vscc:    "vscc",
		},
	}
	assert.Equal(t, "Name: ccName, Version: 1.0, Input: input, Escc: escc, Vscc: vscc, Id: 0102030405", ccInf.String())
}
