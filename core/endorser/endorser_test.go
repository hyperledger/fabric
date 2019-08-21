/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/fake"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	mspproto "github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/golang/protobuf/proto"
)

var _ = Describe("Endorser", func() {
	var (
		fakeProposalDuration         *metricsfakes.Histogram
		fakeProposalsReceived        *metricsfakes.Counter
		fakeSuccessfulProposals      *metricsfakes.Counter
		fakeProposalValidationFailed *metricsfakes.Counter
		fakeProposalACLCheckFailed   *metricsfakes.Counter
		fakeInitFailed               *metricsfakes.Counter
		fakeEndorsementsFailed       *metricsfakes.Counter
		fakeDuplicateTxsFailure      *metricsfakes.Counter

		fakeLocalIdentity                *fake.Identity
		fakeLocalMSPIdentityDeserializer *fake.IdentityDeserializer

		fakeChannelIdentity                *fake.Identity
		fakeChannelMSPIdentityDeserializer *fake.IdentityDeserializer

		fakeChannelFetcher *fake.ChannelFetcher

		fakePrivateDataDistributor *fake.PrivateDataDistributor

		fakeSupport     *fake.Support
		fakeTxSimulator *fake.TxSimulator

		signedProposal *pb.SignedProposal
		channelID      string
		chaincodeName  string

		chaincodeResponse *pb.Response
		chaincodeEvent    *pb.ChaincodeEvent

		e *endorser.Endorser
	)

	BeforeEach(func() {
		fakeProposalDuration = &metricsfakes.Histogram{}
		fakeProposalDuration.WithReturns(fakeProposalDuration)

		fakeProposalACLCheckFailed = &metricsfakes.Counter{}
		fakeProposalACLCheckFailed.WithReturns(fakeProposalACLCheckFailed)

		fakeInitFailed = &metricsfakes.Counter{}
		fakeInitFailed.WithReturns(fakeInitFailed)

		fakeEndorsementsFailed = &metricsfakes.Counter{}
		fakeEndorsementsFailed.WithReturns(fakeEndorsementsFailed)

		fakeDuplicateTxsFailure = &metricsfakes.Counter{}
		fakeDuplicateTxsFailure.WithReturns(fakeDuplicateTxsFailure)

		fakeProposalsReceived = &metricsfakes.Counter{}
		fakeSuccessfulProposals = &metricsfakes.Counter{}
		fakeProposalValidationFailed = &metricsfakes.Counter{}

		fakeLocalIdentity = &fake.Identity{}
		fakeLocalMSPIdentityDeserializer = &fake.IdentityDeserializer{}
		fakeLocalMSPIdentityDeserializer.DeserializeIdentityReturns(fakeLocalIdentity, nil)

		fakeChannelIdentity = &fake.Identity{}
		fakeChannelMSPIdentityDeserializer = &fake.IdentityDeserializer{}
		fakeChannelMSPIdentityDeserializer.DeserializeIdentityReturns(fakeChannelIdentity, nil)

		fakeChannelFetcher = &fake.ChannelFetcher{}
		fakeChannelFetcher.ChannelReturns(&endorser.Channel{
			IdentityDeserializer: fakeChannelMSPIdentityDeserializer,
		})

		fakePrivateDataDistributor = &fake.PrivateDataDistributor{}

		channelID = "channel-id"
		chaincodeName = "chaincode-name"

		chaincodeResponse = &pb.Response{
			Status:  200,
			Payload: []byte("response-payload"),
		}
		chaincodeEvent = &pb.ChaincodeEvent{
			ChaincodeId: "chaincode-id",
			TxId:        "event-txid",
			EventName:   "event-name",
			Payload:     []byte("event-payload"),
		}

		fakeSupport = &fake.Support{}
		fakeSupport.ExecuteReturns(
			chaincodeResponse,
			chaincodeEvent,
			nil,
		)

		fakeSupport.GetChaincodeDefinitionReturns(&lifecycle.LegacyDefinition{
			Version:           "chaincode-definition-version",
			EndorsementPlugin: "plugin-name",
		}, nil)

		fakeSupport.GetLedgerHeightReturns(7, nil)

		fakeSupport.EndorseWithPluginReturns(
			&pb.Endorsement{
				Endorser:  []byte("endorser-identity"),
				Signature: []byte("endorser-signature"),
			},
			[]byte("endorser-modified-payload"),
			nil,
		)

		fakeTxSimulator = &fake.TxSimulator{}
		fakeTxSimulator.GetTxSimulationResultsReturns(
			&ledger.TxSimulationResults{
				PubSimulationResults: &rwset.TxReadWriteSet{},
				PvtSimulationResults: &rwset.TxPvtReadWriteSet{},
			},
			nil,
		)

		fakeSupport.GetTxSimulatorReturns(fakeTxSimulator, nil)

		fakeSupport.GetTransactionByIDReturns(nil, fmt.Errorf("txid-error"))

		e = &endorser.Endorser{
			LocalMSP:               fakeLocalMSPIdentityDeserializer,
			PrivateDataDistributor: fakePrivateDataDistributor,
			Metrics: &endorser.Metrics{
				ProposalDuration:         fakeProposalDuration,
				ProposalsReceived:        fakeProposalsReceived,
				SuccessfulProposals:      fakeSuccessfulProposals,
				ProposalValidationFailed: fakeProposalValidationFailed,
				ProposalACLCheckFailed:   fakeProposalACLCheckFailed,
				InitFailed:               fakeInitFailed,
				EndorsementsFailed:       fakeEndorsementsFailed,
				DuplicateTxsFailure:      fakeDuplicateTxsFailure,
			},
			Support:        fakeSupport,
			ChannelFetcher: fakeChannelFetcher,
		}
	})

	JustBeforeEach(func() {
		signedProposal = &pb.SignedProposal{
			ProposalBytes: protoutil.MarshalOrPanic(&pb.Proposal{
				Header: protoutil.MarshalOrPanic(&cb.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
						Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
						ChannelId: channelID,
						Extension: protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{
							ChaincodeId: &pb.ChaincodeID{
								Name: chaincodeName,
							},
						}),
						TxId: "6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015",
					}),
					SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{
						Creator: protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
							Mspid: "msp-id",
						}),
						Nonce: []byte("nonce"),
					}),
				}),
				Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
					Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
						ChaincodeSpec: &pb.ChaincodeSpec{
							Input: &pb.ChaincodeInput{
								Args: [][]byte{[]byte("arg1"), []byte("arg2"), []byte("arg3")},
							},
						},
					}),
				}),
			}),
			Signature: []byte("signature"),
		}
	})

	It("successfully endorses the proposal", func() {
		proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(proposalResponse.Endorsement).To(Equal(&pb.Endorsement{
			Endorser:  []byte("endorser-identity"),
			Signature: []byte("endorser-signature"),
		}))
		Expect(proposalResponse.Timestamp).To(BeNil())
		Expect(proposalResponse.Version).To(Equal(int32(1)))
		Expect(proposalResponse.Payload).To(Equal([]byte("endorser-modified-payload")))
		Expect(proto.Equal(proposalResponse.Response, &pb.Response{
			Status:  200,
			Payload: []byte("response-payload"),
		})).To(BeTrue())

		Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(1))
		ledgerName := fakeSupport.GetHistoryQueryExecutorArgsForCall(0)
		Expect(ledgerName).To(Equal("channel-id"))

		Expect(fakeSupport.GetTransactionByIDCallCount()).To(Equal(1))
		channelID, txid := fakeSupport.GetTransactionByIDArgsForCall(0)
		Expect(channelID).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))

		Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(1))
		ledgerName, txid = fakeSupport.GetTxSimulatorArgsForCall(0)
		Expect(ledgerName).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))

		Expect(fakeChannelMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
		identity := fakeChannelMSPIdentityDeserializer.DeserializeIdentityArgsForCall(0)
		Expect(identity).To(Equal(protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
			Mspid: "msp-id",
		})))

		Expect(fakeLocalMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(0))

		Expect(fakePrivateDataDistributor.DistributePrivateDataCallCount()).To(Equal(1))
		cid, txid, privateData, blkHt := fakePrivateDataDistributor.DistributePrivateDataArgsForCall(0)
		Expect(cid).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))
		Expect(blkHt).To(Equal(uint64(7)))

		// TODO, this deserves a better test, but there was none before and this logic,
		// really seems far too jumbled to be in the endorser package.  There are seperate
		// tests of the private data assembly functions in their test file.
		Expect(privateData).NotTo(BeNil())
		Expect(privateData.EndorsedAt).To(Equal(uint64(7)))

		Expect(fakeSupport.EndorseWithPluginCallCount()).To(Equal(1))
		pluginName, cid, propRespPayloadBytes, sp := fakeSupport.EndorseWithPluginArgsForCall(0)
		Expect(sp).To(Equal(signedProposal))
		Expect(pluginName).To(Equal("plugin-name"))
		Expect(cid).To(Equal("channel-id"))

		prp := &pb.ProposalResponsePayload{}
		err = proto.Unmarshal(propRespPayloadBytes, prp)
		Expect(err).NotTo(HaveOccurred())
		Expect(fmt.Sprintf("%x", prp.ProposalHash)).To(Equal("6fa450b00ebef6c7de9f3479148f6d6ff2c645762e17fcaae989ff7b668be001"))

		ccAct := &pb.ChaincodeAction{}
		err = proto.Unmarshal(prp.Extension, ccAct)
		Expect(err).NotTo(HaveOccurred())
		Expect(ccAct.Events).To(Equal(protoutil.MarshalOrPanic(chaincodeEvent)))
		Expect(proto.Equal(ccAct.Response, &pb.Response{
			Status:  200,
			Payload: []byte("response-payload"),
		})).To(BeTrue())

		Expect(fakeProposalsReceived.AddCallCount()).To(Equal(1))
		Expect(fakeSuccessfulProposals.AddCallCount()).To(Equal(1))
		Expect(fakeProposalValidationFailed.AddCallCount()).To(Equal(0))

		Expect(fakeProposalDuration.WithCallCount()).To(Equal(1))
		Expect(fakeProposalDuration.WithArgsForCall(0)).To(Equal([]string{
			"channel", "channel-id",
			"chaincode", "chaincode-name",
			"success", "true",
		}))

		Expect(fakeProposalACLCheckFailed.WithCallCount()).To(Equal(0))
		Expect(fakeInitFailed.WithCallCount()).To(Equal(0))
		Expect(fakeEndorsementsFailed.WithCallCount()).To(Equal(0))
		Expect(fakeDuplicateTxsFailure.WithCallCount()).To(Equal(0))
	})

	Context("when the channel id is empty", func() {
		BeforeEach(func() {
			channelID = ""
		})

		It("returns a successful proposal response with no endorsement", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Endorsement).To(BeNil())
			Expect(proposalResponse.Timestamp).To(BeNil())
			Expect(proposalResponse.Version).To(Equal(int32(0)))
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proto.Equal(proposalResponse.Response, &pb.Response{
				Status:  200,
				Payload: []byte("response-payload"),
			})).To(BeTrue())

			Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(0))
			Expect(fakeSupport.GetTransactionByIDCallCount()).To(Equal(0))
			Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(0))
			Expect(fakeChannelMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(0))

			Expect(fakeLocalMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
			identity := fakeLocalMSPIdentityDeserializer.DeserializeIdentityArgsForCall(0)
			Expect(identity).To(Equal(protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
				Mspid: "msp-id",
			})))

			Expect(fakeProposalsReceived.AddCallCount()).To(Equal(1))
			Expect(fakeSuccessfulProposals.AddCallCount()).To(Equal(1))
			Expect(fakeProposalValidationFailed.AddCallCount()).To(Equal(0))

			Expect(fakeProposalDuration.WithCallCount()).To(Equal(1))
			Expect(fakeProposalDuration.WithArgsForCall(0)).To(Equal([]string{
				"channel", "",
				"chaincode", "chaincode-name",
				"success", "true",
			}))

			Expect(fakeProposalACLCheckFailed.WithCallCount()).To(Equal(0))
			Expect(fakeInitFailed.WithCallCount()).To(Equal(0))
			Expect(fakeEndorsementsFailed.WithCallCount()).To(Equal(0))
			Expect(fakeDuplicateTxsFailure.WithCallCount()).To(Equal(0))
		})

		Context("when the chaincode response is >= 500", func() {
			BeforeEach(func() {
				chaincodeResponse.Status = 500
			})

			It("returns the result, but with the proposal encoded, and no endorsements", func() {
				proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Endorsement).To(BeNil())
				Expect(proposalResponse.Timestamp).To(BeNil())
				Expect(proposalResponse.Version).To(Equal(int32(0)))
				Expect(proto.Equal(proposalResponse.Response, &pb.Response{
					Status:  500,
					Payload: []byte("response-payload"),
				})).To(BeTrue())

				// This is almost definitely a bug, but, adding a test in case someone is relying on this behavior.
				// When the response is >= 500, we return a payload, but not on success.  A payload is only meaningful
				// if it is endorsed, so it's unclear why we're returning it here.
				prp := &pb.ProposalResponsePayload{}
				err = proto.Unmarshal(proposalResponse.Payload, prp)
				Expect(err).NotTo(HaveOccurred())
				Expect(fmt.Sprintf("%x", prp.ProposalHash)).To(Equal("f2c27f04f897dc28fd1b2983e7b22ebc8fbbb3d0617c140d913b33e463886788"))

				ccAct := &pb.ChaincodeAction{}
				err = proto.Unmarshal(prp.Extension, ccAct)
				Expect(err).NotTo(HaveOccurred())
				Expect(proto.Equal(ccAct.Response, &pb.Response{
					Status:  500,
					Payload: []byte("response-payload"),
				})).To(BeTrue())

				// This is an especially weird bit of the behavior, the chaincode event is nil-ed before creating
				// the proposal response. (That probably shouldn't be created)
				Expect(ccAct.Events).To(BeNil())
			})
		})

		Context("when the 200 < chaincode response < 500", func() {
			BeforeEach(func() {
				chaincodeResponse.Status = 499
			})

			It("returns the result, but with the proposal encoded, and no endorsements", func() {
				proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Endorsement).To(BeNil())
				Expect(proposalResponse.Timestamp).To(BeNil())
				Expect(proposalResponse.Version).To(Equal(int32(0)))
				Expect(proto.Equal(proposalResponse.Response, &pb.Response{
					Status:  499,
					Payload: []byte("response-payload"),
				})).To(BeTrue())
				Expect(proposalResponse.Payload).To(BeNil())
			})
		})
	})

	Context("when the proposal is malformed", func() {
		JustBeforeEach(func() {
			signedProposal = &pb.SignedProposal{
				ProposalBytes: []byte("garbage"),
			}
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).To(MatchError("error unmarshaling Proposal: proto: can't skip unknown wire type 7"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "error unmarshaling Proposal: proto: can't skip unknown wire type 7",
				},
			}))
		})
	})

	Context("when the proposal is not validly signed", func() {
		BeforeEach(func() {
			fakeChannelMSPIdentityDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("fake-deserialize-error"))
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).To(MatchError("access denied: channel [channel-id] creator org [msp-id]"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "access denied: channel [channel-id] creator org [msp-id]",
				},
			}))
		})
	})

	Context("when the proposal is not validly signed", func() {
		BeforeEach(func() {
			fakeChannelMSPIdentityDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("fake-deserialize-error"))
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).To(MatchError("access denied: channel [channel-id] creator org [msp-id]"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "access denied: channel [channel-id] creator org [msp-id]",
				},
			}))
		})
	})

	Context("when the chaincode response is >= 500", func() {
		BeforeEach(func() {
			chaincodeResponse.Status = 500
		})

		It("returns the result, but with the proposal encoded, and no endorsements", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Endorsement).To(BeNil())
			Expect(proposalResponse.Timestamp).To(BeNil())
			Expect(proposalResponse.Version).To(Equal(int32(0)))
			Expect(proto.Equal(proposalResponse.Response, &pb.Response{
				Status:  500,
				Payload: []byte("response-payload"),
			})).To(BeTrue())
			Expect(proposalResponse.Payload).NotTo(BeNil())
		})
	})

	Context("when the chaincode endorsement fails", func() {
		BeforeEach(func() {
			fakeSupport.EndorseWithPluginReturns(nil, nil, fmt.Errorf("fake-endorserment-error"))
		})

		It("returns the error, but with no payload encoded", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "endorsing with plugin failed: fake-endorserment-error",
			}))
		})
	})

	Context("when getting the tx simulator fails", func() {
		BeforeEach(func() {
			fakeSupport.GetTxSimulatorReturns(nil, fmt.Errorf("fake-simulator-error"))
		})

		It("returns a response with the error", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "fake-simulator-error",
			}))
		})
	})

	Context("when getting the history query executor fails", func() {
		BeforeEach(func() {
			fakeSupport.GetHistoryQueryExecutorReturns(nil, fmt.Errorf("fake-history-error"))
		})

		It("returns a response with the error", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "fake-history-error",
			}))
		})
	})

	Context("when the channel context cannot be retrieved", func() {
		BeforeEach(func() {
			fakeChannelFetcher.ChannelReturns(nil)
		})

		It("returns a response with the error", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "channel 'channel-id' not found",
			}))
		})
	})

	Context("when the chaincode name is qscc", func() {
		BeforeEach(func() {
			chaincodeName = "qscc"
		})

		It("skips fetching the tx simulator and history query exucutor", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(0))
			Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(0))
		})
	})

	Context("when the chaincode name is cscc", func() {
		BeforeEach(func() {
			chaincodeName = "cscc"
		})

		It("skips fetching the tx simulator and history query exucutor", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(0))
			Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(0))
		})
	})

	Context("when calling the chaincode returns an error", func() {
		BeforeEach(func() {
			fakeSupport.ExecuteReturns(nil, nil, fmt.Errorf("fake-chaincode-execution-error"))
		})

		It("returns a response with the error and no payload", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "error in simulation: fake-chaincode-execution-error",
			}))
		})
	})

	Context("when the private data cannot be distributed", func() {
		BeforeEach(func() {
			fakePrivateDataDistributor.DistributePrivateDataReturns(fmt.Errorf("fake-private-data-error"))
		})

		It("returns a response with the error and no payload", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "error in simulation: fake-private-data-error",
			}))
		})
	})

	Context("when the block height cannot be determined", func() {
		BeforeEach(func() {
			fakeSupport.GetLedgerHeightReturns(0, fmt.Errorf("fake-block-height-error"))
		})

		It("returns a response with the error and no payload", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "error in simulation: failed to obtain ledger height for channel 'channel-id': fake-block-height-error",
			}))
		})
	})
})
