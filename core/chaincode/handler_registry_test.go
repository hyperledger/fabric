/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var _ = Describe("HandlerRegistry", func() {
	var hr *chaincode.HandlerRegistry
	var handler *chaincode.Handler

	BeforeEach(func() {
		hr = chaincode.NewHandlerRegistry(true)
		handler = &chaincode.Handler{}
		chaincode.SetHandlerChaincodeID(handler, &pb.ChaincodeID{Name: "chaincode-name"})
	})

	Describe("HasLaunched", func() {
		It("returns false when not launched or registered", func() {
			launched := hr.HasLaunched("chaincode-name")
			Expect(launched).To(BeFalse())
		})

		It("returns true when launching", func() {
			_, err := hr.Launching("chaincode-name")
			Expect(err).NotTo(HaveOccurred())

			launched := hr.HasLaunched("chaincode-name")
			Expect(launched).To(BeTrue())
		})

		It("returns true when registered", func() {
			err := hr.Register(handler)
			Expect(err).NotTo(HaveOccurred())

			launched := hr.HasLaunched("chaincode-name")
			Expect(launched).To(BeTrue())
		})
	})

	Describe("Launching", func() {
		It("returns a LaunchState to wait on for registration", func() {
			launchState, err := hr.Launching("chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Consistently(launchState.Done()).ShouldNot(Receive())
			Consistently(launchState.Done()).ShouldNot(BeClosed())
		})

		Context("when a chaincode instance is already launching", func() {
			BeforeEach(func() {
				_, err := hr.Launching("chaincode-name")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				_, err := hr.Launching("chaincode-name")
				Expect(err).To(MatchError("chaincode chaincode-name has already been launched"))
			})
		})

		Context("when a handler has already been registered", func() {
			BeforeEach(func() {
				err := hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				_, err := hr.Launching("chaincode-name")
				Expect(err).To(MatchError("chaincode chaincode-name has already been launched"))
			})
		})
	})

	Describe("Ready", func() {
		var launchState *chaincode.LaunchState

		BeforeEach(func() {
			var err error
			launchState, err = hr.Launching("chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(launchState.Done()).NotTo(BeClosed())
		})

		It("closes the done channel associated with the chaincode name", func() {
			hr.Ready("chaincode-name")
			Expect(launchState.Done()).To(BeClosed())
		})

		It("does not set an error on launch state", func() {
			hr.Ready("chaincode-name")
			Expect(launchState.Err()).To(BeNil())
		})

		It("cleans up the launching state", func() {
			hr.Ready("chaincode-name")
			launching := hr.HasLaunched("chaincode-name")
			Expect(launching).To(BeFalse())
		})
	})

	Describe("Failed", func() {
		var launchState *chaincode.LaunchState

		BeforeEach(func() {
			var err error
			launchState, err = hr.Launching("chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(launchState.Done()).NotTo(BeClosed())
		})

		It("closes the done channel associated with the chaincode name", func() {
			hr.Failed("chaincode-name", errors.New("coconut"))
			Expect(launchState.Done()).To(BeClosed())
		})

		It("sets a persistent error on launch state", func() {
			hr.Failed("chaincode-name", errors.New("star-fruit"))
			Expect(launchState.Err()).To(MatchError("star-fruit"))
			Expect(launchState.Err()).To(MatchError("star-fruit"))
		})

		It("leaves the launching state in the registry for explicit cleanup", func() {
			hr.Failed("chaincode-name", errors.New("mango"))
			launching := hr.HasLaunched("chaincode-name")
			Expect(launching).To(BeTrue())
		})
	})

	Describe("Handler", func() {
		BeforeEach(func() {
			err := hr.Register(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the registered handler", func() {
			h := hr.Handler("chaincode-name")
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
				Expect(err).To(MatchError(`peer will not accept external chaincode connection chaincode-name (except in dev mode)`))

				h := hr.Handler("chaincode-name")
				Expect(h).To(BeNil())
			})

			It("allows registration of launching chaincode", func() {
				_, err := hr.Launching("chaincode-name")
				Expect(err).NotTo(HaveOccurred())

				err = hr.Register(handler)
				Expect(err).NotTo(HaveOccurred())

				h := hr.Handler("chaincode-name")
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

				h := hr.Handler("chaincode-name")
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
				Expect(err).To(MatchError("duplicate chaincodeID: chaincode-name"))
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

			_, err = hr.Launching("chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			err = hr.Register(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes references to the handler", func() {
			err := hr.Deregister("chaincode-name")
			Expect(err).NotTo(HaveOccurred())

			launched := hr.HasLaunched("chaincode-name")
			Expect(launched).To(BeFalse())
		})

		It("closes transaction contexts", func() {
			err := hr.Deregister("chaincode-name")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeResultsIterator.CloseCallCount()).To(Equal(1))
		})
	})
})
