/*
Copyright Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io"
	"os"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// PackageIDCalculator holds the dependencies needed to calculate
// the package ID for a packaged chaincode
type PackageIDCalculator struct {
	Command *cobra.Command
	Input   *CalculatePackageIDInput
	Reader  Reader
	Writer  io.Writer
}

// CalculatePackageIDInput holds the input parameters for calculating
// the package ID of a packaged chaincode
type CalculatePackageIDInput struct {
	PackageFile string
}

// Validate checks that the required parameters are provided
func (i *CalculatePackageIDInput) Validate() error {
	if i.PackageFile == "" {
		return errors.New("chaincode install package must be provided")
	}

	return nil
}

// CalculatePackageIDCmd returns the cobra command for calculating
// the package ID for a packaged chaincode
func CalculatePackageIDCmd(p *PackageIDCalculator) *cobra.Command {
	calculatePackageIDCmd := &cobra.Command{
		Use:       "calculatepackageid packageFile",
		Short:     "Calculate the package ID for a chaincode.",
		Long:      "Calculate the package ID for a packaged chaincode.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if p == nil {
				p = &PackageIDCalculator{
					Reader: &persistence.FilesystemIO{},
					Writer: os.Stdout,
				}
			}
			p.Command = cmd

			return p.CalculatePackageID(args)
		},
	}
	flagList := []string{
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(calculatePackageIDCmd, flagList)

	return calculatePackageIDCmd
}

// PackageIDCalculator calculates the package ID for a packaged chaincode.
func (p *PackageIDCalculator) CalculatePackageID(args []string) error {
	if p.Command != nil {
		// Parsing of the command line is done so silence cmd usage
		p.Command.SilenceUsage = true
	}

	if len(args) != 1 {
		return errors.New("invalid number of args. expected only the packaged chaincode file")
	}
	p.setInput(args[0])

	return p.PackageID()
}

// PackageID calculates the package ID for a packaged chaincode and print it.
func (p *PackageIDCalculator) PackageID() error {
	err := p.Input.Validate()
	if err != nil {
		return err
	}
	pkgBytes, err := p.Reader.ReadFile(p.Input.PackageFile)
	if err != nil {
		return errors.WithMessagef(err, "failed to read chaincode package at '%s'", p.Input.PackageFile)
	}

	metadata, _, err := persistence.ParseChaincodePackage(pkgBytes)
	if err != nil {
		return errors.WithMessage(err, "could not parse as a chaincode install package")
	}

	packageID := persistence.PackageID(metadata.Label, pkgBytes)

	fmt.Fprintf(p.Writer, "Package ID: %s\n", packageID)
	return nil
}

func (p *PackageIDCalculator) setInput(packageFile string) {
	p.Input = &CalculatePackageIDInput{
		PackageFile: packageFile,
	}
}
