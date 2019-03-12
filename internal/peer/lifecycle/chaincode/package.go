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
	"os"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodePackageCmd *cobra.Command

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
	ChaincodeCmdFactory *CmdFactory
	Command             *cobra.Command
	Input               *PackageInput
	PlatformRegistry    PlatformRegistry
	Writer              Writer
}

// PackageInput holds the input parameters for packaging a
// ChaincodeInstallPackage
type PackageInput struct {
	OutputFile string
	Path       string
	Type       string
	Label      string
}

// packageCmd returns the cobra command for packaging chaincode
func packageCmd(cf *CmdFactory, p *Packager) *cobra.Command {
	chaincodePackageCmd = &cobra.Command{
		Use:       "package [outputfile]",
		Short:     "Package a chaincode",
		Long:      "Package a chaincode and write the package to a file.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if p == nil {
				pr := platforms.NewRegistry(platforms.SupportedPlatforms...)

				p = &Packager{
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
		"label",
		"lang",
		"path",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
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

	return p.packageCC()
}

func (p *Packager) setInput(outputFile string) {
	p.Input = &PackageInput{
		OutputFile: outputFile,
		Path:       chaincodePath,
		Type:       chaincodeLang,
		Label:      packageLabel,
	}
}

// packageCC packages chaincodes into the package type,
// (.tar.gz) used by _lifecycle and writes it to disk
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

// validateInput checks for the required inputs (chaincode language and path)
func (p *Packager) validateInput() error {
	if p.Input.Path == "" {
		return errors.New("chaincode path must be set")
	}
	if p.Input.Type == "" {
		return errors.New("chaincode language must be set")
	}
	if p.Input.Label == "" {
		return errors.New("package label must be set")
	}

	return nil
}

func (p *Packager) getTarGzBytes() ([]byte, error) {
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	metadataBytes, err := toJSON(p.Input.Path, p.Input.Type, p.Input.Label)
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
	Path  string `json:"Path"`
	Type  string `json:"Type"`
	Label string `json:"Label"`
}

func toJSON(path, ccType, label string) ([]byte, error) {
	metadata := &PackageMetadata{
		Path:  path,
		Type:  ccType,
		Label: label,
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal chaincode package metadata into JSON")
	}

	return metadataBytes, nil
}
