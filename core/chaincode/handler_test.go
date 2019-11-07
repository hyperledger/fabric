/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"io"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/fake"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Handler", func() {
	var (
		fakeTransactionRegistry        *mock.TransactionRegistry
		fakeContextRegistry            *fake.ContextRegistry
		fakeChatStream                 *mock.ChaincodeStream
		fakeSystemCCProvider           *mock.SystemCCProvider
		fakeTxSimulator                *mock.TxSimulator
		fakeHistoryQueryExecutor       *mock.HistoryQueryExecutor
		fakeQueryResponseBuilder       *fake.QueryResponseBuilder
		fakeACLProvider                *mock.ACLProvider
		fakeDefinitionGetter           *mock.ChaincodeDefinitionGetter
		fakeInstantiationPolicyChecker *mock.InstantiationPolicyChecker
		fakeInvoker                    *mock.Invoker
		fakeLedgerGetter               *mock.LedgerGetter
		fakeHandlerRegistry            *fake.Registry
		fakeApplicationConfigRetriever *fake.ApplicationConfigRetriever
		fakeCollectionStore            *mock.CollectionStore
		fakeShimRequestsReceived       *metricsfakes.Counter
		fakeShimRequestsCompleted      *metricsfakes.Counter
		fakeShimRequestDuration        *metricsfakes.Histogram
		fakeExecuteTimeouts            *metricsfakes.Counter

		responseNotifier chan *pb.ChaincodeMessage
		txContext        *chaincode.TransactionContext

		handler *chaincode.Handler
	)

	BeforeEach(func() {
		fakeTransactionRegistry = &mock.TransactionRegistry{}
		fakeTransactionRegistry.AddReturns(true)

		fakeChatStream = &mock.ChaincodeStream{}
		fakeSystemCCProvider = &mock.SystemCCProvider{}

		fakeTxSimulator = &mock.TxSimulator{}
		fakeHistoryQueryExecutor = &mock.HistoryQueryExecutor{}
		fakeCollectionStore = &mock.CollectionStore{}

		responseNotifier = make(chan *pb.ChaincodeMessage, 1)
		txContext = &chaincode.TransactionContext{
			ChainID:                 "channel-id",
			TXSimulator:             fakeTxSimulator,
			HistoryQueryExecutor:    fakeHistoryQueryExecutor,
			ResponseNotifier:        responseNotifier,
			CollectionStore:         fakeCollectionStore,
			AllowedCollectionAccess: make(map[string]bool),
		}

		fakeACLProvider = &mock.ACLProvider{}
		fakeDefinitionGetter = &mock.ChaincodeDefinitionGetter{}
		fakeInvoker = &mock.Invoker{}
		fakeLedgerGetter = &mock.LedgerGetter{}
		fakeInstantiationPolicyChecker = &mock.InstantiationPolicyChecker{}
		fakeQueryResponseBuilder = &fake.QueryResponseBuilder{}
		fakeHandlerRegistry = &fake.Registry{}

		fakeContextRegistry = &fake.ContextRegistry{}
		fakeContextRegistry.GetReturns(txContext)
		fakeContextRegistry.CreateReturns(txContext, nil)

		fakeApplicationConfigRetriever = &fake.ApplicationConfigRetriever{}
		applicationCapability := &config.MockApplication{
			CapabilitiesRv: &config.MockApplicationCapabilities{KeyLevelEndorsementRv: true},
		}
		fakeApplicationConfigRetriever.GetApplicationConfigReturns(applicationCapability, true)

		fakeShimRequestsReceived = &metricsfakes.Counter{}
		fakeShimRequestsReceived.WithReturns(fakeShimRequestsReceived)
		fakeShimRequestsCompleted = &metricsfakes.Counter{}
		fakeShimRequestsCompleted.WithReturns(fakeShimRequestsCompleted)
		fakeShimRequestDuration = &metricsfakes.Histogram{}
		fakeShimRequestDuration.WithReturns(fakeShimRequestDuration)
		fakeExecuteTimeouts = &metricsfakes.Counter{}
		fakeExecuteTimeouts.WithReturns(fakeExecuteTimeouts)

		chaincodeMetrics := &chaincode.HandlerMetrics{
			ShimRequestsReceived:  fakeShimRequestsReceived,
			ShimRequestsCompleted: fakeShimRequestsCompleted,
			ShimRequestDuration:   fakeShimRequestDuration,
			ExecuteTimeouts:       fakeExecuteTimeouts,
		}

		handler = &chaincode.Handler{
			ACLProvider:                fakeACLProvider,
			ActiveTransactions:         fakeTransactionRegistry,
			DefinitionGetter:           fakeDefinitionGetter,
			Invoker:                    fakeInvoker,
			LedgerGetter:               fakeLedgerGetter,
			InstantiationPolicyChecker: fakeInstantiationPolicyChecker,
			QueryResponseBuilder:       fakeQueryResponseBuilder,
			Registry:                   fakeHandlerRegistry,
			SystemCCProvider:           fakeSystemCCProvider,
			SystemCCVersion:            "system-cc-version",
			TXContexts:                 fakeContextRegistry,
			UUIDGenerator: chaincode.UUIDGeneratorFunc(func() string {
				return "generated-query-id"
			}),
			AppConfig: fakeApplicationConfigRetriever,
			Metrics:   chaincodeMetrics,
		}
		chaincode.SetHandlerChatStream(handler, fakeChatStream)
		chaincode.SetHandlerChaincodeID(handler, &pb.ChaincodeID{Name: "test-handler-name", Version: "1.0"})
		chaincode.SetHandlerCCInstance(handler, &sysccprovider.ChaincodeInstance{ChaincodeName: "cc-instance-name"})
	})

	Describe("HandleTransaction", func() {
		var (
			incomingMessage    *pb.ChaincodeMessage
			fakeMessageHandler *fake.MessageHandler
			expectedResponse   *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedResponse = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_UNDEFINED,
				Payload:   []byte("handler-response-payload"),
				Txid:      "response-tx-id",
				ChannelId: "response-channel-id",
			}
			fakeMessageHandler = &fake.MessageHandler{}
			fakeMessageHandler.HandleReturns(expectedResponse, nil)
		})

		It("registers the transaction ID from the message", func() {
			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

			Expect(fakeTransactionRegistry.AddCallCount()).To(Equal(1))
			channelID, transactionID := fakeTransactionRegistry.AddArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(transactionID).To(Equal("tx-id"))
		})

		It("ensures there is a valid transaction simulator available in the context", func() {
			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

			Expect(fakeContextRegistry.GetCallCount()).To(Equal(1))
			channelID, txID := fakeContextRegistry.GetArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(txID).To(Equal("tx-id"))
		})

		It("calls the delegate with the correct arguments", func() {
			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

			Expect(fakeMessageHandler.HandleCallCount()).To(Equal(1))
			msg, ctx := fakeMessageHandler.HandleArgsForCall(0)
			Expect(msg).To(Equal(incomingMessage))
			Expect(ctx).To(Equal(txContext))
		})

		It("sends the response message returned by the delegate", func() {
			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
			msg := fakeChatStream.SendArgsForCall(0)
			Expect(msg).To(Equal(expectedResponse))
		})

		It("deregisters the transaction ID before sending the response", func() {
			fakeTransactionRegistry.RemoveStub = func(channelID, txID string) {
				Consistently(fakeChatStream.SendCallCount).Should(Equal(0))
			}

			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

			Expect(fakeTransactionRegistry.RemoveCallCount()).To(Equal(1))
			channelID, transactionID := fakeTransactionRegistry.RemoveArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(transactionID).To(Equal("tx-id"))
		})

		It("records shim requests received before requests completed", func() {
			fakeShimRequestsReceived.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeShimRequestsCompleted.AddCallCount()).To(Equal(0))
			}

			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
			msg := fakeChatStream.SendArgsForCall(0)
			Expect(msg).To(Equal(expectedResponse))

			Expect(fakeShimRequestsReceived.WithCallCount()).To(Equal(1))
			labelValues := fakeShimRequestsReceived.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"type", "GET_STATE",
				"channel", "channel-id",
				"chaincode", "test-handler-name:1.0",
			}))
			Expect(fakeShimRequestsReceived.AddCallCount()).To(Equal(1))
			Expect(fakeShimRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records transactions completed after transactions received", func() {
			fakeShimRequestsCompleted.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeShimRequestsReceived.AddCallCount()).To(Equal(1))
			}

			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))

			Expect(fakeShimRequestsCompleted.WithCallCount()).To(Equal(1))
			labelValues := fakeShimRequestsCompleted.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"type", "GET_STATE",
				"channel", "channel-id",
				"chaincode", "test-handler-name:1.0",
				"success", "true",
			}))
			Expect(fakeShimRequestsCompleted.AddCallCount()).To(Equal(1))
			Expect(fakeShimRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records transactions duration", func() {
			handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))

			Expect(fakeShimRequestDuration.WithCallCount()).To(Equal(1))
			labelValues := fakeShimRequestDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"type", "GET_STATE",
				"channel", "channel-id",
				"chaincode", "test-handler-name:1.0",
				"success", "true",
			}))
			Expect(fakeShimRequestDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			Expect(fakeShimRequestDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})

		Context("when the transaction returns an error", func() {
			BeforeEach(func() {
				fakeMessageHandler.HandleReturns(nil, errors.New("I am a total failure"))
			})

			It("records metrics with success=false", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))

				Expect(fakeShimRequestsCompleted.WithCallCount()).To(Equal(1))
				labelValues := fakeShimRequestsCompleted.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"type", "GET_STATE",
					"channel", "channel-id",
					"chaincode", "test-handler-name:1.0",
					"success", "false",
				}))
				Expect(fakeShimRequestsCompleted.AddCallCount()).To(Equal(1))
				Expect(fakeShimRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))

				Expect(fakeShimRequestDuration.WithCallCount()).To(Equal(1))
				labelValues = fakeShimRequestDuration.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"type", "GET_STATE",
					"channel", "channel-id",
					"chaincode", "test-handler-name:1.0",
					"success", "false",
				}))
				Expect(fakeShimRequestDuration.ObserveArgsForCall(0)).NotTo(BeZero())
				Expect(fakeShimRequestDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
			})
		})

		Context("when the transaction ID has already been registered", func() {
			BeforeEach(func() {
				fakeTransactionRegistry.AddReturns(false)
			})

			It("returns without sending a response", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
				Consistently(fakeChatStream.SendCallCount).Should(Equal(0))
			})

			It("does not call the delegate", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
				Expect(fakeMessageHandler.HandleCallCount()).To(Equal(0))
			})

			It("does not attempt to deregister the transaction ID", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)
				Expect(fakeTransactionRegistry.RemoveCallCount()).To(Equal(0))
			})
		})

		Context("when the transaction context does not exist", func() {
			BeforeEach(func() {
				fakeContextRegistry.GetReturns(nil)
			})

			It("sends an error message", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_ERROR,
					Payload:   []byte("GET_STATE failed: transaction ID: tx-id: no ledger context"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})

			It("deregisters the message transaction ID", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Expect(fakeTransactionRegistry.RemoveCallCount()).To(Equal(1))
				channelID, transactionID := fakeTransactionRegistry.RemoveArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(transactionID).To(Equal("tx-id"))
			})
		})

		Context("when the transaction context is missing a transaction simulator", func() {
			BeforeEach(func() {
				txContext.TXSimulator = nil
			})

			It("sends an error response", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_ERROR,
					Payload:   []byte("GET_STATE failed: transaction ID: tx-id: no ledger context"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})

			It("deregisters the message transaction ID", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Expect(fakeTransactionRegistry.RemoveCallCount()).To(Equal(1))
				channelID, transactionID := fakeTransactionRegistry.RemoveArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(transactionID).To(Equal("tx-id"))
			})
		})

		Context("when the incoming message is INVOKE_CHAINCODE", func() {
			var chaincodeSpec *pb.ChaincodeSpec

			BeforeEach(func() {
				chaincodeSpec = &pb.ChaincodeSpec{
					Type: pb.ChaincodeSpec_GOLANG,
					ChaincodeId: &pb.ChaincodeID{
						Name:    "target-chaincode-name",
						Version: "target-chaincode-version",
					},
					Input: &pb.ChaincodeInput{
						Args: util.ToChaincodeArgs("command", "arg"),
					},
				}
				payloadBytes, err := proto.Marshal(chaincodeSpec)
				Expect(err).NotTo(HaveOccurred())

				incomingMessage.Type = pb.ChaincodeMessage_INVOKE_CHAINCODE
				incomingMessage.Payload = payloadBytes
			})

			It("validates the transaction context", func() {
				txContext.TXSimulator = nil
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_ERROR,
					Payload:   []byte("INVOKE_CHAINCODE failed: transaction ID: tx-id: could not get valid transaction"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})

			Context("and the channel ID is not set", func() {
				BeforeEach(func() {
					incomingMessage.ChannelId = ""
				})

				It("checks if the target is system chaincode", func() {
					handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

					Expect(fakeSystemCCProvider.IsSysCCCallCount()).To(Equal(1))
					name := fakeSystemCCProvider.IsSysCCArgsForCall(0)
					Expect(name).To(Equal("target-chaincode-name"))
				})

				Context("when the target chaincode is not system chaincode", func() {
					BeforeEach(func() {
						fakeSystemCCProvider.IsSysCCReturns(false)
					})

					It("validates the transaction context", func() {
						txContext.TXSimulator = nil
						handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

						Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
						msg := fakeChatStream.SendArgsForCall(0)
						Expect(msg).To(Equal(&pb.ChaincodeMessage{
							Type:    pb.ChaincodeMessage_ERROR,
							Payload: []byte("INVOKE_CHAINCODE failed: transaction ID: tx-id: could not get valid transaction"),
							Txid:    "tx-id",
						}))
					})

					Context("when the target is system chaincode", func() {
						BeforeEach(func() {
							fakeSystemCCProvider.IsSysCCReturns(true)
						})

						It("gets the transaction context without validation", func() {
							txContext.TXSimulator = nil
							handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

							Expect(fakeContextRegistry.GetCallCount()).To(Equal(1))
							Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
							msg := fakeChatStream.SendArgsForCall(0)
							Expect(msg).To(Equal(expectedResponse))
						})

						Context("and the transaction context is missing", func() {
							It("returns an error", func() {
								fakeContextRegistry.GetReturns(nil)
								handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

								Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
								msg := fakeChatStream.SendArgsForCall(0)
								Expect(msg).To(Equal(&pb.ChaincodeMessage{
									Type:    pb.ChaincodeMessage_ERROR,
									Payload: []byte("INVOKE_CHAINCODE failed: transaction ID: tx-id: failed to get transaction context"),
									Txid:    "tx-id",
								}))
							})
						})
					})
				})

				Context("when the payload fails to unmarshal into a chaincode spec", func() {
					BeforeEach(func() {
						incomingMessage.Payload = []byte("this-is-a-bogus-payload")
					})

					It("sends an error response", func() {
						handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

						Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
						msg := fakeChatStream.SendArgsForCall(0)
						Expect(msg.Type).To(Equal(pb.ChaincodeMessage_ERROR))
						Expect(msg.Txid).To(Equal("tx-id"))
						Expect(string(msg.Payload)).To(HavePrefix("INVOKE_CHAINCODE failed: transaction ID: tx-id: unmarshal failed: proto: "))
					})
				})
			})
		})

		Context("when the delegate returns an error", func() {
			BeforeEach(func() {
				fakeMessageHandler.HandleReturns(nil, errors.New("watermelon-swirl"))
			})

			It("sends an error response", func() {
				handler.HandleTransaction(incomingMessage, fakeMessageHandler.Handle)

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_ERROR,
					Payload:   []byte("GET_STATE failed: transaction ID: tx-id: watermelon-swirl"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})
		})
	})

	Describe("HandlePutState", func() {
		var incomingMessage *pb.ChaincodeMessage
		var request *pb.PutState

		BeforeEach(func() {
			request = &pb.PutState{
				Key:   "put-state-key",
				Value: []byte("put-state-value"),
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_PUT_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		It("returns a response message", func() {
			resp, err := handler.HandlePutState(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}))
		})

		Context("when unmarshaling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandlePutState(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when the collection is not provided", func() {
			It("calls SetState on the transaction simulator", func() {
				_, err := handler.HandlePutState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.SetStateCallCount()).To(Equal(1))
				ccname, key, value := fakeTxSimulator.SetStateArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(key).To(Equal("put-state-key"))
				Expect(value).To(Equal([]byte("put-state-value")))
			})

			Context("when SeteState fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.SetStateReturns(errors.New("king-kong"))
				})

				It("returns an error", func() {
					_, err := handler.HandlePutState(incomingMessage, txContext)
					Expect(err).To(MatchError("king-kong"))
				})
			})
		})

		Context("when the collection is provided", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload
			})

			It("calls SetPrivateData on the transaction simulator", func() {
				_, err := handler.HandlePutState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.SetPrivateDataCallCount()).To(Equal(1))
				ccname, collection, key, value := fakeTxSimulator.SetPrivateDataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(key).To(Equal("put-state-key"))
				Expect(value).To(Equal([]byte("put-state-value")))
			})

			Context("when SetPrivateData fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.SetPrivateDataReturns(errors.New("godzilla"))
				})

				It("returns an error", func() {
					_, err := handler.HandlePutState(incomingMessage, txContext)
					Expect(err).To(MatchError("godzilla"))
				})
			})

			Context("when SetPrivateData fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandlePutState(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})
		})
	})

	Describe("HandlePutStateMetadata", func() {
		var incomingMessage *pb.ChaincodeMessage
		var request *pb.PutStateMetadata

		BeforeEach(func() {
			request = &pb.PutStateMetadata{
				Key: "put-state-key",
				Metadata: &pb.StateMetadata{
					Metakey: "put-state-metakey",
					Value:   []byte("put-state-metadata-value"),
				},
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_PUT_STATE_METADATA,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		It("returns a response message", func() {
			resp, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}))
		})

		It("acquires application config for the channel", func() {
			_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeApplicationConfigRetriever.GetApplicationConfigCallCount()).To(Equal(1))
			cid := fakeApplicationConfigRetriever.GetApplicationConfigArgsForCall(0)
			Expect(cid).To(Equal("channel-id"))
		})

		Context("when getting the app config metadata fails", func() {
			BeforeEach(func() {
				fakeApplicationConfigRetriever.GetApplicationConfigReturns(nil, false)
			})

			It("returns an error", func() {
				_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("application config does not exist for channel-id"))
			})
		})

		Context("when key level endorsement is not supported", func() {
			BeforeEach(func() {
				applicationCapability := &config.MockApplication{
					CapabilitiesRv: &config.MockApplicationCapabilities{KeyLevelEndorsementRv: false},
				}
				fakeApplicationConfigRetriever.GetApplicationConfigReturns(applicationCapability, true)
			})

			It("returns an error", func() {
				_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("key level endorsement is not enabled, channel application capability of V1_3 or later is required"))
			})
		})

		Context("when unmarshaling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when the collection is not provided", func() {
			It("calls SetStateMetadata on the transaction simulator", func() {
				_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.SetStateMetadataCallCount()).To(Equal(1))
				ccname, key, value := fakeTxSimulator.SetStateMetadataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(key).To(Equal("put-state-key"))
				Expect(value).To(Equal(map[string][]byte{
					"put-state-metakey": []byte("put-state-metadata-value"),
				}))
			})

			Context("when SetStateMetadata fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.SetStateMetadataReturns(errors.New("king-kong"))
				})

				It("returns an error", func() {
					_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("king-kong"))
				})
			})
		})

		Context("when the collection is provided", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload
			})

			It("calls SetPrivateDataMetadata on the transaction simulator", func() {
				_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.SetPrivateDataMetadataCallCount()).To(Equal(1))
				ccname, collection, key, value := fakeTxSimulator.SetPrivateDataMetadataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(key).To(Equal("put-state-key"))
				Expect(value).To(Equal(map[string][]byte{
					"put-state-metakey": []byte("put-state-metadata-value"),
				}))
			})

			Context("when SetPrivateDataMetadata fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.SetPrivateDataMetadataReturns(errors.New("godzilla"))
				})

				It("returns an error", func() {
					_, err := handler.HandlePutStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("godzilla"))
				})
			})
		})
	})

	Describe("HandleDelState", func() {
		var incomingMessage *pb.ChaincodeMessage
		var request *pb.DelState

		BeforeEach(func() {
			request = &pb.DelState{
				Key: "del-state-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_DEL_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		It("returns a response message", func() {
			resp, err := handler.HandleDelState(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}))
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleDelState(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when collection is not set", func() {
			It("calls DeleteState on the transaction simulator", func() {
				_, err := handler.HandleDelState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.DeleteStateCallCount()).To(Equal(1))
				ccname, key := fakeTxSimulator.DeleteStateArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(key).To(Equal("del-state-key"))
			})

			Context("when DeleteState returns an error", func() {
				BeforeEach(func() {
					fakeTxSimulator.DeleteStateReturns(errors.New("orange"))
				})

				It("return an error", func() {
					_, err := handler.HandleDelState(incomingMessage, txContext)
					Expect(err).To(MatchError("orange"))
				})
			})
		})

		Context("when collection is set", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload
			})

			It("calls DeletePrivateData on the transaction simulator", func() {
				_, err := handler.HandleDelState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.DeletePrivateDataCallCount()).To(Equal(1))
				ccname, collection, key := fakeTxSimulator.DeletePrivateDataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(key).To(Equal("del-state-key"))
			})

			Context("when DeletePrivateData fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.DeletePrivateDataReturns(errors.New("mango"))
				})

				It("returns an error", func() {
					_, err := handler.HandleDelState(incomingMessage, txContext)
					Expect(err).To(MatchError("mango"))
				})
			})

			Context("when DeletePrivateData fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandleDelState(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})
		})
	})

	Describe("HandleGetState", func() {
		var (
			incomingMessage  *pb.ChaincodeMessage
			request          *pb.GetState
			expectedResponse *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			request = &pb.GetState{
				Key: "get-state-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedResponse = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetState(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when collection is set", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload

				fakeCollectionStore.HasReadAccessReturns(true, nil)
				fakeTxSimulator.GetPrivateDataReturns([]byte("get-private-data-response"), nil)
				expectedResponse.Payload = []byte("get-private-data-response")
			})

			It("calls GetPrivateData on the transaction simulator", func() {
				_, err := handler.HandleGetState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetPrivateDataCallCount()).To(Equal(1))
				ccname, collection, key := fakeTxSimulator.GetPrivateDataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(key).To(Equal("get-state-key"))
			})

			Context("and GetPrivateData fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetPrivateDataReturns(nil, errors.New("french fries"))
				})

				It("returns the error from GetPrivateData", func() {
					_, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).To(MatchError("french fries"))
				})
			})

			Context("and GetPrivateData fails due to no read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, nil)
				})

				It("returns the error from errorIfCreatorHasNoReadAccess", func() {
					_, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).To(MatchError("tx creator does not have read access" +
						" permission on privatedata in chaincodeName:cc-instance-name" +
						" collectionName: collection-name"))
				})
			})

			Context("and GetPrivateData fails due to error in checking the read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, errors.New("no collection config"))
				})

				It("returns the error from errorIfCreatorHasNoReadAccess", func() {
					_, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).To(MatchError("no collection config"))
				})
			})

			Context("and GetPrivateData fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})

			Context("and GetPrivateData returns the response message", func() {
				BeforeEach(func() {
					txContext.AllowedCollectionAccess["collection-name"] = true
					fakeCollectionStore.HasReadAccessReturns(false, nil) // to
					// ensure that the access cache is used
				})

				It("returns the the response message from GetPrivateData", func() {
					resp, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal(&pb.ChaincodeMessage{
						Type:      pb.ChaincodeMessage_RESPONSE,
						Payload:   []byte("get-private-data-response"),
						Txid:      "tx-id",
						ChannelId: "channel-id",
					}))
				})
			})

			It("returns the response message from GetPrivateData", func() {
				resp, err := handler.HandleGetState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_RESPONSE,
					Payload:   []byte("get-private-data-response"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})
		})

		Context("when collection is not set", func() {
			BeforeEach(func() {
				fakeTxSimulator.GetStateReturns([]byte("get-state-response"), nil)
				expectedResponse.Payload = []byte("get-state-response")
			})

			It("calls GetState on the transaction simulator", func() {
				_, err := handler.HandleGetState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetStateCallCount()).To(Equal(1))
				ccname, key := fakeTxSimulator.GetStateArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(key).To(Equal("get-state-key"))
			})

			Context("and GetState fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetStateReturns(nil, errors.New("tomato"))
				})

				It("returns the error from GetState", func() {
					_, err := handler.HandleGetState(incomingMessage, txContext)
					Expect(err).To(MatchError("tomato"))
				})
			})

			It("returns the response from GetState", func() {
				resp, err := handler.HandleGetState(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_RESPONSE,
					Payload:   []byte("get-state-response"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				}))
			})
		})
	})

	Describe("HandleGetPrivateDataHash", func() {
		var (
			incomingMessage  *pb.ChaincodeMessage
			request          *pb.GetState
			expectedResponse *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			request = &pb.GetState{
				Collection: "collection-name",
				Key:        "get-pvtdata-hash-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_PRIVATE_DATA_HASH,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedResponse = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Payload:   []byte("get-private-data-hash-response"),
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
			fakeTxSimulator.GetPrivateDataHashReturns([]byte("get-private-data-hash-response"), nil)
		})

		It("calls GetPrivateDataHash on the transaction simulator and receives expected response", func() {
			response, err := handler.HandleGetPrivateDataHash(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTxSimulator.GetPrivateDataHashCallCount()).To(Equal(1))
			ccname, collection, key := fakeTxSimulator.GetPrivateDataHashArgsForCall(0)
			Expect(ccname).To(Equal("cc-instance-name"))
			Expect(collection).To(Equal("collection-name"))
			Expect(key).To(Equal("get-pvtdata-hash-key"))
			Expect(response).To(Equal(expectedResponse))
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetPrivateDataHash(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("and GetPrivateDataHash fails due to ledger error", func() {
			BeforeEach(func() {
				fakeTxSimulator.GetPrivateDataHashReturns(nil, errors.New("french fries"))
			})

			It("returns the error from GetPrivateData", func() {
				_, err := handler.HandleGetPrivateDataHash(incomingMessage, txContext)
				Expect(err).To(MatchError("french fries"))
			})
		})

		Context("and GetPrivateData fails due to Init transaction", func() {
			BeforeEach(func() {
				txContext.IsInitTransaction = true
			})

			It("returns the error from errorIfInitTransaction", func() {
				_, err := handler.HandleGetPrivateDataHash(incomingMessage, txContext)
				Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
			})
		})
	})

	Describe("HandleGetStateMetadata", func() {
		var (
			incomingMessage  *pb.ChaincodeMessage
			request          *pb.GetStateMetadata
			expectedResponse *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			request = &pb.GetStateMetadata{
				Key: "get-state-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE_METADATA,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedResponse = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		It("acquires application config for the channel", func() {
			_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeApplicationConfigRetriever.GetApplicationConfigCallCount()).To(Equal(1))
			cid := fakeApplicationConfigRetriever.GetApplicationConfigArgsForCall(0)
			Expect(cid).To(Equal("channel-id"))
		})

		Context("when getting the app config metadata fails", func() {
			BeforeEach(func() {
				fakeApplicationConfigRetriever.GetApplicationConfigReturns(nil, false)
			})

			It("returns an error", func() {
				_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("application config does not exist for channel-id"))
			})
		})

		Context("when key level endorsement is not supported", func() {
			BeforeEach(func() {
				applicationCapability := &config.MockApplication{
					CapabilitiesRv: &config.MockApplicationCapabilities{KeyLevelEndorsementRv: false},
				}
				fakeApplicationConfigRetriever.GetApplicationConfigReturns(applicationCapability, true)
			})

			It("returns an error", func() {
				_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("key level endorsement is not enabled, channel application capability of V1_3 or later is required"))
			})
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when collection is set", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload

				metadata := map[string][]byte{
					"get-state-metakey": []byte("get-private-metadata-response"),
				}
				fakeCollectionStore.HasReadAccessReturns(true, nil)
				fakeTxSimulator.GetPrivateDataMetadataReturns(metadata, nil)
				responsePayload, err := proto.Marshal(&pb.StateMetadataResult{
					Entries: []*pb.StateMetadata{{
						Metakey: "get-state-metakey",
						Value:   []byte("get-private-metadata-response"),
					}},
				})
				Expect(err).NotTo(HaveOccurred())
				expectedResponse.Payload = responsePayload
			})

			It("calls GetPrivateDataMetadata on the transaction simulator", func() {
				_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetPrivateDataMetadataCallCount()).To(Equal(1))
				ccname, collection, key := fakeTxSimulator.GetPrivateDataMetadataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(key).To(Equal("get-state-key"))
			})

			It("returns the response message from GetPrivateDataMetadata", func() {
				resp, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(expectedResponse))
			})

			Context("and GetPrivateDataMetadata fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetPrivateDataMetadataReturns(nil, errors.New("french fries"))
				})

				It("returns the error from GetPrivateDataMetadata", func() {
					_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("french fries"))
				})
			})

			Context("and GetPrivateDataMetadata fails due to no read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, nil)
				})

				It("returns the error from GetPrivateDataMetadata", func() {
					_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("tx creator does not have read access" +
						" permission on privatedata in chaincodeName:cc-instance-name" +
						" collectionName: collection-name"))
				})
			})

			Context("and GetPrivateDataMetadata fails due to error in checking the read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, errors.New("no collection config"))
				})

				It("returns the error from GetPrivateDataMetadata", func() {
					_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("no collection config"))
				})
			})

			Context("and GetPrivateDataMetadata fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})
		})

		Context("when collection is not set", func() {
			BeforeEach(func() {
				metadata := map[string][]byte{
					"get-state-metakey": []byte("get-state-metadata-response"),
				}
				fakeTxSimulator.GetStateMetadataReturns(metadata, nil)
				responsePayload, err := proto.Marshal(&pb.StateMetadataResult{
					Entries: []*pb.StateMetadata{{
						Metakey: "get-state-metakey",
						Value:   []byte("get-state-metadata-response"),
					}},
				})
				Expect(err).NotTo(HaveOccurred())
				expectedResponse.Payload = responsePayload
			})

			It("calls GetStateMetadata on the transaction simulator", func() {
				_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetStateMetadataCallCount()).To(Equal(1))
				ccname, key := fakeTxSimulator.GetStateMetadataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(key).To(Equal("get-state-key"))
			})

			It("returns the response from GetStateMetadata", func() {
				resp, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(expectedResponse))
			})

			Context("and GetStateMetadata fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetStateMetadataReturns(nil, errors.New("tomato"))
				})

				It("returns the error from GetStateMetadata", func() {
					_, err := handler.HandleGetStateMetadata(incomingMessage, txContext)
					Expect(err).To(MatchError("tomato"))
				})
			})
		})
	})

	Describe("HandleGetStateByRange", func() {
		var (
			incomingMessage       *pb.ChaincodeMessage
			request               *pb.GetStateByRange
			expectedResponse      *pb.ChaincodeMessage
			fakeIterator          *mock.QueryResultsIterator
			expectedQueryResponse *pb.QueryResponse
			expectedPayload       []byte
		)

		BeforeEach(func() {
			request = &pb.GetStateByRange{
				StartKey: "get-state-start-key",
				EndKey:   "get-state-end-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			fakeIterator = &mock.QueryResultsIterator{}
			fakeTxSimulator.GetStateRangeScanIteratorReturns(fakeIterator, nil)

			expectedQueryResponse = &pb.QueryResponse{
				Results: nil,
				HasMore: true,
				Id:      "query-response-id",
			}
			fakeQueryResponseBuilder.BuildQueryResponseReturns(expectedQueryResponse, nil)

			expectedPayload, err = proto.Marshal(expectedQueryResponse)
			Expect(err).NotTo(HaveOccurred())

			expectedResponse = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Txid:      "tx-id",
				Payload:   expectedPayload,
				ChannelId: "channel-id",
			}
		})

		It("initializes a query context", func() {
			_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			pqr := txContext.GetPendingQueryResult("generated-query-id")
			Expect(pqr).To(Equal(&chaincode.PendingQueryResult{}))
			iter := txContext.GetQueryIterator("generated-query-id")
			Expect(iter).To(Equal(fakeIterator))
			retCount := txContext.GetTotalReturnCount("generated-query-id")
			Expect(*retCount).To(Equal(int32(0)))
		})

		It("returns the response message", func() {
			resp, err := handler.HandleGetStateByRange(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(expectedResponse))
		})

		Context("when collection is not set", func() {
			It("calls GetStateRangeScanIterator on the transaction simulator", func() {
				_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetStateRangeScanIteratorCallCount()).To(Equal(1))
				ccname, startKey, endKey := fakeTxSimulator.GetStateRangeScanIteratorArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(startKey).To(Equal("get-state-start-key"))
				Expect(endKey).To(Equal("get-state-end-key"))
			})

			Context("and GetStateRangeScanIterator fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetStateRangeScanIteratorReturns(nil, errors.New("tomato"))
				})

				It("returns the error from GetStateRangeScanIterator", func() {
					_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
					Expect(err).To(MatchError("tomato"))
				})
			})

			It("returns the response from GetStateRangeScanIterator", func() {
				resp, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(expectedResponse))
			})
		})

		Context("when collection is set", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload

				fakeCollectionStore.HasReadAccessReturns(true, nil)
				fakeTxSimulator.GetPrivateDataRangeScanIteratorReturns(fakeIterator, nil)
			})

			It("calls GetPrivateDataRangeScanIterator on the transaction simulator", func() {
				_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.GetPrivateDataRangeScanIteratorCallCount()).To(Equal(1))
				ccname, collection, startKey, endKey := fakeTxSimulator.GetPrivateDataRangeScanIteratorArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(startKey).To(Equal("get-state-start-key"))
				Expect(endKey).To(Equal("get-state-end-key"))
			})

			Context("and GetPrivateDataRangeScanIterator fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.GetPrivateDataRangeScanIteratorReturns(nil, errors.New("french fries"))
				})

				It("returns the error from GetPrivateDataRangeScanIterator", func() {
					_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
					Expect(err).To(MatchError("french fries"))
				})
			})

			Context("and GetPrivateDataRangeScanIterator fails due to no read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, nil)
				})

				It("returns the error from GetPrivateDataRangeScanIterator", func() {
					_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
					Expect(err).To(MatchError("tx creator does not have read access" +
						" permission on privatedata in chaincodeName:cc-instance-name" +
						" collectionName: collection-name"))
				})
			})

			Context("and GetPrivateDataRangeScanIterator fails due to error in checking the read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, errors.New("no collection config"))
				})

				It("returns the error from GetPrivateDataRangeScanIterator", func() {
					_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
					Expect(err).To(MatchError("no collection config"))
				})
			})

			Context("and GetPrivateDataRangeScanIterator fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when building the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, errors.New("garbanzo"))
			})

			It("returns the error", func() {
				_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).To(MatchError("garbanzo"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetStateByRange(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
			})
		})

		Context("when marshling the response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, nil)
			})

			It("returns the error", func() {
				_, err := handler.HandleGetStateByRange(incomingMessage, txContext)
				Expect(err).To(MatchError("marshal failed: proto: Marshal called with nil"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetStateByRange(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})
	})

	Describe("HandleQueryStateNext", func() {
		var (
			fakeIterator          *mock.QueryResultsIterator
			expectedQueryResponse *pb.QueryResponse
			request               *pb.QueryStateNext
			incomingMessage       *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			request = &pb.QueryStateNext{
				Id: "query-state-next-id",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			fakeIterator = &mock.QueryResultsIterator{}
			txContext.InitializeQueryContext("query-state-next-id", fakeIterator)

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
			expectedQueryResponse = &pb.QueryResponse{
				Results: nil,
				HasMore: true,
				Id:      "query-response-id",
			}
			fakeQueryResponseBuilder.BuildQueryResponseReturns(expectedQueryResponse, nil)
		})

		It("builds a query response", func() {
			_, err := handler.HandleQueryStateNext(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeQueryResponseBuilder.BuildQueryResponseCallCount()).To(Equal(1))
			tctx, iter, id, _, _ := fakeQueryResponseBuilder.BuildQueryResponseArgsForCall(0)
			Expect(tctx).To(Equal(txContext))
			Expect(iter).To(Equal(fakeIterator))
			Expect(id).To(Equal("query-state-next-id"))
		})

		It("returns a chaincode message with the query response", func() {
			resp, err := handler.HandleQueryStateNext(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			expectedPayload, err := proto.Marshal(expectedQueryResponse)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp).To(Equal(&pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Payload:   expectedPayload,
				ChannelId: "channel-id",
				Txid:      "tx-id",
			}))
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleQueryStateNext(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when the query iterator is missing", func() {
			BeforeEach(func() {
				txContext.CleanupQueryContext("query-state-next-id")
			})

			It("returns an error", func() {
				_, err := handler.HandleQueryStateNext(incomingMessage, txContext)
				Expect(err).To(MatchError("query iterator not found"))
			})
		})

		Context("when building the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, errors.New("potato"))
			})

			It("returns an error", func() {
				_, err := handler.HandleQueryStateNext(incomingMessage, txContext)
				Expect(err).To(MatchError("potato"))
			})

			It("cleans up the query context", func() {
				handler.HandleQueryStateNext(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})

		Context("when marshaling the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := handler.HandleQueryStateNext(incomingMessage, txContext)
				Expect(err).To(MatchError("marshal failed: proto: Marshal called with nil"))
			})

			It("cleans up the query context", func() {
				handler.HandleQueryStateNext(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})
	})

	Describe("HandleQueryStateClose", func() {
		var (
			fakeIterator          *mock.QueryResultsIterator
			expectedQueryResponse *pb.QueryResponse
			request               *pb.QueryStateClose
			incomingMessage       *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			request = &pb.QueryStateClose{
				Id: "query-state-close-id",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			fakeIterator = &mock.QueryResultsIterator{}
			txContext.InitializeQueryContext("query-state-close-id", fakeIterator)

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
			expectedQueryResponse = &pb.QueryResponse{
				Id: "query-state-close-id",
			}
		})

		It("returns a chaincode message with the query response", func() {
			resp, err := handler.HandleQueryStateClose(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			expectedPayload, err := proto.Marshal(expectedQueryResponse)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp).To(Equal(&pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_RESPONSE,
				Payload:   expectedPayload,
				ChannelId: "channel-id",
				Txid:      "tx-id",
			}))
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleQueryStateClose(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when the query iterator is missing", func() {
			BeforeEach(func() {
				txContext.CleanupQueryContext("query-state-close-id")
			})

			It("keeps calm and carries on", func() {
				_, err := handler.HandleQueryStateClose(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("HandleGetQueryResult", func() {
		var (
			request               *pb.GetQueryResult
			incomingMessage       *pb.ChaincodeMessage
			expectedQueryResponse *pb.QueryResponse
			fakeIterator          *mock.QueryResultsIterator
		)

		BeforeEach(func() {
			request = &pb.GetQueryResult{
				Query: "query-result",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_STATE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedQueryResponse = &pb.QueryResponse{
				Id: "query-response-id",
			}
			fakeQueryResponseBuilder.BuildQueryResponseReturns(expectedQueryResponse, nil)

			fakeIterator = &mock.QueryResultsIterator{}
			fakeTxSimulator.ExecuteQueryReturns(fakeIterator, nil)
		})

		Context("when collection is not set", func() {
			It("calls ExecuteQuery on the transaction simulator", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.ExecuteQueryCallCount()).To(Equal(1))
				ccname, query := fakeTxSimulator.ExecuteQueryArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(query).To(Equal("query-result"))
			})

			Context("and ExecuteQuery fails", func() {
				BeforeEach(func() {
					fakeTxSimulator.ExecuteQueryReturns(nil, errors.New("mushrooms"))
				})

				It("returns the error", func() {
					_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
					Expect(err).To(MatchError("mushrooms"))
				})
			})
		})

		Context("when collection is set", func() {
			BeforeEach(func() {
				request.Collection = "collection-name"
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload

				fakeCollectionStore.HasReadAccessReturns(true, nil)
				fakeTxSimulator.ExecuteQueryOnPrivateDataReturns(fakeIterator, nil)
			})

			It("calls ExecuteQueryOnPrivateDataon the transaction simulator", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTxSimulator.ExecuteQueryOnPrivateDataCallCount()).To(Equal(1))
				ccname, collection, query := fakeTxSimulator.ExecuteQueryOnPrivateDataArgsForCall(0)
				Expect(ccname).To(Equal("cc-instance-name"))
				Expect(collection).To(Equal("collection-name"))
				Expect(query).To(Equal("query-result"))
			})

			It("initializes a query context", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(Equal(&chaincode.PendingQueryResult{}))
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(Equal(fakeIterator))
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(*retCount).To(Equal(int32(0)))
			})

			Context("and ExecuteQueryOnPrivateData fails due to ledger error", func() {
				BeforeEach(func() {
					fakeTxSimulator.ExecuteQueryOnPrivateDataReturns(nil, errors.New("pizza"))
				})

				It("returns the error", func() {
					_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
					Expect(err).To(MatchError("pizza"))
				})
			})

			Context("and ExecuteQueryOnPrivateData fails due to no read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, nil)
				})

				It("returns the error", func() {
					_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
					Expect(err).To(MatchError("tx creator does not have read access" +
						" permission on privatedata in chaincodeName:cc-instance-name" +
						" collectionName: collection-name"))
				})
			})

			Context("and ExecuteQueryOnPrivateData fails due to error in checking the read access", func() {
				BeforeEach(func() {
					fakeCollectionStore.HasReadAccessReturns(false, errors.New("no collection config"))
				})

				It("returns the error", func() {
					_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
					Expect(err).To(MatchError("no collection config"))
				})
			})

			Context("and ExecuteQueryOnPrivateData fails due to Init transaction", func() {
				BeforeEach(func() {
					txContext.IsInitTransaction = true
				})

				It("returns the error from errorIfInitTransaction", func() {
					_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
					Expect(err).To(MatchError("private data APIs are not allowed in chaincode Init()"))
				})
			})
		})

		It("builds the query response", func() {
			_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeQueryResponseBuilder.BuildQueryResponseCallCount()).To(Equal(1))
			tctx, iter, iterID, _, _ := fakeQueryResponseBuilder.BuildQueryResponseArgsForCall(0)
			Expect(tctx).To(Equal(txContext))
			Expect(iter).To(Equal(fakeIterator))
			Expect(iterID).To(Equal("generated-query-id"))
		})

		Context("when building the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, errors.New("peppers"))
			})

			It("returns the error", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).To(MatchError("peppers"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetQueryResult(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when marshaling the response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := handler.HandleGetQueryResult(incomingMessage, txContext)
				Expect(err).To(MatchError("marshal failed: proto: Marshal called with nil"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetQueryResult(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})
	})

	Describe("HandleGetHistoryForKey", func() {
		var (
			request               *pb.GetHistoryForKey
			incomingMessage       *pb.ChaincodeMessage
			expectedQueryResponse *pb.QueryResponse
			fakeIterator          *mock.QueryResultsIterator
		)

		BeforeEach(func() {
			request = &pb.GetHistoryForKey{
				Key: "history-key",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_GET_HISTORY_FOR_KEY,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			expectedQueryResponse = &pb.QueryResponse{
				Id: "query-response-id",
			}
			fakeQueryResponseBuilder.BuildQueryResponseReturns(expectedQueryResponse, nil)

			fakeIterator = &mock.QueryResultsIterator{}
			fakeHistoryQueryExecutor.GetHistoryForKeyReturns(fakeIterator, nil)
		})

		It("calls GetHistoryForKey on the history query executor", func() {
			_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeHistoryQueryExecutor.GetHistoryForKeyCallCount()).To(Equal(1))
			ccname, key := fakeHistoryQueryExecutor.GetHistoryForKeyArgsForCall(0)
			Expect(ccname).To(Equal("cc-instance-name"))
			Expect(key).To(Equal("history-key"))
		})

		It("initializes a query context", func() {
			_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			pqr := txContext.GetPendingQueryResult("generated-query-id")
			Expect(pqr).To(Equal(&chaincode.PendingQueryResult{}))
			iter := txContext.GetQueryIterator("generated-query-id")
			Expect(iter).To(Equal(fakeIterator))
			retCount := txContext.GetTotalReturnCount("generated-query-id")
			Expect(*retCount).To(Equal(int32(0)))
		})

		It("builds a query response", func() {
			_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeQueryResponseBuilder.BuildQueryResponseCallCount()).To(Equal(1))
			tctx, iter, iterID, _, _ := fakeQueryResponseBuilder.BuildQueryResponseArgsForCall(0)
			Expect(tctx).To(Equal(txContext))
			Expect(iter).To(Equal(fakeIterator))
			Expect(iterID).To(Equal("generated-query-id"))
		})

		Context("when unmarshalling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when the history query executor fails", func() {
			BeforeEach(func() {
				fakeHistoryQueryExecutor.GetHistoryForKeyReturns(nil, errors.New("pepperoni"))
			})

			It("returns an error", func() {
				_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
				Expect(err).To(MatchError("pepperoni"))
			})
		})

		Context("when building the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, errors.New("mushrooms"))
			})

			It("returns an error", func() {
				_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
				Expect(err).To(MatchError("mushrooms"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetHistoryForKey(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})

		Context("when marshaling the query response fails", func() {
			BeforeEach(func() {
				fakeQueryResponseBuilder.BuildQueryResponseReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := handler.HandleGetHistoryForKey(incomingMessage, txContext)
				Expect(err).To(MatchError("marshal failed: proto: Marshal called with nil"))
			})

			It("cleans up the query context", func() {
				handler.HandleGetHistoryForKey(incomingMessage, txContext)

				pqr := txContext.GetPendingQueryResult("generated-query-id")
				Expect(pqr).To(BeNil())
				iter := txContext.GetQueryIterator("generated-query-id")
				Expect(iter).To(BeNil())
				retCount := txContext.GetTotalReturnCount("generated-query-id")
				Expect(retCount).To(BeNil())
			})
		})
	})

	Describe("HandleInvokeChaincode", func() {
		var (
			expectedSignedProp      *pb.SignedProposal
			expectedProposal        *pb.Proposal
			targetDefinition        *ccprovider.ChaincodeData
			fakePeerLedger          *mock.PeerLedger
			newTxSimulator          *mock.TxSimulator
			newHistoryQueryExecutor *mock.HistoryQueryExecutor
			request                 *pb.ChaincodeSpec
			incomingMessage         *pb.ChaincodeMessage
			responseMessage         *pb.ChaincodeMessage
		)

		BeforeEach(func() {
			expectedProposal = &pb.Proposal{
				Header:  []byte("proposal-header"),
				Payload: []byte("proposal-payload"),
			}
			expectedSignedProp = &pb.SignedProposal{
				ProposalBytes: []byte("signed-proposal-bytes"),
				Signature:     []byte("signature"),
			}
			txContext.Proposal = expectedProposal
			txContext.SignedProp = expectedSignedProp

			newTxSimulator = &mock.TxSimulator{}
			newHistoryQueryExecutor = &mock.HistoryQueryExecutor{}
			fakePeerLedger = &mock.PeerLedger{}
			fakePeerLedger.NewTxSimulatorReturns(newTxSimulator, nil)
			fakeLedgerGetter.GetLedgerReturns(fakePeerLedger)
			fakePeerLedger.NewHistoryQueryExecutorReturns(newHistoryQueryExecutor, nil)

			targetDefinition = &ccprovider.ChaincodeData{
				Name:    "target-chaincode-data-name",
				Version: "target-chaincode-version",
			}
			fakeDefinitionGetter.ChaincodeDefinitionReturns(targetDefinition, nil)

			request = &pb.ChaincodeSpec{
				ChaincodeId: &pb.ChaincodeID{
					Name: "target-chaincode-name:target-version",
				},
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_INVOKE_CHAINCODE,
				Payload:   payload,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}

			responseMessage = &pb.ChaincodeMessage{}
			fakeInvoker.InvokeReturns(responseMessage, nil)
		})

		It("checks if the target is not invokable", func() {
			_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeSystemCCProvider.IsSysCCAndNotInvokableCC2CCCallCount()).To(Equal(1))
			name := fakeSystemCCProvider.IsSysCCAndNotInvokableCC2CCArgsForCall(0)
			Expect(name).To(Equal("target-chaincode-name"))
		})

		It("checks if the target is a system chaincode", func() {
			_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeSystemCCProvider.IsSysCCCallCount() >= 1).To(BeTrue())
			name := fakeSystemCCProvider.IsSysCCArgsForCall(0)
			Expect(name).To(Equal("target-chaincode-name"))
		})

		It("evaluates the access control policy", func() {
			_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeACLProvider.CheckACLCallCount()).To(Equal(1))
			resource, chainID, proposal := fakeACLProvider.CheckACLArgsForCall(0)
			Expect(resource).To(Equal(resources.Peer_ChaincodeToChaincode))
			Expect(chainID).To(Equal("channel-id"))
			Expect(proposal).To(Equal(expectedSignedProp))
		})

		Context("when the target channel is different from the context", func() {
			BeforeEach(func() {
				request = &pb.ChaincodeSpec{
					ChaincodeId: &pb.ChaincodeID{
						Name: "target-chaincode-name:target-version/target-channel-id",
					},
				}
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())
				incomingMessage.Payload = payload
			})

			It("uses the channel form the target for access checks", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeACLProvider.CheckACLCallCount()).To(Equal(1))
				resource, chainID, proposal := fakeACLProvider.CheckACLArgsForCall(0)
				Expect(resource).To(Equal(resources.Peer_ChaincodeToChaincode))
				Expect(chainID).To(Equal("target-channel-id"))
				Expect(proposal).To(Equal(expectedSignedProp))
			})

			It("gets the ledger for the target channel", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeLedgerGetter.GetLedgerCallCount()).To(Equal(1))
				chainID := fakeLedgerGetter.GetLedgerArgsForCall(0)
				Expect(chainID).To(Equal("target-channel-id"))
			})

			It("creates a new tx simulator for target execution", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakePeerLedger.NewTxSimulatorCallCount()).To(Equal(1))
				txid := fakePeerLedger.NewTxSimulatorArgsForCall(0)
				Expect(txid).To(Equal("tx-id"))
			})

			It("provides the new simulator in the context used for execution", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeInvoker.InvokeCallCount()).To(Equal(1))
				txParams, _, _ := fakeInvoker.InvokeArgsForCall(0)
				Expect(txParams.TXSimulator).To(BeIdenticalTo(newTxSimulator)) // same instance, not just equal
			})

			It("creates a new history query executor for target execution", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakePeerLedger.NewHistoryQueryExecutorCallCount()).To(Equal(1))
			})

			It("provides the new history query executor in the context used for execution", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeInvoker.InvokeCallCount()).To(Equal(1))
				txParams, _, _ := fakeInvoker.InvokeArgsForCall(0)
				Expect(txParams.HistoryQueryExecutor).To(BeIdenticalTo(newHistoryQueryExecutor)) // same instance, not just equal
			})

			It("marks the new transaction simulator as done after execute", func() {
				fakeInvoker.InvokeStub = func(*ccprovider.TransactionParams, *ccprovider.CCContext, *pb.ChaincodeInput) (*pb.ChaincodeMessage, error) {
					Expect(newTxSimulator.DoneCallCount()).To(Equal(0))
					return responseMessage, nil
				}
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeInvoker.InvokeCallCount()).To(Equal(1))
				Expect(newTxSimulator.DoneCallCount()).To(Equal(1))
			})

			Context("when getting the ledger for the target channel fails", func() {
				BeforeEach(func() {
					fakeLedgerGetter.GetLedgerReturns(nil)
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("failed to find ledger for channel: target-channel-id"))
				})
			})

			Context("when creating the new tx simulator fails", func() {
				BeforeEach(func() {
					fakePeerLedger.NewTxSimulatorReturns(nil, errors.New("bonkers"))
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("bonkers"))
				})
			})

			Context("when creating the new history query executor fails", func() {
				BeforeEach(func() {
					fakePeerLedger.NewHistoryQueryExecutorReturns(nil, errors.New("razzies"))
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("razzies"))
				})
			})
		})

		Context("when the target is a system chaincode", func() {
			BeforeEach(func() {
				fakeSystemCCProvider.IsSysCCReturns(true)
			})

			It("does not perform acl checks", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeACLProvider.CheckACLCallCount()).To(Equal(0))
			})

			It("uses the system chaincode version", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeInvoker.InvokeCallCount()).To(Equal(1))
				_, cccid, _ := fakeInvoker.InvokeArgsForCall(0)
				Expect(cccid.Version).To(Equal("system-cc-version"))
			})
		})

		Context("when the target is not a system chaincode", func() {
			BeforeEach(func() {
				fakeSystemCCProvider.IsSysCCReturns(false)
			})

			It("gets the chaincode definition", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeDefinitionGetter.ChaincodeDefinitionCallCount()).To(Equal(1))
				ccname, txSim := fakeDefinitionGetter.ChaincodeDefinitionArgsForCall(0)
				Expect(ccname).To(Equal("target-chaincode-name"))
				Expect(txSim).To(Equal(newTxSimulator))
			})

			It("checks the instantiation policy on the target", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeInstantiationPolicyChecker.CheckInstantiationPolicyCallCount()).To(Equal(1))
				name, version, cd := fakeInstantiationPolicyChecker.CheckInstantiationPolicyArgsForCall(0)
				Expect(name).To(Equal("target-chaincode-name"))
				Expect(version).To(Equal("target-chaincode-version"))
				Expect(cd).To(Equal(targetDefinition))
			})

			Context("when getting the chaincode definition fails", func() {
				BeforeEach(func() {
					fakeDefinitionGetter.ChaincodeDefinitionReturns(nil, errors.New("blueberry-cobbler"))
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("blueberry-cobbler"))
				})
			})

			Context("when the signed proposal is nil", func() {
				BeforeEach(func() {
					txContext.SignedProp = nil
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("signed proposal must not be nil from caller [channel-id.target-chaincode-name#target-version]"))
				})
			})

			Context("when the instantiation policy evaluation returns an error", func() {
				BeforeEach(func() {
					fakeInstantiationPolicyChecker.CheckInstantiationPolicyReturns(errors.New("raspberry-pie"))
				})

				It("returns an error", func() {
					_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
					Expect(err).To(MatchError("raspberry-pie"))
				})
			})
		})

		Context("when the target is not invokable", func() {
			BeforeEach(func() {
				fakeSystemCCProvider.IsSysCCAndNotInvokableCC2CCReturns(true)
			})

			It("returns an error", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).To(MatchError("system chaincode target-chaincode-name cannot be invoked with a cc2cc invocation"))
			})
		})

		Context("when the access control check fails", func() {
			BeforeEach(func() {
				fakeACLProvider.CheckACLReturns(errors.New("no-soup-for-you"))
			})

			It("returns the error", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).To(MatchError("no-soup-for-you"))
			})
		})

		Context("when execute fails", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns(nil, errors.New("lemons"))
			})

			It("returns an error", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).To(MatchError("execute failed: lemons"))
			})
		})

		Context("when unmarshaling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("returns an error", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).To(MatchError("unmarshal failed: proto: can't skip unknown wire type 4"))
			})
		})

		Context("when marshaling the response fails", func() {
			BeforeEach(func() {
				fakeInvoker.InvokeReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := handler.HandleInvokeChaincode(incomingMessage, txContext)
				Expect(err).To(MatchError("marshal failed: proto: Marshal called with nil"))
			})
		})
	})

	Describe("Execute", func() {
		var (
			cccid              *ccprovider.CCContext
			incomingMessage    *pb.ChaincodeMessage
			expectedProposal   *pb.Proposal
			expectedSignedProp *pb.SignedProposal
			txParams           *ccprovider.TransactionParams
		)

		BeforeEach(func() {
			request := &pb.ChaincodeInput{
				Args: util.ToChaincodeArgs("arg1", "arg2"),
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			expectedProposal = &pb.Proposal{
				Header:  []byte("proposal-header"),
				Payload: []byte("proposal-payload"),
			}

			expectedSignedProp = &pb.SignedProposal{
				ProposalBytes: []byte("signed-proposal-bytes"),
				Signature:     []byte("signature"),
			}

			cccid = &ccprovider.CCContext{
				Name:    "chaincode-name",
				Version: "chaincode-version",
			}
			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_TRANSACTION,
				Txid:      "tx-id",
				Payload:   payload,
				ChannelId: "channel-id",
			}

			txParams = &ccprovider.TransactionParams{
				TxID:       "tx-id",
				ChannelID:  "channel-id",
				SignedProp: expectedSignedProp,
				Proposal:   expectedProposal,
			}
		})

		It("creates transaction context", func() {
			close(responseNotifier)
			handler.Execute(txParams, cccid, incomingMessage, time.Second)

			Expect(fakeContextRegistry.CreateCallCount()).To(Equal(1))
			Expect(fakeContextRegistry.CreateArgsForCall(0)).To(Equal(txParams))
		})

		It("sends an execute message to the chaincode with the correct proposal", func() {
			expectedMessage := *incomingMessage
			expectedMessage.Proposal = expectedSignedProp

			close(responseNotifier)
			handler.Execute(txParams, cccid, incomingMessage, time.Second)

			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
			Consistently(fakeChatStream.SendCallCount).Should(Equal(1))
			msg := fakeChatStream.SendArgsForCall(0)
			Expect(msg).To(Equal(&expectedMessage))
			Expect(msg.Proposal).To(Equal(expectedSignedProp))
		})

		It("waits for the chaincode to respond", func() {
			doneCh := make(chan struct{})
			go func() {
				handler.Execute(txParams, cccid, incomingMessage, time.Second)
				close(doneCh)
			}()

			Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
			Consistently(fakeChatStream.SendCallCount).Should(Equal(1))

			Consistently(doneCh).ShouldNot(Receive())
			Eventually(responseNotifier).Should(BeSent(&pb.ChaincodeMessage{}))
			Eventually(doneCh).Should(BeClosed())
		})

		It("returns the chaincode response", func() {
			Eventually(responseNotifier).Should(BeSent(&pb.ChaincodeMessage{Txid: "a-transaction-id"}))

			resp, err := handler.Execute(txParams, cccid, incomingMessage, time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&pb.ChaincodeMessage{Txid: "a-transaction-id"}))
		})

		It("deletes the transaction context", func() {
			close(responseNotifier)
			handler.Execute(txParams, cccid, incomingMessage, time.Second)

			Expect(fakeContextRegistry.DeleteCallCount()).Should(Equal(1))
			channelID, txid := fakeContextRegistry.DeleteArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(txid).To(Equal("tx-id"))
		})

		Context("when the serial send fails", func() {
			BeforeEach(func() {
				fakeChatStream.SendReturns(errors.New("where-is-waldo?"))
			})

			It("returns an error before before timing out", func() {
				respCh := make(chan *pb.ChaincodeMessage, 1)
				go func() {
					defer GinkgoRecover()
					resp, err := handler.Execute(txParams, cccid, incomingMessage, time.Second)
					Expect(err).NotTo(HaveOccurred())
					Eventually(respCh).Should(BeSent(resp))
				}()

				Eventually(respCh).Should(Receive(Equal(&pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_ERROR,
					Payload:   []byte("[tx-id] error sending TRANSACTION: where-is-waldo?"),
					Txid:      "tx-id",
					ChannelId: "channel-id",
				})))
			})
		})

		Context("when the proposal is missing", func() {
			BeforeEach(func() {
				txParams.Proposal = nil
			})

			It("sends a nil proposal", func() {
				close(responseNotifier)
				_, err := handler.Execute(txParams, cccid, incomingMessage, time.Second)
				Expect(err).NotTo(HaveOccurred())

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg).NotTo(BeNil())
				Expect(msg.Proposal).To(BeNil())
			})
		})

		Context("when the signed proposal is missing", func() {
			BeforeEach(func() {
				txParams.SignedProp = nil
			})

			It("returns an error", func() {
				close(responseNotifier)
				_, err := handler.Execute(txParams, cccid, incomingMessage, time.Second)

				Expect(err).To(MatchError("failed getting proposal context. Signed proposal is nil"))
			})

			It("deletes the transaction context", func() {
				close(responseNotifier)
				handler.Execute(txParams, cccid, incomingMessage, time.Second)

				Expect(fakeContextRegistry.DeleteCallCount()).Should(Equal(1))
				channelID, txid := fakeContextRegistry.DeleteArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(txid).To(Equal("tx-id"))
			})
		})

		Context("when creating the transaction context fails", func() {
			BeforeEach(func() {
				fakeContextRegistry.CreateReturns(nil, errors.New("burger"))
			})

			It("returns an error", func() {
				_, err := handler.Execute(txParams, cccid, incomingMessage, time.Second)
				Expect(err).To(MatchError("burger"))
			})

			It("does not try to delete the tranasction context", func() {
				handler.Execute(txParams, cccid, incomingMessage, time.Second)
				Expect(fakeContextRegistry.CreateCallCount()).To(Equal(1))
				Expect(fakeContextRegistry.DeleteCallCount()).To(Equal(0))
			})
		})

		Context("when the chaincode stream terminates", func() {
			It("returns an error", func() {
				streamDoneChan := make(chan struct{})
				chaincode.SetStreamDoneChan(handler, streamDoneChan)

				errCh := make(chan error, 1)
				go func() {
					_, err := handler.Execute(txParams, cccid, incomingMessage, time.Hour)
					errCh <- err
				}()
				Consistently(errCh).ShouldNot(Receive())

				close(streamDoneChan)
				Eventually(errCh).Should(Receive(MatchError("chaincode stream terminated")))
			})
		})

		Context("when execute times out", func() {
			It("returns an error", func() {
				errCh := make(chan error, 1)
				go func() {
					_, err := handler.Execute(txParams, cccid, incomingMessage, time.Millisecond)
					errCh <- err
				}()
				Eventually(errCh).Should(Receive(MatchError("timeout expired while executing transaction")))
			})

			It("records execute timeouts", func() {
				errCh := make(chan error, 1)
				go func() {
					_, err := handler.Execute(txParams, cccid, incomingMessage, time.Millisecond)
					errCh <- err
				}()
				Eventually(errCh).Should(Receive(MatchError("timeout expired while executing transaction")))
				Expect(fakeExecuteTimeouts.WithCallCount()).To(Equal(1))
				labelValues := fakeExecuteTimeouts.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"chaincode", "chaincode-name:chaincode-version",
				}))
				Expect(fakeExecuteTimeouts.AddCallCount()).To(Equal(1))
				Expect(fakeExecuteTimeouts.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
			})

			It("deletes the transaction context", func() {
				handler.Execute(txParams, cccid, incomingMessage, time.Millisecond)

				Expect(fakeContextRegistry.DeleteCallCount()).Should(Equal(1))
				channelID, txid := fakeContextRegistry.DeleteArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(txid).To(Equal("tx-id"))
			})
		})
	})

	Describe("HandleRegister", func() {
		var incomingMessage *pb.ChaincodeMessage

		BeforeEach(func() {
			chaincode.SetHandlerCCInstance(handler, nil)

			request := &pb.ChaincodeID{
				Name:    "chaincode-id-name",
				Version: "chaincode-id-version",
			}
			payload, err := proto.Marshal(request)
			Expect(err).NotTo(HaveOccurred())

			incomingMessage = &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_REGISTER,
				Txid:      "tx-id",
				ChannelId: "channel-id",
				Payload:   payload,
			}
		})

		It("sets chainodeID on the handler", func() {
			Expect(handler.ChaincodeName()).To(Equal(""))

			handler.HandleRegister(incomingMessage)
			Expect(handler.ChaincodeName()).To(Equal("chaincode-id-name"))
		})

		It("registers the handler with the registry", func() {
			handler.HandleRegister(incomingMessage)

			Expect(fakeHandlerRegistry.RegisterCallCount()).To(Equal(1))
			h := fakeHandlerRegistry.RegisterArgsForCall(0)
			Expect(h).To(Equal(handler))
		})

		It("transitions the handler into ready state", func() {
			handler.HandleRegister(incomingMessage)
			Eventually(handler.State).Should(Equal(chaincode.Ready))
		})

		It("notifies the registry that the handler is ready", func() {
			handler.HandleRegister(incomingMessage)
			Expect(fakeHandlerRegistry.FailedCallCount()).To(Equal(0))
			Expect(fakeHandlerRegistry.ReadyCallCount()).To(Equal(1))
			name := fakeHandlerRegistry.ReadyArgsForCall(0)
			Expect(name).To(Equal("chaincode-id-name"))
		})

		It("sends registered and ready messsages", func() {
			handler.HandleRegister(incomingMessage)

			Eventually(fakeChatStream.SendCallCount).Should(Equal(2))
			Consistently(fakeChatStream.SendCallCount).Should(Equal(2))
			registeredMessage := fakeChatStream.SendArgsForCall(0)
			readyMessage := fakeChatStream.SendArgsForCall(1)

			Expect(registeredMessage).To(Equal(&pb.ChaincodeMessage{
				Type: pb.ChaincodeMessage_REGISTERED,
			}))

			Expect(readyMessage).To(Equal(&pb.ChaincodeMessage{
				Type: pb.ChaincodeMessage_READY,
			}))
		})

		Context("when sending the ready message fails", func() {
			BeforeEach(func() {
				fakeChatStream.SendReturnsOnCall(1, errors.New("carrot"))
			})

			It("state remains in established", func() {
				Expect(handler.State()).To(Equal(chaincode.Created))
				handler.HandleRegister(incomingMessage)
				Expect(handler.State()).To(Equal(chaincode.Established))
			})

			It("notifies the registry of the failure", func() {
				handler.HandleRegister(incomingMessage)
				Expect(fakeHandlerRegistry.ReadyCallCount()).To(Equal(0))
				Expect(fakeHandlerRegistry.FailedCallCount()).To(Equal(1))
				name, err := fakeHandlerRegistry.FailedArgsForCall(0)
				Expect(name).To(Equal("chaincode-id-name"))
				Expect(err).To(MatchError("[] error sending READY: carrot"))
			})
		})

		Context("when registering the handler with registry fails", func() {
			BeforeEach(func() {
				fakeHandlerRegistry.RegisterReturns(errors.New("cake"))
			})

			It("remains in created state", func() {
				Expect(handler.State()).To(Equal(chaincode.Created))
				handler.HandleRegister(incomingMessage)
				Expect(handler.State()).To(Equal(chaincode.Created))
			})
		})

		Context("when sending a registered message failed", func() {
			BeforeEach(func() {
				fakeChatStream.SendReturns(errors.New("potato"))
			})

			It("ready is not sent", func() {
				handler.HandleRegister(incomingMessage)

				Eventually(fakeChatStream.SendCallCount).Should(Equal(1))
				Consistently(fakeChatStream.SendCallCount).Should(Equal(1))
				msg := fakeChatStream.SendArgsForCall(0)
				Expect(msg.Type).To(Equal(pb.ChaincodeMessage_REGISTERED))
			})

			It("remains in created state", func() {
				Expect(handler.State()).To(Equal(chaincode.Created))
				handler.HandleRegister(incomingMessage)
				Expect(handler.State()).To(Equal(chaincode.Created))
			})
		})

		Context("when unmarshaling the request fails", func() {
			BeforeEach(func() {
				incomingMessage.Payload = []byte("this-is-a-bogus-payload")
			})

			It("sends no messages", func() {
				handler.HandleRegister(incomingMessage)
				Consistently(fakeChatStream.SendCallCount).Should(Equal(0))
			})
		})
	})

	Describe("ProcessStream", func() {
		BeforeEach(func() {
			incomingMessage := &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_KEEPALIVE,
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
			fakeChatStream.RecvReturns(incomingMessage, nil)
		})

		It("receives messages until an error is received", func() {
			fakeChatStream.RecvReturnsOnCall(99, nil, errors.New("done-for-now"))
			handler.ProcessStream(fakeChatStream)

			Eventually(fakeChatStream.RecvCallCount).Should(Equal(100))
		})

		It("manages the stream done channel", func() {
			releaseChan := make(chan struct{})
			fakeChatStream.RecvStub = func() (*pb.ChaincodeMessage, error) {
				<-releaseChan
				return nil, errors.New("cc-went-away")
			}
			go handler.ProcessStream(fakeChatStream)
			Eventually(fakeChatStream.RecvCallCount).Should(Equal(1))

			streamDoneChan := chaincode.StreamDone(handler)
			Consistently(streamDoneChan).ShouldNot(Receive())

			close(releaseChan)
			Eventually(streamDoneChan).Should(BeClosed())
		})

		Context("when receive fails with an io.EOF", func() {
			BeforeEach(func() {
				fakeChatStream.RecvReturns(nil, io.EOF)
			})

			It("returns the error", func() {
				err := handler.ProcessStream(fakeChatStream)
				Expect(err).To(Equal(io.EOF))
			})
		})

		Context("when receive fails", func() {
			BeforeEach(func() {
				fakeChatStream.RecvReturns(nil, errors.New("chocolate"))
			})

			It("returns an error", func() {
				err := handler.ProcessStream((fakeChatStream))
				Expect(err).To(MatchError("receive failed: chocolate"))
			})
		})

		Context("when a nil message is received", func() {
			BeforeEach(func() {
				fakeChatStream.RecvReturns(nil, nil)
			})

			It("retuns an error", func() {
				err := handler.ProcessStream(fakeChatStream)
				Expect(err).To(MatchError("received nil message, ending chaincode support stream"))
			})
		})

		Describe("keepalive messages", func() {
			var recvChan chan *pb.ChaincodeMessage

			BeforeEach(func() {
				recvChan = make(chan *pb.ChaincodeMessage, 1)
				fakeChatStream.RecvStub = func() (*pb.ChaincodeMessage, error) {
					msg := <-recvChan
					return msg, nil
				}

				handler.Keepalive = 50 * time.Millisecond
			})

			It("sends a keep alive messages until the stream ends", func() {
				errChan := make(chan error, 1)
				go func() { errChan <- handler.ProcessStream(fakeChatStream) }()

				Eventually(fakeChatStream.SendCallCount).Should(Equal(5))
				recvChan <- nil
				Eventually(errChan).Should(Receive())

				for i := 0; i < 5; i++ {
					m := fakeChatStream.SendArgsForCall(i)
					Expect(m.Type).To(Equal(pb.ChaincodeMessage_KEEPALIVE))
				}
			})

			Context("when keepalive is disabled", func() {
				BeforeEach(func() {
					handler.Keepalive = 0
				})

				It("does not send keep alive messages", func() {
					errChan := make(chan error, 1)
					go func() { errChan <- handler.ProcessStream(fakeChatStream) }()

					Consistently(fakeChatStream.SendCallCount).Should(Equal(0))
					recvChan <- nil
					Eventually(errChan).Should(Receive())
				})
			})
		})

		Context("when handling a received message fails", func() {
			var recvChan chan *pb.ChaincodeMessage

			BeforeEach(func() {
				recvChan = make(chan *pb.ChaincodeMessage, 1)
				fakeChatStream.RecvStub = func() (*pb.ChaincodeMessage, error) {
					msg := <-recvChan
					return msg, nil
				}

				recvChan <- &pb.ChaincodeMessage{
					Txid: "tx-id",
					Type: pb.ChaincodeMessage_Type(9999),
				}
			})

			It("returns an error", func() {
				err := handler.ProcessStream(fakeChatStream)
				Expect(err).To(MatchError("error handling message, ending stream: [tx-id] Fabric side handler cannot handle message (9999) while in created state"))
			})
		})

		Context("when an async error is sent", func() {
			var (
				cccid           *ccprovider.CCContext
				incomingMessage *pb.ChaincodeMessage
				recvChan        chan *pb.ChaincodeMessage
			)

			BeforeEach(func() {
				request := &pb.ChaincodeInput{}
				payload, err := proto.Marshal(request)
				Expect(err).NotTo(HaveOccurred())

				cccid = &ccprovider.CCContext{
					Name:    "chaincode-name",
					Version: "chaincode-version",
				}

				incomingMessage = &pb.ChaincodeMessage{
					Type:      pb.ChaincodeMessage_TRANSACTION,
					Txid:      "tx-id",
					Payload:   payload,
					ChannelId: "channel-id",
				}

				recvChan = make(chan *pb.ChaincodeMessage, 1)
				shadowRecvChan := recvChan
				fakeChatStream.RecvStub = func() (*pb.ChaincodeMessage, error) {
					msg := <-shadowRecvChan
					return msg, nil
				}
				fakeChatStream.SendReturns(errors.New("candy"))
			})

			AfterEach(func() {
				close(recvChan)
			})

			It("returns an error", func() {
				errChan := make(chan error, 1)
				go func() { errChan <- handler.ProcessStream(fakeChatStream) }()
				Eventually(fakeChatStream.RecvCallCount).ShouldNot(Equal(0))                               // wait for loop to start
				handler.Execute(&ccprovider.TransactionParams{}, cccid, incomingMessage, time.Millisecond) // force async error

				Eventually(errChan).Should(Receive(MatchError("received error while sending message, ending chaincode support stream: [tx-id] error sending TRANSACTION: candy")))
			})

			It("stops receiving messages", func() {
				errChan := make(chan error, 1)
				go func() { errChan <- handler.ProcessStream(fakeChatStream) }()
				Eventually(fakeChatStream.RecvCallCount).ShouldNot(Equal(0))                               // wait for loop to start
				handler.Execute(&ccprovider.TransactionParams{}, cccid, incomingMessage, time.Millisecond) // force async error

				Eventually(fakeChatStream.RecvCallCount).Should(Equal(1))
				Consistently(fakeChatStream.RecvCallCount).Should(Equal(1))
			})
		})
	})

	Describe("Notify", func() {
		var fakeIterator *mock.QueryResultsIterator
		var incomingMessage *pb.ChaincodeMessage

		BeforeEach(func() {
			fakeIterator = &mock.QueryResultsIterator{}
			incomingMessage = &pb.ChaincodeMessage{
				Txid:      "tx-id",
				ChannelId: "channel-id",
			}
		})

		It("gets the transaction context", func() {
			handler.Notify(incomingMessage)

			Expect(fakeContextRegistry.GetCallCount()).To(Equal(1))
			channelID, txID := fakeContextRegistry.GetArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(txID).To(Equal("tx-id"))
		})

		It("sends the message on response notifier", func() {
			handler.Notify(incomingMessage)

			Eventually(responseNotifier).Should(Receive(Equal(incomingMessage)))
		})

		It("should close query iterators on the transaction context", func() {
			txContext.InitializeQueryContext("query-id", fakeIterator)
			Expect(fakeIterator.CloseCallCount()).To(Equal(0))

			handler.Notify(incomingMessage)
			Eventually(fakeIterator.CloseCallCount).Should(Equal(1))
		})

		Context("when the transaction context cannot be found", func() {
			BeforeEach(func() {
				fakeContextRegistry.GetReturns(nil)
			})

			It("keeps calm and carries on", func() {
				handler.Notify(incomingMessage)
				Expect(fakeContextRegistry.GetCallCount()).To(Equal(1))
			})
		})
	})

	Describe("ParseName", func() {
		It("parses the chaincode name", func() {
			ci := chaincode.ParseName("name")
			Expect(ci).To(Equal(&sysccprovider.ChaincodeInstance{ChaincodeName: "name"}))
			ci = chaincode.ParseName("name:version")
			Expect(ci).To(Equal(&sysccprovider.ChaincodeInstance{ChaincodeName: "name", ChaincodeVersion: "version"}))
			ci = chaincode.ParseName("name/chain-id")
			Expect(ci).To(Equal(&sysccprovider.ChaincodeInstance{ChaincodeName: "name", ChainID: "chain-id"}))
			ci = chaincode.ParseName("name:version/chain-id")
			Expect(ci).To(Equal(&sysccprovider.ChaincodeInstance{ChaincodeName: "name", ChaincodeVersion: "version", ChainID: "chain-id"}))
		})
	})

	DescribeTable("Handler State",
		func(state chaincode.State, strval string) {
			Expect(state.String()).To(Equal(strval))
		},
		Entry("created", chaincode.Created, "created"),
		Entry("ready", chaincode.Ready, "ready"),
		Entry("established", chaincode.Established, "established"),
		Entry("unknown", chaincode.State(999), "UNKNOWN"),
	)
})
