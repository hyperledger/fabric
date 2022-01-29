/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Install", func() {
	Describe("Installer", func() {
		var (
			mockProposalResponse *pb.ProposalResponse
			mockEndorserClient   *mock.EndorserClient
			mockReader           *mock.Reader
			mockSigner           *mock.Signer
			input                *chaincode.InstallInput
			installer            *chaincode.Installer
		)

		BeforeEach(func() {
			mockEndorserClient = &mock.EndorserClient{}
			mockProposalResponse = &pb.ProposalResponse{
				Response: &pb.Response{
					Status: 200,
				},
			}
			mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)

			input = &chaincode.InstallInput{
				PackageFile: "pkgFile",
			}

			mockReader = &mock.Reader{}
			mockSigner = &mock.Signer{}

			installer = &chaincode.Installer{
				Input:          input,
				EndorserClient: mockEndorserClient,
				Reader:         mockReader,
				Signer:         mockSigner,
			}
		})

		It("installs chaincodes", func() {
			err := installer.Install()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the chaincode install package is not provided", func() {
			BeforeEach(func() {
				installer.Input.PackageFile = ""
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("chaincode install package must be provided"))
			})
		})

		Context("when the package file cannot be read", func() {
			BeforeEach(func() {
				mockReader.ReadFileReturns(nil, errors.New("coffee"))
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("failed to read chaincode package at 'pkgFile': coffee"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("failed to serialize signer: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("failed to create signed proposal for chaincode install: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("failed to endorse chaincode install: latte"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("chaincode install failed: received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError("chaincode install failed: received proposal response with nil response"))
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
				err := installer.Install()
				Expect(err).To(MatchError("chaincode install failed with status: 500 - capuccino"))
			})
		})

		Context("when the payload contains bytes that aren't an InstallChaincodeResult", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Payload: []byte("badpayloadbadpayload"),
					Status:  200,
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := installer.Install()
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal proposal response's response payload")))
			})
		})
	})

	Describe("InstallCmd", func() {
		var installCmd *cobra.Command

		BeforeEach(func() {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			Expect(err).To(BeNil())
			installCmd = chaincode.InstallCmd(nil, cryptoProvider)
			installCmd.SilenceErrors = true
			installCmd.SilenceUsage = true
			installCmd.SetArgs([]string{
				"testpkg",
				"--peerAddresses=test1",
				"--tlsRootCertFiles=tls1",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("sets up the installer and attempts to install the chaincode", func() {
			err := installCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client for install")))
		})

		Context("when more than one peer address is provided", func() {
			BeforeEach(func() {
				installCmd.SetArgs([]string{
					"--peerAddresses=test3",
					"--peerAddresses=test4",
				})
			})

			It("returns an error", func() {
				err := installCmd.Execute()
				Expect(err).To(MatchError(ContainSubstring("failed to validate peer connection parameters")))
			})
		})
	})
})
