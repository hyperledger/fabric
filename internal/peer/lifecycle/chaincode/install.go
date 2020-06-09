/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Reader defines the interface needed for reading a file.
type Reader interface {
	ReadFile(string) ([]byte, error)
}

// Installer holds the dependencies needed to install
// a chaincode.
type Installer struct {
	Command        *cobra.Command
	EndorserClient EndorserClient
	Input          *InstallInput
	Reader         Reader
	Signer         Signer
}

// InstallInput holds the input parameters for installing
// a chaincode.
type InstallInput struct {
	PackageFile string
}

// Validate checks that the required install parameters
// are provided.
func (i *InstallInput) Validate() error {
	if i.PackageFile == "" {
		return errors.New("chaincode install package must be provided")
	}

	return nil
}

// InstallCmd returns the cobra command for chaincode install.
func InstallCmd(i *Installer, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeInstallCmd := &cobra.Command{
		Use:       "install",
		Short:     "Install a chaincode.",
		Long:      "Install a chaincode on a peer.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if i == nil {
				ccInput := &ClientConnectionsInput{
					CommandName:           cmd.Name(),
					EndorserRequired:      true,
					PeerAddresses:         peerAddresses,
					TLSRootCertFiles:      tlsRootCertFiles,
					ConnectionProfilePath: connectionProfilePath,
					TargetPeer:            targetPeer,
					TLSEnabled:            viper.GetBool("peer.tls.enabled"),
				}
				c, err := NewClientConnections(ccInput, cryptoProvider)
				if err != nil {
					return err
				}

				// install is currently only supported for one peer so just use
				// the first endorser client
				i = &Installer{
					Command:        cmd,
					EndorserClient: c.EndorserClients[0],
					Reader:         &persistence.FilesystemIO{},
					Signer:         c.Signer,
				}
			}
			return i.InstallChaincode(args)
		},
	}
	flagList := []string{
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"targetPeer",
	}
	attachFlags(chaincodeInstallCmd, flagList)

	return chaincodeInstallCmd
}

// InstallChaincode installs the chaincode.
func (i *Installer) InstallChaincode(args []string) error {
	if i.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		i.Command.SilenceUsage = true
	}

	i.setInput(args)

	return i.Install()
}

func (i *Installer) setInput(args []string) {
	i.Input = &InstallInput{}

	if len(args) > 0 {
		i.Input.PackageFile = args[0]
	}
}

// Install installs a chaincode for use with _lifecycle.
func (i *Installer) Install() error {
	err := i.Input.Validate()
	if err != nil {
		return err
	}

	pkgBytes, err := i.Reader.ReadFile(i.Input.PackageFile)
	if err != nil {
		return errors.WithMessagef(err, "failed to read chaincode package at '%s'", i.Input.PackageFile)
	}

	serializedSigner, err := i.Signer.Serialize()
	if err != nil {
		return errors.Wrap(err, "failed to serialize signer")
	}

	proposal, err := i.createInstallProposal(pkgBytes, serializedSigner)
	if err != nil {
		return err
	}

	signedProposal, err := signProposal(proposal, i.Signer)
	if err != nil {
		return errors.WithMessage(err, "failed to create signed proposal for chaincode install")
	}

	return i.submitInstallProposal(signedProposal)
}

func (i *Installer) submitInstallProposal(signedProposal *pb.SignedProposal) error {
	proposalResponse, err := i.EndorserClient.ProcessProposal(context.Background(), signedProposal)
	if err != nil {
		return errors.WithMessage(err, "failed to endorse chaincode install")
	}

	if proposalResponse == nil {
		return errors.New("chaincode install failed: received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.New("chaincode install failed: received proposal response with nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("chaincode install failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}
	logger.Infof("Installed remotely: %v", proposalResponse)

	icr := &lb.InstallChaincodeResult{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, icr)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal proposal response's response payload")
	}
	logger.Infof("Chaincode code package identifier: %s", icr.PackageId)

	return nil
}

func (i *Installer) createInstallProposal(pkgBytes []byte, creatorBytes []byte) (*pb.Proposal, error) {
	installChaincodeArgs := &lb.InstallChaincodeArgs{
		ChaincodeInstallPackage: pkgBytes,
	}

	installChaincodeArgsBytes, err := proto.Marshal(installChaincodeArgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal InstallChaincodeArgs")
	}

	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("InstallChaincode"), installChaincodeArgsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: lifecycleName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
