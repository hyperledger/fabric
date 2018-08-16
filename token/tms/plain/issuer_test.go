/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms/plain"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Issuer", func() {
	var (
		issuer        *plain.Issuer
		tokensToIssue []*token.TokenToIssue
	)

	BeforeEach(func() {
		tokensToIssue = []*token.TokenToIssue{
			{Recipient: []byte("R1"), Type: "TOK1", Quantity: 1001},
			{Recipient: []byte("R2"), Type: "TOK2", Quantity: 1002},
			{Recipient: []byte("R3"), Type: "TOK3", Quantity: 1003},
		}
		issuer = &plain.Issuer{}
	})

	It("converts an import request to a token transaction", func() {
		tt, err := issuer.RequestImport(tokensToIssue)
		Expect(err).NotTo(HaveOccurred())
		Expect(tt).To(Equal(&token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainImport{
						PlainImport: &token.PlainImport{
							Outputs: []*token.PlainOutput{
								{Owner: []byte("R1"), Type: "TOK1", Quantity: 1001},
								{Owner: []byte("R2"), Type: "TOK2", Quantity: 1002},
								{Owner: []byte("R3"), Type: "TOK3", Quantity: 1003},
							},
						},
					},
				},
			},
		}))
	})

	Context("when tokens to issue is nil", func() {
		It("creates a token transaction with no outputs", func() {
			tt, err := issuer.RequestImport(nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{PlainImport: &token.PlainImport{}},
					},
				},
			}))
		})
	})

	Context("when tokens to issue is empty", func() {
		It("creates a token transaction with no outputs", func() {
			tt, err := issuer.RequestImport([]*token.TokenToIssue{})
			Expect(err).NotTo(HaveOccurred())

			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{PlainImport: &token.PlainImport{}},
					},
				},
			}))
		})
	})
})
