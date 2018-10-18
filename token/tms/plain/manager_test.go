/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"github.com/hyperledger/fabric/token/tms/plain"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/hyperledger/fabric/token/transaction/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	var (
		manager *plain.Manager
	)

	BeforeEach(func() {
		manager = &plain.Manager{PolicyValidators: make(map[string]transaction.PolicyValidator)}
	})

	Describe("Get a TxProcessor for a non-existent channel", func() {
		It("returns an error", func() {
			_, err := manager.GetTxProcessor("boguschannel")
			Expect(err).To(MatchError("no policy validator found for channel 'boguschannel'"))
		})
	})

	Context("When a channel exists", func() {
		var (
			fakePolicyValidator *mock.PolicyValidator
			channel             string
		)
		BeforeEach(func() {
			channel = "ch0"
			fakePolicyValidator = &mock.PolicyValidator{}
			manager.PolicyValidators[channel] = fakePolicyValidator
		})

		Describe("Get a TxProcessor for an existing channel", func() {
			It("returns a Verifier that implements the TxProcessor interface", func() {
				txProcessor, err := manager.GetTxProcessor(channel)
				Expect(err).NotTo(HaveOccurred())
				Expect(txProcessor).NotTo(BeNil())
				Expect(txProcessor).To(Equal(&plain.Verifier{PolicyValidator: fakePolicyValidator}))
			})
		})
	})
})
