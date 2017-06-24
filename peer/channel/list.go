/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package channel

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/scc/cscc"
	common2 "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
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
	prop, _, err = utils.CreateProposalFromCIS(common2.HeaderType_ENDORSER_TRANSACTION, "", invocation, c)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot create proposal, due to %s", err))
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cc.cf.Signer)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot create signed proposal, due to %s", err))
	}

	proposalResp, err := cc.cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed sending proposal, got %s", err))
	}

	if proposalResp.Response == nil || proposalResp.Response.Status != 200 {
		return nil, errors.New(fmt.Sprint("Received bad response, status", proposalResp.Response.Status))
	}

	var channelQueryResponse pb.ChannelQueryResponse
	err = proto.Unmarshal(proposalResp.Response.Payload, &channelQueryResponse)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot read channels list response, %s", err))
	}

	return channelQueryResponse.Channels, nil
}

func list(cf *ChannelCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	client := &endorserClient{cf}

	if channels, err := client.getChannels(); err != nil {
		return err
	} else {
		logger.Infof("Channels peers has joined to: ")

		for _, channel := range channels {
			logger.Infof("%s ", channel.ChannelId)
		}
	}

	return nil
}
