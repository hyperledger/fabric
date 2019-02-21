/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
			mockEndorserClient = &mock.EndorserClient{}
			mockProposalResponse = &pb.ProposalResponse{
				Response: &pb.Response{
					Status: 200,
				},
			}
			mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)

			input = &chaincode.CommittedQueryInput{
				ChannelID: "test-channel",
				Name:      "test-cc",
			}

			mockSigner = &mock.Signer{}

			committedQuerier = &chaincode.CommittedQuerier{
				Input:          input,
				EndorserClient: mockEndorserClient,
				Signer:         mockSigner,
			}
		})

		It("queries installed chaincodes", func() {
			err := committedQuerier.Query()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the chaincode name is not provided", func() {
			BeforeEach(func() {
				committedQuerier.Input.ChannelID = ""
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("channel name must be specified"))
			})
		})

		Context("when the chaincode name is not provided", func() {
			BeforeEach(func() {
				committedQuerier.Input.Name = ""
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("chaincode name must be specified"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to create signed proposal: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := committedQuerier.Query()
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
				err := committedQuerier.Query()
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
				err := committedQuerier.Query()
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
				err := committedQuerier.Query()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("query failed with status: 500 - capuccino"))
			})
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
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal proposal response's response payload"))
			})
		})
	})

	Describe("QueryCommittedCmd", func() {
		var (
			queryCommittedCmd *cobra.Command
		)

		BeforeEach(func() {
			queryCommittedCmd = chaincode.QueryCommittedCmd(nil)
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
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve endorser client"))
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
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to validate peer connection parameters"))
			})
		})
	})
})
