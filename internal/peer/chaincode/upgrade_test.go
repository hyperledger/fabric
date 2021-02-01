/*
Copyright Digital Asset Holdings, LLC. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"fmt"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestUpgradeCmd(t *testing.T) {
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

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cmd := upgradeCmd(mockCF, cryptoProvider)
	addFlags(cmd)

	args := []string{
		"-n", "mychaincode", "-p", "mychaincodepath",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}",
	}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "'peer chaincode upgrade' command should have failed without -C flag")

	args = []string{
		"-C", "mychannel", "-n", "mychaincode", "-p", "mychaincodepath",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}",
	}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.NoError(t, err, "'peer chaincode upgrade' command failed")
}

func TestUpgradeCmdEndorseFail(t *testing.T) {
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

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cmd := upgradeCmd(mockCF, cryptoProvider)
	addFlags(cmd)

	args := []string{
		"-C", "mychannel", "-n", "mychaincode", "-p", "mychaincodepath",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}",
	}
	cmd.SetArgs(args)

	expectErrMsg := fmt.Sprintf("could not assemble transaction, err proposal response was not successful, error code %d, msg %s", errCode, errMsg)
	if err := cmd.Execute(); err == nil {
		t.Errorf("Run chaincode upgrade cmd error:%v", err)
	} else {
		if err.Error() != expectErrMsg {
			t.Errorf("Run chaincode upgrade cmd get unexpected error: %s", err.Error())
		}
	}
}

func TestUpgradeCmdSendTXFail(t *testing.T) {
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

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cmd := upgradeCmd(mockCF, cryptoProvider)
	addFlags(cmd)

	args := []string{"-C", "mychannel", "-n", "mychaincode", "-p", "mychaincodepath", "-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}"}
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
		require.NoError(t, err, "'peer chaincode upgrade' command should have failed without a panic")
	}()

	channelID = ""

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cmd := upgradeCmd(nil, cryptoProvider)
	addFlags(cmd)

	args := []string{
		"-C", "mychannel", "-n", "mychaincode", "-p", "mychaincodepath",
		"-v", "anotherversion", "-c", "{\"Function\":\"init\",\"Args\": [\"param\",\"1\"]}",
	}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "'peer chaincode upgrade' command should have failed without a panic")
}
