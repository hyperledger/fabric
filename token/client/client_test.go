/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("TokenClient", func() {
	var (
		payload       *common.Payload
		payloadBytes  []byte
		envelope      *common.Envelope
		envelopeBytes []byte

		fakeSigningIdentity *mock.SigningIdentity
		fakeProver          *mock.Prover
		fakeTxSubmitter     *mock.FabricTxSubmitter

		tokenClient *client.Client
	)

	BeforeEach(func() {
		payload = &common.Payload{Data: []byte("tx-payload")}
		payloadBytes = ProtoMarshal(payload)
		envelope = &common.Envelope{Payload: payloadBytes, Signature: []byte("tx-signature")}
		envelopeBytes = ProtoMarshal(envelope)

		fakeProver = &mock.Prover{}
		fakeProver.RequestImportReturns([]byte("tx-payload"), nil) // same data as payload
		fakeProver.RequestTransferReturns([]byte("tx-payload"), nil)

		fakeSigningIdentity = &mock.SigningIdentity{}
		fakeSigningIdentity.SignReturns([]byte("tx-signature"), nil) // same signature as envelope

		fakeTxSubmitter = &mock.FabricTxSubmitter{}
		fakeTxSubmitter.SubmitReturns(nil)

		tokenClient = &client.Client{
			SigningIdentity: fakeSigningIdentity,
			Prover:          fakeProver,
			TxSubmitter:     fakeTxSubmitter,
		}
	})

	Describe("Issue", func() {
		var (
			tokensToIssue []*token.TokenToIssue
		)

		BeforeEach(func() {
			// input data for Issue
			tokensToIssue = []*token.TokenToIssue{
				{
					Type:      "type",
					Quantity:  1,
					Recipient: []byte("alice"),
				},
			}
		})

		It("returns tx envelope without error", func() {
			serializedTx, err := tokenClient.Issue(tokensToIssue)
			Expect(err).NotTo(HaveOccurred())
			Expect(serializedTx).To(Equal(envelopeBytes))

			Expect(fakeProver.RequestImportCallCount()).To(Equal(1))
			tokens, signingIdentity := fakeProver.RequestImportArgsForCall(0)
			Expect(tokens).To(Equal(tokensToIssue))
			Expect(signingIdentity).To(Equal(fakeSigningIdentity))

			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(payloadBytes))

			Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			raw = fakeTxSubmitter.SubmitArgsForCall(0)
			Expect(raw).To(Equal(envelopeBytes))
		})

		Context("when prover.RequestImport fails", func() {
			BeforeEach(func() {
				fakeProver.RequestImportReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Issue(tokensToIssue)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestImportCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity.Sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Issue(tokensToIssue)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestImportCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter.Submit fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.SubmitReturns(errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Issue(tokensToIssue)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestImportCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Transfer", func() {
		var (
			tokenIDs       [][]byte
			transferShares []*token.RecipientTransferShare
		)

		BeforeEach(func() {
			// input data for Transfer
			tokenIDs = [][]byte{[]byte("id1"), []byte("id2")}
			transferShares = []*token.RecipientTransferShare{
				{Recipient: []byte("alice"), Quantity: 100},
				{Recipient: []byte("Bob"), Quantity: 50},
			}
		})

		It("returns tx envelope without error", func() {
			serializedTx, err := tokenClient.Transfer(tokenIDs, transferShares)
			Expect(err).NotTo(HaveOccurred())
			Expect(serializedTx).To(Equal(envelopeBytes))

			Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
			ids, shares, signingIdentity := fakeProver.RequestTransferArgsForCall(0)
			Expect(ids).To(Equal(tokenIDs))
			Expect(shares).To(Equal(transferShares))
			Expect(signingIdentity).To(Equal(fakeSigningIdentity))

			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(payloadBytes))

			Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			raw = fakeTxSubmitter.SubmitArgsForCall(0)
			Expect(raw).To(Equal(envelopeBytes))
		})

		Context("when prover.RequestImport fails", func() {
			BeforeEach(func() {
				fakeProver.RequestTransferReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Transfer(tokenIDs, transferShares)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity.Sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Transfer(tokenIDs, transferShares)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter.Submit fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.SubmitReturns(errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := tokenClient.Transfer(tokenIDs, transferShares)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			})
		})
	})
})
