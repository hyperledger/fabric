/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/internal/peer/chaincode"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodeQueryApprovalStatusCmd *cobra.Command

// QueryApprovalStatus holds the dependencies needed to approve
// a chaincode definition for an organization
type QueryApprovalStatus struct {
	Certificate     tls.Certificate
	Command         *cobra.Command
	EndorserClients []pb.EndorserClient
	Input           *QueryApprovalStatusInput
	Signer          identity.SignerSerializer
}

type QueryApprovalStatusInput struct {
	ChannelID         string
	Name              string
	Version           string
	PackageID         string
	Sequence          int64
	EndorsementPlugin string
	ValidationPlugin  string
	// ValidationParameterBytes is the (marshalled) endorsement policy
	// when using default escc/vscc
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
func queryApprovalStatusCmd(cf *CmdFactory, a *QueryApprovalStatus) *cobra.Command {
	chaincodeQueryApprovalStatusCmd = &cobra.Command{
		Use:   "queryapprovalstatus",
		Short: fmt.Sprintf("Query approval status for chaincode definition."),
		Long:  fmt.Sprintf("Query approval status for chaincode definition."),
		RunE: func(cmd *cobra.Command, args []string) error {
			if a == nil {
				var err error
				if cf == nil {
					cf, err = InitCmdFactory(cmd.Name(), true, false)
					if err != nil {
						return err
					}
				}
				a = &QueryApprovalStatus{
					Command:         cmd,
					Certificate:     cf.Certificate,
					EndorserClients: cf.EndorserClients,
					Signer:          cf.Signer,
				}
			}
			return a.Approve()
		},
	}
	flagList := []string{
		"channelID",
		"name",
		"version",
		"package-id",
		"sequence",
		"escc",
		"vscc",
		"policy",
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
	proposalResponse, err := a.EndorserClients[0].ProcessProposal(context.Background(), signedProposal)
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
		return errors.Errorf("bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
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
	var (
		policyBytes []byte
		ccp         *cb.CollectionConfigPackage
	)

	if policy != "" {
		signaturePolicyEnvelope, err := cauthdsl.FromString(policy)
		if err != nil {
			return errors.Errorf("invalid signature policy: %s", policy)
		}

		applicationPolicy := &pb.ApplicationPolicy{
			Type: &pb.ApplicationPolicy_SignaturePolicy{
				SignaturePolicy: signaturePolicyEnvelope,
			},
		}
		policyBytes = protoutil.MarshalOrPanic(applicationPolicy)
	}

	if collectionsConfigFile != "" {
		var err error
		ccp, _, err = chaincode.GetCollectionConfigFromFile(collectionsConfigFile)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("invalid collection configuration in file %s", collectionsConfigFile))
		}
	}

	a.Input = &QueryApprovalStatusInput{
		ChannelID:                channelID,
		Name:                     chaincodeName,
		Version:                  chaincodeVersion,
		PackageID:                packageID,
		Sequence:                 int64(sequence),
		EndorsementPlugin:        escc,
		ValidationPlugin:         vscc,
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
