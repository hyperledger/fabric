/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/chaincode"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/common/api"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodeApproveForMyOrgCmd *cobra.Command

// ApproverForMyOrg holds the dependencies needed to approve
// a chaincode definition for an organization
type ApproverForMyOrg struct {
	Certificate     tls.Certificate
	Command         *cobra.Command
	BroadcastClient common.BroadcastClient
	DeliverClients  []api.PeerDeliverClient
	EndorserClients []pb.EndorserClient
	Input           *ApproveForMyOrgInput
	Signer          msp.SigningIdentity
}

type ApproveForMyOrgInput struct {
	ChannelID         string
	Name              string
	Version           string
	Hash              []byte
	Sequence          int64
	EndorsementPlugin string
	ValidationPlugin  string
	// ValidationParameterBytes is the (marshalled) endorsement policy
	// when using default escc/vscc
	ValidationParameterBytes []byte
	CollectionConfigPackage  *cb.CollectionConfigPackage
	InitRequired             bool
	PeerAddresses            []string
	WaitForEvent             bool
	WaitForEventTimeout      time.Duration
	TxID                     string
}

func (a *ApproveForMyOrgInput) Validate() error {
	if a == nil {
		return errors.New("nil input")
	}

	if a.ChannelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}

	if a.Name == "" {
		return errors.New("The required parameter 'name' is empty. Rerun the command with -n flag")
	}

	if a.Version == "" {
		return errors.New("The required parameter 'version' is empty. Rerun the command with -v flag")
	}

	if a.Hash == nil {
		return errors.New("The required parameter 'hash' is empty. Rerun the command with --hash flag")
	}

	if a.Sequence == 0 {
		return errors.New("The required parameter 'sequence' is empty. Rerun the command with --sequence flag")
	}

	if a.EndorsementPlugin == "" {
		a.EndorsementPlugin = "escc"
	}

	if a.ValidationPlugin == "" {
		a.ValidationPlugin = "vscc"
	}

	return nil
}

// approveForMyOrgCmd returns the cobra command for chaincode ApproveForMyOrg
func approveForMyOrgCmd(cf *CmdFactory, a *ApproverForMyOrg) *cobra.Command {
	chaincodeApproveForMyOrgCmd = &cobra.Command{
		Use:   "approveformyorg",
		Short: fmt.Sprintf("Approve the chaincode definition for my org."),
		Long:  fmt.Sprintf("Approve the chaincode definition for my organization."),
		RunE: func(cmd *cobra.Command, args []string) error {
			if a == nil {
				var err error
				if cf == nil {
					cf, err = InitCmdFactory(cmd.Name(), true, true)
					if err != nil {
						return err
					}
					defer cf.BroadcastClient.Close()
				}
				a = &ApproverForMyOrg{
					Command:         cmd,
					Certificate:     cf.Certificate,
					BroadcastClient: cf.BroadcastClient,
					DeliverClients:  cf.DeliverClients,
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
		"hash",
		"sequence",
		"escc",
		"vscc",
		"policy",
		"init-required",
		"collections-config",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"waitForEvent",
		"waitForEventTimeout",
	}
	attachFlags(chaincodeApproveForMyOrgCmd, flagList)

	return chaincodeApproveForMyOrgCmd
}

func (a *ApproverForMyOrg) Approve() error {
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

	proposal, signedProposal, txID, err := a.createProposals(a.Input.TxID)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	var responses []*pb.ProposalResponse
	for _, endorser := range a.EndorserClients {
		proposalResponse, err := endorser.ProcessProposal(context.Background(), signedProposal)
		if err != nil {
			return errors.WithMessage(err, "error endorsing proposal")
		}
		responses = append(responses, proposalResponse)
	}

	if len(responses) == 0 {
		// this should only happen if some new code has introduced a bug
		return errors.New("no proposal responses received - this might indicate a bug")
	}

	// all responses will be checked when the signed transaction is created.
	// for now, just set this so we check the first response's status
	proposalResponse := responses[0]

	if proposalResponse == nil {
		return errors.New("received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("proposal response had nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}
	// assemble a signed transaction (it's an Envelope message)
	env, err := protoutil.CreateSignedTx(proposal, a.Signer, responses...)
	if err != nil {
		return errors.WithMessage(err, "could not assemble transaction")
	}
	var dg *chaincode.DeliverGroup
	var ctx context.Context
	if a.Input.WaitForEvent {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(context.Background(), a.Input.WaitForEventTimeout)
		defer cancelFunc()

		dg = chaincode.NewDeliverGroup(a.DeliverClients, a.Input.PeerAddresses, a.Certificate, a.Input.ChannelID, txID)
		// connect to deliver service on all peers
		err := dg.Connect(ctx)
		if err != nil {
			return err
		}
	}

	if err = a.BroadcastClient.Send(env); err != nil {
		return errors.WithMessage(err, "error sending transaction for approveformyorg")
	}

	if dg != nil && ctx != nil {
		// wait for event that contains the txID from all peers
		err = dg.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return err
}

// setInput sets the input struct based on the CLI flags
func (a *ApproverForMyOrg) setInput() error {
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

	a.Input = &ApproveForMyOrgInput{
		ChannelID:                channelID,
		Name:                     chaincodeName,
		Version:                  chaincodeVersion,
		Hash:                     hash,
		Sequence:                 int64(sequence),
		EndorsementPlugin:        escc,
		ValidationPlugin:         vscc,
		ValidationParameterBytes: policyBytes,
		InitRequired:             initRequired,
		CollectionConfigPackage:  ccp,
		PeerAddresses:            peerAddresses,
		WaitForEvent:             waitForEvent,
		WaitForEventTimeout:      waitForEventTimeout,
	}

	return nil
}

func (a *ApproverForMyOrg) createProposals(inputTxID string) (proposal *pb.Proposal, signedProposal *pb.SignedProposal, txID string, err error) {
	if a.Signer == nil {
		return nil, nil, "", errors.New("nil signer provided")
	}

	args := &lb.ApproveChaincodeDefinitionForMyOrgArgs{
		Name:                a.Input.Name,
		Version:             a.Input.Version,
		Hash:                a.Input.Hash,
		Sequence:            a.Input.Sequence,
		EndorsementPlugin:   a.Input.EndorsementPlugin,
		ValidationPlugin:    a.Input.ValidationPlugin,
		ValidationParameter: a.Input.ValidationParameterBytes,
		InitRequired:        a.Input.InitRequired,
		Collections:         a.Input.CollectionConfigPackage,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, nil, "", err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("ApproveChaincodeDefinitionForMyOrg"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := a.Signer.Serialize()
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, fmt.Sprintf("error serializing identity for %s", a.Signer.GetIdentifier()))
	}

	proposal, txID, err = protoutil.CreateChaincodeProposalWithTxIDAndTransient(cb.HeaderType_ENDORSER_TRANSACTION, a.Input.ChannelID, cis, creatorBytes, inputTxID, nil)
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	signedProposal, err = protoutil.GetSignedProposal(proposal, a.Signer)
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error signing proposal")
	}

	return proposal, signedProposal, txID, nil
}
