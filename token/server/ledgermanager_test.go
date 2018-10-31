/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"github.com/hyperledger/fabric/token/server"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LedgerManager", func() {
	var (
		ledgerManager *server.PeerLedgerManager
	)

	BeforeEach(func() {
		ledgerManager = &server.PeerLedgerManager{}
	})

	Context("when asking a LedgerReader for a channel that does not exists", func() {
		It("returns the error", func() {
			_, err := ledgerManager.GetLedgerReader("non-existing-channel")
			Expect(err).To(MatchError("ledger not found for channel non-existing-channel"))
		})
	})

})
