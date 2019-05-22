/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var chaincodeQueryApprovalStatusCmd *cobra.Command

// QueryApprovalStatus holds the dependencies needed to approve
// a chaincode definition for an organization
type QueryApprovalStatus struct {
	Command        *cobra.Command
	EndorserClient pb.EndorserClient
	Input          *QueryApprovalStatusInput
	Signer         identity.SignerSerializer
}

// QueryApprovalStatusInput holds all of the input parameters for committing
// a chaincode definition. ValidationParameter bytes is the (marshalled)
// endorsement policy when using the default endorsement and validation
// plugins
type QueryApprovalStatusInput struct {
	ChannelID                string
	Name                     string
	Version                  string
	PackageID                string
	Sequence                 int64
	EndorsementPlugin        string
	ValidationPlugin         string
	ValidationParameterBytes []byte
	CollectionConfigPackage  *cb.CollectionConfigPackage
	InitRequired             bool
	PeerAddresses            []string
	TxID                     string
}

func (a *QueryApprovalStatusInput) Validate() error {
	if a.ChannelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}

	if a.Name == "" {
		return errors.New("The required parameter 'name' is empty. Rerun the command with -n flag")
	}

	if a.Version == "" {
		return errors.New("The required parameter 'version' is empty. Rerun the command with -v flag")
	}

	if a.Sequence == 0 {
		return errors.New("The required parameter 'sequence' is empty. Rerun the command with --sequence flag")
	}

	return nil
}

// queryApprovalStatusCmd returns the cobra command for the QueryApprovalStatus lifecycle operation
func queryApprovalStatusCmd(a *QueryApprovalStatus) *cobra.Command {
	chaincodeQueryApprovalStatusCmd = &cobra.Command{
		Use:   "queryapprovalstatus",
		Short: fmt.Sprintf("Query approval status for chaincode definition."),
		Long:  fmt.Sprintf("Query approval status for chaincode definition."),
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

				cc, err := NewClientConnections(ccInput)
				if err != nil {
					return err
				}

				a = &QueryApprovalStatus{
					Command:        cmd,
					EndorserClient: cc.EndorserClients[0],
					Signer:         cc.Signer,
				}
			}

			return a.Approve()
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
	}
	attachFlags(chaincodeQueryApprovalStatusCmd, flagList)

	return chaincodeQueryApprovalStatusCmd
}

func (a *QueryApprovalStatus) Approve() error {
	if a.Input == nil {
		// set input from CLI flags
		err := a.setInput()
		if err != nil {
			return err
		}
	}

	err := a.Input.Validate()
	if err != nil {
		return err
	}

	if a.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		a.Command.SilenceUsage = true
	}

	signedProposal, err := a.createProposal(a.Input.TxID)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	// queryapprovalstatus currently only supports a single peer
	proposalResponse, err := a.EndorserClient.ProcessProposal(context.Background(), signedProposal)
	if err != nil {
		return errors.WithMessage(err, "error endorsing proposal")
	}

	if proposalResponse == nil {
		return errors.New("received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("proposal response had nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("query failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	return printQueryApprovalStatusResponse(proposalResponse, os.Stdout)
}

type ApprovalStatusOutput struct {
	Approved map[string]bool `json:"Approved"`
}

// printQueryApprovalStatusResponse prints the information included in the response
// from the peer.
func printQueryApprovalStatusResponse(proposalResponse *pb.ProposalResponse, out io.Writer) error {
	qasr := &lb.QueryApprovalStatusResults{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, qasr)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal QueryApprovalStatusResults")
	}

	bytes, err := json.MarshalIndent(&ApprovalStatusOutput{
		Approved: qasr.Approved,
	}, "", "\t")
	if err != nil {
		return errors.Wrap(err, "failed to marshal QueryApprovalStatus outputs")
	}

	fmt.Fprintf(out, "%s\n", string(bytes))

	return nil
}

// setInput sets the input struct based on the CLI flags
func (a *QueryApprovalStatus) setInput() error {
	policyBytes, err := createPolicyBytes(signaturePolicy, channelConfigPolicy)
	if err != nil {
		return err
	}

	ccp, err := createCollectionConfigPackage(collectionsConfigFile)
	if err != nil {
		return err
	}

	a.Input = &QueryApprovalStatusInput{
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
	}

	return nil
}

func (a *QueryApprovalStatus) createProposal(inputTxID string) (*pb.SignedProposal, error) {
	if a.Signer == nil {
		return nil, errors.New("nil signer provided")
	}

	args := &lb.QueryApprovalStatusArgs{
		Name:                a.Input.Name,
		Version:             a.Input.Version,
		Sequence:            a.Input.Sequence,
		EndorsementPlugin:   a.Input.EndorsementPlugin,
		ValidationPlugin:    a.Input.ValidationPlugin,
		ValidationParameter: a.Input.ValidationParameterBytes,
		InitRequired:        a.Input.InitRequired,
		Collections:         a.Input.CollectionConfigPackage,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("QueryApprovalStatus"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := a.Signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "error serializing identity")
	}

	proposal, _, err := protoutil.CreateChaincodeProposalWithTxIDAndTransient(cb.HeaderType_ENDORSER_TRANSACTION, a.Input.ChannelID, cis, creatorBytes, inputTxID, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	signedProposal, err := protoutil.GetSignedProposal(proposal, a.Signer)
	if err != nil {
		return nil, errors.WithMessage(err, "error signing proposal")
	}

	return signedProposal, nil
}
