/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func checkinstantiatedCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "checkinstantiated",
		Short: "Check if a chaincode is instantiated on channel.",
		Long:  "Check if a chaincode is instantiated on channel.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			return checkinstantiated(cmd, args, cf)
		},
	}

	flagList := []string{
		"channelID",
		"name",
		"version",
		"peerAddresses",
		"connectionProfile",
	}
	attachFlags(checkCmd, flagList)

	return checkCmd
}

func checkinstantiated(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	if channelID == common.UndefinedParamValue {
		return fmt.Errorf("Must supply channel ID")
	}
	if chaincodeName == common.UndefinedParamValue {
		return fmt.Errorf("Must supply chaincode name")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, false, nil)
		if err != nil {
			return err
		}
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("error serializing identity: %s", err)
	}

	var prop *pb.Proposal
	prop, _, err = protoutil.CreateGetChaincodesProposal(channelID, creator)
	if err != nil {
		return fmt.Errorf("Error creating proposal %s: %s", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	// list is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return errors.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("Proposal response had nil 'response'")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("Bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, cqr)
	if err != nil {
		return err
	}

	for _, chaincode := range cqr.Chaincodes {
		if chaincodeName == chaincode.GetName() {
			if chaincodeVersion == common.UndefinedParamValue {
				// is not asking for a specific version, return success
				fmt.Printf("Chaincode %s is instantiated on channel %s \n", chaincodeName, channelID)
				osExit(0)
				return nil
			} else if chaincodeVersion == chaincode.GetVersion() {
				fmt.Printf("Chaincode %s version %s is instantiated on channel %s \n", chaincodeName, chaincodeVersion, channelID)
				osExit(0)
				return nil
			}
		}
	}

	// chaincode is not instantiated on channel, exit with code 1
	if chaincodeVersion == common.UndefinedParamValue {
		fmt.Printf("Chaincode %s is not instantiated on channel %s \n", chaincodeName, channelID)
	} else {
		fmt.Printf("Chaincode %s version %s is not instantiated on channel %s \n", chaincodeName, chaincodeVersion, channelID)
	}
	osExit(99)

	return nil
}
