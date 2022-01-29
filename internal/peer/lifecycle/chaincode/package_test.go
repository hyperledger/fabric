/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo/v2"
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
			mockPlatformRegistry.NormalizePathReturns("normalizedPath", nil)

			input = &chaincode.PackageInput{
				OutputFile: "testDir/testPackage",
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

			Expect(mockPlatformRegistry.NormalizePathCallCount()).To(Equal(1))
			ccType, path := mockPlatformRegistry.NormalizePathArgsForCall(0)
			Expect(ccType).To(Equal("TESTTYPE"))
			Expect(path).To(Equal("testPath"))

			Expect(mockPlatformRegistry.GetDeploymentPayloadCallCount()).To(Equal(1))
			ccType, path = mockPlatformRegistry.GetDeploymentPayloadArgsForCall(0)
			Expect(ccType).To(Equal("TESTTYPE"))
			Expect(path).To(Equal("testPath"))

			Expect(mockWriter.WriteFileCallCount()).To(Equal(1))
			dir, name, pkgTarGzBytes := mockWriter.WriteFileArgsForCall(0)
			wd, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			Expect(dir).To(Equal(filepath.Join(wd, "testDir")))
			Expect(name).To(Equal("testPackage"))
			Expect(pkgTarGzBytes).NotTo(BeNil())

			metadata, err := readMetadataFromBytes(pkgTarGzBytes)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(MatchJSON(`{"path":"normalizedPath","type":"testType","label":"testLabel"}`))
		})

		Context("when the path is not provided", func() {
			BeforeEach(func() {
				input.Path = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("chaincode path must be specified"))
			})
		})

		Context("when the type is not provided", func() {
			BeforeEach(func() {
				input.Type = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("chaincode language must be specified"))
			})
		})

		Context("when the output file is not provided", func() {
			BeforeEach(func() {
				input.OutputFile = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("output file must be specified"))
			})
		})

		Context("when the label is not provided", func() {
			BeforeEach(func() {
				input.Label = ""
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("package label must be specified"))
			})
		})

		Context("when the label is invalid", func() {
			BeforeEach(func() {
				input.Label = "label with spaces"
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("invalid label 'label with spaces'. Label must be non-empty, can only consist of alphanumerics, symbols from '.+-_', and can only begin with alphanumerics"))
			})
		})

		Context("when the platform registry fails to normalize the path", func() {
			BeforeEach(func() {
				mockPlatformRegistry.NormalizePathReturns("", errors.New("cortado"))
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("failed to normalize chaincode path: cortado"))
			})
		})

		Context("when the platform registry fails to get the deployment payload", func() {
			BeforeEach(func() {
				mockPlatformRegistry.GetDeploymentPayloadReturns(nil, errors.New("americano"))
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("error getting chaincode bytes: americano"))
			})
		})

		Context("when writing the file fails", func() {
			BeforeEach(func() {
				mockWriter.WriteFileReturns(errors.New("espresso"))
			})

			It("returns an error", func() {
				err := packager.Package()
				Expect(err).To(MatchError("error writing chaincode package to testDir/testPackage: espresso"))
			})
		})
	})

	Describe("PackageCmd", func() {
		var packageCmd *cobra.Command

		BeforeEach(func() {
			packageCmd = chaincode.PackageCmd(nil)
			packageCmd.SilenceErrors = true
			packageCmd.SilenceUsage = true
			packageCmd.SetArgs([]string{
				"testPackage",
				"--path=testPath",
				"--lang=golang",
				"--label=testLabel",
			})
		})

		It("sets up the packager and attempts to package the chaincode", func() {
			err := packageCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("error getting chaincode bytes")))
		})

		Context("when more than one argument is provided", func() {
			BeforeEach(func() {
				packageCmd.SetArgs([]string{
					"testPackage",
					"whatthe",
				})
			})

			It("returns an error", func() {
				err := packageCmd.Execute()
				Expect(err).To(MatchError("invalid number of args. expected only the output file"))
			})
		})

		Context("when no argument is provided", func() {
			BeforeEach(func() {
				packageCmd.SetArgs([]string{})
			})

			It("returns an error", func() {
				err := packageCmd.Execute()
				Expect(err).To(MatchError("invalid number of args. expected only the output file"))
			})
		})
	})
})

func readMetadataFromBytes(pkgTarGzBytes []byte) ([]byte, error) {
	buffer := bytes.NewBuffer(pkgTarGzBytes)
	gzr, err := gzip.NewReader(buffer)
	Expect(err).NotTo(HaveOccurred())
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "metadata.json" {
			return ioutil.ReadAll(tr)
		}
	}
	return nil, errors.New("metadata.json not found")
}
