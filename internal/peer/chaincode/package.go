/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	pcommon "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	chaincodePackageCmd   *cobra.Command
	createSignedCCDepSpec bool
	signCCDepSpec         bool
	instantiationPolicy   string
)

const packageCmdName = "package"

type ccDepSpecFactory func(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error)

func defaultCDSFactory(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	return getChaincodeDeploymentSpec(spec, true)
}

// Packager holds the dependencies needed to package
// a chaincode and write it
type Packager struct {
	CDSFactory          ccDepSpecFactory
	ChaincodeCmdFactory *ChaincodeCmdFactory
	Command             *cobra.Command
	Input               *PackageInput
	CryptoProvider      bccsp.BCCSP
}

// PackageInput holds the input parameters for packaging a
// ChaincodeInstallPackage
type PackageInput struct {
	Name                  string
	Version               string
	InstantiationPolicy   string
	CreateSignedCCDepSpec bool
	SignCCDepSpec         bool
	OutputFile            string
	Path                  string
	Type                  string
}

// packageCmd returns the cobra command for packaging chaincode
func packageCmd(cf *ChaincodeCmdFactory, cdsFact ccDepSpecFactory, p *Packager, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodePackageCmd = &cobra.Command{
		Use:       "package [outputfile]",
		Short:     "Package a chaincode",
		Long:      "Package a chaincode and write the package to a file.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if p == nil {
				// UT will supply its own mock factory
				if cdsFact == nil {
					cdsFact = defaultCDSFactory
				}
				p = &Packager{
					CDSFactory:          cdsFact,
					ChaincodeCmdFactory: cf,
					CryptoProvider:      cryptoProvider,
				}
			}
			p.Command = cmd

			return p.packageChaincode(args)
		},
	}
	flagList := []string{
		"lang",
		"path",
		"ctor",
		"name",
		"version",
		"cc-package",
		"sign",
		"instantiate-policy",
	}
	attachFlags(chaincodePackageCmd, flagList)

	return chaincodePackageCmd
}

// packageChaincode packages the chaincode.
func (p *Packager) packageChaincode(args []string) error {
	if p.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		p.Command.SilenceUsage = true
	}

	if len(args) != 1 {
		return errors.New("output file not specified or invalid number of args (filename should be the only arg)")
	}
	p.setInput(args[0])

	// LSCC package
	return p.packageCC()
}

func (p *Packager) setInput(outputFile string) {
	p.Input = &PackageInput{
		Name:                  chaincodeName,
		Version:               chaincodeVersion,
		InstantiationPolicy:   instantiationPolicy,
		CreateSignedCCDepSpec: createSignedCCDepSpec,
		SignCCDepSpec:         signCCDepSpec,
		OutputFile:            outputFile,
		Path:                  chaincodePath,
		Type:                  chaincodeLang,
	}
}

// packageCC creates the LSCC chaincode packages
// (ChaincodeDeploymentSpec and SignedChaincodeDeploymentSpec)
func (p *Packager) packageCC() error {
	if p.CDSFactory == nil {
		return errors.New("chaincode deployment spec factory not specified")
	}

	var err error
	if p.ChaincodeCmdFactory == nil {
		p.ChaincodeCmdFactory, err = InitCmdFactory(p.Command.Name(), false, false, p.CryptoProvider)
		if err != nil {
			return err
		}
	}
	spec, err := getChaincodeSpec(p.Command)
	if err != nil {
		return err
	}

	cds, err := p.CDSFactory(spec)
	if err != nil {
		return errors.WithMessagef(err, "error getting chaincode code %s", p.Input.Name)
	}

	var bytesToWrite []byte
	if createSignedCCDepSpec {
		bytesToWrite, err = getChaincodeInstallPackage(cds, p.ChaincodeCmdFactory)
		if err != nil {
			return err
		}
	} else {
		bytesToWrite = protoutil.MarshalOrPanic(cds)
	}

	logger.Debugf("Packaged chaincode into deployment spec of size %d, output file %s", len(bytesToWrite), p.Input.OutputFile)
	err = ioutil.WriteFile(p.Input.OutputFile, bytesToWrite, 0o700)
	if err != nil {
		logger.Errorf("failed writing deployment spec to file [%s]: [%s]", p.Input.OutputFile, err)
		return err
	}

	return err
}

func getInstantiationPolicy(policy string) (*pcommon.SignaturePolicyEnvelope, error) {
	p, err := policydsl.FromString(policy)
	if err != nil {
		return nil, errors.WithMessagef(err, "invalid policy %s", policy)
	}
	return p, nil
}

// getChaincodeInstallPackage returns either a raw ChaincodeDeploymentSpec or
// a Envelope with ChaincodeDeploymentSpec and (optional) signature
func getChaincodeInstallPackage(cds *pb.ChaincodeDeploymentSpec, cf *ChaincodeCmdFactory) ([]byte, error) {
	var owner identity.SignerSerializer
	// check if we need to sign and set the owner
	if createSignedCCDepSpec && signCCDepSpec {
		if cf.Signer == nil {
			return nil, errors.New("signing identity not found")
		}
		owner = cf.Signer
	}

	ip := instantiationPolicy
	if ip == "" {
		// if an instantiation policy is not given, default
		// to "admin  must sign chaincode instantiation proposals"
		mspid, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetIdentifier()
		if err != nil {
			return nil, err
		}
		ip = "AND('" + mspid + ".admin')"
	}

	sp, err := getInstantiationPolicy(ip)
	if err != nil {
		return nil, err
	}

	// we get the Envelope of type CHAINCODE_PACKAGE
	objToWrite, err := ccpackage.OwnerCreateSignedCCDepSpec(cds, sp, owner)
	if err != nil {
		return nil, err
	}

	// convert the proto object to bytes
	bytesToWrite, err := proto.Marshal(objToWrite)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling chaincode package")
	}

	return bytesToWrite, nil
}
