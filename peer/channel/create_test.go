/*
 Copyright Digital Asset Holdings, LLC 2017 All Rights Reserved.

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

package channel

import (
	"fmt"
	"os"
	"sync"
	"testing"

	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/common"
	cb "github.com/hyperledger/fabric/protos/common"
)

var once sync.Once

/// mock deliver client for UT
type mockDeliverClient struct {
	err error
}

func (m *mockDeliverClient) readBlock() (*cb.Block, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &cb.Block{}, nil
}

func (m *mockDeliverClient) getBlock() (*cb.Block, error) {
	b, err := m.readBlock()
	if err != nil {
		return nil, err
	}

	return b, nil
}

// InitMSP init MSP
func InitMSP() {
	once.Do(initMSP)
}

func initMSP() {
	// TODO: determine the location of this config file
	var alternativeCfgPath = os.Getenv("PEER_CFG_PATH")
	var mspMgrConfigDir string
	if alternativeCfgPath != "" {
		mspMgrConfigDir = alternativeCfgPath + "/msp/sampleconfig/"
	} else if _, err := os.Stat("./msp/sampleconfig/"); err == nil {
		mspMgrConfigDir = "./msp/sampleconfig/"
	} else {
		mspMgrConfigDir = os.Getenv("GOPATH") + "/src/github.com/hyperledger/fabric/msp/sampleconfig/"
	}

	err := mspmgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading MSP config file %s: err %s\n", mspMgrConfigDir, err))
	}
}

func TestCreateChain(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChannelCmdFactory{
		BroadcastClient:  mockBroadcastClient,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
		AnchorPeerParser: common.GetAnchorPeersParser("../common/testdata/anchorPeersOrg1.txt"),
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-a", "../common/testdata/anchorPeersOrg1.txt"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected join command to succeed")
	}
}

func TestCreateChainWithDefaultAnchorPeers(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChannelCmdFactory{
		BroadcastClient: mockBroadcastClient,
		Signer:          signer,
		DeliverClient:   &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected join command to succeed")
	}
}

func TestCreateChainInvalidAnchorPeers(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChannelCmdFactory{
		BroadcastClient:  mockBroadcastClient,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
		AnchorPeerParser: common.GetAnchorPeersParser("../common/testdata/anchorPeersBadPEM.txt"),
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-a", "../common/testdata/anchorPeersBadPEM.txt"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected create chain to fail because of invalid anchor peer file")
	}
}

func TestCreateChainBCFail(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	sendErr := fmt.Errorf("send create tx failed")
	mockBroadcastClient := common.GetMockBroadcastClient(sendErr)

	mockCF := &ChannelCmdFactory{
		BroadcastClient:  mockBroadcastClient,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
		AnchorPeerParser: common.GetAnchorPeersParser("../common/testdata/anchorPeersOrg1.txt"),
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-a", "../common/testdata/anchorPeersOrg1.txt"}
	cmd.SetArgs(args)

	expectedErrMsg := sendErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected create chain to fail with broadcast error")
	} else {
		if err.Error() != expectedErrMsg {
			t.Errorf("Run create chain get unexpected error: %s(expected %s)", err.Error(), expectedErrMsg)
		}
	}
}

func TestCreateChainDeliverFail(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	recvErr := fmt.Errorf("deliver create tx failed")

	mockCF := &ChannelCmdFactory{
		BroadcastClient:  mockBroadcastClient,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{recvErr},
		AnchorPeerParser: common.GetAnchorPeersParser("../common/testdata/anchorPeersOrg1.txt"),
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-a", "../common/testdata/anchorPeersOrg1.txt"}
	cmd.SetArgs(args)

	expectedErrMsg := recvErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected create chain to fail with deliver error")
	} else {
		if err.Error() != expectedErrMsg {
			t.Errorf("Run create chain get unexpected error: %s(expected %s)", err.Error(), expectedErrMsg)
		}
	}
}
