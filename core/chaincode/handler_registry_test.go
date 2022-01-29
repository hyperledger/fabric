/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("HandlerRegistry", func() {
	var hr *chaincode.HandlerRegistry
	var handler *chaincode.Handler

	BeforeEach(func() {
		hr = chaincode.NewHandlerRegistry(true)
		handler = &chaincode.Handler{}
		chaincode.SetHandlerChaincodeID(handler, "chaincode-id")
	})

	Describe("Launching", func() {
		It("returns a LaunchState to wait on for registration", func() {
			launchState, _ := hr.Launching("chaincode-id")
			Consistently(launchState.Done()).ShouldNot(Receive())
			Consistently(launchState.Done()).ShouldNot(BeClosed())
		})

		It("indicates whether or not the chaincode needs to start", func() {
			_, started := hr.Launching("chaincode-id")
			Expect(started).To(BeFalse())
		})

		Context("when a chaincode instance is already launching", func() {
			BeforeEach(func() {
				_, started := hr.Launching("chaincode-id")
				Expect(started).To(BeFalse())
			})

			It("returns a LaunchState", func() {
				launchState, _ := hr.Launching("chaincode-id")
				Consistently(launchState.Done()).ShouldNot(Receive())
				Consistently(launchState.Done()).ShouldNot(BeClosed())
			})

			It("indicates already started", func() {
				_, started := hr.Launching("chaincode-id")
				Expect(started).To(BeTrue())
			})
		})

		Context("when a handler has already been registered", func() {
			BeforeEach(func() {
				err := hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a ready LaunchState", func() {
				launchState, _ := hr.Launching("chaincode-id")
				Expect(launchState.Done()).To(BeClosed())
				Expect(launchState.Err()).NotTo(HaveOccurred())
			})

			It("indicates the chaincode has already been started", func() {
				_, started := hr.Launching("chaincode-id")
				Expect(started).To(BeTrue())
			})
		})
	})

	Describe("Ready", func() {
		var launchState *chaincode.LaunchState

		BeforeEach(func() {
			launchState, _ = hr.Launching("chaincode-id")
			Expect(launchState.Done()).NotTo(BeClosed())
		})

		It("closes the done channel associated with the chaincode id", func() {
			hr.Ready("chaincode-id")
			Expect(launchState.Done()).To(BeClosed())
		})

		It("does not set an error on launch state", func() {
			hr.Ready("chaincode-id")
			Expect(launchState.Err()).To(BeNil())
		})

		It("leaves the launching state in the registry", func() {
			hr.Ready("chaincode-id")
			ls, exists := hr.Launching("chaincode-id")
			Expect(exists).To(BeTrue())
			Expect(ls).To(BeIdenticalTo(launchState))
		})
	})

	Describe("Failed", func() {
		var launchState *chaincode.LaunchState

		BeforeEach(func() {
			launchState, _ = hr.Launching("chaincode-id")
			Expect(launchState.Done()).NotTo(BeClosed())
		})

		It("closes the done channel associated with the chaincode id", func() {
			hr.Failed("chaincode-id", errors.New("coconut"))
			Expect(launchState.Done()).To(BeClosed())
		})

		It("sets a persistent error on launch state", func() {
			hr.Failed("chaincode-id", errors.New("star-fruit"))
			Expect(launchState.Err()).To(MatchError("star-fruit"))
			Expect(launchState.Err()).To(MatchError("star-fruit"))
		})

		It("leaves the launching state in the registry for explicit cleanup", func() {
			hr.Failed("chaincode-id", errors.New("mango"))
			ls, exists := hr.Launching("chaincode-id")
			Expect(exists).To(BeTrue())
			Expect(ls).To(BeIdenticalTo(launchState))
		})
	})

	Describe("Handler", func() {
		BeforeEach(func() {
			err := hr.Register(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the registered handler", func() {
			h := hr.Handler("chaincode-id")
			Expect(h).To(BeIdenticalTo(handler))
		})

		Context("when a handler has not been registered for the chaincode", func() {
			It("returns nil when a handler has not bee registered", func() {
				h := hr.Handler("unregistered-handler-name")
				Expect(h).To(BeNil())
			})
		})
	})

	Describe("Register", func() {
		Context("when unsolicited registration is disallowed", func() {
			BeforeEach(func() {
				hr = chaincode.NewHandlerRegistry(false)
			})

			It("disallows direct registrion without launching", func() {
				err := hr.Register(handler)
				Expect(err).To(MatchError(`peer will not accept external chaincode connection chaincode-id (except in dev mode)`))

				h := hr.Handler("chaincode-id")
				Expect(h).To(BeNil())
			})

			It("allows registration of launching chaincode", func() {
				_, started := hr.Launching("chaincode-id")
				Expect(started).To(BeFalse())

				err := hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())

				h := hr.Handler("chaincode-id")
				Expect(h).To(Equal(handler))
			})
		})

		Context("when unsolicited registrations are allowed", func() {
			BeforeEach(func() {
				hr = chaincode.NewHandlerRegistry(true)
			})

			It("allows direct registration without launching", func() {
				err := hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())

				h := hr.Handler("chaincode-id")
				Expect(h).To(BeIdenticalTo(handler))
			})
		})

		Context("when a handler has already been registered", func() {
			BeforeEach(func() {
				hr = chaincode.NewHandlerRegistry(true)
				err := hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				err := hr.Register(handler)
				Expect(err).To(MatchError("duplicate chaincodeID: chaincode-id"))
			})
		})
	})

	Describe("Deregister", func() {
		var fakeResultsIterator *mock.QueryResultsIterator

		BeforeEach(func() {
			fakeResultsIterator = &mock.QueryResultsIterator{}
			transactionContexts := chaincode.NewTransactionContexts()

			txContext, err := transactionContexts.Create(&ccprovider.TransactionParams{
				ChannelID: "chain-id",
				TxID:      "transaction-id",
			})
			Expect(err).NotTo(HaveOccurred())

			handler.TXContexts = transactionContexts
			txContext.InitializeQueryContext("query-id", fakeResultsIterator)

			_, started := hr.Launching("chaincode-id")
			Expect(started).To(BeFalse())

			err = hr.Register(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes references to the handler", func() {
			err := hr.Deregister("chaincode-id")
			Expect(err).NotTo(HaveOccurred())

			handler := hr.Handler("chaincode-id")
			Expect(handler).To(BeNil())
			_, exists := hr.Launching("chaincode-id")
			Expect(exists).To(BeFalse())
		})

		It("closes transaction contexts", func() {
			err := hr.Deregister("chaincode-id")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeResultsIterator.CloseCallCount()).To(Equal(1))
		})
	})
})

