/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodeInstallCmd *cobra.Command

// Reader defines the interface needed for reading a file
type Reader interface {
	ReadFile(string) ([]byte, error)
}

// Installer holds the dependencies needed to install
// a chaincode
type Installer struct {
	Command         *cobra.Command
	EndorserClients []pb.EndorserClient
	Input           *InstallInput
	Reader          Reader
	Signer          identity.SignerSerializer
}

// InstallInput holds the input parameters for installing
// a chaincode
type InstallInput struct {
	PackageFile string
}

// installCmd returns the cobra command for chaincode install
func installCmd(cf *CmdFactory, i *Installer) *cobra.Command {
	chaincodeInstallCmd = &cobra.Command{
		Use:       "install",
		Short:     "Install a chaincode.",
		Long:      "Install a chaincode on a peer.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if i == nil {
				var err error
				if cf == nil {
					cf, err = InitCmdFactory(cmd.Name(), true, false)
					if err != nil {
						return err
					}
				}
				i = &Installer{
					Command:         cmd,
					EndorserClients: cf.EndorserClients,
					Reader:          &persistence.FilesystemIO{},
					Signer:          cf.Signer,
				}
			}
			return i.installChaincode(args)
		},
	}
	flagList := []string{
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(chaincodeInstallCmd, flagList)

	return chaincodeInstallCmd
}

// installChaincode installs the chaincode
func (i *Installer) installChaincode(args []string) error {
	if i.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		i.Command.SilenceUsage = true
	}

	i.setInput(args)

	return i.install()
}

func (i *Installer) setInput(args []string) {
	i.Input = &InstallInput{}

	if len(args) > 0 {
		i.Input.PackageFile = args[0]
	}
}

// install installs a chaincode for use with _lifecycle
func (i *Installer) install() error {
	err := i.validateInput()
	if err != nil {
		return err
	}

	pkgBytes, err := i.Reader.ReadFile(i.Input.PackageFile)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("error reading chaincode package at %s", i.Input.PackageFile))
	}

	serializedSigner, err := i.Signer.Serialize()
	if err != nil {
		return err
	}

	proposal, err := i.createInstallProposal(pkgBytes, serializedSigner)
	if err != nil {
		return err
	}

	signedProposal, err := protoutil.GetSignedProposal(proposal, i.Signer)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal for chaincode install")
	}

	return i.submitInstallProposal(signedProposal)
}

func (i *Installer) submitInstallProposal(signedProposal *pb.SignedProposal) error {
	// install is currently only supported for one peer
	proposalResponse, err := i.EndorserClients[0].ProcessProposal(context.Background(), signedProposal)
	if err != nil {
		return errors.WithMessage(err, "error endorsing chaincode install")
	}

	if proposalResponse == nil {
		return errors.New("error during install: received nil proposal response")
	}

	if proposalResponse.Response == nil {
		return errors.New("error during install: received proposal response with nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("install failed with status: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}
	logger.Infof("Installed remotely: %v", proposalResponse)

	icr := &lb.InstallChaincodeResult{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, icr)
	if err != nil {
		return errors.Wrap(err, "error unmarshaling proposal response's response payload")
	}
	logger.Infof("Chaincode code package identifier: %s", icr.PackageId)

	return nil
}

func (i *Installer) validateInput() error {
	if i.Input.PackageFile == "" {
		return errors.New("chaincode install package must be provided")
	}

	return nil
}

func (i *Installer) createInstallProposal(pkgBytes []byte, creatorBytes []byte) (*pb.Proposal, error) {
	installChaincodeArgs := &lb.InstallChaincodeArgs{
		ChaincodeInstallPackage: pkgBytes,
	}

	installChaincodeArgsBytes, err := proto.Marshal(installChaincodeArgs)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling InstallChaincodeArgs")
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
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
