/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
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

// PlatformRegistry defines the interface to get the code bytes
// for a chaincode given the type and path
type PlatformRegistry interface {
	GetDeploymentPayload(ccType, path string) ([]byte, error)
}

// Writer defines the interface needed for writing a file
type Writer interface {
	WriteFile(string, []byte, os.FileMode) error
}

// Packager holds the dependencies needed to package
// a chaincode and write it
type Packager struct {
	CDSFactory          ccDepSpecFactory
	ChaincodeCmdFactory *ChaincodeCmdFactory
	Command             *cobra.Command
	Input               *PackageInput
	PlatformRegistry    PlatformRegistry
	Writer              Writer
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
	NewLifecycle          bool
	Path                  string
	Type                  string
}

// packageCmd returns the cobra command for packaging chaincode
func packageCmd(cf *ChaincodeCmdFactory, cdsFact ccDepSpecFactory, p *Packager) *cobra.Command {
	chaincodePackageCmd = &cobra.Command{
		Use:       "package [outputfile]",
		Short:     "Package a chaincode",
		Long:      "Package a chaincode and write the package to a file.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if p == nil {
				pr := platforms.NewRegistry(platforms.SupportedPlatforms...)

				// UT will supply its own mock factory
				if cdsFact == nil {
					cdsFact = defaultCDSFactory
				}
				p = &Packager{
					CDSFactory:          cdsFact,
					ChaincodeCmdFactory: cf,
					PlatformRegistry:    pr,
					Writer:              &persistence.FilesystemIO{},
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
		"newLifecycle",
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

	// _lifecycle package
	if p.Input.NewLifecycle {
		return p.packageCC()
	}

	// legacy LSCC package
	return p.packageCCLegacy()
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
		NewLifecycle:          newLifecycle,
	}
}

// packageCC packages chaincodes into the package type,
// (.tar.gz) used by the new _lifecycle and writes it to disk
func (p *Packager) packageCC() error {
	err := p.validateInput()
	if err != nil {
		return err
	}

	pkgTarGzBytes, err := p.getTarGzBytes()
	if err != nil {
		return err
	}

	err = p.Writer.WriteFile(p.Input.OutputFile, pkgTarGzBytes, 0600)
	if err != nil {
		err = errors.Wrapf(err, "error writing chaincode package to %s", p.Input.OutputFile)
		logger.Error(err.Error())
		return err
	}

	return nil
}

// packageCCLegacy creates the legacy chaincode packages
// (ChaincodeDeploymentSpec and SignedChaincodeDeploymentSpec)
func (p *Packager) packageCCLegacy() error {
	if p.CDSFactory == nil {
		return errors.New("chaincode deployment spec factory not specified")
	}

	var err error
	if p.ChaincodeCmdFactory == nil {
		p.ChaincodeCmdFactory, err = InitCmdFactory(p.Command.Name(), false, false)
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
		return errors.WithMessage(err, fmt.Sprintf("error getting chaincode code %s", p.Input.Name))
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

	logger.Debugf("Packaged chaincode into deployment spec of size <%d>, output file ", len(bytesToWrite), p.Input.OutputFile)
	err = ioutil.WriteFile(p.Input.OutputFile, bytesToWrite, 0700)
	if err != nil {
		logger.Errorf("failed writing deployment spec to file [%s]: [%s]", p.Input.OutputFile, err)
		return err
	}

	return err
}

// validateInput checks for the required inputs (chaincode language and path)
// and any flags supported by the legacy lscc but not _lifecycle
func (p *Packager) validateInput() error {
	if p.Input.Path == "" {
		return errors.New("chaincode path must be set")
	}
	if p.Input.Type == "" {
		return errors.New("chaincode language must be set")
	}
	if p.Input.Name != "" {
		return errors.New("chaincode name not supported by _lifecycle")
	}
	if p.Input.Version != "" {
		return errors.New("chaincode version not supported by _lifecycle")
	}
	if p.Input.InstantiationPolicy != "" {
		return errors.New("instantiation policy not supported by _lifecycle")
	}
	if p.Input.CreateSignedCCDepSpec {
		return errors.New("signed package not supported by _lifecycle")
	}
	if p.Input.SignCCDepSpec {
		return errors.New("signing of chaincode package not supported by _lifecycle")
	}

	return nil
}

func (p *Packager) getTarGzBytes() ([]byte, error) {
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	metadataBytes, err := toJSON(p.Input.Path, p.Input.Type)
	if err != nil {
		return nil, err
	}
	err = cutil.WriteBytesToPackage("Chaincode-Package-Metadata.json", metadataBytes, tw)
	if err != nil {
		return nil, errors.Wrap(err, "error writing package metadata to tar")
	}

	codeBytes, err := p.PlatformRegistry.GetDeploymentPayload(strings.ToUpper(p.Input.Type), p.Input.Path)
	if err != nil {
		err = errors.WithMessage(err, "error getting chaincode bytes")
		return nil, err
	}

	codePackageName := "Code-Package.tar.gz"
	if strings.ToLower(p.Input.Type) == "car" {
		codePackageName = "Code-Package.car"
	}

	err = cutil.WriteBytesToPackage(codePackageName, codeBytes, tw)
	if err != nil {
		return nil, errors.Wrap(err, "error writing package code bytes to tar")
	}

	err = tw.Close()
	if err == nil {
		err = gw.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create tar for chaincode")
	}

	return payload.Bytes(), nil
}

// PackageMetadata holds the path and type for a chaincode package
type PackageMetadata struct {
	Path string `json:"Path"`
	Type string `json:"Type"`
}

func toJSON(path, ccType string) ([]byte, error) {
	metadata := &PackageMetadata{
		Path: path,
		Type: ccType,
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal chaincode package metadata into JSON")
	}

	return metadataBytes, nil
}
func getInstantiationPolicy(policy string) (*pcommon.SignaturePolicyEnvelope, error) {
	p, err := cauthdsl.FromString(policy)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("invalid policy %s", policy))
	}
	return p, nil
}

// getChaincodeInstallPackage returns either a raw ChaincodeDeploymentSpec or
// a Envelope with ChaincodeDeploymentSpec and (optional) signature
func getChaincodeInstallPackage(cds *pb.ChaincodeDeploymentSpec, cf *ChaincodeCmdFactory) ([]byte, error) {
	// this can be raw ChaincodeDeploymentSpec or Envelope with signatures
	var objToWrite proto.Message

	// start with default cds
	objToWrite = cds

	var err error

	var owner msp.SigningIdentity

	// create a chaincode package...
	if createSignedCCDepSpec {
		// ...and optionally get the signer so the package can be signed
		// by the local MSP.  This package can be given to other owners
		// to sign using "peer chaincode sign <package file>"
		if signCCDepSpec {
			if cf.Signer == nil {
				return nil, errors.New("error getting signer")
			}
			owner = cf.Signer
		}
	}

	ip := instantiationPolicy
	if ip == "" {
		// if an instantiation policy is not given, default
		// to "admin  must sign chaincode instantiation proposals"
		mspid, err := mspmgmt.GetLocalMSP().GetIdentifier()
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
	objToWrite, err = ccpackage.OwnerCreateSignedCCDepSpec(cds, sp, owner)
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
