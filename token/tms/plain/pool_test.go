/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"io"

	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MemoryPool", func() {
	var (
		transactionData []tms.TransactionData

		memoryPool *plain.MemoryPool
	)

	BeforeEach(func() {
		memoryPool = plain.NewMemoryPool()

		transactionData = []tms.TransactionData{{
			Tx: &token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{
							PlainImport: &token.PlainImport{
								Outputs: []*token.PlainOutput{
									{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111},
									{Owner: []byte("owner-2"), Type: "TOK1", Quantity: 222},
								},
							},
						},
					},
				},
			},
			TxID: "0",
		}, {
			Tx: &token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{
							PlainImport: &token.PlainImport{
								Outputs: []*token.PlainOutput{
									{Owner: []byte("owner-1"), Type: "TOK2", Quantity: 111},
									{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222},
								},
							},
						},
					},
				},
			},
			TxID: "1",
		}}
		Expect(transactionData).NotTo(BeNil())
	})

	Describe("import", func() {
		It("checks and commits", func() {
			By("checking and committing the import transaction")
			err := memoryPool.CommitUpdate(transactionData)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring the outputs are in the pool")
			po, err := memoryPool.OutputByID("0.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111}))
			po, err = memoryPool.OutputByID("0.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{Owner: []byte("owner-2"), Type: "TOK1", Quantity: 222}))
			po, err = memoryPool.OutputByID("1.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{Owner: []byte("owner-1"), Type: "TOK2", Quantity: 111}))
			po, err = memoryPool.OutputByID("1.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222}))

			By("ensuring the transactions are recorded")
			tt, err := memoryPool.TxByID("0")
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(transactionData[0].Tx))
			tt, err = memoryPool.TxByID("1")
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(transactionData[1].Tx))
		})

		Context("when an import transaction contains an existing output", func() {
			BeforeEach(func() {
				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).To(MatchError("check update failed: pool entry already exists: 0.0"))
			})
		})

		Context("when an import transaction ID already exists", func() {
			BeforeEach(func() {
				transactionData = []tms.TransactionData{{
					TxID: "0",
					Tx: &token.TokenTransaction{
						Action: &token.TokenTransaction_PlainAction{
							PlainAction: &token.PlainTokenAction{
								Data: &token.PlainTokenAction_PlainImport{
									PlainImport: &token.PlainImport{Outputs: []*token.PlainOutput{}},
								},
							},
						},
					},
				}}

				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).To(MatchError("transaction already exists: 0"))
			})
		})

		Context("when a plain action is not provided", func() {
			BeforeEach(func() {
				transactionData = []tms.TransactionData{{
					TxID: "255",
					Tx:   &token.TokenTransaction{},
				}}
			})

			It("returns an error", func() {
				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).To(MatchError("check update failed for transaction '255': missing token action"))
			})
		})

		Context("when an unknown plain token action is provided", func() {
			BeforeEach(func() {
				transactionData = []tms.TransactionData{{
					TxID: "254",
					Tx: &token.TokenTransaction{
						Action: &token.TokenTransaction_PlainAction{
							PlainAction: &token.PlainTokenAction{},
						},
					},
				}}
			})

			It("returns an error", func() {
				err := memoryPool.CommitUpdate(transactionData)
				Expect(err).To(MatchError("check update failed: unknown plain token action: <nil>"))
			})
		})
	})

	Describe("OutputByID", func() {
		BeforeEach(func() {
			err := memoryPool.CommitUpdate(transactionData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("retrieves the PlainOutput associated with the entry ID", func() {
			po, err := memoryPool.OutputByID("0.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{
				Owner:    []byte("owner-1"),
				Type:     "TOK1",
				Quantity: 111,
			}))

			po, err = memoryPool.OutputByID("1.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal(&token.PlainOutput{
				Owner:    []byte("owner-2"),
				Type:     "TOK2",
				Quantity: 222,
			}))
		})

		Context("when the output does not exist", func() {
			It("returns a typed error", func() {
				_, err := memoryPool.OutputByID("george")
				Expect(err).To(Equal(&plain.OutputNotFoundError{ID: "george"}))
				Expect(err).To(MatchError("entry not found: george"))
			})
		})
	})

	Describe("TxByID", func() {
		BeforeEach(func() {
			err := memoryPool.CommitUpdate(transactionData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("retrieves the TokenTranasction associated with the entry ID", func() {
			tt, err := memoryPool.TxByID("0")
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(transactionData[0].Tx))

			tt, err = memoryPool.TxByID("1")
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(transactionData[1].Tx))
		})

		Context("when the transaction does not exist", func() {
			It("returns a typed error", func() {
				_, err := memoryPool.TxByID("missing-txid")
				Expect(err).To(Equal(&plain.TxNotFoundError{TxID: "missing-txid"}))
				Expect(err).To(MatchError("transaction not found: missing-txid"))
			})
		})
	})

	Describe("Pool Iteration", func() {
		BeforeEach(func() {
			err := memoryPool.CommitUpdate(transactionData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("provides iteration", func() {
			outputs := map[string]*token.PlainOutput{}
			iter := memoryPool.Iterator()
			for {
				id, po, err := iter.Next()
				if err != nil {
					break
				}

				outputs[string(id)] = po
			}

			Expect(outputs).To(Equal(map[string]*token.PlainOutput{
				"0.0": {Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111},
				"0.1": {Owner: []byte("owner-2"), Type: "TOK1", Quantity: 222},
				"1.0": {Owner: []byte("owner-1"), Type: "TOK2", Quantity: 111},
				"1.1": {Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222},
			}))
		})

		Context("when the pool is empty", func() {
			BeforeEach(func() {
				memoryPool = plain.NewMemoryPool()
			})

			It("returns an empty iterator", func() {
				iter := memoryPool.Iterator()
				_, _, err := iter.Next()
				Expect(err).To(Equal(io.EOF))
			})
		})
	})

	Describe("History Iteration", func() {
		BeforeEach(func() {
			err := memoryPool.CommitUpdate(transactionData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("provides history iteration", func() {
			transactions := map[string]*token.TokenTransaction{}
			iter := memoryPool.HistoryIterator()
			for {
				txid, tx, err := iter.Next()
				if err != nil {
					break
				}
				transactions[string(txid)] = tx
			}

			Expect(transactions).To(Equal(map[string]*token.TokenTransaction{
				string(transactionData[0].TxID): transactionData[0].Tx,
				string(transactionData[1].TxID): transactionData[1].Tx,
			}))
		})

		Context("when no transactions have been processed", func() {
			BeforeEach(func() {
				memoryPool = plain.NewMemoryPool()
			})

			It("returns an empty iterator", func() {
				iter := memoryPool.HistoryIterator()
				_, _, err := iter.Next()
				Expect(err).To(Equal(io.EOF))
			})
		})
	})
})
