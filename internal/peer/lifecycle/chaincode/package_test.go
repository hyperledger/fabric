/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Package", func() {
	Describe("Packager", func() {
		var (
			mockPlatformRegistry *mock.PlatformRegistry
			mockWriter           *mock.Writer
			input                *chaincode.PackageInput
			packager             *chaincode.Packager
		)

		BeforeEach(func() {
			mockPlatformRegistry = &mock.PlatformRegistry{}

			input = &chaincode.PackageInput{
				OutputFile: "testPackage",
				Path:       "testPath",
				Type:       "testType",
				Label:      "testLabel",
			}

			mockWriter = &mock.Writer{}

			packager = &chaincode.Packager{
				PlatformRegistry: mockPlatformRegistry,
				Writer:           mockWriter,
				Input:            input,
			}
		})

		It("packages chaincodes", func() {
			err := packager.Package()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the path is not provided", func() {
			BeforeEach(func() {
				input.Path = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode path must be specified"))
			})
		})

		Context("when the type is not provided", func() {
			BeforeEach(func() {
				input.Type = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode language must be specified"))
			})
		})

		Context("when the output file is not provided", func() {
			BeforeEach(func() {
				input.OutputFile = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("output file must be specified"))
			})
		})

		Context("when the label is not provided", func() {
			BeforeEach(func() {
				input.Label = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("package label must be specified"))
			})
		})

		Context("when the platform registry fails to get the deployment payload", func() {
			BeforeEach(func() {
				mockPlatformRegistry.GetDeploymentPayloadReturns(nil, errors.New("americano"))
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("error getting chaincode bytes: americano"))
			})
		})

		Context("when writing the file fails", func() {
			BeforeEach(func() {
				mockWriter.WriteFileReturns(errors.New("espresso"))
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("error writing chaincode package to testPackage: espresso"))
			})
		})
	})

	Describe("PackageCmd", func() {
		var (
			packageCmd *cobra.Command
		)

		BeforeEach(func() {
			packageCmd = chaincode.PackageCmd(nil)
			packageCmd.SetArgs([]string{
				"testPackage",
				"--path=testPath",
				"--lang=golang",
				"--label=testLabel",
			})
		})

		It("sets up the packager and attempts to package the chaincode", func() {
			err := packageCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error getting chaincode bytes"))
		})

		Context("when more than one argument is provided", func() {
			BeforeEach(func() {
				packageCmd = chaincode.PackageCmd(nil)
				packageCmd.SetArgs([]string{
					"testPackage",
					"whatthe",
				})
			})

			It("returns an error", func() {
				err := packageCmd.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("invalid number of args. expected only the output file"))
			})
		})

		Context("when no argument is provided", func() {
			BeforeEach(func() {
				packageCmd = chaincode.PackageCmd(nil)
				packageCmd.SetArgs([]string{})
			})

			It("returns an error", func() {
				err := packageCmd.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("invalid number of args. expected only the output file"))
			})
		})
	})
})
