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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestJoin(t *testing.T) {
	InitMSP()

	dir, err := ioutil.TempDir("/tmp", "jointest")
	assert.NoError(t, err, "Could not create the directory %s", dir)
	mockblockfile := filepath.Join(dir, "mockjointest.block")
	err = ioutil.WriteFile(mockblockfile, []byte(""), 0644)
	assert.NoError(t, err, "Could not write to the file %s", mockblockfile)
	defer os.RemoveAll(dir)
	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorerClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Error("expected join command to succeed")
	}
}

func TestJoinNonExistentBlock(t *testing.T) {
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

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorerClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-b", "mockchain.block"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fail()
		t.Error("expected join command to fail")
	} else if err, _ = err.(GBFileNotFoundErr); err == nil {
		t.Fail()
		t.Error("expected file not found error")
	}
}

func TestBadProposalResponse(t *testing.T) {
	InitMSP()

	mockblockfile := "/tmp/mockjointest.block"
	ioutil.WriteFile(mockblockfile, []byte(""), 0644)
	defer os.Remove(mockblockfile)
	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 500},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorerClient := common.GetMockEndorserClient(mockResponse, nil)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorerClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fail()
		t.Error("expected join command to fail")
	} else if err, _ = err.(ProposalFailedErr); err == nil {
		t.Fail()
		t.Error("expected proposal failure error")
	}
}
func TestJoinNilCF(t *testing.T) {
	InitMSP()

	dir, err := ioutil.TempDir("/tmp", "jointest")
	assert.NoError(t, err, "Could not create the directory %s", dir)
	mockblockfile := filepath.Join(dir, "mockjointest.block")
	defer os.RemoveAll(dir)
	cmd := joinCmd(nil)
	AddFlags(cmd)
	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error trying to connect to local peer")
}
