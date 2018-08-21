/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MemoryLedger", func() {
	var (
		txID1 string
		tx1   []byte
		tx2   []byte

		namespace string

		memoryLedger *plain.MemoryLedger
	)

	BeforeEach(func() {
		memoryLedger = plain.NewMemoryLedger()

		txID1 = "1"
		tx1 = []byte{1}
		tx2 = []byte{2}

		namespace = "ledgerNamespace"
	})

	Describe("get and set", func() {
		It("sets state", func() {
			By("adding a transaction")
			err := memoryLedger.SetState(namespace, txID1, tx1)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring the transaction is in the ledger")
			po, err := memoryLedger.GetState(namespace, "1")
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal([]byte{1}))
		})

		Context("when an entry exists", func() {
			BeforeEach(func() {
				err := memoryLedger.SetState(namespace, txID1, tx1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("overwrites the entry", func() {
				By("setting the new entry")
				err := memoryLedger.SetState(namespace, txID1, tx2)
				Expect(err).NotTo(HaveOccurred())

				By("ensuring the transaction has the new value")
				po, err := memoryLedger.GetState(namespace, "1")
				Expect(err).NotTo(HaveOccurred())
				Expect(po).To(Equal([]byte{2}))
			})
		})

		Context("when an entry does not exist", func() {
			It("returns an error", func() {
				val, err := memoryLedger.GetState(namespace, "badTxID")
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNil())
			})
		})
	})

	Describe("when the dummy function is invoked", func() {
		It("returns nil", func() {
			res, err := memoryLedger.GetStateRangeScanIterator("", "", "")
			Expect(res).To(BeNil())
			Expect(err).To(BeNil())
		})
	})
})
