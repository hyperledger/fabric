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
func listPendingCmd(cl *client, cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotGenerateRequestCmd := &cobra.Command{
		Use:   "listpending",
		Short: "List pending requests for snapshots.",
		Long:  "List pending requests for snapshots.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listPending(cmd, cl, cryptoProvider)
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

func listPending(cmd *cobra.Command, cl *client, cryptoProvider bccsp.BCCSP) error {
	if err := validateListPending(); err != nil {
		return err
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	// create a client if not provided
	if cl == nil {
		var err error
		cl, err = newClient(cryptoProvider)
		if err != nil {
			return err
		}
	}

	signatureHdr, err := createSignatureHeader(cl.signer)
	if err != nil {
		return err
	}

	query := &pb.SnapshotQuery{
		SignatureHeader: signatureHdr,
		ChannelId:       channelID,
	}
	signedRequest, err := signSnapshotRequest(cl.signer, query)
	if err != nil {
		return err
	}

	resp, err := cl.snapshotClient.QueryPendings(context.Background(), signedRequest)
	if err != nil {
		return errors.WithMessage(err, "failed to list pending requests")
	}

	fmt.Fprintf(cl.writer, "Successfully got pending snapshot requests: %v\n", resp.BlockNumbers)
	return nil
}

func validateListPending() error {
	if channelID == "" {
		return errors.New("the required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	return nil
}
