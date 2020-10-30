/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/internal/peer/snapshot/mock"
	"github.com/onsi/gomega/gbytes"
	"github.com/stretchr/testify/require"
)

func TestCancelRequestCmd(t *testing.T) {
	mockSigner := &mock.Signer{}
	mockSigner.SignReturns([]byte("snapshot-request-signature"), nil)
	mockSnapshotClient := &mock.SnapshotClient{}
	mockSnapshotClient.CancelReturns(&empty.Empty{}, nil)
	buffer := gbytes.NewBuffer()
	mockClient := &client{mockSnapshotClient, mockSigner, buffer}

	resetFlags()
	cmd := cancelRequestCmd(mockClient, nil)
	cmd.SetArgs([]string{"-c", "mychannel", "-b", "100"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Equal(t, []byte("Snapshot request cancelled successfully\n"), buffer.Contents())

	// error tests
	mockSnapshotClient.CancelReturns(nil, fmt.Errorf("fake-cancel-error"))
	require.EqualError(t, cmd.Execute(), "failed to cancel the request: fake-cancel-error")

	mockSigner.SignReturns(nil, fmt.Errorf("fake-sign-error"))
	require.EqualError(t, cmd.Execute(), "fake-sign-error")

	mockSigner.SerializeReturns(nil, fmt.Errorf("fake-serialize-error"))
	require.EqualError(t, cmd.Execute(), "fake-serialize-error")

	resetFlags()
	cmd.SetArgs([]string{"-b", "100"})
	require.EqualError(t, cmd.Execute(), "the required parameter 'channelID' is empty. Rerun the command with -c flag")

	resetFlags()
	cmd.SetArgs([]string{"-c", "mychannel"})
	require.EqualError(t, cmd.Execute(), "the required parameter 'blockNumber' is empty or set to 0. Rerun the command with -b flag")
}
