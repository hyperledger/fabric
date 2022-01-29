/*
Copyright Hitachi America, Ltd. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("QueryApproved", func() {
	Describe("ApprovedQuerier", func() {
		var (
			mockProposalResponse *pb.ProposalResponse
			mockEndorserClient   *mock.EndorserClient
			mockSigner           *mock.Signer
			input                *chaincode.ApprovedQueryInput
			approvedQuerier      *chaincode.ApprovedQuerier
		)

		BeforeEach(func() {
			mockResult := &lb.QueryApprovedChaincodeDefinitionResult{
				Sequence:            7,
				Version:             "version_1.0",
				EndorsementPlugin:   "endorsement-plugin",
				ValidationPlugin:    "validation-plugin",
				ValidationParameter: []byte("validation-parameter"),
				InitRequired:        true,
				Collections:         &pb.CollectionConfigPackage{},
				Source: &lb.ChaincodeSource{
					Type: &lb.ChaincodeSource_LocalPackage{
						LocalPackage: &lb.ChaincodeSource_Local{
							PackageId: "hash",
						},
					},
				},
			}

			mockResultBytes, err := proto.Marshal(mockResult)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockResultBytes).NotTo(BeNil())
			mockProposalResponse = &pb.ProposalResponse{
				Response: &pb.Response{
					Status:  200,
					Payload: mockResultBytes,
				},
			}

			mockEndorserClient = &mock.EndorserClient{}
			mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)

			mockSigner = &mock.Signer{}
			buffer := gbytes.NewBuffer()

			input = &chaincode.ApprovedQueryInput{
				ChannelID: "test-channel",
				Name:      "cc_name",
			}

			approvedQuerier = &chaincode.ApprovedQuerier{
				Input:          input,
				EndorserClient: mockEndorserClient,
				Signer:         mockSigner,
				Writer:         buffer,
			}
		})

		It("queries a approved chaincode and writes the output as human readable plain-text", func() {
			err := approvedQuerier.Query()
			Expect(err).NotTo(HaveOccurred())
			Eventually(approvedQuerier.Writer).Should(gbytes.Say("Approved chaincode definition for chaincode 'cc_name' on channel 'test-channel':\n"))
			Eventually(approvedQuerier.Writer).Should(gbytes.Say("sequence: 7, version: version_1.0, init-required: true, package-id: hash, endorsement plugin: endorsement-plugin, validation plugin: validation-plugin\n"))
		})

		Context("when the payload contains bytes that aren't a QueryApprovedChaincodeDefinitionResult", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Payload: []byte("badpayloadbadpayload"),
					Status:  200,
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal proposal response's response payload")))
			})
		})

		Context("when JSON-formatted output is requested", func() {
			BeforeEach(func() {
				approvedQuerier.Input.OutputFormat = "json"
			})

			It("queries the approved chaincode and writes the output as JSON", func() {
				err := approvedQuerier.Query()
				Expect(err).NotTo(HaveOccurred())
				expectedOutput := &lb.QueryApprovedChaincodeDefinitionResult{
					Sequence:            7,
					Version:             "version_1.0",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					InitRequired:        true,
					Collections:         &pb.CollectionConfigPackage{},
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_LocalPackage{
							LocalPackage: &lb.ChaincodeSource_Local{
								PackageId: "hash",
							},
						},
					},
				}
				json, err := json.MarshalIndent(expectedOutput, "", "\t")
				Expect(err).NotTo(HaveOccurred())
				Eventually(approvedQuerier.Writer).Should(gbytes.Say(fmt.Sprintf(`\Q%s\E`, string(json))))
			})
		})

		Context("when the chaincode source is unavailable", func() {
			BeforeEach(func() {
				mockResult := &lb.QueryApprovedChaincodeDefinitionResult{
					Sequence:            7,
					Version:             "version_1.0",
					EndorsementPlugin:   "endorsement-plugin",
					ValidationPlugin:    "validation-plugin",
					ValidationParameter: []byte("validation-parameter"),
					InitRequired:        true,
					Collections:         &pb.CollectionConfigPackage{},
					Source: &lb.ChaincodeSource{
						Type: &lb.ChaincodeSource_Unavailable_{
							Unavailable: &lb.ChaincodeSource_Unavailable{},
						},
					},
				}
				mockResultBytes, err := proto.Marshal(mockResult)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockResultBytes).NotTo(BeNil())

				mockProposalResponse = &pb.ProposalResponse{
					Response: &pb.Response{
						Status:  200,
						Payload: mockResultBytes,
					},
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("queries the approved chaincode and writes the output as human readable plain-text with an empty package-id", func() {
				err := approvedQuerier.Query()
				Expect(err).NotTo(HaveOccurred())
				Eventually(approvedQuerier.Writer).Should(gbytes.Say("Approved chaincode definition for chaincode 'cc_name' on channel 'test-channel':\n"))
				Eventually(approvedQuerier.Writer).Should(gbytes.Say("sequence: 7, version: version_1.0, init-required: true, package-id: , endorsement plugin: endorsement-plugin, validation plugin: validation-plugin\n"))
			})
		})

		Context("when the channel is not provided", func() {
			BeforeEach(func() {
				approvedQuerier.Input.ChannelID = ""
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("The required parameter 'channelID' is empty. Rerun the command with -C flag"))
			})
		})

		Context("when the chaincode name is not provided", func() {
			BeforeEach(func() {
				approvedQuerier.Input.Name = ""
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("The required parameter 'name' is empty. Rerun the command with -n flag"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("bad serialization"))
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("failed to create proposal: failed to serialize identity: bad serialization"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("bad sign"))
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("failed to create signed proposal: bad sign"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("bad endorsement"))
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("failed to endorse proposal: bad endorsement"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("received proposal response with nil response"))
			})
		})

		Context("when the endorser returns a non-success status", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Status:  500,
					Message: "message",
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approvedQuerier.Query()
				Expect(err).To(MatchError("query failed with status: 500 - message"))
			})
		})
	})

	Describe("QueryApprovedCmd", func() {
		var queryApprovedCmd *cobra.Command

		BeforeEach(func() {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			Expect(err).To(BeNil())
			queryApprovedCmd = chaincode.QueryApprovedCmd(nil, cryptoProvider)
			queryApprovedCmd.SilenceErrors = true
			queryApprovedCmd.SilenceUsage = true
			queryApprovedCmd.SetArgs([]string{
				"--name=testcc",
				"--channelID=testchannel",
				"--peerAddresses=queryapprovedpeer1",
				"--tlsRootCertFiles=tls1",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("attempts to connect to the endorser", func() {
			err := queryApprovedCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client")))
		})

		Context("when more than one peer address is provided", func() {
			BeforeEach(func() {
				queryApprovedCmd.SetArgs([]string{
					"--name=testcc",
					"--channelID=testchannel",
					"--peerAddresses=queryapprovedpeer1",
					"--tlsRootCertFiles=tls1",
					"--peerAddresses=queryapprovedpeer2",
					"--tlsRootCertFiles=tls2",
				})
			})

			It("returns an error", func() {
				err := queryApprovedCmd.Execute()
				Expect(err).To(MatchError(ContainSubstring("failed to validate peer connection parameters")))
			})
		})
	})
})
