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
func submitRequestCmd(cl *client, cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotSubmitRequestCmd := &cobra.Command{
		Use:   "submitrequest",
		Short: "Submit a request for a snapshot at the specified block.",
		Long:  "Submit a request for a snapshot at the specified block. When the blockNumber parameter is set to 0 or not provided, it will submit a request for the last committed block.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return submitRequest(cmd, cl, cryptoProvider)
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

func submitRequest(cmd *cobra.Command, cl *client, cryptoProvider bccsp.BCCSP) error {
	if err := validateSubmitRequest(); err != nil {
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

	request := &pb.SnapshotRequest{
		SignatureHeader: signatureHdr,
		ChannelId:       channelID,
		BlockNumber:     blockNumber,
	}
	signedRequest, err := signSnapshotRequest(cl.signer, request)
	if err != nil {
		return err
	}

	_, err = cl.snapshotClient.Generate(context.Background(), signedRequest)
	if err != nil {
		return errors.WithMessage(err, "failed to submit the request")
	}

	fmt.Fprint(cl.writer, "Snapshot request submitted successfully\n")
	return nil
}

func validateSubmitRequest() error {
	if channelID == "" {
		return errors.New("the required parameter 'channelID' is empty. Rerun the command with -c flag")
	}
	return nil
}
