/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/hyperledger/fabric/core/chaincode"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ActiveTransactions", func() {
	var activeTx *chaincode.ActiveTransactions

	BeforeEach(func() {
		activeTx = chaincode.NewActiveTransactions()
	})

	It("tracks active transactions", func() {
		// Add unique transactions
		ok := activeTx.Add("channel-id", "tx-id")
		Expect(ok).To(BeTrue(), "a new transaction should return true")
		ok = activeTx.Add("channel-id", "tx-id-2")
		Expect(ok).To(BeTrue(), "adding a different transaction id should return true")
		ok = activeTx.Add("channel-id-2", "tx-id")
		Expect(ok).To(BeTrue(), "adding a different channel-id should return true")

		// Attempt to add a transaction that already exists
		ok = activeTx.Add("channel-id", "tx-id")
		Expect(ok).To(BeFalse(), "attempting to an existing transaction should return false")

		// Remove existing and make sure the ID can be reused
		activeTx.Remove("channel-id", "tx-id")
		ok = activeTx.Add("channel-id", "tx-id")
		Expect(ok).To(BeTrue(), "using a an id that has been removed should return true")
	})

	DescribeTable("NewTxKey",
		func(channelID, txID, expected string) {
			result := chaincode.NewTxKey(channelID, txID)
			Expect(result).To(Equal(expected))
		},
		Entry("empty channel and tx", "", "", ""),
		Entry("empty channel", "", "tx-1", "tx-1"),
		Entry("empty tx", "chan-1", "", "chan-1"),
		Entry("channel and tx", "chan-1", "tx-1", "chan-1tx-1"),
	)
})
