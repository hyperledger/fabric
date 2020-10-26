/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"errors"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
)

func joinBySnapshotCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.
	joinbysnapshotCmd := &cobra.Command{
		Use:   "joinbysnapshot",
		Short: "Joins the peer to a channel by the specified snapshot",
		Long:  "Joins the peer to a channel by the specified snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return joinBySnapshot(cmd, args, cf)
		},
	}
	flagList := []string{
		"snapshotpath",
	}
	attachFlags(joinbysnapshotCmd, flagList)

	return joinbysnapshotCmd
}

func joinBySnapshot(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	if snapshotPath == common.UndefinedParamValue {
		return errors.New("the required parameter 'snapshotpath' is empty. Rerun the command with --snapshotpath flag")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserRequired, PeerDeliverNotRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
		ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
		Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.JoinChainBySnapshot), []byte(snapshotPath)}},
	}

	if err = executeJoin(cf, spec); err != nil {
		return err
	}

	logger.Info(`The joinbysnapshot operation is in progress. Use "peer channel joinbysnapshotstatus" to check the status.`)
	return nil
}
