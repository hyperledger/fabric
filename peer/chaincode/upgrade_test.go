/*
 Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package chaincode

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var once sync.Once

// InitMSP init MSP
func InitMSP() {
	once.Do(initMSP)
}

func initMSP() {
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading MSP config: %s\n", err))
	}
}

func TestUpgradeCmd(t *testing.T) {
	InitMSP()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorerClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Errorf("Run chaincode upgrade cmd error:%v", err)
	}
}

func TestUpgradeCmdEndorseFail(t *testing.T) {
	InitMSP()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	errCode := int32(500)
	errMsg := "upgrade error"
	mockResponse := &pb.ProposalResponse{Response: &pb.Response{Status: errCode, Message: errMsg}}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorerClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)

	expectErrMsg := fmt.Sprintf("Could not assemble transaction, err Proposal response was not successful, error code %d, msg %s", errCode, errMsg)
	if err := cmd.Execute(); err == nil {
		t.Errorf("Run chaincode upgrade cmd error:%v", err)
	} else {
		if err.Error() != expectErrMsg {
			t.Errorf("Run chaincode upgrade cmd get unexpected error: %s", err.Error())
		}
	}
}

func TestUpgradeCmdSendTXFail(t *testing.T) {
	InitMSP()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	sendErr := errors.New("send tx failed")
	mockBroadcastClient := common.GetMockBroadcastClient(sendErr)

	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorerClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)

	expectErrMsg := sendErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Errorf("Run chaincode upgrade cmd error:%v", err)
	} else {
		if err.Error() != expectErrMsg {
			t.Errorf("Run chaincode upgrade cmd get unexpected error: %s", err.Error())
		}
	}
}
