/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockcutter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/blockcutter/mock"
)

var _ = Describe("Blockcutter", func() {
	var (
		bc                blockcutter.Receiver
		fakeConfig        *mock.OrdererConfig
		fakeConfigFetcher *mock.OrdererConfigFetcher

		metrics               *blockcutter.Metrics
		fakeBlockFillDuration *mock.MetricsHistogram
	)

	BeforeEach(func() {
		fakeConfig = &mock.OrdererConfig{}
		fakeConfigFetcher = &mock.OrdererConfigFetcher{}
		fakeConfigFetcher.OrdererConfigReturns(fakeConfig, true)

		fakeBlockFillDuration = &mock.MetricsHistogram{}
		fakeBlockFillDuration.WithReturns(fakeBlockFillDuration)
		metrics = &blockcutter.Metrics{
			BlockFillDuration: fakeBlockFillDuration,
		}

		bc = blockcutter.NewReceiverImpl("mychannel", fakeConfigFetcher, metrics)
	})

	Describe("Ordered", func() {
		var message *cb.Envelope

		BeforeEach(func() {
			fakeConfig.BatchSizeReturns(&ab.BatchSize{
				MaxMessageCount:   2,
				PreferredMaxBytes: 100,
			})

			message = &cb.Envelope{Payload: []byte("Twenty Bytes of Data"), Signature: []byte("Twenty Bytes of Data")}
		})

		It("adds the message to the pending batches", func() {
			batches, pending := bc.Ordered(message)
			Expect(batches).To(BeEmpty())
			Expect(pending).To(BeTrue())
			Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(0))
		})

		Context("when enough batches to fill the max message count are enqueued", func() {
			It("cuts the batch", func() {
				batches, pending := bc.Ordered(message)
				Expect(batches).To(BeEmpty())
				Expect(pending).To(BeTrue())
				batches, pending = bc.Ordered(message)
				Expect(len(batches)).To(Equal(1))
				Expect(len(batches[0])).To(Equal(2))
				Expect(pending).To(BeFalse())

				Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically(">", 0))
				Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1))
				Expect(fakeBlockFillDuration.WithCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.WithArgsForCall(0)).To(Equal([]string{"channel", "mychannel"}))
			})
		})

		Context("when the message does not exceed max message count or preferred size", func() {
			BeforeEach(func() {
				fakeConfig.BatchSizeReturns(&ab.BatchSize{
					MaxMessageCount:   3,
					PreferredMaxBytes: 100,
				})
			})

			It("adds the message to the pending batches", func() {
				batches, pending := bc.Ordered(message)
				Expect(batches).To(BeEmpty())
				Expect(pending).To(BeTrue())
				batches, pending = bc.Ordered(message)
				Expect(batches).To(BeEmpty())
				Expect(pending).To(BeTrue())
				Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(0))
			})
		})

		Context("when the message is larger than the preferred max bytes", func() {
			BeforeEach(func() {
				fakeConfig.BatchSizeReturns(&ab.BatchSize{
					MaxMessageCount:   3,
					PreferredMaxBytes: 30,
				})
			})

			It("cuts the batch immediately", func() {
				batches, pending := bc.Ordered(message)
				Expect(len(batches)).To(Equal(1))
				Expect(pending).To(BeFalse())
				Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(Equal(float64(0)))
				Expect(fakeBlockFillDuration.WithCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.WithArgsForCall(0)).To(Equal([]string{"channel", "mychannel"}))
			})
		})

		Context("when the message causes the batch to exceed the preferred max bytes", func() {
			BeforeEach(func() {
				fakeConfig.BatchSizeReturns(&ab.BatchSize{
					MaxMessageCount:   3,
					PreferredMaxBytes: 50,
				})
			})

			It("cuts the previous batch immediately, enqueueing the second", func() {
				batches, pending := bc.Ordered(message)
				Expect(batches).To(BeEmpty())
				Expect(pending).To(BeTrue())

				batches, pending = bc.Ordered(message)
				Expect(len(batches)).To(Equal(1))
				Expect(len(batches[0])).To(Equal(1))
				Expect(pending).To(BeTrue())

				Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically(">", 0))
				Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1))
				Expect(fakeBlockFillDuration.WithCallCount()).To(Equal(1))
				Expect(fakeBlockFillDuration.WithArgsForCall(0)).To(Equal([]string{"channel", "mychannel"}))
			})

			Context("when the new message is larger than the preferred max bytes", func() {
				var bigMessage *cb.Envelope

				BeforeEach(func() {
					bigMessage = &cb.Envelope{Payload: make([]byte, 1000)}
				})

				It("cuts both the previous batch and the next batch immediately", func() {
					batches, pending := bc.Ordered(message)
					Expect(batches).To(BeEmpty())
					Expect(pending).To(BeTrue())

					batches, pending = bc.Ordered(bigMessage)
					Expect(len(batches)).To(Equal(2))
					Expect(len(batches[0])).To(Equal(1))
					Expect(len(batches[1])).To(Equal(1))
					Expect(pending).To(BeFalse())

					Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(2))
					Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically(">", 0))
					Expect(fakeBlockFillDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1))
					Expect(fakeBlockFillDuration.ObserveArgsForCall(1)).To(Equal(float64(0)))
					Expect(fakeBlockFillDuration.WithCallCount()).To(Equal(2))
					Expect(fakeBlockFillDuration.WithArgsForCall(0)).To(Equal([]string{"channel", "mychannel"}))
					Expect(fakeBlockFillDuration.WithArgsForCall(1)).To(Equal([]string{"channel", "mychannel"}))
				})
			})
		})

		Context("when the orderer config cannot be retrieved", func() {
			BeforeEach(func() {
				fakeConfigFetcher.OrdererConfigReturns(nil, false)
			})

			It("panics", func() {
				Expect(func() { bc.Ordered(message) }).To(Panic())
			})
		})
	})

	Describe("Cut", func() {
		It("cuts an empty batch", func() {
			batch := bc.Cut()
			Expect(batch).To(BeNil())
			Expect(fakeBlockFillDuration.ObserveCallCount()).To(Equal(0))
		})
	})
})
