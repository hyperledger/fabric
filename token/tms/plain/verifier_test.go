/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms"
	"github.com/hyperledger/fabric/token/tms/plain"
	"github.com/hyperledger/fabric/token/tms/plain/mock"
)

var _ = Describe("Verifier", func() {
	var (
		fakeCred            *mock.Credential
		fakePolicyValidator *mock.PolicyValidator
		fakePool            *mock.Pool

		transactionData []tms.TransactionData

		verifier *plain.Verifier
	)

	BeforeEach(func() {
		fakeCred = &mock.Credential{}
		fakePolicyValidator = &mock.PolicyValidator{}
		fakePool = &mock.Pool{}
		fakePool.CommitUpdateReturns(nil)

		transactionData = []tms.TransactionData{{
			TxID: "0",
			Tx: &token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{
							PlainImport: &token.PlainImport{
								Outputs: []*token.PlainOutput{
									{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111},
									{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222},
								},
							},
						},
					},
				},
			},
		}}

		verifier = &plain.Verifier{
			Pool:            fakePool,
			PolicyValidator: fakePolicyValidator,
		}
	})

	Describe("Validate PlainImport", func() {
		It("does nothing", func() {
			err := verifier.Validate(fakeCred, transactionData[0])
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Commit PlainImport", func() {
		It("evaluates policy for each output", func() {
			err := verifier.Commit(fakeCred, transactionData)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePolicyValidator.IsIssuerCallCount()).To(Equal(2))
			creator, tt := fakePolicyValidator.IsIssuerArgsForCall(0)
			Expect(creator).To(Equal(fakeCred))
			Expect(tt).To(Equal("TOK1"))
			creator, tt = fakePolicyValidator.IsIssuerArgsForCall(1)
			Expect(creator).To(Equal(fakeCred))
			Expect(tt).To(Equal("TOK2"))
		})

		It("checks the pool", func() {
			err := verifier.Commit(fakeCred, transactionData)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePool.CommitUpdateCallCount()).To(Equal(1))
			td := fakePool.CommitUpdateArgsForCall(0)
			Expect(td).To(Equal(transactionData))
		})

		It("commits to the pool", func() {
			err := verifier.Commit(fakeCred, transactionData)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePool.CommitUpdateCallCount()).To(Equal(1))
			td := fakePool.CommitUpdateArgsForCall(0)
			Expect(td).To(Equal(transactionData))
		})

		Context("when policy validation fails", func() {
			BeforeEach(func() {
				fakePolicyValidator.IsIssuerReturns(errors.New("no-way-man"))
			})

			It("returns an error and does not commit to the pool", func() {
				err := verifier.Commit(fakeCred, transactionData)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("commit failed: no-way-man"))
				Expect(fakePool.CommitUpdateCallCount()).To(Equal(0))
			})
		})

		Context("when the pool update check fails", func() {
			BeforeEach(func() {
				fakePool.CommitUpdateReturns(errors.New("no-can-do"))
			})

			It("returns an error", func() {
				err := verifier.Commit(fakeCred, transactionData)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("commit failed: no-can-do"))

				Expect(fakePool.CommitUpdateCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Validate", func() {
		Context("when an unknown action is provided", func() {
			BeforeEach(func() {
				transactionData[0].Tx.Action = nil
			})

			It("returns an error", func() {
				err := verifier.Validate(fakeCred, transactionData[0])
				Expect(err).To(MatchError("validation failed: unknown action: <nil>"))
			})
		})

		Context("when an unexpected plain token action is provided", func() {
			BeforeEach(func() {
				transactionData[0].Tx.Action = &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{},
				}
			})

			It("returns an error", func() {
				err := verifier.Validate(fakeCred, transactionData[0])
				Expect(err).To(MatchError("validation failed: unknown plain token action: <nil>"))
			})
		})
	})

	Describe("Commit", func() {
		Context("when an unknown action is provided", func() {
			BeforeEach(func() {
				transactionData[0].Tx.Action = nil
			})

			It("returns an error", func() {
				err := verifier.Commit(fakeCred, transactionData)
				Expect(err).To(MatchError("commit failed: unknown action: <nil>"))
			})
		})

		Context("when an unexpected plain token action is provided", func() {
			BeforeEach(func() {
				transactionData[0].Tx.Action = &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{},
				}
			})

			It("returns an error", func() {
				err := verifier.Commit(fakeCred, transactionData)
				Expect(err).To(MatchError("commit failed: unknown plain token action: <nil>"))
			})
		})
	})
})
