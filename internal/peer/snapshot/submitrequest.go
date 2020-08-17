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

// submitRequestCmd returns the cobra command for snapshot submitrequest command
func submitRequestCmd(client *Client, cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotSubmitRequestCmd := &cobra.Command{
		Use:   "submitrequest",
		Short: "Submit a request for a snapshot at the specified block.",
		Long:  "Submit a request for a snapshot at the specified block. When the blockNumber parameter is set to 0 or not provided, it will submit a request for the last committed block.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return submitRequest(cmd, client, cryptoProvider)
		},
	}

	flagList := []string{
		"channelID",
		"blockNumber",
		"peerAddress",
		"tlsRootCertFile",
	}
	attachFlags(snapshotSubmitRequestCmd, flagList)

	return snapshotSubmitRequestCmd
}

func submitRequest(cmd *cobra.Command, client *Client, cryptoProvider bccsp.BCCSP) error {
	if err := validateSubmitRequest(); err != nil {
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

	_, err = client.SnapshotClient.Generate(context.Background(), signedRequest)
	if err != nil {
		return errors.WithMessage(err, "failed to submit the request")
	}

	fmt.Fprint(client.Writer, "Snapshot request submitted successfully\n")
	return nil
}

func validateSubmitRequest() error {
	if channelID == "" {
		return errors.New("the required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	return nil
}
