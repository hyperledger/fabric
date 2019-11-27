/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/stretchr/testify/assert"
)

func TestJoinedChannelMissingChannelID(t *testing.T) {

	// Save current os.Exit function and restore at the end:
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	exitCode = -1
	mockExit := func(code int) {
		t.Logf("os.Exit is called with exit code %d", code)
		exitCode = code
	}
	osExit = mockExit

	InitMSP()

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		DeliverClient:    getMockDeliverClient(mockChannel),
		Signer:           signer,
	}

	cmd := checkjoinedCmd(mockCF)
	AddFlags(cmd)

	err = cmd.Execute()
	t.Logf("Error: %s", err)

	assert.Error(t, err, "Command should return error")
	assert.Contains(t, err.Error(), "must supply channel ID")
}

func TestJoinedChannel(t *testing.T) {
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

	InitMSP()

	mockChannelResponse := &pb.ChannelQueryResponse{
		Channels: []*pb.ChannelInfo{{
			ChannelId: "mockChannel",
		}},
	}

	mockPayload, err := proto.Marshal(mockChannelResponse)
	assert.NoError(t, err)

	mockResponse := &pb.ProposalResponse{
		Response: &pb.Response{
			Status:  200,
			Payload: mockPayload,
		},
		Endorsement: &pb.Endorsement{},
	}

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   common.GetMockEndorserClient(mockResponse, nil),
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := checkjoinedCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 0, exitCode, "os.Exit code is not 0")
}

func TestNotJoinedChannel(t *testing.T) {
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

	InitMSP()

	mockChannelResponse := &pb.ChannelQueryResponse{
		Channels: []*pb.ChannelInfo{},
	}

	mockPayload, err := proto.Marshal(mockChannelResponse)
	assert.NoError(t, err)

	mockResponse := &pb.ProposalResponse{
		Response: &pb.Response{
			Status:  200,
			Payload: mockPayload,
		},
		Endorsement: &pb.Endorsement{},
	}

	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   common.GetMockEndorserClient(mockResponse, nil),
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := checkjoinedCmd(mockCF)
	AddFlags(cmd)

	args := []string{"-c", "mockChannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()

	assert.NoError(t, err, "Command returned error")
	assert.NotEqual(t, -1, exitCode, "os.Exit is not called")
	assert.Equal(t, 99, exitCode, "os.Exit code is not 99")
}
