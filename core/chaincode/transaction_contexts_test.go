/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
)

var _ = Describe("TransactionContexts", func() {
	var txContexts *TransactionContexts

	BeforeEach(func() {
		txContexts = NewTransactionContexts()
	})

	Describe("Create", func() {
		var (
			signedProp               *pb.SignedProposal
			proposal                 *pb.Proposal
			fakeTxSimulator          *mock.TxSimulator
			fakeHistoryQueryExecutor *mock.HistoryQueryExecutor

			ctx context.Context
		)

		BeforeEach(func() {
			signedProp = &pb.SignedProposal{ProposalBytes: []byte("some-proposal-bytes")}
			proposal = &pb.Proposal{Payload: []byte("some-payload-bytes")}
			fakeTxSimulator = &mock.TxSimulator{}
			fakeHistoryQueryExecutor = &mock.HistoryQueryExecutor{}

			ctx = context.Background()
			ctx = context.WithValue(ctx, TXSimulatorKey, fakeTxSimulator)
			ctx = context.WithValue(ctx, HistoryQueryExecutorKey, fakeHistoryQueryExecutor)
		})

		It("creates a new transaction context", func() {
			txContext, err := txContexts.Create(ctx, "chainID", "transactionID", signedProp, proposal)
			Expect(err).NotTo(HaveOccurred())

			Expect(txContext.chainID).To(Equal("chainID"))
			Expect(txContext.signedProp).To(Equal(signedProp))
			Expect(txContext.proposal).To(Equal(proposal))
			Expect(txContext.responseNotifier).NotTo(BeNil())
			Expect(txContext.responseNotifier).NotTo(BeClosed())
			Expect(txContext.queryIteratorMap).To(Equal(map[string]commonledger.ResultsIterator{}))
			Expect(txContext.pendingQueryResults).To(Equal(map[string]*pendingQueryResult{}))
			Expect(txContext.txsimulator).To(Equal(fakeTxSimulator))
			Expect(txContext.historyQueryExecutor).To(Equal(fakeHistoryQueryExecutor))
		})

		It("keeps track of the created context", func() {
			txContext, err := txContexts.Create(ctx, "chainID", "transactionID", signedProp, proposal)
			Expect(err).NotTo(HaveOccurred())

			c := txContexts.Get("chainID", "transactionID")
			Expect(c).To(Equal(txContext))
		})

		Context("when the transaction context already exists", func() {
			BeforeEach(func() {
				_, err := txContexts.Create(ctx, "chainID", "transactionID", nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns a meaningful error", func() {
				_, err := txContexts.Create(ctx, "chainID", "transactionID", nil, nil)
				Expect(err).To(MatchError("txid: transactionID(chainID) exists"))
			})
		})
	})

	Describe("Get", func() {
		var c1, c2 *TransactionContext

		BeforeEach(func() {
			var err error
			c1, err = txContexts.Create(context.Background(), "chainID1", "transactionID1", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			c2, err = txContexts.Create(context.Background(), "chainID1", "transactionID2", nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the correct transaction context", func() {
			c := txContexts.Get("chainID1", "transactionID1")
			Expect(c).To(Equal(c1))

			c = txContexts.Get("chainID1", "transactionID2")
			Expect(c).To(Equal(c2))

			c = txContexts.Get("non-existent", "transactionID1")
			Expect(c).To(BeNil())
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			_, err := txContexts.Create(context.Background(), "chainID2", "transactionID1", nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes transaction context", func() {
			c := txContexts.Get("chainID2", "transactionID1")
			Expect(c).NotTo(BeNil())

			txContexts.Delete("chainID2", "transactionID1")

			c = txContexts.Get("chainID2", "transactionID1")
			Expect(c).To(BeNil())
		})

		Context("when the context doesn't exist", func() {
			It("keeps calm and carries on", func() {
				txContexts.Delete("not-existent", "transactionID1")
			})
		})
	})

	Describe("Close", func() {
		var fakeIterators []*mock.ResultsIterator

		BeforeEach(func() {
			fakeIterators = make([]*mock.ResultsIterator, 6)
			for i := 0; i < len(fakeIterators); i++ {
				fakeIterators[i] = &mock.ResultsIterator{}
			}

			txContext, err := txContexts.Create(context.Background(), "chainID", "transactionID", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			txContext.queryIteratorMap["key1"] = fakeIterators[0]
			txContext.queryIteratorMap["key2"] = fakeIterators[1]
			txContext.queryIteratorMap["key3"] = fakeIterators[2]

			txContext2, err := txContexts.Create(context.Background(), "chainID", "transactionID2", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			txContext2.queryIteratorMap["key1"] = fakeIterators[3]
			txContext2.queryIteratorMap["key2"] = fakeIterators[4]
			txContext2.queryIteratorMap["key3"] = fakeIterators[5]
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
				txContexts = NewTransactionContexts()
			})

			It("keeps calm and carries on", func() {
				txContexts.Close()
			})
		})

		Context("when there is a context with no query iterators", func() {
			BeforeEach(func() {
				_, err := txContexts.Create(context.Background(), "chainID", "no-query-iterators", nil, nil)
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
