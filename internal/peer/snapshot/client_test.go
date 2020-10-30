/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/snapshot_client.go -fake-name SnapshotClient . snapshotClient

type snapshotClient interface {
	pb.SnapshotClient
}

//go:generate counterfeiter -o mock/signer.go -fake-name Signer . signer

type signer interface {
	common.Signer
}

func TestValidatePeerConnectionParameters(t *testing.T) {
	viper.Set("peer.tls.enabled", false)
	require.NoError(t, validatePeerConnectionParameters())

	viper.Set("peer.tls.enabled", true)
	expectedErrMsg := "the required parameter 'tlsRootCertFile' is empty. Rerun the command with --tlsRootCertFile flag"
	require.EqualError(t, validatePeerConnectionParameters(), expectedErrMsg)

	tlsRootCertFile = "cert1_file"
	require.NoError(t, validatePeerConnectionParameters())

	// test error propagation
	args := []string{"-c", "mychannel"}
	resetFlags()
	cmd := listPendingCmd(nil, nil)
	cmd.SetArgs(args)
	err := cmd.Execute()
	require.EqualError(t, err, expectedErrMsg)

	resetFlags()
	cmd = submitRequestCmd(nil, nil)
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.EqualError(t, err, expectedErrMsg)

	resetFlags()
	cmd = cancelRequestCmd(nil, nil)
	// append required block number parameter for cancel request
	args = append(args, []string{"-b", "10"}...)
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.EqualError(t, err, expectedErrMsg)
}
