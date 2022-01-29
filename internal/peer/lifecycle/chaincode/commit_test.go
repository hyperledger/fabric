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

var _ = Describe("Commit", func() {
	Describe("Committer", func() {
		var (
			mockProposalResponse *pb.ProposalResponse
			mockEndorserClient   *mock.EndorserClient
			mockEndorserClients  []chaincode.EndorserClient
			mockDeliverClient    *mock.PeerDeliverClient
			mockSigner           *mock.Signer
			certificate          tls.Certificate
			mockBroadcastClient  *mock.BroadcastClient
			input                *chaincode.CommitInput
			committer            *chaincode.Committer
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
			input = &chaincode.CommitInput{
				ChannelID: "testchannel",
				Name:      "testcc",
				Version:   "1.0",
				Sequence:  1,
			}

			mockSigner = &mock.Signer{}

			certificate = tls.Certificate{}
			mockBroadcastClient = &mock.BroadcastClient{}

			committer = &chaincode.Committer{
				Certificate:     certificate,
				BroadcastClient: mockBroadcastClient,
				DeliverClients:  []pb.DeliverClient{mockDeliverClient},
				EndorserClients: mockEndorserClients,
				Input:           input,
				Signer:          mockSigner,
			}
		})

		It("commits a chaincode definition", func() {
			err := committer.Commit()
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the channel name is not provided", func() {
			BeforeEach(func() {
				committer.Input.ChannelID = ""
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("The required parameter 'channelID' is empty. Rerun the command with -C flag"))
			})
		})

		Context("when the chaincode name is not provided", func() {
			BeforeEach(func() {
				committer.Input.Name = ""
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("The required parameter 'name' is empty. Rerun the command with -n flag"))
			})
		})

		Context("when the chaincode version is not provided", func() {
			BeforeEach(func() {
				committer.Input.Version = ""
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("The required parameter 'version' is empty. Rerun the command with -v flag"))
			})
		})

		Context("when the sequence is not provided", func() {
			BeforeEach(func() {
				committer.Input.Sequence = 0
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("The required parameter 'sequence' is empty. Rerun the command with --sequence flag"))
			})
		})

		Context("when the signer cannot be serialized", func() {
			BeforeEach(func() {
				mockSigner.SerializeReturns(nil, errors.New("cafe"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to create proposal: failed to serialize identity: cafe"))
			})
		})

		Context("when the signer fails to sign the proposal", func() {
			BeforeEach(func() {
				mockSigner.SignReturns(nil, errors.New("tea"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to create signed proposal: tea"))
			})
		})

		Context("when the endorser fails to endorse the proposal", func() {
			BeforeEach(func() {
				mockEndorserClient.ProcessProposalReturns(nil, errors.New("latte"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to endorse proposal: latte"))
			})
		})

		Context("when no endorser clients are set", func() {
			BeforeEach(func() {
				committer.EndorserClients = nil
			})

			It("doesn't receive any responses and returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("no proposal responses received"))
			})
		})

		Context("when the endorser returns a nil proposal response", func() {
			BeforeEach(func() {
				mockProposalResponse = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("received nil proposal response"))
			})
		})

		Context("when the endorser returns a proposal response with a nil response", func() {
			BeforeEach(func() {
				mockProposalResponse.Response = nil
				mockEndorserClient.ProcessProposalReturns(mockProposalResponse, nil)
			})

			It("returns an error", func() {
				err := committer.Commit()
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
				err := committer.Commit()
				Expect(err).To(MatchError("proposal failed with status: 500 - capuccino"))
			})
		})

		Context("when the signer fails to sign the transaction", func() {
			BeforeEach(func() {
				mockSigner.SignReturnsOnCall(1, nil, errors.New("peaberry"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to create signed transaction: peaberry"))
				Expect(mockSigner.SignCallCount()).To(Equal(2))
			})
		})

		Context("when the broadcast client fails to send the envelope", func() {
			BeforeEach(func() {
				mockBroadcastClient.SendReturns(errors.New("arabica"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to send transaction: arabica"))
			})
		})

		Context("when the wait for event flag is enabled and the transaction is committed", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 3 * time.Second
				input.TxID = "testtx"
				input.PeerAddresses = []string{"commitpeer0"}
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
				err := committer.Commit()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the wait for event flag is enabled and the client can't connect", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 3 * time.Second
				input.TxID = "testtx"
				input.PeerAddresses = []string{"commitpeer0"}
				mockDeliverClient.DeliverFilteredReturns(nil, errors.New("robusta"))
			})

			It("returns an error", func() {
				err := committer.Commit()
				Expect(err).To(MatchError("failed to connect to deliver on all peers: error connecting to deliver filtered at commitpeer0: robusta"))
			})
		})

		Context("when the wait for event flag is enabled and the transaction isn't returned before the timeout", func() {
			BeforeEach(func() {
				input.WaitForEvent = true
				input.WaitForEventTimeout = 10 * time.Millisecond
				input.TxID = "testtx"
				input.PeerAddresses = []string{"commitpeer0"}
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
				err := committer.Commit()
				Expect(err).To(MatchError("timed out waiting for txid on all peers"))
			})
		})
	})

	Describe("CommitCmd", func() {
		var commitCmd *cobra.Command

		BeforeEach(func() {
			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			Expect(err).To(BeNil())
			commitCmd = chaincode.CommitCmd(nil, cryptoProvider)
			commitCmd.SilenceErrors = true
			commitCmd.SilenceUsage = true
			commitCmd.SetArgs([]string{
				"--channelID=testchannel",
				"--name=testcc",
				"--version=testversion",
				"--sequence=1",
				"--peerAddresses=querypeer1",
				"--tlsRootCertFiles=tls1",
				"--signature-policy=AND ('Org1MSP.member','Org2MSP.member')",
			})
		})

		AfterEach(func() {
			chaincode.ResetFlags()
		})

		It("sets up the committer and attempts to commit the chaincode definition", func() {
			err := commitCmd.Execute()
			Expect(err).To(MatchError(ContainSubstring("failed to retrieve endorser client")))
		})

		Context("when the policy is invalid", func() {
			BeforeEach(func() {
				commitCmd.SetArgs([]string{
					"--signature-policy=notapolicy",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("returns an error", func() {
				err := commitCmd.Execute()
				Expect(err).To(MatchError("invalid signature policy: notapolicy"))
			})
		})

		Context("when the collections config is invalid", func() {
			BeforeEach(func() {
				commitCmd.SetArgs([]string{
					"--collections-config=idontexist.json",
					"--channelID=testchannel",
					"--name=testcc",
					"--version=testversion",
					"--sequence=1",
					"--peerAddresses=querypeer1",
					"--tlsRootCertFiles=tls1",
				})
			})

			It("returns an error", func() {
				err := commitCmd.Execute()
				Expect(err).To(MatchError("invalid collection configuration in file idontexist.json: could not read file 'idontexist.json': open idontexist.json: no such file or directory"))
			})
		})
	})
})
