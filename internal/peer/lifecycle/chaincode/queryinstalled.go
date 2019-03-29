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
	"github.com/spf13/viper"
)

// InstalledQuerier holds the dependencies needed to query
// the installed chaincodes
type InstalledQuerier struct {
	Command        *cobra.Command
	EndorserClient EndorserClient
	Signer         Signer
}

// QueryInstalledCmd returns the cobra command for listing
// the installed chaincodes
func QueryInstalledCmd(i *InstalledQuerier) *cobra.Command {
	chaincodeQueryInstalledCmd := &cobra.Command{
		Use:   "queryinstalled",
		Short: "Query the installed chaincodes on a peer.",
		Long:  "Query the installed chaincodes on a peer.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if i == nil {
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

				// queryinstalled only supports one peer connection,
				// which is why we only wire in the first endorser
				// client
				i = &InstalledQuerier{
					Command:        cmd,
					EndorserClient: cc.EndorserClients[0],
					Signer:         cc.Signer,
				}
			}
			return i.Query()
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

// Query returns the chaincodes installed on a peer
func (i *InstalledQuerier) Query() error {
	if i.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		i.Command.SilenceUsage = true
	}

	proposal, err := i.createProposal()
	if err != nil {
		return errors.WithMessage(err, "failed to create proposal")
	}

	signedProposal, err := signProposal(proposal, i.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal")
	}

	proposalResponse, err := i.EndorserClient.ProcessProposal(context.Background(), signedProposal)
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

	return i.printResponse(proposalResponse)
}

// printResponse prints the information included in the response
// from the server.
func (i *InstalledQuerier) printResponse(proposalResponse *pb.ProposalResponse) error {
	qicr := &lb.QueryInstalledChaincodesResult{}
	err := proto.Unmarshal(proposalResponse.Response.Payload, qicr)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}
	fmt.Println("Installed chaincodes on peer:")
	for _, chaincode := range qicr.InstalledChaincodes {
		fmt.Printf("Package ID: %s, Label: %s\n", chaincode.PackageId, chaincode.Label)
	}
	return nil

}

func (i *InstalledQuerier) createProposal() (*pb.Proposal, error) {
	args := &lb.QueryInstalledChaincodesArgs{}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal args")
	}

	ccInput := &pb.ChaincodeInput{
		Args: [][]byte{[]byte("QueryInstalledChaincodes"), argsBytes},
	}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	signerSerialized, err := i.Signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to serialize identity")
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, signerSerialized)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create ChaincodeInvocationSpec proposal")
	}

	return proposal, nil
}
