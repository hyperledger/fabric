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

type endorserClient struct {
	cf *ChannelCmdFactory
}

func listCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.
	return &cobra.Command{
		Use:   "list",
		Short: "List of channels peer has joined.",
		Long:  "List of channels peer has joined.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return list(cf)
		},
	}
}

func (cc *endorserClient) getChannels() ([]*pb.ChannelInfo, error) {
	var err error

	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.GetChannels)}},
		},
	}

	var prop *pb.Proposal
	c, _ := cc.cf.Signer.Serialize()
	prop, _, err = protoutil.CreateProposalFromCIS(common2.HeaderType_ENDORSER_TRANSACTION, "", invocation, c)
	if err != nil {
		return nil, fmt.Errorf("Cannot create proposal, due to %s", err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cc.cf.Signer)
	if err != nil {
		return nil, fmt.Errorf("Cannot create signed proposal, due to %s", err)
	}

	proposalResp, err := cc.cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Failed sending proposal, got %s", err)
	}

	if proposalResp.Response == nil || proposalResp.Response.Status != 200 {
		return nil, fmt.Errorf("Received bad response, status %d: %s", proposalResp.Response.Status, proposalResp.Response.Message)
	}

	var channelQueryResponse pb.ChannelQueryResponse
	err = proto.Unmarshal(proposalResp.Response.Payload, &channelQueryResponse)
	if err != nil {
		return nil, fmt.Errorf("Cannot read channels list response, %s", err)
	}

	return channelQueryResponse.Channels, nil
}

func list(cf *ChannelCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserRequired, PeerDeliverNotRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	client := &endorserClient{cf}

	if channels, err := client.getChannels(); err != nil {
		return err
	} else {
		fmt.Println("Channels peers has joined: ")

		for _, channel := range channels {
			fmt.Printf("%s\n", channel.ChannelId)
		}
	}

	return nil
}
