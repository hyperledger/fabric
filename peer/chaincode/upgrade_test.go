/*
Copyright Digital Asset Holdings, LLC. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	// reset channelID, it might have been set by previous test
	channelID = ""

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "'peer chaincode upgrade' command should have failed without -C flag")

	args = []string{"-C", "mychannel", "-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.NoError(t, err, "'peer chaincode upgrade' command failed")
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

	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-C", "mychannel", "-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)

	expectErrMsg := fmt.Sprintf("could not assemble transaction, err Proposal response was not successful, error code %d, msg %s", errCode, errMsg)
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
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil)}
	sendErr := errors.New("send tx failed")
	mockBroadcastClient := common.GetMockBroadcastClient(sendErr)
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}

	cmd := upgradeCmd(mockCF)
	addFlags(cmd)

	args := []string{"-C", "mychannel", "-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd", "-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
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

func TestUpgradeCmdWithNilCF(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	// set timeout for failure cases
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)

	// trap possible SIGSEV panic
	defer func() {
		var err error
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
		assert.NoError(t, err, "'peer chaincode upgrade' command should have failed without a panic")
	}()

	channelID = ""
	InitMSP()

	cmd := upgradeCmd(nil)
	addFlags(cmd)

	args := []string{"-C", "mychannel", "-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
	cmd.SetArgs(args)
	err := cmd.Execute()
	assert.Error(t, err, "'peer chaincode upgrade' command should have failed without a panic")
}
