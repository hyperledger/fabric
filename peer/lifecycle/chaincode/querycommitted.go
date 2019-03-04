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
	chaincodeQueryCommittedCmd *cobra.Command
)

// queryCommittedCmd returns the cobra command for
// querying a committed chaincode definition given
// the chaincode name
func queryCommittedCmd(cf *CmdFactory) *cobra.Command {
	chaincodeQueryCommittedCmd = &cobra.Command{
		Use:   "querycommitted",
		Short: "Query a committed chaincode definition by name on a peer.",
		Long:  "Query a committed chaincode definition by name on a peer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return queryCommitted(cmd, cf)
		},
	}

	flagList := []string{
		"channelID",
		"name",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(chaincodeQueryCommittedCmd, flagList)

	return chaincodeQueryCommittedCmd
}

func queryCommitted(cmd *cobra.Command, cf *CmdFactory) error {
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

	proposal, err := createQueryCommittedChaincodeDefinitionProposal(channelID, chaincodeName, creator)
	if err != nil {
		return errors.WithMessage(err, "error creating proposal")
	}

	signedProposal, err := protoutil.GetSignedProposal(proposal, cf.Signer)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	// QueryCommittedChaincodeDefinition is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProposal)
	if err != nil {
		return errors.WithMessage(err, "error endorsing proposal")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("proposal response had nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	return printQueryCommittedResponse(proposalResponse)
}

// printResponse prints the information included in the response
// from the server.
func printQueryCommittedResponse(proposalResponse *pb.ProposalResponse) error {
	qdcr := &lb.QueryChaincodeDefinitionResult{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, qdcr)
	if err != nil {
		return err
	}
	fmt.Printf("Committed chaincode definition for chaincode '%s' on channel '%s':\n", chaincodeName, channelID)
	fmt.Printf("Version: %s, Sequence: %d, Endorsement Plugin: %s, Validation Plugin: %s\n", qdcr.Version, qdcr.Sequence, qdcr.EndorsementPlugin, qdcr.ValidationPlugin)
	return nil

}

func createQueryCommittedChaincodeDefinitionProposal(channelID, chaincodeName string, creatorBytes []byte) (*pb.Proposal, error) {
	args := &lb.QueryChaincodeDefinitionArgs{
		Name: chaincodeName,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("QueryChaincodeDefinition"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, channelID, cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
