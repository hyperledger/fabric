/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"fmt"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/snapshot/mock"
	"github.com/onsi/gomega/gbytes"
	"github.com/stretchr/testify/require"
)

func TestListPendingCmd(t *testing.T) {
	mockSigner := &mock.Signer{}
	mockSigner.SignReturns([]byte("snapshot-request-signature"), nil)
	mockSnapshotClient := &mock.SnapshotClient{}
	mockSnapshotClient.QueryPendingsReturns(&pb.QueryPendingSnapshotsResponse{BlockNumbers: []uint64{100, 200}}, nil)
	buffer := gbytes.NewBuffer()
	mockClient := &client{mockSnapshotClient, mockSigner, buffer}

	resetFlags()
	cmd := listPendingCmd(mockClient, nil)
	cmd.SetArgs([]string{"-c", "mychannel"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Equal(t, []byte("Successfully got pending snapshot requests: [100 200]\n"), buffer.Contents())

	// error tests
	mockSnapshotClient.QueryPendingsReturns(nil, fmt.Errorf("fake-querypendings-error"))
	require.EqualError(t, cmd.Execute(), "failed to list pending requests: fake-querypendings-error")

	mockSigner.SignReturns(nil, fmt.Errorf("fake-sign-error"))
	require.EqualError(t, cmd.Execute(), "fake-sign-error")

	mockSigner.SerializeReturns(nil, fmt.Errorf("fake-serialize-error"))
	require.EqualError(t, cmd.Execute(), "fake-serialize-error")

	resetFlags()
	cmd.SetArgs([]string{})
	require.EqualError(t, cmd.Execute(), "the required parameter 'channelID' is empty. Rerun the command with -c flag")
}
