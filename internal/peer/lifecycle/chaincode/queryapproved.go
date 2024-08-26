/*
Copyright Hitachi America, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	lb "github.com/hyperledger/fabric-protos-go-apiv2/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/proto"
)

// ApprovedQuerier holds the dependencies needed to query
// the approved chaincode definition for the organization
type ApprovedQuerier struct {
	Command        *cobra.Command
	EndorserClient EndorserClient
	Input          *ApprovedQueryInput
	Signer         Signer
	Writer         io.Writer
}

type ApprovedQueryInput struct {
	ChannelID    string
	Name         string
	Sequence     int64
	OutputFormat string
}

// QueryApprovedCmd returns the cobra command for
// querying the approved chaincode definition for the organization
func QueryApprovedCmd(a *ApprovedQuerier, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeQueryApprovedCmd := &cobra.Command{
		Use:   "queryapproved",
		Short: "Query org's approved chaincode definitions from its peer.",
		Long:  "Query organization's approved chaincode definitions from its peer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a == nil {
				ccInput := &ClientConnectionsInput{
					CommandName:           cmd.Name(),
					EndorserRequired:      true,
					ChannelID:             channelID,
					PeerAddresses:         peerAddresses,
					TLSRootCertFiles:      tlsRootCertFiles,
					ConnectionProfilePath: connectionProfilePath,
					TLSEnabled:            viper.GetBool("peer.tls.enabled"),
				}

				cc, err := NewClientConnections(ccInput, cryptoProvider)
				if err != nil {
					return err
				}

				aqInput := &ApprovedQueryInput{
					ChannelID:    channelID,
					Name:         chaincodeName,
					Sequence:     int64(sequence),
					OutputFormat: output,
				}

				a = &ApprovedQuerier{
					Command:        cmd,
					EndorserClient: cc.EndorserClients[0],
					Input:          aqInput,
					Signer:         cc.Signer,
					Writer:         os.Stdout,
				}
			}
			return a.Query()
		},
	}
	flagList := []string{
		"channelID",
		"name",
		"sequence",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"output",
	}
	attachFlags(chaincodeQueryApprovedCmd, flagList)

	return chaincodeQueryApprovedCmd
}

// Query returns the approved chaincode definition
// for a given channel and chaincode name
func (a *ApprovedQuerier) Query() error {
	if a.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		a.Command.SilenceUsage = true
	}

	err := a.validateInput()
	if err != nil {
		return err
	}

	proposal, err := a.createProposal()
	if err != nil {
		return errors.WithMessage(err, "failed to create proposal")
	}

	signedProposal, err := signProposal(proposal, a.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal")
	}

	proposalResponse, err := a.EndorserClient.ProcessProposal(context.Background(), signedProposal)
	if err != nil {
		return errors.WithMessage(err, "failed to endorse proposal")
	}

	if proposalResponse == nil {
		return errors.New("received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.New("received proposal response with nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("query failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	if strings.ToLower(a.Input.OutputFormat) == "json" {
		return a.printResponseAsJSON(proposalResponse)
	}
	return a.printResponse(proposalResponse)
}

func (a *ApprovedQuerier) printResponseAsJSON(proposalResponse *pb.ProposalResponse) error {
	if a.Input.Name != "" {
		return printResponseAsJSON(proposalResponse, &lb.QueryApprovedChaincodeDefinitionResult{}, a.Writer)
	}
	return printResponseAsJSON(proposalResponse, &lb.QueryApprovedChaincodeDefinitionsResult{}, a.Writer)
}

// printResponse prints the information included in the response
// from the server as human readable plain-text.
func (a *ApprovedQuerier) printResponse(proposalResponse *pb.ProposalResponse) error {
	if a.Input.Name != "" {
		result := &lb.QueryApprovedChaincodeDefinitionResult{}
		if err := proto.Unmarshal(proposalResponse.Response.Payload, result); err != nil {
			return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
		}
		fmt.Fprintf(a.Writer, "Approved chaincode definition for chaincode '%s' on channel '%s':\n", a.Input.Name, a.Input.ChannelID)
		a.printSingleApprovedChaincodeDefinition(result)
		fmt.Fprintf(a.Writer, "\n")
		return nil
	}

	result := &lb.QueryApprovedChaincodeDefinitionsResult{}
	if err := proto.Unmarshal(proposalResponse.Response.Payload, result); err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}
	fmt.Fprintf(a.Writer, "Approved chaincode definitions on channel '%s':\n", a.Input.ChannelID)

	for _, acd := range result.ApprovedChaincodeDefinitions {
		fmt.Fprintf(a.Writer, "name: %s, ", acd.Name)
		a.printSingleApprovedChaincodeDefinition(acd)
		fmt.Fprintf(a.Writer, "\n")
	}
	return nil
}

type ApprovedChaincodeDefinition interface {
	GetVersion() string
	GetSequence() int64
	GetEndorsementPlugin() string
	GetValidationPlugin() string
	GetInitRequired() bool
	GetSource() *lb.ChaincodeSource
}

func (a *ApprovedQuerier) printSingleApprovedChaincodeDefinition(acd ApprovedChaincodeDefinition) {
	var packageID string
	if acd.GetSource() != nil {
		switch source := acd.GetSource().Type.(type) {
		case *lb.ChaincodeSource_LocalPackage:
			packageID = source.LocalPackage.PackageId
		case *lb.ChaincodeSource_Unavailable_:
		}
	}
	fmt.Fprintf(a.Writer, "sequence: %d, version: %s, init-required: %t, package-id: %s, endorsement plugin: %s, validation plugin: %s",
		acd.GetSequence(), acd.GetVersion(), acd.GetInitRequired(), packageID, acd.GetEndorsementPlugin(), acd.GetValidationPlugin())
}

func (a *ApprovedQuerier) validateInput() error {
	if a.Input.ChannelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	if a.Input.Name == "" && a.Input.Sequence != 0 {
		return errors.New("Specifying parameter 'sequence' is invalid without parameter 'name'. Rerun the command with -n flag")
	}

	return nil
}

func (a *ApprovedQuerier) createProposal() (*pb.Proposal, error) {
	var function string
	var args proto.Message

	if a.Input.Name != "" {
		function = "QueryApprovedChaincodeDefinition"
		args = &lb.QueryApprovedChaincodeDefinitionArgs{
			Name:     a.Input.Name,
			Sequence: a.Input.Sequence,
		}
	} else {
		function = "QueryApprovedChaincodeDefinitions"
		args = &lb.QueryApprovedChaincodeDefinitionsArgs{}
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal args")
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(function), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	signerSerialized, err := a.Signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, a.Input.ChannelID, cis, signerSerialized)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}
