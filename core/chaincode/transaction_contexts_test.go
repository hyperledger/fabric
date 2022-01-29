/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TransactionContexts", func() {
	var txContexts *chaincode.TransactionContexts

	BeforeEach(func() {
		txContexts = chaincode.NewTransactionContexts()
	})

	Describe("Create", func() {
		var (
			signedProp               *pb.SignedProposal
			proposal                 *pb.Proposal
			fakeTxSimulator          *mock.TxSimulator
			fakeHistoryQueryExecutor *mock.HistoryQueryExecutor

			txParams *ccprovider.TransactionParams
		)

		BeforeEach(func() {
			signedProp = &pb.SignedProposal{ProposalBytes: []byte("some-proposal-bytes")}
			proposal = &pb.Proposal{Payload: []byte("some-payload-bytes")}
			fakeTxSimulator = &mock.TxSimulator{}
			fakeHistoryQueryExecutor = &mock.HistoryQueryExecutor{}

			txParams = &ccprovider.TransactionParams{
				ChannelID:            "channelID",
				TxID:                 "transactionID",
				SignedProp:           signedProp,
				Proposal:             proposal,
				TXSimulator:          fakeTxSimulator,
				HistoryQueryExecutor: fakeHistoryQueryExecutor,
			}
		})

		It("creates a new transaction context", func() {
			txContext, err := txContexts.Create(txParams)
			Expect(err).NotTo(HaveOccurred())

			Expect(txContext.ChannelID).To(Equal("channelID"))
			Expect(txContext.SignedProp).To(Equal(signedProp))
			Expect(txContext.Proposal).To(Equal(proposal))
			Expect(txContext.ResponseNotifier).NotTo(BeNil())
			Expect(txContext.ResponseNotifier).NotTo(BeClosed())
			Expect(txContext.TXSimulator).To(Equal(fakeTxSimulator))
			Expect(txContext.HistoryQueryExecutor).To(Equal(fakeHistoryQueryExecutor))
		})

		It("keeps track of the created context", func() {
			txContext, err := txContexts.Create(txParams)
			Expect(err).NotTo(HaveOccurred())

			c := txContexts.Get("channelID", "transactionID")
			Expect(c).To(Equal(txContext))
		})

		Context("when the transaction context already exists", func() {
			BeforeEach(func() {
				_, err := txContexts.Create(txParams)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a meaningful error", func() {
				_, err := txContexts.Create(txParams)
				Expect(err).To(MatchError("txid: transactionID(channelID) exists"))
			})
		})
	})

	Describe("Get", func() {
		var c1, c2 *chaincode.TransactionContext

		BeforeEach(func() {
			var err error
			txParams1 := &ccprovider.TransactionParams{
				ChannelID: "channelID1",
				TxID:      "transactionID1",
			}
			c1, err = txContexts.Create(txParams1)
			Expect(err).NotTo(HaveOccurred())

			txParams2 := &ccprovider.TransactionParams{
				ChannelID: "channelID1",
				TxID:      "transactionID2",
			}
			c2, err = txContexts.Create(txParams2)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the correct transaction context", func() {
			c := txContexts.Get("channelID1", "transactionID1")
			Expect(c).To(Equal(c1))

			c = txContexts.Get("channelID1", "transactionID2")
			Expect(c).To(Equal(c2))

			c = txContexts.Get("non-existent", "transactionID1")
			Expect(c).To(BeNil())
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			_, err := txContexts.Create(&ccprovider.TransactionParams{
				ChannelID: "channelID2",
				TxID:      "transactionID1",
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes transaction context", func() {
			c := txContexts.Get("channelID2", "transactionID1")
			Expect(c).NotTo(BeNil())

			txContexts.Delete("channelID2", "transactionID1")

			c = txContexts.Get("channelID2", "transactionID1")
			Expect(c).To(BeNil())
		})

		Context("when the context doesn't exist", func() {
			It("keeps calm and carries on", func() {
				txContexts.Delete("not-existent", "transactionID1")
			})
		})
	})

	Describe("Close", func() {
		var fakeIterators []*mock.QueryResultsIterator

		BeforeEach(func() {
			fakeIterators = make([]*mock.QueryResultsIterator, 6)
			for i := 0; i < len(fakeIterators); i++ {
				fakeIterators[i] = &mock.QueryResultsIterator{}
			}

			txContext, err := txContexts.Create(&ccprovider.TransactionParams{
				ChannelID: "channelID",
				TxID:      "transactionID",
			})
			Expect(err).NotTo(HaveOccurred())
			txContext.InitializeQueryContext("key1", fakeIterators[0])
			txContext.InitializeQueryContext("key2", fakeIterators[1])
			txContext.InitializeQueryContext("key3", fakeIterators[2])

			txContext2, err := txContexts.Create(&ccprovider.TransactionParams{
				ChannelID: "channelID",
				TxID:      "transactionID2",
			})
			Expect(err).NotTo(HaveOccurred())
			txContext2.InitializeQueryContext("key1", fakeIterators[3])
			txContext2.InitializeQueryContext("key2", fakeIterators[4])
			txContext2.InitializeQueryContext("key3", fakeIterators[5])
		})

		It("closes all iterators in iterator map", func() {
			for _, ri := range fakeIterators {
				Expect(ri.CloseCallCount()).To(Equal(0))
			}
			txContexts.Close()
			for _, ri := range fakeIterators {
				Expect(ri.CloseCallCount()).To(Equal(1))
			}
		})

		Context("when there are no contexts", func() {
			BeforeEach(func() {
				txContexts = chaincode.NewTransactionContexts()
			})

			It("keeps calm and carries on", func() {
				txContexts.Close()
			})
		})

		Context("when there is a context with no query iterators", func() {
			BeforeEach(func() {
				_, err := txContexts.Create(&ccprovider.TransactionParams{
					ChannelID: "channelID",
					TxID:      "no-query-iterators",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("closes iterators associated with other contexts", func() {
				txContexts.Close()
				for _, ri := range fakeIterators {
					Expect(ri.CloseCallCount()).To(Equal(1))
				}
			})
		})
	})
})
