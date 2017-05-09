/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
)

var getInstalledChaincodes bool
var getInstantiatedChaincodes bool
var chaincodeListCmd *cobra.Command

const list_cmdname = "list"

// installCmd returns the cobra command for Chaincode Deploy
func listCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeListCmd = &cobra.Command{
		Use:   "list",
		Short: "Get the instantiated chaincodes on a channel or installed chaincodes on a peer.",
		Long:  "Get the instantiated chaincodes in the channel if specify channel, or get installed chaincodes on the peer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getChaincodes(cmd, cf)
		},
	}

	flagList := []string{
		"channelID",
		"installed",
		"instantiated",
	}
	attachFlags(chaincodeListCmd, flagList)

	return chaincodeListCmd
}

func getChaincodes(cmd *cobra.Command, cf *ChaincodeCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(true, false)
		if err != nil {
			return err
		}
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s", cf.Signer.GetIdentifier(), err)
	}

	var prop *pb.Proposal
	if getInstalledChaincodes && (!getInstantiatedChaincodes) {
		prop, _, err = utils.CreateGetInstalledChaincodesProposal(creator)
	} else if getInstantiatedChaincodes && (!getInstalledChaincodes) {
		prop, _, err = utils.CreateGetChaincodesProposal(chainID, creator)
	} else {
		return fmt.Errorf("Must explicitly specify \"--installed\" or \"--instantiated\"")
	}

	if err != nil {
		return fmt.Errorf("Error creating proposal %s: %s", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	proposalResponse, err := cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return fmt.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, cqr)
	if err != nil {
		return err
	}

	if getInstalledChaincodes {
		fmt.Println("Get installed chaincodes on peer:")
	} else {
		fmt.Printf("Get instantiated chaincodes on channel %s:\n", chainID)
	}
	for _, chaincode := range cqr.Chaincodes {
		fmt.Printf("%v\n", chaincode)
	}
	return nil
}
