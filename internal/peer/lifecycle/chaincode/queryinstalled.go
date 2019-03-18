/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	chaincodeQueryInstalledCmd *cobra.Command
)

// queryInstalledCmd returns the cobra command for listing
// the installed chaincodes
func queryInstalledCmd(cf *CmdFactory) *cobra.Command {
	chaincodeQueryInstalledCmd = &cobra.Command{
		Use:   "queryinstalled",
		Short: "Query the installed chaincodes on a peer.",
		Long:  "Query the installed chaincodes on a peer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getChaincodes(cmd, cf)
		},
	}

	flagList := []string{
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(chaincodeQueryInstalledCmd, flagList)

	return chaincodeQueryInstalledCmd
}

func getChaincodes(cmd *cobra.Command, cf *CmdFactory) error {
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, false)
		if err != nil {
			return err
		}
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity: %s", err)
	}

	var prop *pb.Proposal

	prop, err = createQueryInstalledChaincodeProposal(creator)
	if err != nil {
		return errors.WithMessage(err, "error creating proposal")
	}

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	// list is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return errors.WithMessage(err, "error endorsing proposal")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("proposal response had nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	return printQueryInstalledResponse(proposalResponse)
}

// printResponse prints the information included in the response
// from the server.
func printQueryInstalledResponse(proposalResponse *pb.ProposalResponse) error {
	qicr := &lb.QueryInstalledChaincodesResult{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, qicr)
	if err != nil {
		return err
	}
	fmt.Println("Get installed chaincodes on peer:")
	for _, chaincode := range qicr.InstalledChaincodes {
		fmt.Printf("Package ID: %s, Label: %s\n", chaincode.PackageId, chaincode.Label)
	}
	return nil

}

func createQueryInstalledChaincodeProposal(creatorBytes []byte) (*pb.Proposal, error) {
	args := &lb.QueryInstalledChaincodesArgs{}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("QueryInstalledChaincodes"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
