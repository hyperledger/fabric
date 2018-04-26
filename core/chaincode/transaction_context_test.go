/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TransactionContext", func() {
	var (
		resultsIterator    *mock.ResultsIterator
		transactionContext *chaincode.TransactionContext
	)

	BeforeEach(func() {
		resultsIterator = &mock.ResultsIterator{}
		transactionContext = &chaincode.TransactionContext{}
	})

	Describe("InitializeQueryContext", func() {
		var iter1, iter2 *mock.ResultsIterator

		BeforeEach(func() {
			iter1 = &mock.ResultsIterator{}
			iter2 = &mock.ResultsIterator{}
		})

		It("stores a references to the results iterator", func() {
			transactionContext.InitializeQueryContext("query-id-1", iter1)
			transactionContext.InitializeQueryContext("query-id-2", iter2)

			iter := transactionContext.GetQueryIterator("query-id-1")
			Expect(iter).To(Equal(iter1))
			iter = transactionContext.GetQueryIterator("query-id-2")
			Expect(iter).To(Equal(iter2))
		})

		It("populates a pending query result", func() {
			transactionContext.InitializeQueryContext("query-id", iter1)
			pqr := transactionContext.GetPendingQueryResult("query-id")

			Expect(pqr).To(Equal(&chaincode.PendingQueryResult{}))
		})
	})

	Describe("GetQueryIterator", func() {
		It("returns the results iteraterator provided to initialize query context", func() {
			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			iter := transactionContext.GetQueryIterator("query-id")
			Expect(iter).To(Equal(resultsIterator))

			transactionContext.InitializeQueryContext("query-with-nil", nil)
			iter = transactionContext.GetQueryIterator("query-with-nil")
			Expect(iter).To(BeNil())
		})

		Context("when an unknown query id is used", func() {
			It("returns a nil query iterator", func() {
				iter := transactionContext.GetQueryIterator("unknown-id")
				Expect(iter).To(BeNil())
			})
		})
	})

	Describe("GetPendingQueryResult", func() {
		Context("when a query context has been initialized", func() {
			BeforeEach(func() {
				transactionContext.InitializeQueryContext("query-id", nil)
			})

			It("returns a non-nil pending query result", func() {
				pqr := transactionContext.GetPendingQueryResult("query-id")
				Expect(pqr).To(Equal(&chaincode.PendingQueryResult{}))
			})
		})

		Context("when a query context has not been initialized", func() {
			It("returns a nil pending query result", func() {
				pqr := transactionContext.GetPendingQueryResult("query-id")
				Expect(pqr).To(BeNil())
			})
		})
	})

	Describe("CleanupQueryContext", func() {
		It("removes references to the the iterator and results", func() {
			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			transactionContext.CleanupQueryContext("query-id")

			iter := transactionContext.GetQueryIterator("query-id")
			Expect(iter).To(BeNil())
			pqr := transactionContext.GetPendingQueryResult("query-id")
			Expect(pqr).To(BeNil())
		})

		It("closes the query iterator", func() {
			transactionContext.InitializeQueryContext("query-id", resultsIterator)
			transactionContext.CleanupQueryContext("query-id")

			Expect(resultsIterator.CloseCallCount()).To(Equal(1))
		})

		Context("when the query iterator is nil", func() {
			It("keeps calm and carries on", func() {
				transactionContext.InitializeQueryContext("query-id", nil)
				transactionContext.CleanupQueryContext("query-id")

				pqr := transactionContext.GetPendingQueryResult("query-id")
				Expect(pqr).To(BeNil())
			})
		})
	})

	Describe("CloseQueryIterators", func() {
		var resultsIterators []*mock.ResultsIterator

		BeforeEach(func() {
			for i := 0; i < 5; i++ {
				resultsIterators = append(resultsIterators, &mock.ResultsIterator{})
				transactionContext.InitializeQueryContext(fmt.Sprintf("query-id-%d", i+1), resultsIterators[i])
			}
		})

		It("closes all initialized results iterators", func() {
			transactionContext.CloseQueryIterators()
			for _, iter := range resultsIterators {
				Expect(iter.CloseCallCount()).To(Equal(1))
			}
		})
	})
})
