/*
Copyright Digital Asset Holdings, LLC 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"testing"
	"time"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestJoinBySnapshot(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	InitMSP()
	signer, err := common.GetDefaultSigner()
	require.NoError(t, err)

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

	// successful test
	resetFlags()
	cmd := joinBySnapshotCmd(mockCF)
	AddFlags(cmd)
	cmd.SetArgs([]string{"--snapshotpath", "path_to_snapshot_directory"})
	require.NoError(t, cmd.Execute())

	// error due to missing snapshotpath
	resetFlags()
	cmd = joinBySnapshotCmd(mockCF)
	AddFlags(cmd)
	cmd.SetArgs([]string{})
	require.EqualError(t, cmd.Execute(), "the required parameter 'snapshotpath' is empty. Rerun the command with --snapshotpath flag")

	// error due to EndoserClient returning bad response
	mockResponse.Response = &pb.Response{Status: 500}
	resetFlags()
	cmd = joinBySnapshotCmd(mockCF)
	AddFlags(cmd)
	args := []string{"--snapshotpath", "snapshot_path"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.EqualError(t, err, "proposal failed (err: bad proposal response 500: )")
	require.IsType(t, ProposalFailedErr(err.Error()), err, "expected error type of ProposalFailedErr")

	// error due to connection failure to endorser client
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)
	resetFlags()
	cmd = joinBySnapshotCmd(nil)
	AddFlags(cmd)
	cmd.SetArgs([]string{"--snapshotpath", "snapshot_path"})
	err = cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endorser client failed to connect to")
}
