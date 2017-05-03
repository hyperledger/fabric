/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func TestChaincodeListCmd(t *testing.T) {
	InitMSP()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %s", err)
	}

	installedCqr := &pb.ChaincodeQueryResponse{
		Chaincodes: []*pb.ChaincodeInfo{
			{Name: "mycc1", Version: "1.0", Path: "codePath1", Input: "input", Escc: "escc", Vscc: "vscc"},
			{Name: "mycc2", Version: "1.0", Path: "codePath2", Input: "input", Escc: "escc", Vscc: "vscc"},
		},
	}
	installedCqrBytes, err := proto.Marshal(installedCqr)
	if err != nil {
		t.Fatalf("Marshale error: %s", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200, Payload: installedCqrBytes},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorerClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	// Get installed chaincodes
	installedChaincodesCmd := listCmd(mockCF)

	args := []string{"--installed"}
	installedChaincodesCmd.SetArgs(args)

	if err := installedChaincodesCmd.Execute(); err != nil {
		t.Errorf("Run chaincode list cmd to get installed chaincodes error:%v", err)
	}

	resetFlags()

	// Get instantiated chaincodes
	instantiatedChaincodesCmd := listCmd(mockCF)

	args = []string{"--instantiated"}
	instantiatedChaincodesCmd.SetArgs(args)

	if err := instantiatedChaincodesCmd.Execute(); err != nil {
		t.Errorf("Run chaincode list cmd to get instantiated chaincodes error:%v", err)
	}

	resetFlags()

	// Wrong case: Set both "--installed" and "--instantiated"
	Cmd := listCmd(mockCF)

	args = []string{"--installed", "--instantiated"}
	Cmd.SetArgs(args)

	expectErr := fmt.Errorf("Must explicitly specify \"--installed\" or \"--instantiated\"")
	if err := Cmd.Execute(); err == nil || err.Error() != expectErr.Error() {
		t.Errorf("Expect error: %s", expectErr)
	}

	resetFlags()

	// Wrong case: Miss "--intsalled" and "--instantiated"
	nilCmd := listCmd(mockCF)

	args = []string{}
	nilCmd.SetArgs(args)

	expectErr = fmt.Errorf("Must explicitly specify \"--installed\" or \"--instantiated\"")
	if err := nilCmd.Execute(); err == nil || err.Error() != expectErr.Error() {
		t.Errorf("Expect error: %s", expectErr)
	}
}
