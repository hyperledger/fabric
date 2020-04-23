/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CommittedQuerier holds the dependencies needed to query
// the committed chaincode definitions
type CommittedQuerier struct {
	Command        *cobra.Command
	Input          *CommittedQueryInput
	EndorserClient EndorserClient
	Signer         Signer
	Writer         io.Writer
}

type CommittedQueryInput struct {
	ChannelID    string
	Name         string
	OutputFormat string
}

// QueryCommittedCmd returns the cobra command for
// querying a committed chaincode definition given
// the chaincode name
func QueryCommittedCmd(c *CommittedQuerier, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeQueryCommittedCmd := &cobra.Command{
		Use:   "querycommitted",
		Short: "Query the committed chaincode definitions by channel on a peer.",
		Long:  "Query the committed chaincode definitions by channel on a peer. Optional: provide a chaincode name to query a specific definition.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c == nil {
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

				cqInput := &CommittedQueryInput{
					ChannelID:    channelID,
					Name:         chaincodeName,
					OutputFormat: output,
				}

				c = &CommittedQuerier{
					Command:        cmd,
					EndorserClient: cc.EndorserClients[0],
					Input:          cqInput,
					Signer:         cc.Signer,
					Writer:         os.Stdout,
				}
			}
			return c.Query()
		},
	}

	flagList := []string{
		"channelID",
		"name",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"output",
	}
	attachFlags(chaincodeQueryCommittedCmd, flagList)

	return chaincodeQueryCommittedCmd
}

// Query returns the committed chaincode definition
// for a given channel and chaincode name
func (c *CommittedQuerier) Query() error {
	if c.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		c.Command.SilenceUsage = true
	}

	err := c.validateInput()
	if err != nil {
		return err
	}

	proposal, err := c.createProposal()
	if err != nil {
		return errors.WithMessage(err, "failed to create proposal")
	}

	signedProposal, err := signProposal(proposal, c.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal")
	}

	proposalResponse, err := c.EndorserClient.ProcessProposal(context.Background(), signedProposal)
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

	if strings.ToLower(c.Input.OutputFormat) == "json" {
		return c.printResponseAsJSON(proposalResponse)
	}
	return c.printResponse(proposalResponse)
}

func (c *CommittedQuerier) printResponseAsJSON(proposalResponse *pb.ProposalResponse) error {
	if c.Input.Name != "" {
		return printResponseAsJSON(proposalResponse, &lb.QueryChaincodeDefinitionResult{}, c.Writer)
	}
	return printResponseAsJSON(proposalResponse, &lb.QueryChaincodeDefinitionsResult{}, c.Writer)
}

// printResponse prints the information included in the response
// from the server as human readable plain-text.
func (c *CommittedQuerier) printResponse(proposalResponse *pb.ProposalResponse) error {
	if c.Input.Name != "" {
		result := &lb.QueryChaincodeDefinitionResult{}
		err := proto.Unmarshal(proposalResponse.Response.Payload, result)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
		}
		fmt.Fprintf(c.Writer, "Committed chaincode definition for chaincode '%s' on channel '%s':\n", c.Input.Name, c.Input.ChannelID)
		c.printSingleChaincodeDefinition(result)
		c.printApprovals(result)

		return nil
	}

	result := &lb.QueryChaincodeDefinitionsResult{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, result)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}
	fmt.Fprintf(c.Writer, "Committed chaincode definitions on channel '%s':\n", c.Input.ChannelID)
	for _, cd := range result.ChaincodeDefinitions {
		fmt.Fprintf(c.Writer, "Name: %s, ", cd.Name)
		c.printSingleChaincodeDefinition(cd)
		fmt.Fprintf(c.Writer, "\n")
	}
	return nil
}

type ChaincodeDefinition interface {
	GetVersion() string
	GetSequence() int64
	GetEndorsementPlugin() string
	GetValidationPlugin() string
}

func (c *CommittedQuerier) printSingleChaincodeDefinition(cd ChaincodeDefinition) {
	fmt.Fprintf(c.Writer, "Version: %s, Sequence: %d, Endorsement Plugin: %s, Validation Plugin: %s", cd.GetVersion(), cd.GetSequence(), cd.GetEndorsementPlugin(), cd.GetValidationPlugin())
}

func (c *CommittedQuerier) printApprovals(qcdr *lb.QueryChaincodeDefinitionResult) {
	orgs := []string{}
	approved := qcdr.GetApprovals()
	for org := range approved {
		orgs = append(orgs, org)
	}
	sort.Strings(orgs)

	approvals := ""
	for _, org := range orgs {
		approvals += fmt.Sprintf("%s: %t, ", org, approved[org])
	}
	approvals = strings.TrimSuffix(approvals, ", ")

	fmt.Fprintf(c.Writer, ", Approvals: [%s]\n", approvals)
}

func (c *CommittedQuerier) validateInput() error {
	if c.Input.ChannelID == "" {
		return errors.New("channel name must be specified")
	}

	return nil
}

func (c *CommittedQuerier) createProposal() (*pb.Proposal, error) {
	var function string
	var args proto.Message

	if c.Input.Name != "" {
		function = "QueryChaincodeDefinition"
		args = &lb.QueryChaincodeDefinitionArgs{
			Name: c.Input.Name,
		}
	} else {
		function = "QueryChaincodeDefinitions"
		args = &lb.QueryChaincodeDefinitionsArgs{}
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

	signerSerialized, err := c.Signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, c.Input.ChannelID, cis, signerSerialized)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}