var _ = Describe("LaunchState", func() {
	var launchState *chaincode.LaunchState

	BeforeEach(func() {
		launchState = chaincode.NewLaunchState()
	})

	It("coordinates notification and errors", func() {
		Expect(launchState.Done()).NotTo(BeNil())
		Consistently(launchState.Done()).ShouldNot(BeClosed())

		launchState.Notify(errors.New("jelly"))
		Eventually(launchState.Done()).Should(BeClosed())
		Expect(launchState.Err()).To(MatchError("jelly"))
	})

	It("can notify with a nil error", func() {
		Expect(launchState.Done()).NotTo(BeNil())
		Consistently(launchState.Done()).ShouldNot(BeClosed())

		launchState.Notify(nil)
		Eventually(launchState.Done()).Should(BeClosed())
		Expect(launchState.Err()).To(BeNil())
	})

	It("can be notified multitple times but honors the first", func() {
		Expect(launchState.Done()).NotTo(BeNil())
		Consistently(launchState.Done()).ShouldNot(BeClosed())

		launchState.Notify(errors.New("mango"))
		launchState.Notify(errors.New("tango"))
		launchState.Notify(errors.New("django"))
		launchState.Notify(nil)
		Eventually(launchState.Done()).Should(BeClosed())
		Expect(launchState.Err()).To(MatchError("mango"))
	})
})

var _ = Describe("TxSimulatorGetter", func() {
	var (
		hr              *chaincode.HandlerRegistry
		handler         *chaincode.Handler
		txQEGetter      *chaincode.TxQueryExecutorGetter
		fakeTxSimulator *mock.TxSimulator
	)

	BeforeEach(func() {
		hr = chaincode.NewHandlerRegistry(true)
		handler = &chaincode.Handler{
			TXContexts: chaincode.NewTransactionContexts(),
		}
		fakeTxSimulator = &mock.TxSimulator{}
		txQEGetter = &chaincode.TxQueryExecutorGetter{
			HandlerRegistry: hr,
			CCID:            "chaincode-id",
		}
	})

	When("No Handler is created for chaincode-id", func() {
		It("returns a nil TxSimulator", func() {
			sim := txQEGetter.TxQueryExecutor("channel-ID", "tx-ID")
			Expect(sim).To(BeNil())
		})
	})

	When("No TxContext is created", func() {
		BeforeEach(func() {
			chaincode.SetHandlerChaincodeID(handler, "chaincode-id")
			hr.Register(handler)
		})

		It("returns a nil TxSimulator", func() {
			sim := txQEGetter.TxQueryExecutor("channel-ID", "tx-ID")
			Expect(sim).To(BeNil())
		})
	})

	When("Handler is created for chaincode-id and TxContext is created for channel-ID/tx-ID", func() {
		BeforeEach(func() {
			chaincode.SetHandlerChaincodeID(handler, "chaincode-id")
			hr.Register(handler)
			handler.TXContexts.Create(&ccprovider.TransactionParams{
				ChannelID:   "channel-ID",
				TxID:        "tx-ID",
				TXSimulator: fakeTxSimulator,
			})
		})
		It("returns associated TxSimulator", func() {
			sim := txQEGetter.TxQueryExecutor("channel-ID", "tx-ID")
			Expect(sim).To(Equal(fakeTxSimulator))
		})
	})
})
