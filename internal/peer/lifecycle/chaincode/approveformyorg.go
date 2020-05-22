/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/internal/peer/chaincode"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ApproverForMyOrg holds the dependencies needed to approve
// a chaincode definition for an organization
type ApproverForMyOrg struct {
	Certificate     tls.Certificate
	Command         *cobra.Command
	BroadcastClient common.BroadcastClient
	DeliverClients  []pb.DeliverClient
	EndorserClients []EndorserClient
	Input           *ApproveForMyOrgInput
	Signer          Signer
}

// ApproveForMyOrgInput holds all of the input parameters for approving a
// chaincode definition for an organization. ValidationParameter bytes is
// the (marshalled) endorsement policy when using the default endorsement
// and validation plugins
type ApproveForMyOrgInput struct {
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
	WaitForEvent             bool
	WaitForEventTimeout      time.Duration
	TxID                     string
}

// Validate the input for an ApproveChaincodeDefinitionForMyOrg proposal
func (a *ApproveForMyOrgInput) Validate() error {
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

// ApproveForMyOrgCmd returns the cobra command for chaincode ApproveForMyOrg
func ApproveForMyOrgCmd(a *ApproverForMyOrg, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeApproveForMyOrgCmd := &cobra.Command{
		Use:   "approveformyorg",
		Short: "Approve the chaincode definition for my org.",
		Long:  "Approve the chaincode definition for my organization.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a == nil {
				// set input from CLI flags
				input, err := a.createInput()
				if err != nil {
					return err
				}

				ccInput := &ClientConnectionsInput{
					CommandName:           cmd.Name(),
					EndorserRequired:      true,
					OrdererRequired:       true,
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

				endorserClients := make([]EndorserClient, len(cc.EndorserClients))
				for i, e := range cc.EndorserClients {
					endorserClients[i] = e
				}

				a = &ApproverForMyOrg{
					Command:         cmd,
					Input:           input,
					Certificate:     cc.Certificate,
					BroadcastClient: cc.BroadcastClient,
					DeliverClients:  cc.DeliverClients,
					EndorserClients: endorserClients,
					Signer:          cc.Signer,
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
		"endorsement-plugin",
		"validation-plugin",
		"signature-policy",
		"channel-config-policy",
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

// Approve submits a ApproveChaincodeDefinitionForMyOrg
// proposal
func (a *ApproverForMyOrg) Approve() error {
	err := a.Input.Validate()
	if err != nil {
		return err
	}

	if a.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		a.Command.SilenceUsage = true
	}

	proposal, txID, err := a.createProposal(a.Input.TxID)
	if err != nil {
		return errors.WithMessage(err, "failed to create proposal")
	}

	signedProposal, err := signProposal(proposal, a.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal")
	}

	var responses []*pb.ProposalResponse
	for _, endorser := range a.EndorserClients {
		proposalResponse, err := endorser.ProcessProposal(context.Background(), signedProposal)
		if err != nil {
			return errors.WithMessage(err, "failed to endorse proposal")
		}
		responses = append(responses, proposalResponse)
	}

	if len(responses) == 0 {
		// this should only be empty due to a programming bug
		return errors.New("no proposal responses received")
	}

	// all responses will be checked when the signed transaction is created.
	// for now, just set this so we check the first response's status
	proposalResponse := responses[0]

	if proposalResponse == nil {
		return errors.New("received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("received proposal response with nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("proposal failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}
	// assemble a signed transaction (it's an Envelope message)
	env, err := protoutil.CreateSignedTx(proposal, a.Signer, responses...)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed transaction")
	}
	var dg *chaincode.DeliverGroup
	var ctx context.Context
	if a.Input.WaitForEvent {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(context.Background(), a.Input.WaitForEventTimeout)
		defer cancelFunc()

		dg = chaincode.NewDeliverGroup(
			a.DeliverClients,
			a.Input.PeerAddresses,
			a.Signer,
			a.Certificate,
			a.Input.ChannelID,
			txID,
		)
		// connect to deliver service on all peers
		err := dg.Connect(ctx)
		if err != nil {
			return err
		}
	}

	if err = a.BroadcastClient.Send(env); err != nil {
		return errors.WithMessage(err, "failed to send transaction")
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

// createInput creates the input struct based on the CLI flags
func (a *ApproverForMyOrg) createInput() (*ApproveForMyOrgInput, error) {
	policyBytes, err := createPolicyBytes(signaturePolicy, channelConfigPolicy)
	if err != nil {
		return nil, err
	}

	ccp, err := createCollectionConfigPackage(collectionsConfigFile)
	if err != nil {
		return nil, err
	}

	input := &ApproveForMyOrgInput{
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
		WaitForEvent:             waitForEvent,
		WaitForEventTimeout:      waitForEventTimeout,
	}

	return input, nil
}

func (a *ApproverForMyOrg) createProposal(inputTxID string) (proposal *pb.Proposal, txID string, err error) {
	if a.Signer == nil {
		return nil, "", errors.New("nil signer provided")
	}

	var ccsrc *lb.ChaincodeSource
	if a.Input.PackageID != "" {
		ccsrc = &lb.ChaincodeSource{
			Type: &lb.ChaincodeSource_LocalPackage{
				LocalPackage: &lb.ChaincodeSource_Local{
					PackageId: a.Input.PackageID,
				},
			},
		}
	} else {
		ccsrc = &lb.ChaincodeSource{
			Type: &lb.ChaincodeSource_Unavailable_{
				Unavailable: &lb.ChaincodeSource_Unavailable{},
			},
		}
	}

	args := &lb.ApproveChaincodeDefinitionForMyOrgArgs{
		Name:                a.Input.Name,
		Version:             a.Input.Version,
		Sequence:            a.Input.Sequence,
		EndorsementPlugin:   a.Input.EndorsementPlugin,
		ValidationPlugin:    a.Input.ValidationPlugin,
		ValidationParameter: a.Input.ValidationParameterBytes,
		InitRequired:        a.Input.InitRequired,
		Collections:         a.Input.CollectionConfigPackage,
		Source:              ccsrc,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, "", err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte(approveFuncName), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := a.Signer.Serialize()
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, txID, err = protoutil.CreateChaincodeProposalWithTxIDAndTransient(cb.HeaderType_ENDORSER_TRANSACTION, a.Input.ChannelID, cis, creatorBytes, inputTxID, nil)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, txID, nil
}
