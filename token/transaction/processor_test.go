/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/transaction"
	"github.com/hyperledger/fabric/token/transaction/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Processor", func() {
	var (
		txProcessor     *transaction.Processor
		invalidEnvelope *common.Envelope
		fakeManager     *mock.TMSManager
		validEnvelope   *common.Envelope
		validTtx        *token.TokenTransaction
	)

	BeforeEach(func() {
		txProcessor = &transaction.Processor{}
		invalidEnvelope = &common.Envelope{
			Payload:   []byte("wild_payload"),
			Signature: nil,
		}
		fakeManager = &mock.TMSManager{}
		txProcessor.TMSManager = fakeManager
		validTtx = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{},
			},
		}

		ch := &common.ChannelHeader{
			Type: int32(common.HeaderType_TOKEN_TRANSACTION), ChannelId: "wild_channel",
			TxId: "tx0",
		}
		marshaledChannelHeader, err := proto.Marshal(ch)
		Expect(err).ToNot(HaveOccurred())
		hdr := &common.Header{
			ChannelHeader: marshaledChannelHeader,
		}
		marshaledData, err := proto.Marshal(validTtx)
		Expect(err).ToNot(HaveOccurred())
		payload := &common.Payload{
			Header: hdr,
			Data:   marshaledData,
		}
		marshaledPayload, err := proto.Marshal(payload)
		Expect(err).ToNot(HaveOccurred())
		validEnvelope = &common.Envelope{
			Payload:   marshaledPayload,
			Signature: nil,
		}
	})

	Describe("GenerateSimulationResults", func() {
		Context("when an invalid token transaction is passed", func() {
			It("returns an error", func() {
				err := txProcessor.GenerateSimulationResults(invalidEnvelope, nil, false)
				Expect(err).To(MatchError("failed unmarshalling token transaction: error unmarshaling Payload: proto: can't skip unknown wire type 7"))
			})
		})

		Context("when no TxProcessor can be retrieved for the specified channel", func() {
			BeforeEach(func() {
				fakeManager.GetTxProcessorReturns(nil, errors.New("no policy validator found for channel 'wild_channel'"))
			})
			It("returns an error", func() {
				err := txProcessor.GenerateSimulationResults(validEnvelope, nil, false)
				Expect(err).To(MatchError("failed getting committer: no policy validator found for channel 'wild_channel'"))
				Expect(fakeManager.GetTxProcessorCallCount()).To(Equal(1))
				Expect(fakeManager.GetTxProcessorArgsForCall(0)).To(Equal("wild_channel"))
			})
		})

		Context("when a call to the channel TxProcessor fails", func() {
			var (
				verifier *mock.TMSTxProcessor
			)
			BeforeEach(func() {
				verifier = &mock.TMSTxProcessor{}
				verifier.ProcessTxReturns(errors.New("mock TMSTxProcessor error"))
				fakeManager.GetTxProcessorReturns(verifier, nil)
			})
			It("returns an error", func() {
				err := txProcessor.GenerateSimulationResults(validEnvelope, nil, false)
				Expect(err).To(MatchError("failed committing transaction for channel wild_channel: mock TMSTxProcessor error"))
				Expect(fakeManager.GetTxProcessorCallCount()).To(Equal(1))
				Expect(fakeManager.GetTxProcessorArgsForCall(0)).To(Equal("wild_channel"))
				Expect(verifier.ProcessTxCallCount()).To(Equal(1))
				txID, creatorInfo, ttx, simulator := verifier.ProcessTxArgsForCall(0)
				Expect(txID).To(Equal("tx0"))
				Expect(creatorInfo.Public()).To(BeNil())
				Expect(proto.Equal(ttx, validTtx)).To(BeTrue())
				Expect(simulator).To(BeNil())
			})
		})

		Context("when valid input is passed to an existing channel TxProcessor", func() {
			var (
				verifier *mock.TMSTxProcessor
			)
			BeforeEach(func() {
				verifier = &mock.TMSTxProcessor{}
				verifier.ProcessTxReturns(nil)
				fakeManager.GetTxProcessorReturns(verifier, nil)
			})
			It("succeeds", func() {
				err := txProcessor.GenerateSimulationResults(validEnvelope, nil, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeManager.GetTxProcessorCallCount()).To(Equal(1))
				Expect(fakeManager.GetTxProcessorArgsForCall(0)).To(Equal("wild_channel"))
				Expect(verifier.ProcessTxCallCount()).To(Equal(1))
				txID, creatorInfo, ttx, simulator := verifier.ProcessTxArgsForCall(0)
				Expect(txID).To(Equal("tx0"))
				Expect(creatorInfo.Public()).To(BeNil())
				Expect(proto.Equal(ttx, validTtx)).To(BeTrue())
				Expect(simulator).To(BeNil())
			})
		})
	})

})
