/*
Copyright IBM Corp. All Rights Reserved.

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
			{Recipient: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: 1001},
			{Recipient: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK2", Quantity: 1002},
			{Recipient: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK3", Quantity: 1003},
		}
		issuer = &plain.Issuer{TokenOwnerValidator: &TestTokenOwnerValidator{}}
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
								{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: 1001},
								{Owner: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK2", Quantity: 1002},
								{Owner: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK3", Quantity: 1003},
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

	Describe("RequestExpectation", func() {
		var (
			outputs            []*token.PlainOutput
			expectationRequest *token.ExpectationRequest
		)

		BeforeEach(func() {
			outputs = []*token.PlainOutput{{
				Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
				Type:     "XYZ",
				Quantity: 99,
			}}
			expectationRequest = &token.ExpectationRequest{
				Credential: []byte("credential"),
				Expectation: &token.TokenExpectation{
					Expectation: &token.TokenExpectation_PlainExpectation{
						PlainExpectation: &token.PlainExpectation{
							Payload: &token.PlainExpectation_ImportExpectation{
								ImportExpectation: &token.PlainTokenExpectation{
									Outputs: outputs,
								},
							},
						},
					},
				},
			}
		})

		It("creates a token transaction", func() {
			tt, err := issuer.RequestExpectation(expectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{
							PlainImport: &token.PlainImport{
								Outputs: outputs,
							},
						},
					},
				},
			}))
		})

		Context("when outputs is nil", func() {
			BeforeEach(func() {
				expectationRequest.GetExpectation().GetPlainExpectation().GetImportExpectation().Outputs = nil
			})

			It("returns an error", func() {
				_, err := issuer.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no outputs in ExpectationRequest"))
			})
		})

		Context("when outputs is empty", func() {
			BeforeEach(func() {
				expectationRequest.GetExpectation().GetPlainExpectation().GetImportExpectation().Outputs = []*token.PlainOutput{}
			})

			It("returns an error", func() {
				_, err := issuer.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no outputs in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil Expectation", func() {
			BeforeEach(func() {
				expectationRequest.Expectation = nil
			})

			It("returns the error", func() {
				_, err := issuer.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no token expectation in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				expectationRequest.Expectation = &token.TokenExpectation{}
			})

			It("returns the error", func() {
				_, err := issuer.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no plain expectation in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil ImportExpectation", func() {
			BeforeEach(func() {
				expectationRequest = &token.ExpectationRequest{
					Credential: []byte("credential"),
					Expectation: &token.TokenExpectation{
						Expectation: &token.TokenExpectation_PlainExpectation{
							PlainExpectation: &token.PlainExpectation{},
						},
					},
				}
			})

			It("returns the error", func() {
				_, err := issuer.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no import expectation in ExpectationRequest"))
			})
		})
	})
})
