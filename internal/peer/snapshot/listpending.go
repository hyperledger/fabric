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

// listPendingCmd returns the cobra command for snapshot listpending command
func listPendingCmd(client *Client, cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotGenerateRequestCmd := &cobra.Command{
		Use:   "listpending",
		Short: "List pending requests for snapshots.",
		Long:  "List pending requests for snapshots.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listPending(cmd, client, cryptoProvider)
		},
	}
	flagList := []string{
		"channelID",
		"peerAddress",
		"tlsRootCertFile",
	}
	attachFlags(snapshotGenerateRequestCmd, flagList)

	return snapshotGenerateRequestCmd
}

func listPending(cmd *cobra.Command, client *Client, cryptoProvider bccsp.BCCSP) error {
	if err := validateListPending(); err != nil {
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

	query := &pb.SnapshotQuery{
		ChannelId: channelID,
	}
	signedRequest, err := signSnapshotRequest(client.Signer, query)
	if err != nil {
		return err
	}

	resp, err := client.SnapshotClient.QueryPendings(context.Background(), signedRequest)
	if err != nil {
		return errors.WithMessage(err, "failed to list pending requests")
	}

	fmt.Fprintf(client.Writer, "Successfully got pending snapshot requests: %v\n", resp.Heights)
	return nil
}

func validateListPending() error {
	if channelID == "" {
		return errors.New("the required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	return nil
}
