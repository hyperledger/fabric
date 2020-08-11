/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"context"
	"fmt"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// cancelRequestCmd returns the cobra command for snapshot cancelrequest command
func cancelRequestCmd(client *Client, cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotCancelRequestCmd := &cobra.Command{
		Use:   "cancelrequest",
		Short: "Cancel a request for a snapshot at the specified block.",
		Long:  "Cancel a request for a snapshot at the specified block.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cancelRequest(cmd, client, cryptoProvider)
		},
	}

	flagList := []string{
		"channelID",
		"blockNumber",
		"peerAddress",
		"tlsRootCertFile",
	}
	attachFlags(snapshotCancelRequestCmd, flagList)

	return snapshotCancelRequestCmd
}

func cancelRequest(cmd *cobra.Command, client *Client, cryptoProvider bccsp.BCCSP) error {
	if err := validateCancelRequest(); err != nil {
		return err
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	// create a client if not provided
	if client == nil {
		var err error
		client, err = NewClient(cryptoProvider)
		if err != nil {
			return err
		}
	}

	request := &pb.SnapshotRequest{
		ChannelId: channelID,
		Height:    blockNumber,
	}
	signedRequest, err := signSnapshotRequest(client.Signer, request)
	if err != nil {
		return err
	}

	_, err = client.SnapshotClient.Cancel(context.Background(), signedRequest)
	if err != nil {
		return errors.WithMessage(err, "failed to cancel the request")
	}

	fmt.Fprint(client.Writer, "Snapshot request cancelled successfully\n")
	return nil
}

func validateCancelRequest() error {
	if channelID == "" {
		return errors.New("the required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	if blockNumber == 0 {
		return errors.New("the required parameter 'blockNumber' is empty or set to 0. Rerun the command with -b flag")
	}
	return nil
}
