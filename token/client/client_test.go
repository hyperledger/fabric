/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"crypto/rand"
	"net"
	"time"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var _ = Describe("Client", func() {
	var (
		payload      *common.Payload
		payloadBytes []byte
		envelope     *common.Envelope
		expectedTxid string

		fakeSigningIdentity *mock.SigningIdentity
		fakeProver          *mock.Prover
		fakeTxSubmitter     *mock.FabricTxSubmitter

		tokenClient *client.Client
	)

	BeforeEach(func() {
		payload = &common.Payload{Data: []byte("tx-payload")}
		payloadBytes = ProtoMarshal(payload)
		envelope = &common.Envelope{Payload: payloadBytes, Signature: []byte("tx-signature")}
		expectedTxid = "dummy-tx-id"

		fakeProver = &mock.Prover{}
		fakeProver.RequestIssueReturns(payload.Data, nil) // same data as payload
		fakeProver.RequestTransferReturns(payload.Data, nil)
		fakeProver.RequestRedeemReturns(payload.Data, nil)

		fakeSigningIdentity = &mock.SigningIdentity{}
		fakeSigningIdentity.SerializeReturns([]byte("creator"), nil) // same signature as envelope
		fakeSigningIdentity.SignReturns([]byte("tx-signature"), nil) // same signature as envelope

		fakeTxSubmitter = &mock.FabricTxSubmitter{}
		ordererStatus := common.Status_SUCCESS
		fakeTxSubmitter.SubmitReturns(&ordererStatus, true, nil)
		fakeTxSubmitter.CreateTxEnvelopeReturns(envelope, expectedTxid, nil)

		tokenClient = &client.Client{
			SigningIdentity: fakeSigningIdentity,
			Prover:          fakeProver,
			TxSubmitter:     fakeTxSubmitter,
		}
	})

	Describe("Issue", func() {
		var (
			tokensToIssue []*token.Token
		)

		BeforeEach(func() {
			// input data for Issue
			tokensToIssue = []*token.Token{
				{
					Type:     "type",
					Quantity: ToHex(1),
					Owner:    &token.TokenOwner{Raw: []byte("alice")},
				},
			}
		})

		It("returns tx envelope and valid status", func() {
			txEnvelope, txid, ordererStatus, committed, err := tokenClient.Issue(tokensToIssue, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(txEnvelope).To(Equal(envelope))
			Expect(txid).To(Equal(expectedTxid))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(Equal(true))

			Expect(fakeProver.RequestIssueCallCount()).To(Equal(1))
			tokens, signingIdentity := fakeProver.RequestIssueArgsForCall(0)
			Expect(tokens).To(Equal(tokensToIssue))
			Expect(signingIdentity).To(Equal(fakeSigningIdentity))

			Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
			txBytes := fakeTxSubmitter.CreateTxEnvelopeArgsForCall(0)
			Expect(txBytes).To(Equal(payload.Data))

			Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			txEnvelope, waitTime := fakeTxSubmitter.SubmitArgsForCall(0)
			Expect(txEnvelope).To(Equal(envelope))
			Expect(waitTime).To(Equal(10 * time.Second))
		})

		Context("when prover.RequestIssue fails", func() {
			BeforeEach(func() {
				fakeProver.RequestIssueReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Issue(tokensToIssue, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestIssueCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(0))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter CreateTxEnvelope fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.CreateTxEnvelopeReturns(nil, "", errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Issue(tokensToIssue, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestIssueCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter Submit fails", func() {
			BeforeEach(func() {
				status := common.Status_BAD_REQUEST
				fakeTxSubmitter.SubmitReturns(&status, false, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				txEnvelope, txid, ordererStatus, committed, err := tokenClient.Issue(tokensToIssue, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(txEnvelope).To(Equal(envelope))
				Expect(txid).To(Equal(expectedTxid))
				Expect(*ordererStatus).To(Equal(common.Status_BAD_REQUEST))
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestIssueCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Transfer", func() {
		var (
			tokenIDs       []*token.TokenId
			transferShares []*token.RecipientShare
		)

		BeforeEach(func() {
			// input data for Transfer
			tokenIDs = []*token.TokenId{
				{TxId: "id1", Index: 0},
				{TxId: "id2", Index: 0},
			}
			transferShares = []*token.RecipientShare{
				{Recipient: &token.TokenOwner{Raw: []byte("alice")}, Quantity: ToHex(100)},
				{Recipient: &token.TokenOwner{Raw: []byte("bob")}, Quantity: ToHex(50)},
			}
		})

		It("returns tx envelope and valid status", func() {
			txEnvelope, txid, ordererStatus, committed, err := tokenClient.Transfer(tokenIDs, transferShares, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(txEnvelope).To(Equal(envelope))
			Expect(txid).To(Equal(expectedTxid))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(Equal(true))

			Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
			tokens, shares, signingIdentity := fakeProver.RequestTransferArgsForCall(0)
			Expect(tokens).To(Equal(tokenIDs))
			Expect(shares).To(Equal(transferShares))
			Expect(signingIdentity).To(Equal(fakeSigningIdentity))

			Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
			txBytes := fakeTxSubmitter.CreateTxEnvelopeArgsForCall(0)
			Expect(txBytes).To(Equal(payload.Data))

			Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			txEnvelope, waitTime := fakeTxSubmitter.SubmitArgsForCall(0)
			Expect(txEnvelope).To(Equal(envelope))
			Expect(waitTime).To(Equal(10 * time.Second))
		})

		Context("when prover.RequestTransfer fails", func() {
			BeforeEach(func() {
				fakeProver.RequestTransferReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Transfer(tokenIDs, transferShares, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(0))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter CreateTxEnvelope fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.CreateTxEnvelopeReturns(nil, "", errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Transfer(tokenIDs, transferShares, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter Submit fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.SubmitReturns(nil, false, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				txEnvelope, txid, ordererStatus, committed, err := tokenClient.Transfer(tokenIDs, transferShares, 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(txEnvelope).To(Equal(envelope))
				Expect(txid).To(Equal(expectedTxid))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestTransferCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Redeem", func() {
		var (
			tokenIDs []*token.TokenId
			quantity uint64
		)

		BeforeEach(func() {
			// input data for redeem
			tokenIDs = []*token.TokenId{
				{TxId: "id1", Index: 0},
				{TxId: "id2", Index: 0},
			}
			quantity = 100
		})

		It("returns tx envelope without error", func() {
			txEnvelope, txid, ordererStatus, committed, err := tokenClient.Redeem(tokenIDs, ToHex(quantity), 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(txEnvelope).To(Equal(envelope))
			Expect(txid).To(Equal(expectedTxid))
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(Equal(true))

			Expect(fakeProver.RequestRedeemCallCount()).To(Equal(1))
			ids, num, signingIdentity := fakeProver.RequestRedeemArgsForCall(0)
			Expect(ids).To(Equal(tokenIDs))
			Expect(num).To(Equal(ToHex(quantity)))
			Expect(signingIdentity).To(Equal(fakeSigningIdentity))

			Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
			txBytes := fakeTxSubmitter.CreateTxEnvelopeArgsForCall(0)
			Expect(txBytes).To(Equal(payload.Data))

			Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			txEnvelope, waitTime := fakeTxSubmitter.SubmitArgsForCall(0)
			Expect(txEnvelope).To(Equal(envelope))
			Expect(waitTime).To(Equal(10 * time.Second))
		})

		Context("when prover.RequestRedeem fails", func() {
			BeforeEach(func() {
				fakeProver.RequestRedeemReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Redeem(tokenIDs, ToHex(quantity), 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestRedeemCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(0))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter CreateTxEnvelope fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.CreateTxEnvelopeReturns(nil, "", errors.New("wild-banana"))
			})

			It("returns an error", func() {
				envelope, txid, ordererStatus, committed, err := tokenClient.Redeem(tokenIDs, ToHex(quantity), 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(envelope).To(BeNil())
				Expect(txid).To(Equal(""))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestRedeemCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(0))
			})
		})

		Context("when TxSubmitter Submit fails", func() {
			BeforeEach(func() {
				fakeTxSubmitter.SubmitReturns(nil, false, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				txEnvelope, txid, ordererStatus, committed, err := tokenClient.Redeem(tokenIDs, ToHex(quantity), 0)
				Expect(err).To(MatchError("wild-banana"))
				Expect(txEnvelope).To(Equal(envelope))
				Expect(txid).To(Equal(expectedTxid))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))

				Expect(fakeProver.RequestRedeemCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.CreateTxEnvelopeCallCount()).To(Equal(1))
				Expect(fakeTxSubmitter.SubmitCallCount()).To(Equal(1))
			})
		})
	})

	Describe("ListTokens", func() {
		var (
			expectedTokens []*token.UnspentToken
		)

		BeforeEach(func() {
			// prepare CommandResponse for mocked prover to return
			expectedTokens = []*token.UnspentToken{
				{Id: &token.TokenId{TxId: "idaz", Index: 0}, Type: "typeaz", Quantity: ToHex(135)},
				{Id: &token.TokenId{TxId: "idby", Index: 1}, Type: "typeby", Quantity: ToHex(79)},
			}
			fakeProver.ListTokensReturns(expectedTokens, nil)
		})

		It("returns tokens", func() {
			tokens, err := tokenClient.ListTokens()
			Expect(err).NotTo(HaveOccurred())
			Expect(tokens).To(Equal(expectedTokens))

			Expect(fakeProver.ListTokensCallCount()).To(Equal(1))
			arg := fakeProver.ListTokensArgsForCall(0)
			Expect(arg).To(Equal(tokenClient.SigningIdentity))
		})

		Context("when prover.ListTokens returns an error", func() {
			BeforeEach(func() {
				fakeProver.ListTokensReturns(nil, errors.New("banana-loop"))
			})

			It("returns an error", func() {
				_, err := tokenClient.ListTokens()
				Expect(err).To(MatchError("banana-loop"))

				Expect(fakeProver.ListTokensCallCount()).To(Equal(1))
				arg := fakeProver.ListTokensArgsForCall(0)
				Expect(arg).To(Equal(tokenClient.SigningIdentity))
			})
		})
	})

	Describe("NewClient", func() {
		var (
			config          *client.ClientConfig
			ordererListener net.Listener
			deliverListener net.Listener
			ordererServer   *grpc.Server
			deliverServer   *grpc.Server
		)

		BeforeEach(func() {
			// start listener and grpc servers for orderer and deliver services
			var err error
			ordererListener, err = net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())

			deliverListener, err = net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())

			ordererServer = grpc.NewServer()
			go ordererServer.Serve(ordererListener)

			deliverServer = grpc.NewServer()
			go deliverServer.Serve(deliverListener)

			ordererEndpoint := ordererListener.Addr().String()
			deliverEndpoint := deliverListener.Addr().String()
			config = getClientConfig(false, "test-channel", ordererEndpoint, deliverEndpoint, "dummy_endpoint")
		})

		AfterEach(func() {
			if ordererListener != nil {
				ordererListener.Close()
			}
			if deliverListener != nil {
				deliverListener.Close()
			}
			ordererServer.Stop()
			deliverServer.Stop()
		})

		It("creates a client", func() {
			c, err := client.NewClient(*config, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())

			prover, ok := c.Prover.(*client.ProverPeer)
			Expect(ok).To(Equal(true))
			Expect(prover.ChannelID).To(Equal(config.ChannelID))
			Expect(prover.RandomnessReader).To(Equal(rand.Reader))

			submitter, ok := c.TxSubmitter.(*client.TxSubmitter)
			Expect(ok).To(Equal(true))
			Expect(submitter.Config).To(Equal(config))
			Expect(submitter.SigningIdentity).To(Equal(c.SigningIdentity))
		})

		Context("when channel id is missing", func() {
			BeforeEach(func() {
				config.ChannelID = ""
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err).To(MatchError("missing channel id"))
			})
		})

		Context("when TLS root cert file is missing", func() {
			BeforeEach(func() {
				config.Orderer.TLSEnabled = true
				config.Orderer.TLSRootCertFile = ""
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err).To(MatchError("missing orderer TLSRootCertFile"))
			})
		})

		Context("when it fails to load TLS root cert file", func() {
			BeforeEach(func() {
				config.ProverPeer.TLSEnabled = true
				config.ProverPeer.TLSRootCertFile = "/non-file"
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("unable to load TLS cert from %s", config.ProverPeer.TLSRootCertFile))
			})
		})

		Context("when NewTxSumitter fails to connect to orderer", func() {
			BeforeEach(func() {
				ordererServer.Stop()
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("failed to connect to orderer"))
			})
		})

		Context("when NewTxSumitter fails to connect to commit peer", func() {
			BeforeEach(func() {
				deliverServer.Stop()
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("failed to connect to commit peer"))
			})
		})

		Context("when SignIdentity fails to serialize", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := client.NewClient(*config, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
			})
		})
	})
})
