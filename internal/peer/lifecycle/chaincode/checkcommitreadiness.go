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
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// CommitReadinessChecker holds the dependencies needed to
// check whether a chaincode definition is ready to be committed
// on a channel (i.e. has approvals from enough organizations to satisfy
// the lifecycle endorsement policy) and receive the list of orgs that
// have approved the definition.
type CommitReadinessChecker struct {
	Command        *cobra.Command
	EndorserClient pb.EndorserClient
	Input          *CommitReadinessCheckInput
	Signer         identity.SignerSerializer
	Writer         io.Writer
}

// CommitReadinessCheckInput holds all of the input parameters for checking
// whether a chaincode definition is ready to be committed ValidationParameter
// bytes is the (marshalled) endorsement policy when using the default
// endorsement and validation plugins.
type CommitReadinessCheckInput struct {
	ChannelID                string
	Name                     string
	Version                  string
	PackageID                string
	Sequence                 int64
	EndorsementPlugin        string
	ValidationPlugin         string
	ValidationParameterBytes []byte
	CollectionConfigPackage  *pb.CollectionConfigPackage
	InitRequired             bool
	PeerAddresses            []string
	TxID                     string
	OutputFormat             string
}

// Validate the input for a CheckCommitReadiness proposal
func (c *CommitReadinessCheckInput) Validate() error {
	if c.ChannelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}

	if c.Name == "" {
		return errors.New("The required parameter 'name' is empty. Rerun the command with -n flag")
	}

	if c.Version == "" {
		return errors.New("The required parameter 'version' is empty. Rerun the command with -v flag")
	}

	if c.Sequence == 0 {
		return errors.New("The required parameter 'sequence' is empty. Rerun the command with --sequence flag")
	}

	return nil
}

// CheckCommitReadinessCmd returns the cobra command for the
// CheckCommitReadiness lifecycle operation
func CheckCommitReadinessCmd(c *CommitReadinessChecker, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeCheckCommitReadinessCmd := &cobra.Command{
		Use:   "checkcommitreadiness",
		Short: "Check whether a chaincode definition is ready to be committed on a channel.",
		Long:  "Check whether a chaincode definition is ready to be committed on a channel.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if c == nil {
				// set input from CLI flags
				input, err := c.createInput()
				if err != nil {
					return err
				}

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

				c = &CommitReadinessChecker{
					Command:        cmd,
					Input:          input,
					EndorserClient: cc.EndorserClients[0],
					Signer:         cc.Signer,
					Writer:         os.Stdout,
				}
			}

			return c.ReadinessCheck()
		},
	}
	flagList := []string{
		"channelID",
		"name",
		"version",
		"sequence",
		"endorsement-plugin",
		"validation-plugin",
		"signature-policy",
		"channel-config-policy",
		"init-required",
		"collections-config",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"output",
	}
	attachFlags(chaincodeCheckCommitReadinessCmd, flagList)

	return chaincodeCheckCommitReadinessCmd
}

// ReadinessCheck submits a CheckCommitReadiness proposal
// and prints the result.
func (c *CommitReadinessChecker) ReadinessCheck() error {
	err := c.Input.Validate()
	if err != nil {
		return err
	}

	if c.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		c.Command.SilenceUsage = true
	}

	proposal, err := c.createProposal(c.Input.TxID)
	if err != nil {
		return errors.WithMessage(err, "failed to create proposal")
	}

	signedProposal, err := signProposal(proposal, c.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal")
	}

	// checkcommitreadiness currently only supports a single peer
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
		return printResponseAsJSON(proposalResponse, &lb.CheckCommitReadinessResult{}, c.Writer)
	}
	return c.printResponse(proposalResponse)
}

// printResponse prints the information included in the response
// from the server as human readable plain-text.
func (c *CommitReadinessChecker) printResponse(proposalResponse *pb.ProposalResponse) error {
	result := &lb.CheckCommitReadinessResult{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, result)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}

	orgs := []string{}
	for org := range result.Approvals {
		orgs = append(orgs, org)
	}
	sort.Strings(orgs)

	fmt.Fprintf(c.Writer, "Chaincode definition for chaincode '%s', version '%s', sequence '%d' on channel '%s' approval status by org:\n", c.Input.Name, c.Input.Version, c.Input.Sequence, c.Input.ChannelID)
	for _, org := range orgs {
		fmt.Fprintf(c.Writer, "%s: %t\n", org, result.Approvals[org])
	}

	return nil
}

// setInput creates the input struct based on the CLI flags
func (c *CommitReadinessChecker) createInput() (*CommitReadinessCheckInput, error) {
	policyBytes, err := createPolicyBytes(signaturePolicy, channelConfigPolicy)
	if err != nil {
		return nil, err
	}

	ccp, err := createCollectionConfigPackage(collectionsConfigFile)
	if err != nil {
		return nil, err
	}

	input := &CommitReadinessCheckInput{
		ChannelID:                channelID,
		Name:                     chaincodeName,
		Version:                  chaincodeVersion,
		PackageID:                packageID,
		Sequence:                 int64(sequence),
		EndorsementPlugin:        endorsementPlugin,
		ValidationPlugin:         validationPlugin,
		ValidationParameterBytes: policyBytes,
		InitRequired:             initRequired,
		CollectionConfigPackage:  ccp,
		PeerAddresses:            peerAddresses,
		OutputFormat:             output,
	}

	return input, nil
}

func (c *CommitReadinessChecker) createProposal(inputTxID string) (*pb.Proposal, error) {
	args := &lb.CheckCommitReadinessArgs{
		Name:                c.Input.Name,
		Version:             c.Input.Version,
		Sequence:            c.Input.Sequence,
		EndorsementPlugin:   c.Input.EndorsementPlugin,
		ValidationPlugin:    c.Input.ValidationPlugin,
		ValidationParameter: c.Input.ValidationParameterBytes,
		InitRequired:        c.Input.InitRequired,
		Collections:         c.Input.CollectionConfigPackage,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(checkCommitReadinessFuncName), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := c.Signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, _, err := protoutil.CreateChaincodeProposalWithTxIDAndTransient(cb.HeaderType_ENDORSER_TRANSACTION, c.Input.ChannelID, cis, creatorBytes, inputTxID, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}
