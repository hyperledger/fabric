/*
Copyright Digital Asset Holdings, LLC 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestMissingBlockFile(t *testing.T) {
	defer resetFlags()

	resetFlags()

	cmd := joinCmd(nil)
	AddFlags(cmd)
	args := []string{}
	cmd.SetArgs(args)

	require.Error(t, cmd.Execute(), "expected join command to fail due to missing blockfilepath")
}

func TestJoin(t *testing.T) {
	defer resetFlags()

	InitMSP()
	resetFlags()

	dir := t.TempDir()
	mockblockfile := filepath.Join(dir, "mockjointest.block")
	err := ioutil.WriteFile(mockblockfile, []byte(""), 0o644)
	require.NoError(t, err, "Could not write to the file %s", mockblockfile)
	signer, err := common.GetDefaultSigner()
	require.NoError(t, err, "Get default signer error: %v", err)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorserClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)

	require.NoError(t, cmd.Execute(), "expected join command to succeed")
}

func TestJoinNonExistentBlock(t *testing.T) {
	defer resetFlags()

	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorserClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-b", "mockchain.block"}
	cmd.SetArgs(args)

	err = cmd.Execute()
	require.Error(t, err, "expected join command to fail")
	require.IsType(t, GBFileNotFoundErr(err.Error()), err, "expected error type of GBFileNotFoundErr")
}

func TestBadProposalResponse(t *testing.T) {
	defer resetFlags()

	InitMSP()
	resetFlags()

	mockblockfile := "/tmp/mockjointest.block"
	ioutil.WriteFile(mockblockfile, []byte(""), 0o644)
	defer os.Remove(mockblockfile)
	signer, err := common.GetDefaultSigner()
	require.NoError(t, err, "Get default signer error: %v", err)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 500},
		Endorsement: &pb.Endorsement{},
	}

	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorserClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := joinCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)

	err = cmd.Execute()
	require.Error(t, err, "expected join command to fail")
	require.IsType(t, ProposalFailedErr(err.Error()), err, "expected error type of ProposalFailedErr")
}

func TestJoinNilCF(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	resetFlags()

	dir := t.TempDir()
	mockblockfile := filepath.Join(dir, "mockjointest.block")
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)
	cmd := joinCmd(nil)
	AddFlags(cmd)
	args := []string{"-b", mockblockfile}
	cmd.SetArgs(args)

	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endorser client failed to connect to")
}
