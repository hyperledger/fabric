/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast_test

import (
	"context"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var _ = Describe("Broadcast", func() {
	var (
		fakeSupportRegistrar  *mock.ChannelSupportRegistrar
		handler               *broadcast.Handler
		fakeValidateHistogram *mock.MetricsHistogram
		fakeEnqueueHistogram  *mock.MetricsHistogram
		fakeProcessedCounter  *mock.MetricsCounter
	)

	BeforeEach(func() {
		fakeSupportRegistrar = &mock.ChannelSupportRegistrar{}

		fakeValidateHistogram = &mock.MetricsHistogram{}
		fakeValidateHistogram.WithReturns(fakeValidateHistogram)

		fakeEnqueueHistogram = &mock.MetricsHistogram{}
		fakeEnqueueHistogram.WithReturns(fakeEnqueueHistogram)

		fakeProcessedCounter = &mock.MetricsCounter{}
		fakeProcessedCounter.WithReturns(fakeProcessedCounter)

		handler = &broadcast.Handler{
			SupportRegistrar: fakeSupportRegistrar,
			Metrics: &broadcast.Metrics{
				ValidateDuration: fakeValidateHistogram,
				EnqueueDuration:  fakeEnqueueHistogram,
				ProcessedCount:   fakeProcessedCounter,
			},
		}
	})

	Describe("Handle", func() {
		var (
			fakeABServer *mock.ABServer
			fakeSupport  *mock.ChannelSupport
			fakeMsg      *cb.Envelope
		)

		BeforeEach(func() {
			fakeMsg = &cb.Envelope{}

			fakeABServer = &mock.ABServer{}
			fakeABServer.ContextReturns(context.TODO())
			fakeABServer.RecvReturns(fakeMsg, nil)
			fakeABServer.RecvReturnsOnCall(1, nil, io.EOF)

			fakeSupport = &mock.ChannelSupport{}
			fakeSupport.ProcessNormalMsgReturns(5, nil)

			fakeSupportRegistrar.BroadcastChannelSupportReturns(&cb.ChannelHeader{
				Type:      3,
				ChannelId: "fake-channel",
			}, false, fakeSupport, nil)
		})

		It("enqueues the message to the consenter", func() {
			err := handler.Handle(fakeABServer)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeABServer.RecvCallCount()).To(Equal(2))
			Expect(fakeSupportRegistrar.BroadcastChannelSupportCallCount()).To(Equal(1))
			Expect(fakeSupportRegistrar.BroadcastChannelSupportArgsForCall(0)).To(Equal(fakeMsg))

			Expect(fakeSupport.ProcessNormalMsgCallCount()).To(Equal(1))
			Expect(fakeSupport.ProcessNormalMsgArgsForCall(0)).To(Equal(fakeMsg))

			Expect(fakeSupport.WaitReadyCallCount()).To(Equal(1))

			Expect(fakeSupport.OrderCallCount()).To(Equal(1))
			orderedMsg, seq := fakeSupport.OrderArgsForCall(0)
			Expect(orderedMsg).To(Equal(fakeMsg))
			Expect(seq).To(Equal(uint64(5)))

			Expect(fakeValidateHistogram.WithCallCount()).To(Equal(1))
			Expect(fakeValidateHistogram.WithArgsForCall(0)).To(Equal([]string{
				"status", "SUCCESS",
				"channel", "fake-channel",
				"type", "ENDORSER_TRANSACTION",
			}))
			Expect(fakeValidateHistogram.ObserveCallCount()).To(Equal(1))
			Expect(fakeValidateHistogram.ObserveArgsForCall(0)).To(BeNumerically(">", 0))
			Expect(fakeValidateHistogram.ObserveArgsForCall(0)).To(BeNumerically("<", 1))

			Expect(fakeEnqueueHistogram.WithCallCount()).To(Equal(1))
			Expect(fakeEnqueueHistogram.WithArgsForCall(0)).To(Equal([]string{
				"status", "SUCCESS",
				"channel", "fake-channel",
				"type", "ENDORSER_TRANSACTION",
			}))
			Expect(fakeEnqueueHistogram.ObserveCallCount()).To(Equal(1))
			Expect(fakeEnqueueHistogram.ObserveArgsForCall(0)).To(BeNumerically(">", 0))
			Expect(fakeEnqueueHistogram.ObserveArgsForCall(0)).To(BeNumerically("<", 1))

			Expect(fakeProcessedCounter.WithCallCount()).To(Equal(1))
			Expect(fakeProcessedCounter.WithArgsForCall(0)).To(Equal([]string{
				"status", "SUCCESS",
				"channel", "fake-channel",
				"type", "ENDORSER_TRANSACTION",
			}))
			Expect(fakeProcessedCounter.AddCallCount()).To(Equal(1))
			Expect(fakeProcessedCounter.AddArgsForCall(0)).To(Equal(float64(1)))

			Expect(fakeABServer.SendCallCount()).To(Equal(1))
			Expect(proto.Equal(fakeABServer.SendArgsForCall(0), &ab.BroadcastResponse{Status: cb.Status_SUCCESS})).To(BeTrue())
		})

		Context("when the channel support cannot be retrieved", func() {
			BeforeEach(func() {
				fakeSupportRegistrar.BroadcastChannelSupportReturns(&cb.ChannelHeader{
					Type:      2,
					ChannelId: "fake-channel",
				}, false, nil, fmt.Errorf("support-error"))
			})

			It("returns the error to the client with a bad status", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeABServer.SendCallCount()).To(Equal(1))
				Expect(proto.Equal(
					fakeABServer.SendArgsForCall(0),
					&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST, Info: "support-error"}),
				).To(BeTrue())
			})

			Context("when the channel header is not validly decoded", func() {
				BeforeEach(func() {
					fakeSupportRegistrar.BroadcastChannelSupportReturns(nil, false, nil, fmt.Errorf("support-error"))
				})

				It("does not crash", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeValidateHistogram.WithCallCount()).To(Equal(1))
					Expect(fakeValidateHistogram.WithArgsForCall(0)).To(Equal([]string{
						"status", "BAD_REQUEST",
						"channel", "unknown",
						"type", "unknown",
					}))
					Expect(fakeEnqueueHistogram.WithCallCount()).To(Equal(0))
					Expect(fakeProcessedCounter.WithCallCount()).To(Equal(1))
				})
			})

		})

		Context("when the receive from the client fails", func() {
			BeforeEach(func() {
				fakeABServer.RecvReturns(nil, fmt.Errorf("recv-error"))
			})

			It("returns the error", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).To(MatchError("recv-error"))
			})
		})

		Context("when the consenter is not ready for the request", func() {
			BeforeEach(func() {
				fakeSupport.WaitReadyReturns(fmt.Errorf("not-ready"))
			})

			It("returns the error to the client with a service unavailable status", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeABServer.SendCallCount()).To(Equal(1))
				Expect(proto.Equal(
					fakeABServer.SendArgsForCall(0),
					&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: "not-ready"}),
				).To(BeTrue())
			})
		})

		Context("when the send to the client fails", func() {
			BeforeEach(func() {
				fakeABServer.SendReturns(fmt.Errorf("send-error"))
			})

			It("returns the error", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).To(MatchError("send-error"))
			})
		})

		Context("when the consenter cannot enqueue the message", func() {
			BeforeEach(func() {
				fakeSupport.OrderReturns(fmt.Errorf("consenter-error"))
			})

			It("returns the error", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeABServer.SendCallCount()).To(Equal(1))
				Expect(proto.Equal(
					fakeABServer.SendArgsForCall(0),
					&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: "consenter-error"}),
				).To(BeTrue())
			})
		})

		Context("when the message processor returns an error", func() {
			BeforeEach(func() {
				fakeSupport.ProcessNormalMsgReturns(0, fmt.Errorf("normal-messsage-processing-error"))
			})

			It("returns the error and an error status", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeABServer.SendCallCount()).To(Equal(1))
				Expect(proto.Equal(
					fakeABServer.SendArgsForCall(0),
					&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST, Info: "normal-messsage-processing-error"},
				)).To(BeTrue())
			})

			Context("when the error cause is msgprocessor.ErrChannelDoesNotExist", func() {
				BeforeEach(func() {
					fakeSupport.ProcessNormalMsgReturns(0, msgprocessor.ErrChannelDoesNotExist)
				})

				It("returns the error and a not found status", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeABServer.SendCallCount()).To(Equal(1))
					Expect(proto.Equal(
						fakeABServer.SendArgsForCall(0),
						&ab.BroadcastResponse{Status: cb.Status_NOT_FOUND, Info: msgprocessor.ErrChannelDoesNotExist.Error()},
					)).To(BeTrue())
				})
			})

			Context("when the error cause is msgprocessor.ErrPermissionDenied", func() {
				BeforeEach(func() {
					fakeSupport.ProcessNormalMsgReturns(0, msgprocessor.ErrPermissionDenied)
				})

				It("returns the error and a not found status", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeABServer.SendCallCount()).To(Equal(1))
					Expect(proto.Equal(
						fakeABServer.SendArgsForCall(0),
						&ab.BroadcastResponse{Status: cb.Status_FORBIDDEN, Info: msgprocessor.ErrPermissionDenied.Error()},
					)).To(BeTrue())
				})
			})
		})

		Context("when the message is a config message", func() {
			var (
				fakeConfig *cb.Envelope
			)

			BeforeEach(func() {
				fakeConfig = &cb.Envelope{}

				fakeSupportRegistrar.BroadcastChannelSupportReturns(&cb.ChannelHeader{
					Type:      1,
					ChannelId: "fake-channel",
				}, true, fakeSupport, nil)

				fakeSupport.ProcessConfigUpdateMsgReturns(fakeConfig, 3, nil)
			})

			It("enqueues the message as a config message to the consenter", func() {
				err := handler.Handle(fakeABServer)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeSupport.ProcessNormalMsgCallCount()).To(Equal(0))
				Expect(fakeSupport.ProcessConfigUpdateMsgCallCount()).To(Equal(1))
				Expect(fakeSupport.ProcessConfigUpdateMsgArgsForCall(0)).To(Equal(fakeMsg))

				Expect(fakeSupport.WaitReadyCallCount()).To(Equal(1))

				Expect(fakeSupport.OrderCallCount()).To(Equal(0))
				Expect(fakeSupport.ConfigureCallCount()).To(Equal(1))
				configMsg, seq := fakeSupport.ConfigureArgsForCall(0)
				Expect(configMsg).To(Equal(fakeConfig))
				Expect(seq).To(Equal(uint64(3)))

				Expect(fakeABServer.SendCallCount()).To(Equal(1))
				Expect(proto.Equal(fakeABServer.SendArgsForCall(0), &ab.BroadcastResponse{Status: cb.Status_SUCCESS})).To(BeTrue())
			})

			Context("when the consenter is not ready for the request", func() {
				BeforeEach(func() {
					fakeSupport.WaitReadyReturns(fmt.Errorf("not-ready"))
				})

				It("returns the error to the client with a service unavailable status", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeABServer.SendCallCount()).To(Equal(1))
					Expect(proto.Equal(
						fakeABServer.SendArgsForCall(0),
						&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: "not-ready"}),
					).To(BeTrue())
				})
			})

			Context("when the consenter cannot enqueue the message", func() {
				BeforeEach(func() {
					fakeSupport.ConfigureReturns(fmt.Errorf("consenter-error"))
				})

				It("returns the error", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeABServer.SendCallCount()).To(Equal(1))
					Expect(proto.Equal(
						fakeABServer.SendArgsForCall(0),
						&ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: "consenter-error"}),
					).To(BeTrue())
				})
			})

			Context("when the processing of the config update fails", func() {
				BeforeEach(func() {
					fakeSupport.ProcessConfigUpdateMsgReturns(nil, 0, fmt.Errorf("config-processing-error"))
				})

				It("returns the error with a bad_status", func() {
					err := handler.Handle(fakeABServer)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeABServer.SendCallCount()).To(Equal(1))
					Expect(proto.Equal(
						fakeABServer.SendArgsForCall(0),
						&ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST, Info: "config-processing-error"},
					)).To(BeTrue())
				})

				Context("when the error cause is msgprocessor.ErrChannelDoesNotExist", func() {
					BeforeEach(func() {
						fakeSupport.ProcessConfigUpdateMsgReturns(nil, 0, msgprocessor.ErrChannelDoesNotExist)
					})

					It("returns the error and a not found status", func() {
						err := handler.Handle(fakeABServer)
						Expect(err).NotTo(HaveOccurred())

						Expect(fakeABServer.SendCallCount()).To(Equal(1))
						Expect(proto.Equal(
							fakeABServer.SendArgsForCall(0),
							&ab.BroadcastResponse{Status: cb.Status_NOT_FOUND, Info: msgprocessor.ErrChannelDoesNotExist.Error()},
						)).To(BeTrue())
					})
				})

				Context("when the error cause is msgprocessor.ErrPermissionDenied", func() {
					BeforeEach(func() {
						fakeSupport.ProcessConfigUpdateMsgReturns(nil, 0, msgprocessor.ErrPermissionDenied)
					})

					It("returns the error and a not found status", func() {
						err := handler.Handle(fakeABServer)
						Expect(err).NotTo(HaveOccurred())

						Expect(fakeABServer.SendCallCount()).To(Equal(1))
						Expect(proto.Equal(
							fakeABServer.SendArgsForCall(0),
							&ab.BroadcastResponse{Status: cb.Status_FORBIDDEN, Info: msgprocessor.ErrPermissionDenied.Error()},
						)).To(BeTrue())
					})
				})
			})
		})
	})
})
