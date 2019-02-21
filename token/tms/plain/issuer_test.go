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
		tokensToIssue []*token.Token
	)

	BeforeEach(func() {
		tokensToIssue = []*token.Token{
			{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: ToHex(1001)},
			{Owner: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK2", Quantity: ToHex(1002)},
			{Owner: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK3", Quantity: ToHex(1003)},
		}
		issuer = &plain.Issuer{TokenOwnerValidator: &TestTokenOwnerValidator{}}
	})

	It("converts an import request to a token transaction", func() {
		tt, err := issuer.RequestIssue(tokensToIssue)
		Expect(err).NotTo(HaveOccurred())
		Expect(tt).To(Equal(&token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Issue{
						Issue: &token.Issue{
							Outputs: []*token.Token{
								{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: ToHex(1001)},
								{Owner: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK2", Quantity: ToHex(1002)},
								{Owner: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK3", Quantity: ToHex(1003)},
							},
						},
					},
				},
			},
		}))
	})

	Context("when tokens to issue is nil", func() {
		It("creates a token transaction with no outputs", func() {
			tt, err := issuer.RequestIssue(nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Issue{Issue: &token.Issue{}},
					},
				},
			}))
		})
	})

	Context("when tokens to issue is empty", func() {
		It("creates a token transaction with no outputs", func() {
			tt, err := issuer.RequestIssue([]*token.Token{})
			Expect(err).NotTo(HaveOccurred())

			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Issue{Issue: &token.Issue{}},
					},
				},
			}))
		})
	})

	Describe("RequestExpectation", func() {
		var (
			outputs            []*token.Token
			expectationRequest *token.ExpectationRequest
		)

		BeforeEach(func() {
			outputs = []*token.Token{{
				Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
				Type:     "XYZ",
				Quantity: ToHex(99),
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
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Issue{
							Issue: &token.Issue{
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
				expectationRequest.GetExpectation().GetPlainExpectation().GetImportExpectation().Outputs = []*token.Token{}
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
