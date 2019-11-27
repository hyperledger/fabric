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

func checkinstalledCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	checkCmd := &cobra.Command{
		Use:   "checkinstalled",
		Short: "Check if a certain version of chaincode is installed to peer.",
		Long:  "Check if a certain version of chaincode is installed to peer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			return checkinstalled(cmd, args, cf)
		},
	}

	flagList := []string{
		"name",
		"version",
		"peerAddresses",
		"connectionProfile",
	}
	attachFlags(checkCmd, flagList)

	return checkCmd
}

func checkinstalled(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	if chaincodeName == common.UndefinedParamValue {
		return fmt.Errorf("Must supply chaincode name")
	}

	if chaincodeVersion == common.UndefinedParamValue {
		return fmt.Errorf("Must supply chaincode version")
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
	prop, _, err = protoutil.CreateGetInstalledChaincodesProposal(creator)
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
		if chaincodeName == chaincode.GetName() && chaincodeVersion == chaincode.GetVersion() {
			fmt.Printf("Chaincode %s version %s is installed to peer\n", chaincodeName, chaincodeVersion)
			osExit(0)
			return nil
		}
	}

	// chaincode is not installed to peer, exit with code 1
	fmt.Printf("Chaincode %s version %s is not installed to peer\n", chaincodeName, chaincodeVersion)
	osExit(99)

	return nil
}
