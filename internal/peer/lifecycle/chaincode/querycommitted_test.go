/*
Copyright IBM Corp. All Rights Reserved.

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

var _ = Describe("QueryCommitted", func() {
	Describe("CommittedQuerier", func() {
		var (
			mockProposalResponse *pb.ProposalResponse
			mockEndorserClient   *mock.EndorserClient
			mockSigner           *mock.Signer
			input                *chaincode.CommittedQueryInput
			committedQuerier     *chaincode.CommittedQuerier
		)

		BeforeEach(func() {
			mockResult := &lb.QueryChaincodeDefinitionsResult{
				ChaincodeDefinitions: []*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{
					{
						Name:              "woohoo",
						Sequence:          93,
						Version:           "a-version",
						EndorsementPlugin: "e-plugin",
						ValidationPlugin:  "v-plugin",
					},
					{
						Name:              "yahoo",
						Sequence:          20,
						Version:           "another-version",
						EndorsementPlugin: "e-plugin",
						ValidationPlugin:  "v-plugin",
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

			input = &chaincode.CommittedQueryInput{
				ChannelID: "test-channel",
			}

			committedQuerier = &chaincode.CommittedQuerier{
				Input:          input,
				EndorserClient: mockEndorserClient,
				Signer:         mockSigner,
				Writer:         buffer,
			}
		})

		It("queries committed chaincodes and writes the output as human readable plain-text", func() {
			err := committedQuerier.Query()
			Expect(err).NotTo(HaveOccurred())
			Eventually(committedQuerier.Writer).Should(gbytes.Say("Committed chaincode definitions on channel 'test-channel':\n"))
			Eventually(committedQuerier.Writer).Should(gbytes.Say("Name: woohoo, Version: a-version, Sequence: 93, Endorsement Plugin: e-plugin, Validation Plugin: v-plugin\n"))
			Eventually(committedQuerier.Writer).Should(gbytes.Say("Name: yahoo, Version: another-version, Sequence: 20, Endorsement Plugin: e-plugin, Validation Plugin: v-plugin\n"))
		})

		Context("when JSON-formatted output is requested", func() {
			BeforeEach(func() {
				committedQuerier.Input.OutputFormat = "json"
			})

			It("queries committed chaincodes and writes the output as JSON", func() {
				err := committedQuerier.Query()
				Expect(err).NotTo(HaveOccurred())
				expectedOutput := &lb.QueryChaincodeDefinitionsResult{
					ChaincodeDefinitions: []*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{
						{
							Name:              "woohoo",
							Sequence:          93,
							Version:           "a-version",
							EndorsementPlugin: "e-plugin",
							ValidationPlugin:  "v-plugin",
						},
						{
							Name:              "yahoo",
							Sequence:          20,
							Version:           "another-version",
							EndorsementPlugin: "e-plugin",
							ValidationPlugin:  "v-plugin",
						},
					},
				}
				json, err := json.MarshalIndent(expectedOutput, "", "\t")
				Expect(err).NotTo(HaveOccurred())
				Eventually(committedQuerier.Writer).Should(gbytes.Say(fmt.Sprintf(`\Q%s\E`, string(json))))
			})
		})

		Context("when a single chaincode definition is requested", func() {
			BeforeEach(func() {
				input.Name = "test-cc"

				mockResult := &lb.QueryChaincodeDefinitionResult{
					Sequence:          93,
					Version:           "a-version",
					EndorsementPlugin: "e-plugin",
					ValidationPlugin:  "v-plugin",
					Approvals: map[string]bool{
						"whatkindoforgisthis": true,
						"nowaydoiapprove":     false,
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

			It("queries the committed chaincode and writes the output as human readable plain-text", func() {
				err := committedQuerier.Query()
				Expect(err).NotTo(HaveOccurred())
				Eventually(committedQuerier.Writer).Should(gbytes.Say("Committed chaincode definition for chaincode 'test-cc' on channel 'test-channel'"))
				Eventually(committedQuerier.Writer).Should(gbytes.Say(`\QVersion: a-version, Sequence: 93, Endorsement Plugin: e-plugin, Validation Plugin: v-plugin, Approvals: [nowaydoiapprove: false, whatkindoforgisthis: true]\E`))
			})

			Context("when the payload contains bytes that aren't a QueryChaincodeDefinitionResult", func() {
				BeforeEach(func() {
					mockProposalResponse.Response = &pb.Response{
						Payload: []byte("badpayloadbadpayload"),
						Status:  200,
					}
					mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
				})

				It("returns an error", func() {
					err := committedQuerier.Query()
					Expect(err).To(MatchError(ContainSubstring("failed to unmarshal proposal response's response payload")))
				})
			})

			Context("when JSON-formatted output is requested", func() {
				BeforeEach(func() {
					committedQuerier.Input.OutputFormat = "json"
				})

				It("queries the committed chaincodes and writes the output as JSON", func() {
					err := committedQuerier.Query()
					Expect(err).NotTo(HaveOccurred())
					expectedOutput := &lb.QueryChaincodeDefinitionResult{
						Sequence:          93,
						Version:           "a-version",
						EndorsementPlugin: "e-plugin",
						ValidationPlugin:  "v-plugin",
						Approvals: map[string]bool{
							"whatkindoforgisthis": true,
							"nowaydoiapprove":     false,
						},
					}
					json, err := json.MarshalIndent(expectedOutput, "", "\t")
					Expect(err).NotTo(HaveOccurred())
					Eventually(committedQuerier.Writer).Should(gbytes.Say(fmt.Sprintf(`\Q%s\E`, string(json))))
				})
			})
		})

		Context("when the channel is not provided", func() {
			BeforeEach(func() {
				committedQuerier.Input.ChannelID = ""
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("channel name must be specified"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("failed to create signed proposal: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("failed to endorse proposal: latte"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError("received proposal response with nil response"))
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
				err := committedQuerier.Query()
				Expect(err).To(MatchError("query failed with status: 500 - capuccino"))
			})
		})

		Context("when the payload contains bytes that aren't a QueryChaincodeDefinitionsResult", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Payload: []byte("badpayloadbadpayload"),
					Status:  200,
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal proposal response's response payload")))
			})
		})

		Context("when the payload contains bytes that aren't a QueryChaincodeDefinitionsResult and JSON-output is requested", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = &pb.Response{
					Payload: []byte("badpayloadbadpayload"),
					Status:  200,
				}
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
				committedQuerier.Input.OutputFormat = "json"
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal proposal response's response payload as type *lifecycle.QueryChaincodeDefinitionsResult")))
			})
		})
	})

	Describe("QueryCommittedCmd", func() {
		var queryCommittedCmd *cobra.Command

		BeforeEach(func() {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			Expect(err).To(BeNil())
			queryCommittedCmd = chaincode.QueryCommittedCmd(nil, cryptoProvider)
			queryCommittedCmd.SilenceErrors = true
			queryCommittedCmd.SilenceUsage = true
			queryCommittedCmd.SetArgs([]string{
				"--name=testcc",
				"--channelID=testchannel",
				"--peerAddresses=querycommittedpeer1",
				"--tlsRootCertFiles=tls1",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("attempts to connect to the endorser", func() {
			err := queryCommittedCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client")))
		})

		Context("when more than one peer address is provided", func() {
			BeforeEach(func() {
				queryCommittedCmd.SetArgs([]string{
					"--name=testcc",
					"--channelID=testchannel",
					"--peerAddresses=querycommittedpeer1",
					"--tlsRootCertFiles=tls1",
					"--peerAddresses=querycommittedpeer2",
					"--tlsRootCertFiles=tls2",
				})
			})

			It("returns an error", func() {
				err := queryCommittedCmd.Execute()
				Expect(err).To(MatchError(ContainSubstring("failed to validate peer connection parameters")))
			})
		})
	})
})
