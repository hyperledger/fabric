/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/require"
)

func TestListChannels(t *testing.T) {
	InitMSP()

	mockChannelResponse := &pb.ChannelQueryResponse{
		Channels: []*pb.ChannelInfo{{
			ChannelId: "TEST_LIST_CHANNELS",
		}},
	}

	mockPayload, err := proto.Marshal(mockChannelResponse)
	require.NoError(t, err)

	mockResponse := &pb.ProposalResponse{
		Response: &pb.Response{
			Status:  200,
			Payload: mockPayload,
		},
		Endorsement: &pb.Endorsement{},
	}

	signer, err := common.GetDefaultSigner()
	require.NoError(t, err)

	mockCF := &ChannelCmdFactory{
		EndorserClient:   common.GetMockEndorserClient(mockResponse, nil),
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	cmd := listCmd(mockCF)
	AddFlags(cmd)
	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Error(err)
	}

	testListChannelsEmptyCF(t, mockCF)
}

func testListChannelsEmptyCF(t *testing.T, mockCF *ChannelCmdFactory) {
	cmd := listCmd(nil)
	AddFlags(cmd)

	// Error case 1: no orderer endpoints
	getEndorserClient := common.GetEndorserClientFnc
	getBroadcastClient := common.GetBroadcastClientFnc
	getDefaultSigner := common.GetDefaultSignerFnc
	defer func() {
		common.GetEndorserClientFnc = getEndorserClient
		common.GetBroadcastClientFnc = getBroadcastClient
		common.GetDefaultSignerFnc = getDefaultSigner
	}()
	common.GetDefaultSignerFnc = func() (msp.SigningIdentity, error) {
		return nil, errors.New("error")
	}
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	common.GetBroadcastClientFnc = func() (common.BroadcastClient, error) {
		broadcastClient := common.GetMockBroadcastClient(nil)
		return broadcastClient, nil
	}

	err := cmd.Execute()
	require.Error(t, err, "Error expected because GetDefaultSignerFnc returns an error")

	common.GetDefaultSignerFnc = getDefaultSigner
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err, "Error expected because GetEndorserClientFnc returns an error")

	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	err = cmd.Execute()
	require.NoError(t, err, "Error occurred while executing list command")
}
