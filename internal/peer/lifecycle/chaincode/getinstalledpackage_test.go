/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetInstalledPackage", func() {
	Describe("InstalledPackageGetter", func() {
		var (
			mockProposalResponse   *pb.ProposalResponse
			mockEndorserClient     *mock.EndorserClient
			mockWriter             *mock.Writer
			mockSigner             *mock.Signer
			testDir                string
			input                  *chaincode.GetInstalledPackageInput
			installedPackageGetter *chaincode.InstalledPackageGetter
		)

		BeforeEach(func() {
			mockEndorserClient = &mock.EndorserClient{}
			mockProposalResponse = &pb.ProposalResponse{
				Response: &pb.Response{
					Status: 200,
				},
			}
			mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)

			var err error
			testDir, err = ioutil.TempDir("", "getinstalledpackage-test")
			Expect(err).NotTo(HaveOccurred())
			input = &chaincode.GetInstalledPackageInput{
				PackageID:       "pkgFile",
				OutputDirectory: testDir,
			}

			mockWriter = &mock.Writer{}
			mockSigner = &mock.Signer{}

			installedPackageGetter = &chaincode.InstalledPackageGetter{
				Input:          input,
				EndorserClient: mockEndorserClient,
				Writer:         mockWriter,
				Signer:         mockSigner,
			}
		})

		AfterEach(func() {
			os.RemoveAll(testDir)
		})

		It("gets the installed chaincode package and writes it to the specified directory", func() {
			err := installedPackageGetter.Get()
			Expect(err).NotTo(HaveOccurred())
			Expect(mockWriter.WriteFileCallCount()).To(Equal(1))
			dir, name, _ := mockWriter.WriteFileArgsForCall(0)
			Expect(err).NotTo(HaveOccurred())
			Expect(dir).To(Equal(testDir))
			Expect(name).To(Equal("pkgFile.tar.gz"))
		})

		Context("when the output directory is not specified", func() {
			BeforeEach(func() {
				input.OutputDirectory = ""
			})

			It("get the installed chaincode package and writes it to the working directory", func() {
				err := installedPackageGetter.Get()
				Expect(err).NotTo(HaveOccurred())
				Expect(mockWriter.WriteFileCallCount()).To(Equal(1))
				dir, name, _ := mockWriter.WriteFileArgsForCall(0)
				wd, err := os.Getwd()
				Expect(err).NotTo(HaveOccurred())
				Expect(dir).To(Equal(wd))
				Expect(name).To(Equal("pkgFile.tar.gz"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the package id is not specified", func() {
			BeforeEach(func() {
				input.PackageID = ""
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("The required parameter 'package-id' is empty. Rerun the command with --package-id flag"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to create signed proposal: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to endorse proposal: latte"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("received proposal response with nil response"))
			})
		})

		Context("when the endorser returns a non-success status", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Status:  500,
					Message: "capuccino",
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("proposal failed with status: 500 - capuccino"))
			})
		})

		Context("when the payload contains bytes that aren't an GetInstalledChaincodePackageResult", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Payload: []byte("badpayloadbadpayload"),
					Status:  200,
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal proposal response's response payload"))
			})
		})

		Context("when the writer fails to write the chaincode package", func() {
			BeforeEach(func() {
				mockWriter.WriteFileReturns(errors.New("frappuccino"))
			})

			It("returns an error", func() {
				err := installedPackageGetter.Get()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("failed to write chaincode package to %s: frappuccino", filepath.Join(testDir, "pkgFile.tar.gz"))))
			})
		})
	})

	Describe("GetInstalledPackageCmd", func() {
		var (
			getInstalledPackageCmd *cobra.Command
		)

		BeforeEach(func() {
			getInstalledPackageCmd = chaincode.GetInstalledPackageCmd(nil)
			getInstalledPackageCmd.SetArgs([]string{
				"--package-id=test-package",
				"--peerAddresses=test1",
				"--tlsRootCertFiles=tls1",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("sets up the installedPackageGetter and attempts to get the installed chaincode package", func() {
			err := getInstalledPackageCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve endorser client for getinstalledpackage"))
		})

		Context("when more than one peer address is provided", func() {
			BeforeEach(func() {
				getInstalledPackageCmd.SetArgs([]string{
					"--peerAddresses=test3",
					"--peerAddresses=test4",
				})
			})

			It("returns an error", func() {
				err := getInstalledPackageCmd.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate peer connection parameters"))
			})
		})
	})
})
