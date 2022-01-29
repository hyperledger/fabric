/*
Copyright Hitachi, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("CalculatePackageID", func() {
	Describe("PackageIDCalculator", func() {
		var (
			mockReader          *mock.Reader
			input               *chaincode.CalculatePackageIDInput
			packageIDCalculator *chaincode.PackageIDCalculator
		)

		BeforeEach(func() {
			input = &chaincode.CalculatePackageIDInput{
				PackageFile: "pkgFile",
			}

			mockReader = &mock.Reader{}
			data, err := ioutil.ReadFile("testdata/good-package.tar.gz")
			Expect(err).NotTo(HaveOccurred())
			mockReader.ReadFileReturns(data, nil)

			buffer := gbytes.NewBuffer()

			packageIDCalculator = &chaincode.PackageIDCalculator{
				Input:  input,
				Reader: mockReader,
				Writer: buffer,
			}
		})

		It("calculates the package IDs for chaincodes", func() {
			err := packageIDCalculator.PackageID()
			Expect(err).NotTo(HaveOccurred())
			Eventually(packageIDCalculator.Writer).Should(gbytes.Say("Real-Label:fb3edf9621c5e3d864079d8c9764205f4db09d7021cfa4124aa79f4edcc2f64a\n"))
		})

		Context("when the chaincode install package is not provided", func() {
			BeforeEach(func() {
				packageIDCalculator.Input.PackageFile = ""
			})

			It("returns an error", func() {
				err := packageIDCalculator.PackageID()
				Expect(err).To(MatchError("chaincode install package must be provided"))
			})
		})

		Context("when the package file cannot be read", func() {
			BeforeEach(func() {
				mockReader.ReadFileReturns(nil, errors.New("coffee"))
			})

			It("returns an error", func() {
				err := packageIDCalculator.PackageID()
				Expect(err).To(MatchError("failed to read chaincode package at 'pkgFile': coffee"))
			})
		})

		Context("when the package file cannot be parsed", func() {
			BeforeEach(func() {
				data, err := ioutil.ReadFile("testdata/unparsed-package.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				mockReader.ReadFileReturns(data, nil)
			})

			It("returns an error", func() {
				err := packageIDCalculator.PackageID()
				Expect(err).To(MatchError(ContainSubstring("could not parse as a chaincode install package")))
			})
		})

		Context("when JSON-formatted output is requested", func() {
			BeforeEach(func() {
				packageIDCalculator.Input.OutputFormat = "json"
			})

			It("calculates the package IDs for chaincodes and writes the output as JSON", func() {
				err := packageIDCalculator.PackageID()
				Expect(err).NotTo(HaveOccurred())
				expectedOutput := &chaincode.CalculatePackageIDOutput{
					PackageID: "Real-Label:fb3edf9621c5e3d864079d8c9764205f4db09d7021cfa4124aa79f4edcc2f64a",
				}
				json, err := json.MarshalIndent(expectedOutput, "", "\t")
				Expect(err).NotTo(HaveOccurred())
				Eventually(packageIDCalculator.Writer).Should(gbytes.Say(fmt.Sprintf(`\Q%s\E`, string(json))))
			})
		})
	})

	Describe("CalculatePackageIDCmd", func() {
		var calculatePackageIDCmd *cobra.Command

		BeforeEach(func() {
			calculatePackageIDCmd = chaincode.CalculatePackageIDCmd(nil)
			calculatePackageIDCmd.SilenceErrors = true
			calculatePackageIDCmd.SilenceUsage = true
			calculatePackageIDCmd.SetArgs([]string{
				"testpkg",
			})
		})

		It("sets up the calculator and attempts to calculate the package ID for the chaincode", func() {
			err := calculatePackageIDCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to read chaincode package at 'testpkg'")))
		})

		Context("when more than one argument is provided", func() {
			BeforeEach(func() {
				calculatePackageIDCmd.SetArgs([]string{
					"testpkg",
					"whatthe",
				})
			})

			It("returns an error", func() {
				err := calculatePackageIDCmd.Execute()
				Expect(err).To(MatchError("invalid number of args. expected only the packaged chaincode file"))
			})
		})

		Context("when no argument is provided", func() {
			BeforeEach(func() {
				calculatePackageIDCmd.SetArgs([]string{})
			})

			It("returns an error", func() {
				err := calculatePackageIDCmd.Execute()
				Expect(err).To(MatchError("invalid number of args. expected only the packaged chaincode file"))
			})
		})
	})
})
