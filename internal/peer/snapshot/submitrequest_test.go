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

func TestSubmitRequestCmd(t *testing.T) {
	mockSigner := &mock.Signer{}
	mockSigner.SignReturns([]byte("snapshot-request-signature"), nil)
	mockSnapshotClient := &mock.SnapshotClient{}
	mockSnapshotClient.GenerateReturns(&empty.Empty{}, nil)
	buffer := gbytes.NewBuffer()
	mockClient := &client{mockSnapshotClient, mockSigner, buffer}

	resetFlags()
	cmd := submitRequestCmd(mockClient, nil)
	cmd.SetArgs([]string{"-c", "mychannel"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, []byte("Snapshot request submitted successfully\n"), buffer.Contents())

	// use a new buffer to verify new command execution
	buffer2 := gbytes.NewBuffer()
	mockClient.writer = buffer2
	resetFlags()
	cmd = submitRequestCmd(mockClient, nil)
	cmd.SetArgs([]string{"-c", "mychannel", "-b", "100"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, []byte("Snapshot request submitted successfully\n"), buffer2.Contents())

	// error tests
	mockSnapshotClient.GenerateReturns(nil, fmt.Errorf("fake-generate-error"))
	require.EqualError(t, cmd.Execute(), "failed to submit the request: fake-generate-error")

	mockSigner.SignReturns(nil, fmt.Errorf("fake-sign-error"))
	require.EqualError(t, cmd.Execute(), "fake-sign-error")

	mockSigner.SerializeReturns(nil, fmt.Errorf("fake-serialize-error"))
	require.EqualError(t, cmd.Execute(), "fake-serialize-error")

	resetFlags()
	cmd.SetArgs([]string{})
	require.EqualError(t, cmd.Execute(), "the required parameter 'channelID' is empty. Rerun the command with -c flag")
}
