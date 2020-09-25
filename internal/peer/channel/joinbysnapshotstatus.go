/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/cobra"
)

func joinBySnapshotStatusCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.
	return &cobra.Command{
		Use:   "joinbysnapshotstatus",
		Short: "Query if joinbysnapshot is running for any channel",
		Long:  "Query if joinbysnapshot is running for any channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return joinBySnapshotStatus(cf)
		},
	}
}

func joinBySnapshotStatus(cf *ChannelCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserRequired, PeerDeliverNotRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	client := &endorserClient{cf}
	status, err := client.joinBySnapshotStatus()
	if err != nil {
		return err
	}

	if status.InProgress {
		fmt.Printf("A joinbysnapshot operation is in progress for snapshot at %s\n", status.BootstrappingSnapshotDir)
	} else {
		fmt.Println("No joinbysnapshot operation is in progress")
	}

	return nil
}

func (cc *endorserClient) joinBySnapshotStatus() (*pb.JoinBySnapshotStatus, error) {
	var err error

	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.JoinBySnapshotStatus)}},
		},
	}

	var prop *pb.Proposal
	c, _ := cc.cf.Signer.Serialize()
	prop, _, err = protoutil.CreateProposalFromCIS(common2.HeaderType_ENDORSER_TRANSACTION, "", invocation, c)
	if err != nil {
		return nil, fmt.Errorf("cannot create proposal, due to %s", err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cc.cf.Signer)
	if err != nil {
		return nil, fmt.Errorf("cannot create signed proposal, due to %s", err)
	}

	proposalResp, err := cc.cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("failed sending proposal, due to %s", err)
	}

	if proposalResp.Response == nil || proposalResp.Response.Status != 200 {
		return nil, fmt.Errorf("received bad response, status %d: %s", proposalResp.Response.Status, proposalResp.Response.Message)
	}

	joinbysnapshotStatus := &pb.JoinBySnapshotStatus{}
	err = proto.Unmarshal(proposalResp.Response.Payload, joinbysnapshotStatus)
	if err != nil {
		return nil, fmt.Errorf("cannot query joinbysnapshot status, due to %s", err)
	}
	return joinbysnapshotStatus, nil
}
