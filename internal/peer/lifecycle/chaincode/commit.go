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
	"github.com/hyperledger/fabric/internal/peer/chaincode"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/api"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodeCommitCmd *cobra.Command

// Committer holds the dependencies needed to commit
// a chaincode
type Committer struct {
	Certificate     tls.Certificate
	Command         *cobra.Command
	BroadcastClient common.BroadcastClient
	EndorserClients []pb.EndorserClient
	DeliverClients  []api.PeerDeliverClient
	Input           *CommitInput
	Signer          identity.SignerSerializer
}

// CommitInput holds all of the input parameters for committing a
// chaincode definition. ValidationParameter bytes is the (marshalled)
// endorsement policy when using the default endorsement and validation
// plugins
type CommitInput struct {
	ChannelID                string
	Name                     string
	Version                  string
	Hash                     []byte
	Sequence                 int64
	EndorsementPlugin        string
	ValidationPlugin         string
	ValidationParameterBytes []byte
	CollectionConfigPackage  *cb.CollectionConfigPackage
	InitRequired             bool
	PeerAddresses            []string
	WaitForEvent             bool
	WaitForEventTimeout      time.Duration
	TxID                     string
}

func (c *CommitInput) Validate() error {
	if c == nil {
		return errors.New("nil input")
	}

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

	if c.EndorsementPlugin == "" {
		c.EndorsementPlugin = "escc"
	}

	if c.ValidationPlugin == "" {
		c.ValidationPlugin = "vscc"
	}

	return nil
}

// commitCmd returns the cobra command for chaincode Commit
func commitCmd(cf *CmdFactory, c *Committer) *cobra.Command {
	chaincodeCommitCmd = &cobra.Command{
		Use:   "commit",
		Short: fmt.Sprintf("Commit the chaincode definition on the channel."),
		Long:  fmt.Sprintf("Commit the chaincode definition on the channel."),
		RunE: func(cmd *cobra.Command, args []string) error {
			if c == nil {
				var err error
				if cf == nil {
					cf, err = InitCmdFactory(cmd.Name(), true, true)
					if err != nil {
						return err
					}
					defer cf.BroadcastClient.Close()
				}
				c = &Committer{
					Command:         cmd,
					Certificate:     cf.Certificate,
					BroadcastClient: cf.BroadcastClient,
					DeliverClients:  cf.DeliverClients,
					EndorserClients: cf.EndorserClients,
					Signer:          cf.Signer,
				}
			}
			return c.Commit()
		},
	}
	flagList := []string{
		"channelID",
		"name",
		"version",
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
	attachFlags(chaincodeCommitCmd, flagList)

	return chaincodeCommitCmd
}

func (c *Committer) Commit() error {
	if c.Input == nil {
		// set input from CLI flags
		err := c.setInput()
		if err != nil {
			return err
		}
	}

	err := c.Input.Validate()
	if err != nil {
		return err
	}

	if c.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		c.Command.SilenceUsage = true
	}

	proposal, signedProposal, txID, err := c.createProposals(c.Input.TxID)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	var responses []*pb.ProposalResponse
	for _, endorser := range c.EndorserClients {
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
	env, err := protoutil.CreateSignedTx(proposal, c.Signer, responses...)
	if err != nil {
		return errors.WithMessage(err, "could not assemble transaction")
	}

	var dg *chaincode.DeliverGroup
	var ctx context.Context
	if c.Input.WaitForEvent {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(context.Background(), c.Input.WaitForEventTimeout)
		defer cancelFunc()

		dg = chaincode.NewDeliverGroup(
			c.DeliverClients,
			c.Input.PeerAddresses,
			c.Signer,
			c.Certificate,
			c.Input.ChannelID,
			txID,
		)
		// connect to deliver service on all peers
		err := dg.Connect(ctx)
		if err != nil {
			return err
		}
	}

	if err = c.BroadcastClient.Send(env); err != nil {
		return errors.WithMessage(err, "error sending transaction for commit")
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
func (c *Committer) setInput() error {
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

	c.Input = &CommitInput{
		ChannelID:                channelID,
		Name:                     chaincodeName,
		Version:                  chaincodeVersion,
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

func (c *Committer) createProposals(inputTxID string) (proposal *pb.Proposal, signedProposal *pb.SignedProposal, txID string, err error) {
	if c.Signer == nil {
		return nil, nil, "", errors.New("nil signer provided")
	}

	args := &lb.CommitChaincodeDefinitionArgs{
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
		return nil, nil, "", err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("CommitChaincodeDefinition"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	creatorBytes, err := c.Signer.Serialize()
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error serializing identity")
	}

	proposal, txID, err = protoutil.CreateChaincodeProposalWithTxIDAndTransient(cb.HeaderType_ENDORSER_TRANSACTION, c.Input.ChannelID, cis, creatorBytes, inputTxID, nil)
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	signedProposal, err = protoutil.GetSignedProposal(proposal, c.Signer)
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error signing proposal")
	}

	return proposal, signedProposal, txID, nil
}
