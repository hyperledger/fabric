/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"context"
	"crypto/tls"
	"time"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode/mock"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApproverForMyOrg", func() {
	Describe("Approve", func() {
		var (
			mockProposalResponse *pb.ProposalResponse
			mockEndorserClient   *mock.EndorserClient
			mockEndorserClients  []chaincode.EndorserClient
			mockDeliverClient    *mock.PeerDeliverClient
			mockSigner           *mock.Signer
			certificate          tls.Certificate
			mockBroadcastClient  *mock.BroadcastClient
			input                *chaincode.ApproveForMyOrgInput
			approver             *chaincode.ApproverForMyOrg
		)

		BeforeEach(func() {
			mockEndorserClient = &mock.EndorserClient{}

			mockProposalResponse = &pb.ProposalResponse{
				Response: &pb.Response{
					Status: 200,
				},
				Endorsement: &pb.Endorsement{},
			}
			mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			mockEndorserClients = []chaincode.EndorserClient{mockEndorserClient}

			mockDeliverClient = &mock.PeerDeliverClient{}
			input = &chaincode.ApproveForMyOrgInput{
				ChannelID: "testchannel",
				Name:      "testcc",
				Version:   "1.0",
				PackageID: "testpackageid",
				Sequence:  1,
			}

			mockSigner = &mock.Signer{}

			certificate = tls.Certificate{}
			mockBroadcastClient = &mock.BroadcastClient{}

			approver = &chaincode.ApproverForMyOrg{
				Certificate:     certificate,
				BroadcastClient: mockBroadcastClient,
				DeliverClients:  []pb.DeliverClient{mockDeliverClient},
				EndorserClients: mockEndorserClients,
				Input:           input,
				Signer:          mockSigner,
			}
		})

		It("approves a chaincode definition for an organization", func() {
			err := approver.Approve()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the channel name is not provided", func() {
			BeforeEach(func() {
				approver.Input.ChannelID = ""
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("The required parameter 'channelID' is empty. Rerun the command with -C flag"))
			})
		})

		Context("when the chaincode name is not provided", func() {
			BeforeEach(func() {
				approver.Input.Name = ""
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("The required parameter 'name' is empty. Rerun the command with -n flag"))
			})
		})

		Context("when the chaincode version is not provided", func() {
			BeforeEach(func() {
				approver.Input.Version = ""
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("The required parameter 'version' is empty. Rerun the command with -v flag"))
			})
		})

		Context("when the sequence is not provided", func() {
			BeforeEach(func() {
				approver.Input.Sequence = 0
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("The required parameter 'sequence' is empty. Rerun the command with --sequence flag"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to create signed proposal: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to endorse proposal: latte"))
			})
		})

		Context("when no endorser clients are set", func() {
			BeforeEach(func() {
				approver.EndorserClients = nil
			})

			It("doesn't receive any responses and returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("no proposal responses received"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := approver.Approve()
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
				err := approver.Approve()
				Expect(err).To(MatchError("proposal failed with status: 500 - capuccino"))
			})
		})

		Context("when the signer fails to sign the transaction", func() {
			BeforeEach(func() {
				mockSigner.SignReturnsOnCall(1, nil, errors.New("peaberry"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to create signed transaction: peaberry"))
				Expect(mockSigner.SignCallCount()).To(Equal(2))
			})
		})

		Context("when the broadcast client fails to send the envelope", func() {
			BeforeEach(func() {
				mockBroadcastClient.SendReturns(errors.New("arabica"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to send transaction: arabica"))
			})
		})

		Context("when the wait for event flag is enabled and the transaction is committed", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 3 * time.Second
				input.TxID = "testtx"
				input.PeerAddresses = []string{"approvepeer0"}
				mockDeliverClient.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
					mockDF := &mock.Deliver{}
					resp := &pb.DeliverResponse{
						Type: &pb.DeliverResponse_FilteredBlock{
							FilteredBlock: createFilteredBlock(input.ChannelID, "testtx"),
						},
					}
					mockDF.RecvReturns(resp, nil)
					return mockDF, nil
				}
			})

			It("waits for the event containing the txid", func() {
				err := approver.Approve()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the wait for event flag is enabled and the client can't connect", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 3 * time.Second
				input.TxID = "testtx"
				input.PeerAddresses = []string{"approvepeer0"}
				mockDeliverClient.DeliverFilteredReturns(nil, errors.New("robusta"))
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("failed to connect to deliver on all peers: error connecting to deliver filtered at approvepeer0: robusta"))
			})
		})

		Context("when the wait for event flag is enabled and the transaction isn't returned before the timeout", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 10 * time.Millisecond
				input.TxID = "testtx"
				input.PeerAddresses = []string{"approvepeer0"}
				delayChan := make(chan struct{})
				mockDeliverClient.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
					mockDF := &mock.Deliver{}
					mockDF.RecvStub = func() (*pb.DeliverResponse, error) {
						<-delayChan
						resp := &pb.DeliverResponse{
							Type: &pb.DeliverResponse_FilteredBlock{
								FilteredBlock: createFilteredBlock(input.ChannelID, "testtx"),
							},
						}
						return resp, nil
					}
					return mockDF, nil
				}
			})

			It("returns an error", func() {
				err := approver.Approve()
				Expect(err).To(MatchError("timed out waiting for txid on all peers"))
			})
		})
	})

	Describe("ApproveForMyOrgCmd", func() {
		var approveForMyOrgCmd *cobra.Command

		BeforeEach(func() {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			Expect(err).To(BeNil())
			approveForMyOrgCmd = chaincode.ApproveForMyOrgCmd(nil, cryptoProvider)
			approveForMyOrgCmd.SilenceErrors = true
			approveForMyOrgCmd.SilenceUsage = true
			approveForMyOrgCmd.SetArgs([]string{
				"--channelID=testchannel",
				"--name=testcc",
				"--version=testversion",
				"--package-id=testpackageid",
				"--sequence=1",
				"--peerAddresses=querypeer1",
				"--tlsRootCertFiles=tls1",
				"--signature-policy=AND ('Org1MSP.member','Org2MSP.member')",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("sets up the approver for my org and attempts to approve the chaincode definition", func() {
			err := approveForMyOrgCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client")))
		})

		Context("when the channel config policy is specified", func() {
			BeforeEach(func() {
				approveForMyOrgCmd.SetArgs([]string{
					"--channel-config-policy=/Channel/Application/Readers",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--package-id=testpackageid",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("sets up the approver for my org and attempts to approve the chaincode definition", func() {
				err := approveForMyOrgCmd.Execute()
				Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client")))
			})
		})

		Context("when a signature policy and channel config policy are both specified", func() {
			BeforeEach(func() {
				approveForMyOrgCmd.SetArgs([]string{
					"--signature-policy=a_policy",
					"--channel-config-policy=anotha_policy",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--package-id=testpackageid",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("returns an error", func() {
				err := approveForMyOrgCmd.Execute()
				Expect(err).To(MatchError("cannot specify both \"--signature-policy\" and \"--channel-config-policy\""))
			})
		})

		Context("when the signature policy is invalid", func() {
			BeforeEach(func() {
				approveForMyOrgCmd.SetArgs([]string{
					"--signature-policy=notapolicy",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--package-id=testpackageid",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("returns an error", func() {
				err := approveForMyOrgCmd.Execute()
				Expect(err).To(MatchError("invalid signature policy: notapolicy"))
			})
		})

		Context("when the collections config is invalid", func() {
			BeforeEach(func() {
				approveForMyOrgCmd.SetArgs([]string{
					"--collections-config=idontexist.json",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--package-id=testpackageid",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("returns an error", func() {
				err := approveForMyOrgCmd.Execute()
				Expect(err).To(MatchError("invalid collection configuration in file idontexist.json: could not read file 'idontexist.json': open idontexist.json: no such file or directory"))
			})
		})
	})
})

func createFilteredBlock(channelID string, txIDs ...string) *pb.FilteredBlock {
	var filteredTransactions []*pb.FilteredTransaction
	for _, txID := range txIDs {
		ft := &pb.FilteredTransaction{
			Txid:             txID,
			TxValidationCode: pb.TxValidationCode_VALID,
		}
		filteredTransactions = append(filteredTransactions, ft)
	}
	fb := &pb.FilteredBlock{
		Number:               0,
		ChannelId:            channelID,
		FilteredTransactions: filteredTransactions,
	}
	return fb
}
