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

	It("converts an issue request to a token transaction", func() {
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
		It("should return an error", func() {
			tt, err := issuer.RequestIssue(nil)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid tokensToIssue"))
			Expect(tt).To(BeNil())
		})
	})

	Context("when tokens to issue is empty", func() {
		It("should return an error", func() {
			tt, err := issuer.RequestIssue([]*token.Token{})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid tokensToIssue"))
			Expect(tt).To(BeNil())
		})
	})

	Context("when tokens to issue have invalid owner", func() {
		It("creates a token transaction with no outputs", func() {
			tokensToIssue = []*token.Token{
				{Owner: nil, Type: "TOK1", Quantity: ToHex(1001)},
			}

			tt, err := issuer.RequestIssue(tokensToIssue)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid recipient in issue request 'owner is nil'"))
			Expect(tt).To(BeNil())
		})
	})

	Context("when tokens to issue have invalid quantity", func() {
		It("creates a token transaction with no outputs", func() {
			tokensToIssue = []*token.Token{
				{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: "INVALID_QUANTITY"},
			}

			tt, err := issuer.RequestIssue(tokensToIssue)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("invalid quantity in issue request 'invalid input'"))
			Expect(tt).To(BeNil())
		})
	})

	Describe("RequestTokenOperation", func() {
		var (
			outputs               []*token.Token
			tokenOperationRequest *token.TokenOperationRequest
		)

		BeforeEach(func() {
			outputs = []*token.Token{{
				Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
				Type:     "XYZ",
				Quantity: ToHex(99),
			}}
			tokenOperationRequest = &token.TokenOperationRequest{
				Credential: []byte("credential"),
				Operations: []*token.TokenOperation{{
					Operation: &token.TokenOperation_Action{
						Action: &token.TokenOperationAction{
							Payload: &token.TokenOperationAction_Issue{
								Issue: &token.TokenActionTerms{
									Sender:  &token.TokenOwner{Raw: []byte("credential")},
									Outputs: outputs,
								},
							},
						},
					},
				},
				},
			}
		})

		It("creates a token transaction", func() {
			tt, err := issuer.RequestTokenOperation(tokenOperationRequest.Operations[0])
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
				tokenOperationRequest.GetOperations()[0].GetAction().GetIssue().Outputs = nil
			})

			It("returns an error", func() {
				_, err := issuer.RequestTokenOperation(tokenOperationRequest.Operations[0])
				Expect(err).To(MatchError("no outputs in ExpectationRequest"))
			})
		})

		Context("when outputs is empty", func() {
			BeforeEach(func() {
				tokenOperationRequest.GetOperations()[0].GetAction().GetIssue().Outputs = []*token.Token{}
			})

			It("returns an error", func() {
				_, err := issuer.RequestTokenOperation(tokenOperationRequest.Operations[0])
				Expect(err).To(MatchError("no outputs in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				tokenOperationRequest.Operations = []*token.TokenOperation{{}}
			})

			It("returns the error", func() {
				_, err := issuer.RequestTokenOperation(tokenOperationRequest.Operations[0])
				Expect(err).To(MatchError("no action in request"))
			})
		})

		Context("when ExpectationRequest has nil ImportExpectation", func() {
			BeforeEach(func() {
				tokenOperationRequest = &token.TokenOperationRequest{
					Credential: []byte("credential"),
					Operations: []*token.TokenOperation{{
						Operation: &token.TokenOperation_Action{
							Action: &token.TokenOperationAction{
								Payload: &token.TokenOperationAction_Issue{},
							},
						},
					},
					},
				}
			})

			It("returns the error", func() {
				_, err := issuer.RequestTokenOperation(tokenOperationRequest.Operations[0])
				Expect(err).To(MatchError("no issue in action"))
			})
		})
	})
})
