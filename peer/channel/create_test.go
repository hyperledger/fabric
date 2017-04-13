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
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"

	"errors"

	"github.com/hyperledger/fabric/msp/mgmt/testtools"
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

	err := msptesttools.LoadMSPSetupForTesting(mspMgrConfigDir)
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading MSP config file %s: err %s\n", mspMgrConfigDir, err))
	}
}

func mockBroadcastClientFactory() (common.BroadcastClient, error) {
	return common.GetMockBroadcastClient(nil), nil
}

func TestCreateChain(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	defer os.Remove(mockchain + ".block")

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
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

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected join command to succeed")
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

	sendErr := errors.New("send create tx failed")

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: func() (common.BroadcastClient, error) {
			return common.GetMockBroadcastClient(sendErr), nil
		},
		Signer:        signer,
		DeliverClient: &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	expectedErrMsg := sendErr.Error()
	if err := cmd.Execute(); err == nil {
		t.Error("expected create chain to fail with broadcast error")
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

	recvErr := fmt.Errorf("deliver create tx failed")

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{recvErr},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain, "-o", "localhost:7050"}
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

func createTxFile(filename string, typ cb.HeaderType, channelID string) (*cb.Envelope, error) {
	ch := &cb.ChannelHeader{Type: int32(typ), ChannelId: channelID}
	data, err := proto.Marshal(ch)
	if err != nil {
		return nil, err
	}

	p := &cb.Payload{Header: &cb.Header{ChannelHeader: data}}
	data, err = proto.Marshal(p)
	if err != nil {
		return nil, err
	}

	env := &cb.Envelope{Payload: data}
	data, err = proto.Marshal(env)
	if err != nil {
		return nil, err
	}

	if err = ioutil.WriteFile(filename, data, 0644); err != nil {
		return nil, err
	}

	return env, nil
}

func TestCreateChainFromTx(t *testing.T) {
	InitMSP()

	mockchannel := "mockchannel"

	dir, err := ioutil.TempDir("/tmp", "createtestfromtx-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}

	defer os.RemoveAll(dir) // clean up

	//this could be created by the create command
	defer os.Remove(mockchannel + ".block")

	file := filepath.Join(dir, mockchannel)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, mockchannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err != nil {
		t.Errorf("create chain failed")
	}
}

func TestCreateChainInvalidTx(t *testing.T) {
	InitMSP()

	mockchannel := "mockchannel"

	dir, err := ioutil.TempDir("/tmp", "createinvaltest-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}

	defer os.RemoveAll(dir) // clean up

	//this is created by create command
	defer os.Remove(mockchannel + ".block")

	file := filepath.Join(dir, mockchannel)

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchannel, "-f", file, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	//bad type CONFIG
	if _, err = createTxFile(file, cb.HeaderType_CONFIG, mockchannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	defer os.Remove(file)

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	//bad channel name - does not match one specified in command
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, "different_channel"); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}

	//empty channel
	if _, err = createTxFile(file, cb.HeaderType_CONFIG_UPDATE, ""); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error")
	} else if _, ok := err.(InvalidCreateTx); !ok {
		t.Errorf("invalid error")
	}
}
