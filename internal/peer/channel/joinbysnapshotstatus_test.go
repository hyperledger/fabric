/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestJoinBySnapshotStatus(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	signer, err := common.GetDefaultSigner()
	require.NoError(t, err)

	joinBySnapshotStatus := &pb.JoinBySnapshotStatus{InProgress: false, BootstrappingSnapshotDir: ""}
	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200, Payload: protoutil.MarshalOrPanic(joinBySnapshotStatus)},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockCF := &ChannelCmdFactory{
		EndorserClient:   mockEndorserClient,
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
	}

	// successful joinbysnapshotstatuscmd test
	resetFlags()
	cmd := joinBySnapshotStatusCmd(mockCF)
	AddFlags(cmd)
	require.NoError(t, cmd.Execute())

	// very joinbysnapshotstatus returns correct value
	client := &endorserClient{mockCF}
	status, err := client.joinBySnapshotStatus()
	require.NoError(t, err)
	require.True(t, proto.Equal(joinBySnapshotStatus, status))

	joinBySnapshotStatus = &pb.JoinBySnapshotStatus{InProgress: true, BootstrappingSnapshotDir: "mock_snapshot_directory"}
	mockResponse.Response.Payload = protoutil.MarshalOrPanic(joinBySnapshotStatus)
	status, err = client.joinBySnapshotStatus()
	require.NoError(t, err)
	require.True(t, proto.Equal(joinBySnapshotStatus, status))

	// negative test due to EndoserClient returning bad response
	mockResponse.Response = &pb.Response{Status: 500, Message: "mock_bad_response"}
	resetFlags()
	cmd = joinBySnapshotStatusCmd(mockCF)
	AddFlags(cmd)
	err = cmd.Execute()
	require.EqualError(t, err, "received bad response, status 500: mock_bad_response")

	// negative test due to connection failure to endorser client
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)
	resetFlags()
	cmd = joinBySnapshotStatusCmd(nil)
	AddFlags(cmd)
	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endorser client failed to connect to")
}
