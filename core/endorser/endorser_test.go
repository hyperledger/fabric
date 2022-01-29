/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"context"
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/fake"
	"github.com/hyperledger/fabric/core/ledger"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

type CcInterest pb.ChaincodeInterest

func (a CcInterest) Len() int { return len(a.Chaincodes) }
func (a CcInterest) Swap(i, j int) {
	a.Chaincodes[i], a.Chaincodes[j] = a.Chaincodes[j], a.Chaincodes[i]
}

func (a CcInterest) Less(i, j int) bool {
	ai := a.Chaincodes[i]
	aj := a.Chaincodes[j]

	if ai.Name != aj.Name {
		return ai.Name < aj.Name
	}

	if len(ai.CollectionNames) != len(aj.CollectionNames) {
		return len(ai.CollectionNames) < len(aj.CollectionNames)
	}

	for ii := range ai.CollectionNames {
		if ai.CollectionNames[ii] != aj.CollectionNames[ii] {
			return ai.CollectionNames[ii] < aj.CollectionNames[ii]
		}
	}

	return false
}

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
		fakeSimulateFailure          *metricsfakes.Counter

		fakeLocalIdentity                *fake.Identity
		fakeLocalMSPIdentityDeserializer *fake.IdentityDeserializer

		fakeChannelIdentity                *fake.Identity
		fakeChannelMSPIdentityDeserializer *fake.IdentityDeserializer

		fakeChannelFetcher *fake.ChannelFetcher

		fakePrivateDataDistributor *fake.PrivateDataDistributor

		fakeSupport              *fake.Support
		fakeTxSimulator          *fake.TxSimulator
		fakeHistoryQueryExecutor *fake.HistoryQueryExecutor

		signedProposal *pb.SignedProposal
		channelID      string
		chaincodeName  string

		chaincodeResponse *pb.Response
		chaincodeEvent    *pb.ChaincodeEvent
		chaincodeInput    *pb.ChaincodeInput

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

		fakeSimulateFailure = &metricsfakes.Counter{}
		fakeSimulateFailure.WithReturns(fakeSimulateFailure)

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
		chaincodeInput = &pb.ChaincodeInput{
			Args: [][]byte{[]byte("arg1"), []byte("arg2"), []byte("arg3")},
		}

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

		fakeSupport.ChaincodeEndorsementInfoReturns(&lifecycle.ChaincodeEndorsementInfo{
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

		fakeHistoryQueryExecutor = &fake.HistoryQueryExecutor{}
		fakeSupport.GetHistoryQueryExecutorReturns(fakeHistoryQueryExecutor, nil)

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
				SimulationFailure:        fakeSimulateFailure,
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
							Input: chaincodeInput,
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
		Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(1))
		ledgerName := fakeSupport.GetHistoryQueryExecutorArgsForCall(0)
		Expect(ledgerName).To(Equal("channel-id"))
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

	It("checks for duplicate transactions", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.GetTransactionByIDCallCount()).To(Equal(1))
		channelID, txid := fakeSupport.GetTransactionByIDArgsForCall(0)
		Expect(channelID).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))
	})

	Context("when the txid is duplicated", func() {
		BeforeEach(func() {
			fakeSupport.GetTransactionByIDReturns(nil, nil)
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).To(MatchError("duplicate transaction found [6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015]. Creator [0a066d73702d6964]"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "duplicate transaction found [6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015]. Creator [0a066d73702d6964]",
				},
			}))
		})
	})

	It("gets a transaction simulator", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(1))
		ledgerName, txid := fakeSupport.GetTxSimulatorArgsForCall(0)
		Expect(ledgerName).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))
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

	It("gets a history query executor", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(1))
		ledgerName := fakeSupport.GetHistoryQueryExecutorArgsForCall(0)
		Expect(ledgerName).To(Equal("channel-id"))
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

	It("gets the channel context", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeChannelFetcher.ChannelCallCount()).To(Equal(1))
		channelID := fakeChannelFetcher.ChannelArgsForCall(0)
		Expect(channelID).To(Equal("channel-id"))
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

	It("checks the submitter's identity", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeChannelMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
		identity := fakeChannelMSPIdentityDeserializer.DeserializeIdentityArgsForCall(0)
		Expect(identity).To(Equal(protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
			Mspid: "msp-id",
		})))

		Expect(fakeLocalMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(0))
	})

	Context("when the proposal is not validly signed", func() {
		BeforeEach(func() {
			fakeChannelMSPIdentityDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("fake-deserialize-error"))
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).To(MatchError("error validating proposal: access denied: channel [channel-id] creator org unknown, creator is malformed"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "error validating proposal: access denied: channel [channel-id] creator org unknown, creator is malformed",
				},
			}))
		})
	})

	It("checks the ACLs for the identity", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeProposalACLCheckFailed.WithCallCount()).To(Equal(0))
		Expect(fakeInitFailed.WithCallCount()).To(Equal(0))
		Expect(fakeEndorsementsFailed.WithCallCount()).To(Equal(0))
		Expect(fakeDuplicateTxsFailure.WithCallCount()).To(Equal(0))
	})

	Context("when the acl check fails", func() {
		BeforeEach(func() {
			fakeSupport.CheckACLReturns(fmt.Errorf("fake-acl-error"))
		})

		It("wraps and returns an error and responds to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).To(MatchError("fake-acl-error"))
			Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
				Response: &pb.Response{
					Status:  500,
					Message: "fake-acl-error",
				},
			}))
		})

		Context("when it's for a system chaincode", func() {
			BeforeEach(func() {
				fakeSupport.IsSysCCReturns(true)
			})

			It("skips the acl check", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response.Status).To(Equal(int32(200)))
			})
		})
	})

	It("gets the chaincode definition", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.ChaincodeEndorsementInfoCallCount()).To(Equal(1))
		channelID, chaincodeName, txSim := fakeSupport.ChaincodeEndorsementInfoArgsForCall(0)
		Expect(channelID).To(Equal("channel-id"))
		Expect(chaincodeName).To(Equal("chaincode-name"))
		Expect(txSim).To(Equal(fakeTxSimulator))
	})

	Context("when the chaincode definition is not found", func() {
		BeforeEach(func() {
			fakeSupport.ChaincodeEndorsementInfoReturns(nil, fmt.Errorf("fake-definition-error"))
		})

		It("returns an error in the response", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "make sure the chaincode chaincode-name has been successfully defined on channel channel-id and try again: fake-definition-error",
			}))
		})
	})

	It("calls the chaincode", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.ExecuteCallCount()).To(Equal(1))
		txParams, chaincodeName, input := fakeSupport.ExecuteArgsForCall(0)
		Expect(txParams.ChannelID).To(Equal("channel-id"))
		Expect(txParams.SignedProp).To(Equal(signedProposal))
		Expect(txParams.TXSimulator).To(Equal(fakeTxSimulator))
		Expect(txParams.HistoryQueryExecutor).To(Equal(fakeHistoryQueryExecutor))
		Expect(chaincodeName).To(Equal("chaincode-name"))
		Expect(proto.Equal(input, &pb.ChaincodeInput{
			Args: [][]byte{[]byte("arg1"), []byte("arg2"), []byte("arg3")},
		})).To(BeTrue())
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
			Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
		})
	})

	It("distributes private data", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakePrivateDataDistributor.DistributePrivateDataCallCount()).To(Equal(1))
		cid, txid, privateData, blkHt := fakePrivateDataDistributor.DistributePrivateDataArgsForCall(0)
		Expect(cid).To(Equal("channel-id"))
		Expect(txid).To(Equal("6f142589e4ef6a1e62c9c816e2074f70baa9f7cf67c2f0c287d4ef907d6d2015"))
		Expect(blkHt).To(Equal(uint64(7)))

		// TODO, this deserves a better test, but there was none before and this logic,
		// really seems far too jumbled to be in the endorser package.  There are separate
		// tests of the private data assembly functions in their test file.
		Expect(privateData).NotTo(BeNil())
		Expect(privateData.EndorsedAt).To(Equal(uint64(7)))
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
			Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
		})
	})

	It("checks the block height", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeSupport.GetLedgerHeightCallCount()).To(Equal(1))
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

			Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
		})
	})

	It("records metrics about the proposal processing", func() {
		_, err := e.ProcessProposal(context.Background(), signedProposal)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeProposalsReceived.AddCallCount()).To(Equal(1))
		Expect(fakeSuccessfulProposals.AddCallCount()).To(Equal(1))
		Expect(fakeProposalValidationFailed.AddCallCount()).To(Equal(0))

		Expect(fakeProposalDuration.WithCallCount()).To(Equal(1))
		Expect(fakeProposalDuration.WithArgsForCall(0)).To(Equal([]string{
			"channel", "channel-id",
			"chaincode", "chaincode-name",
			"success", "true",
		}))
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
		})

		It("does not attempt to get a history query executor", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSupport.GetHistoryQueryExecutorCallCount()).To(Equal(0))
		})

		It("does not attempt to deduplicate the txid", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSupport.GetTransactionByIDCallCount()).To(Equal(0))
		})

		It("does not attempt to get a tx simulator", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeSupport.GetTxSimulatorCallCount()).To(Equal(0))
		})

		It("uses the local MSP to authorize the creator", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeChannelMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(0))

			Expect(fakeLocalMSPIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
			identity := fakeLocalMSPIdentityDeserializer.DeserializeIdentityArgsForCall(0)
			Expect(identity).To(Equal(protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
				Mspid: "msp-id",
			})))
		})

		Context("when the proposal is not validly signed", func() {
			BeforeEach(func() {
				fakeLocalMSPIdentityDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("fake-deserialize-error"))
			})

			It("wraps and returns an error and responds to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.Background(), signedProposal)
				Expect(err).To(MatchError("error validating proposal: access denied: channel [] creator org unknown, creator is malformed"))
				Expect(proposalResponse).To(Equal(&pb.ProposalResponse{
					Response: &pb.Response{
						Status:  500,
						Message: "error validating proposal: access denied: channel [] creator org unknown, creator is malformed",
					},
				}))
			})
		})

		It("records metrics but without a channel ID set", func() {
			_, err := e.ProcessProposal(context.Background(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
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
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling Proposal"))
			Expect(proposalResponse.Response.Status).To(Equal(int32(500)))
			Expect(proposalResponse.Response.Message).To(HavePrefix("error unmarshalling Proposal"))
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

	Context("when the chaincode response is >= 400 but < 500", func() {
		BeforeEach(func() {
			chaincodeResponse.Status = 400
		})

		It("returns the response with no payload", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Payload).To(BeNil())
			Expect(proto.Equal(proposalResponse.Response, &pb.Response{
				Status:  400,
				Payload: []byte("response-payload"),
			})).To(BeTrue())
		})
	})

	Context("when we're in the degenerate legacy lifecycle case", func() {
		BeforeEach(func() {
			chaincodeName = "lscc"
			chaincodeInput.Args = [][]byte{
				[]byte("deploy"),
				nil,
				protoutil.MarshalOrPanic(&pb.ChaincodeDeploymentSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						ChaincodeId: &pb.ChaincodeID{
							Name:    "deploy-name",
							Version: "deploy-version",
						},
						Input: &pb.ChaincodeInput{
							Args: [][]byte{[]byte("target-arg")},
						},
					},
				}),
			}

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: &rwset.TxReadWriteSet{},
					// We don't return private data in this case because lscc forbids it
				},
				nil,
			)
		})

		It("triggers the legacy init, and returns the response from lscc", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(proposalResponse.Response, &pb.Response{
				Status:  200,
				Payload: []byte("response-payload"),
			})).To(BeTrue())

			Expect(fakeSupport.ExecuteLegacyInitCallCount()).To(Equal(1))
			_, name, version, input := fakeSupport.ExecuteLegacyInitArgsForCall(0)
			Expect(name).To(Equal("deploy-name"))
			Expect(version).To(Equal("deploy-version"))
			Expect(input.Args).To(Equal([][]byte{[]byte("target-arg")}))
		})

		Context("when the chaincode spec contains a code package", func() {
			BeforeEach(func() {
				chaincodeInput.Args = [][]byte{
					[]byte("deploy"),
					nil,
					protoutil.MarshalOrPanic(&pb.ChaincodeDeploymentSpec{
						ChaincodeSpec: &pb.ChaincodeSpec{
							ChaincodeId: &pb.ChaincodeID{
								Name:    "deploy-name",
								Version: "deploy-version",
							},
							Input: &pb.ChaincodeInput{
								Args: [][]byte{[]byte("target-arg")},
							},
						},
						CodePackage: []byte("some-code"),
					}),
				}
			})

			It("returns an error to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: lscc upgrade/deploy should not include a code packages",
				}))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})

		Context("when the simulation uses private data", func() {
			BeforeEach(func() {
				fakeTxSimulator.GetTxSimulationResultsReturns(
					&ledger.TxSimulationResults{
						PubSimulationResults: &rwset.TxReadWriteSet{},
						PvtSimulationResults: &rwset.TxPvtReadWriteSet{},
					},
					nil,
				)
			})

			It("returns an error to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: Private data is forbidden to be used in instantiate",
				}))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})

		Context("when retrieving simulation results", func() {
			BeforeEach(func() {
				fakeTxSimulator.GetTxSimulationResultsReturns(
					&ledger.TxSimulationResults{
						PubSimulationResults: &rwset.TxReadWriteSet{},
						PvtSimulationResults: &rwset.TxPvtReadWriteSet{},
					},
					errors.New("bad simulation"),
				)
			})

			It("returns an error to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: bad simulation",
				}))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})

		Context("when retrieving public simulation results fails", func() {
			BeforeEach(func() {
				fakeTxSimulator.GetTxSimulationResultsReturns(
					&ledger.TxSimulationResults{},
					nil,
				)
			})

			It("returns an error to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: proto: Marshal called with nil",
				}))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})

		Context("when the init fails", func() {
			BeforeEach(func() {
				fakeSupport.ExecuteLegacyInitReturns(nil, nil, fmt.Errorf("fake-legacy-init-error"))
			})

			It("returns an error and increments the metric", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: fake-legacy-init-error",
				}))

				Expect(fakeInitFailed.WithCallCount()).To(Equal(1))
				Expect(fakeInitFailed.WithArgsForCall(0)).To(Equal([]string{
					"channel", "channel-id",
					"chaincode", "deploy-name",
				}))
				Expect(fakeInitFailed.AddCallCount()).To(Equal(1))
			})
		})

		Context("when the deploying chaincode is the name of a builtin system chaincode", func() {
			BeforeEach(func() {
				fakeSupport.IsSysCCStub = func(name string) bool {
					return name == "deploy-name"
				}
			})

			It("triggers the legacy init, and returns the response from lscc", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response).To(Equal(&pb.Response{
					Status:  500,
					Message: "error in simulation: attempting to deploy a system chaincode deploy-name/channel-id",
				}))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})

		Context("when unmarshalling UnmarshalChaincodeDeploymentSpec fails", func() {
			BeforeEach(func() {
				chaincodeInput = &pb.ChaincodeInput{
					Args: [][]byte{[]byte("deploy"), nil, []byte("arg3")},
				}
			})

			It("returns an error to the client", func() {
				proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
				Expect(err).NotTo(HaveOccurred())
				Expect(proposalResponse.Response.Status).To(Equal(int32(500)))
				Expect(proposalResponse.Response.Message).To(ContainSubstring("error in simulation: error unmarshalling ChaincodeDeploymentSpec"))
				Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
			})
		})
	})

	Context("when retrieving simulation results", func() {
		BeforeEach(func() {
			mockDeployedCCInfoProvider := &ledgermock.DeployedChaincodeInfoProvider{}
			fakeSupport.GetDeployedCCInfoProviderReturns(mockDeployedCCInfoProvider)

			pvtSimResults := &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "myCC",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "mycollection-1",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7, 8},
							},
						},
					},
				},
			}

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PvtSimulationResults: pvtSimResults,
				},
				nil,
			)
		})

		It("returns an error to the client", func() {
			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			Expect(proposalResponse.Response).To(Equal(&pb.Response{
				Status:  500,
				Message: "error in simulation: failed to obtain collections config: no collection config for chaincode \"myCC\"",
			}))
			Expect(fakeSimulateFailure.AddCallCount()).To(Equal(1))
		})
	})

	Context("when building the ChaincodeInterest", func() {
		var pvtSimResults *rwset.TxPvtReadWriteSet
		var pubSimResults *rwset.TxReadWriteSet
		var readWrites []byte
		var readSet []byte

		BeforeEach(func() {
			ccPkg := &pb.CollectionConfigPackage{
				Config: []*pb.CollectionConfig{
					{
						Payload: &pb.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &pb.StaticCollectionConfig{
								Name: "myCC",
								MemberOrgsPolicy: &pb.CollectionPolicyConfig{
									Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
										SignaturePolicy: &cb.SignaturePolicyEnvelope{},
									},
								},
							},
						},
					},
				},
			}
			mockDeployedCCInfoProvider := &ledgermock.DeployedChaincodeInfoProvider{}
			mockDeployedCCInfoProvider.AllCollectionsConfigPkgReturns(ccPkg, nil)
			fakeSupport.GetDeployedCCInfoProviderReturns(mockDeployedCCInfoProvider)

			var err error
			readWrites, err = proto.Marshal(
				&kvrwset.KVRWSet{
					Writes: []*kvrwset.KVWrite{
						{Key: "myKey", Value: []byte("myValue")},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())
			readSet, err = proto.Marshal(
				&kvrwset.KVRWSet{
					Reads: []*kvrwset.KVRead{
						{Key: "myKey"},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())

			pubSimResults = &rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "myCC",
						Rwset:     readWrites,
					},
				},
			}

			pvtSimResults = &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "myCC",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "mycollection-1",
								Rwset:          []byte("private RW set"),
							},
						},
					},
				},
			}
		})

		It("add private collection which gets read", func() {
			privateReads := ledger.PrivateReads{}
			privateReads.Add("myCC", "mycollection-1")
			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "mycollection-1", "mykey", nil)

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: &rwset.TxReadWriteSet{},
					PvtSimulationResults: pvtSimResults,
					PrivateReads:         privateReads,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			sort.Sort(CcInterest(*proposalResponse.Interest))
			Expect(proposalResponse.Interest).To(Equal(&pb.ChaincodeInterest{
				Chaincodes: []*pb.ChaincodeCall{{
					Name:            "myCC",
					CollectionNames: []string{"mycollection-1"},
				}},
			}))
		})

		It("add private collection which gets read, but not written", func() {
			privateReads := ledger.PrivateReads{}
			privateReads.Add("myCC", "mycollection-1")
			writesetMetadata := ledger.WritesetMetadata{}

			// a private read will also have an entry in public hashed RWset
			pubSimResults = &rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace:             "myCC",
						CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{{CollectionName: "mycollection-1"}},
					},
				},
			}

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: pubSimResults,
					PvtSimulationResults: &rwset.TxPvtReadWriteSet{},
					PrivateReads:         privateReads,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			sort.Sort(CcInterest(*proposalResponse.Interest))
			Expect(proposalResponse.Interest).To(Equal(&pb.ChaincodeInterest{
				Chaincodes: []*pb.ChaincodeCall{
					{
						Name: "myCC",
					},
					{
						Name:            "myCC",
						CollectionNames: []string{"mycollection-1"},
					},
				},
			}))
		})

		It("add private collection which is not read", func() {
			privateReads := ledger.PrivateReads{}
			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "mycollection-1", "mykey", nil)
			pubSimResults = &rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "myCC",
						Rwset:     readSet,
					},
				},
			}

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: pubSimResults,
					PvtSimulationResults: pvtSimResults,
					PrivateReads:         privateReads,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())
			sort.Sort(CcInterest(*proposalResponse.Interest))
			Expect(proposalResponse.Interest).To(Equal(&pb.ChaincodeInterest{
				Chaincodes: []*pb.ChaincodeCall{{
					Name:            "myCC",
					CollectionNames: []string{"mycollection-1"},
					NoPrivateReads:  true,
				}},
			}))
		})

		It("add private collection and SBE", func() {
			privateReads := ledger.PrivateReads{}

			sbe := &cb.SignaturePolicyEnvelope{
				Rule: &cb.SignaturePolicy{
					Type: &cb.SignaturePolicy_SignedBy{SignedBy: 0},
				},
			}
			sbeBytes, err := proto.Marshal(sbe)
			Expect(err).NotTo(HaveOccurred())
			metadata := map[string][]byte{pb.MetaDataKeys_VALIDATION_PARAMETER.String(): sbeBytes}

			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "mycollection-1", "mykey1", nil)
			writesetMetadata.Add("myCC", "mycollection-1", "mykey2", metadata)

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: &rwset.TxReadWriteSet{},
					PvtSimulationResults: pvtSimResults,
					PrivateReads:         privateReads,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())

			sort.Sort(CcInterest(*proposalResponse.Interest))
			Expect(proto.Equal(
				proposalResponse.Interest,
				&pb.ChaincodeInterest{
					Chaincodes: []*pb.ChaincodeCall{
						{
							Name:                     "myCC",
							KeyPolicies:              []*cb.SignaturePolicyEnvelope{sbe},
							DisregardNamespacePolicy: true,
						},
						{
							Name:            "myCC",
							CollectionNames: []string{"mycollection-1"},
							NoPrivateReads:  true,
						},
					},
				},
			)).To(BeTrue())
		})

		It("SBE only, no chaincode policy updates", func() {
			sbe := &cb.SignaturePolicyEnvelope{
				Rule: &cb.SignaturePolicy{
					Type: &cb.SignaturePolicy_SignedBy{SignedBy: 0},
				},
			}
			sbeBytes, err := proto.Marshal(sbe)
			Expect(err).NotTo(HaveOccurred())
			metadata := map[string][]byte{pb.MetaDataKeys_VALIDATION_PARAMETER.String(): sbeBytes}

			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "", "myKey", metadata)

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: pubSimResults,
					PvtSimulationResults: pvtSimResults,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())

			sort.Sort(CcInterest(*proposalResponse.Interest))
			Expect(proto.Equal(
				proposalResponse.Interest,
				&pb.ChaincodeInterest{
					Chaincodes: []*pb.ChaincodeCall{{
						Name:                     "myCC",
						KeyPolicies:              []*cb.SignaturePolicyEnvelope{sbe},
						DisregardNamespacePolicy: true,
					}},
				},
			)).To(BeTrue())
		})

		It("chaincode to chaincode calls", func() {
			cc3ccSimResults := &rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "myCC",
						Rwset:     readWrites,
					},
					{
						Namespace: "otherCC",
						Rwset:     readWrites,
					},
				},
			}

			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "", "mykey", nil)
			writesetMetadata.Add("otherCC", "", "mykey", nil)

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: cc3ccSimResults,
					PvtSimulationResults: &rwset.TxPvtReadWriteSet{},
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())

			Expect(proposalResponse.Interest.Chaincodes).To(ContainElements([]*pb.ChaincodeCall{
				{Name: "myCC"},
				{Name: "otherCC"},
			}))
		})

		It("ignores system chaincodes", func() {
			fakeSupport.IsSysCCStub = func(cc string) bool {
				return cc == "_lifecycle"
			}
			pubSimResults = &rwset.TxReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "myCC",
						Rwset:     readWrites,
					},
					{
						Namespace: "_lifecycle",
						Rwset:     readWrites,
					},
				},
			}

			pvtSimResults = &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "myCC",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "mycollection-1",
								Rwset:          []byte("private RW set"),
							},
						},
					},
					{
						Namespace: "_lifecycle",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "mycollection-2",
								Rwset:          []byte("should be ignored 2"),
							},
						},
					},
				},
			}
			privateReads := ledger.PrivateReads{}
			privateReads.Add("myCC", "mycollection-1")
			privateReads.Add("_lifecycle", "mycollection-1")
			writesetMetadata := ledger.WritesetMetadata{}
			writesetMetadata.Add("myCC", "mycollection-1", "mykey", nil)
			writesetMetadata.Add("_lifecycle", "mycollection-2", "mykey", nil)

			fakeTxSimulator.GetTxSimulationResultsReturns(
				&ledger.TxSimulationResults{
					PubSimulationResults: pubSimResults,
					PvtSimulationResults: pvtSimResults,
					PrivateReads:         privateReads,
					WritesetMetadata:     writesetMetadata,
				},
				nil,
			)

			proposalResponse, err := e.ProcessProposal(context.TODO(), signedProposal)
			Expect(err).NotTo(HaveOccurred())

			Expect(proposalResponse.Interest.Chaincodes).To(ContainElements([]*pb.ChaincodeCall{
				{Name: "myCC", CollectionNames: []string{"mycollection-1"}},
			}))
		})
	})
})
