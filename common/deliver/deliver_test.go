/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var (
	seekOldest = &ab.SeekPosition{
		Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}},
	}

	seekNewest = &ab.SeekPosition{
		Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}},
	}
)

var _ = Describe("Deliver", func() {
	Describe("NewHandler", func() {
		var fakeChainManager *mock.ChainManager
		var cert *x509.Certificate
		var certBytes []byte
		var serializedIdentity []byte
		BeforeEach(func() {
			fakeChainManager = &mock.ChainManager{}

			ca, err := tlsgen.NewCA()
			Expect(err).NotTo(HaveOccurred())

			certBytes = ca.CertBytes()

			der, _ := pem.Decode(ca.CertBytes())
			cert, err = x509.ParseCertificate(der.Bytes)
			Expect(err).NotTo(HaveOccurred())

			serializedIdentity = protoutil.MarshalOrPanic(&msp.SerializedIdentity{IdBytes: certBytes})
		})

		It("returns a new handler", func() {
			handler := deliver.NewHandler(
				fakeChainManager,
				time.Second,
				false,
				deliver.NewMetrics(&disabled.Provider{}),
				false)
			Expect(handler).NotTo(BeNil())

			Expect(handler.ChainManager).To(Equal(fakeChainManager))
			Expect(handler.TimeWindow).To(Equal(time.Second))
			// binding inspector is func; unable to easily validate
			Expect(handler.BindingInspector).NotTo(BeNil())
		})

		Context("Handler is initialized with expiration checks", func() {
			It("Returns exactly what is found in the certificate", func() {
				handler := deliver.NewHandler(
					fakeChainManager,
					time.Second,
					false,
					deliver.NewMetrics(&disabled.Provider{}),
					false)

				Expect(handler.ExpirationCheckFunc(serializedIdentity)).To(Equal(cert.NotAfter))
			})
		})

		Context("Handler is initialized without expiration checks", func() {
			It("Doesn't parse the NotAfter time of the certificate", func() {
				handler := deliver.NewHandler(
					fakeChainManager,
					time.Second,
					false,
					deliver.NewMetrics(&disabled.Provider{}),
					true)

				Expect(handler.ExpirationCheckFunc(serializedIdentity)).NotTo(Equal(cert.NotAfter))
			})
		})
	})

	Describe("ExtractChannelHeaderCertHash", func() {
		It("extracts the TLS certificate hash from a channel header", func() {
			chdr := &cb.ChannelHeader{TlsCertHash: []byte("tls-cert")}

			result := deliver.ExtractChannelHeaderCertHash(chdr)
			Expect(result).To(Equal([]byte("tls-cert")))
		})

		Context("when the message is not a channel header", func() {
			It("returns nil", func() {
				result := deliver.ExtractChannelHeaderCertHash(&cb.Envelope{})
				Expect(result).To(BeNil())
			})
		})

		Context("when the message is nil", func() {
			It("returns nil", func() {
				var ch *cb.ChannelHeader
				result := deliver.ExtractChannelHeaderCertHash(ch)
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("Handle", func() {
		var (
			errCh                 chan struct{}
			fakeChain             *mock.Chain
			fakeBlockReader       *mock.BlockReader
			fakeBlockIterator     *mock.BlockIterator
			fakeChainManager      *mock.ChainManager
			fakePolicyChecker     *mock.PolicyChecker
			fakeReceiver          *mock.Receiver
			fakeResponseSender    *mock.ResponseSender
			fakeInspector         *mock.Inspector
			fakeStreamsOpened     *metricsfakes.Counter
			fakeStreamsClosed     *metricsfakes.Counter
			fakeRequestsReceived  *metricsfakes.Counter
			fakeRequestsCompleted *metricsfakes.Counter
			fakeBlocksSent        *metricsfakes.Counter

			handler *deliver.Handler
			server  *deliver.Server

			channelHeader *cb.ChannelHeader
			seekInfo      *ab.SeekInfo
			ts            *timestamp.Timestamp

			channelHeaderPayload []byte
			seekInfoPayload      []byte
			envelope             *cb.Envelope
		)

		BeforeEach(func() {
			errCh = make(chan struct{})
			fakeChain = &mock.Chain{}
			fakeChain.ErroredReturns(errCh)

			block := &cb.Block{
				Header: &cb.BlockHeader{Number: 100},
			}
			fakeBlockIterator = &mock.BlockIterator{}
			fakeBlockIterator.NextReturns(block, cb.Status_SUCCESS)

			fakeBlockReader = &mock.BlockReader{}
			fakeBlockReader.HeightReturns(1000)
			fakeBlockReader.IteratorReturns(fakeBlockIterator, 100)
			fakeChain.ReaderReturns(fakeBlockReader)

			fakeChainManager = &mock.ChainManager{}
			fakeChainManager.GetChainReturns(fakeChain)

			fakePolicyChecker = &mock.PolicyChecker{}
			fakeReceiver = &mock.Receiver{}
			fakeResponseSender = &mock.ResponseSender{}
			fakeResponseSender.DataTypeReturns("block")

			fakeInspector = &mock.Inspector{}

			fakeStreamsOpened = &metricsfakes.Counter{}
			fakeStreamsOpened.WithReturns(fakeStreamsOpened)
			fakeStreamsClosed = &metricsfakes.Counter{}
			fakeStreamsClosed.WithReturns(fakeStreamsClosed)
			fakeRequestsReceived = &metricsfakes.Counter{}
			fakeRequestsReceived.WithReturns(fakeRequestsReceived)
			fakeRequestsCompleted = &metricsfakes.Counter{}
			fakeRequestsCompleted.WithReturns(fakeRequestsCompleted)
			fakeBlocksSent = &metricsfakes.Counter{}
			fakeBlocksSent.WithReturns(fakeBlocksSent)

			deliverMetrics := &deliver.Metrics{
				StreamsOpened:     fakeStreamsOpened,
				StreamsClosed:     fakeStreamsClosed,
				RequestsReceived:  fakeRequestsReceived,
				RequestsCompleted: fakeRequestsCompleted,
				BlocksSent:        fakeBlocksSent,
			}

			handler = &deliver.Handler{
				ChainManager:     fakeChainManager,
				TimeWindow:       time.Second,
				BindingInspector: fakeInspector,
				Metrics:          deliverMetrics,
				ExpirationCheckFunc: func([]byte) time.Time {
					return time.Time{}
				},
			}
			server = &deliver.Server{
				Receiver:       fakeReceiver,
				PolicyChecker:  fakePolicyChecker,
				ResponseSender: fakeResponseSender,
			}

			ts = util.CreateUtcTimestamp()
			channelHeader = &cb.ChannelHeader{
				ChannelId: "chain-id",
				Timestamp: ts,
			}
			seekInfo = &ab.SeekInfo{
				Start: &ab.SeekPosition{
					Type: &ab.SeekPosition_Specified{
						Specified: &ab.SeekSpecified{Number: 100},
					},
				},
				Stop: &ab.SeekPosition{
					Type: &ab.SeekPosition_Specified{
						Specified: &ab.SeekSpecified{Number: 100},
					},
				},
			}

			channelHeaderPayload = nil
			seekInfoPayload = nil

			envelope = &cb.Envelope{}
			fakeReceiver.RecvReturns(envelope, nil)
			fakeReceiver.RecvReturnsOnCall(1, nil, io.EOF)
		})

		JustBeforeEach(func() {
			if channelHeaderPayload == nil {
				channelHeaderPayload = protoutil.MarshalOrPanic(channelHeader)
			}
			if seekInfoPayload == nil {
				seekInfoPayload = protoutil.MarshalOrPanic(seekInfo)
			}
			if envelope.Payload == nil {
				payload := &cb.Payload{
					Header: &cb.Header{
						ChannelHeader:   channelHeaderPayload,
						SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{}),
					},
					Data: seekInfoPayload,
				}
				envelope.Payload = protoutil.MarshalOrPanic(payload)
			}
		})

		It("records streams opened before streams closed", func() {
			fakeStreamsOpened.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeStreamsClosed.AddCallCount()).To(Equal(0))
			}

			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeStreamsOpened.AddCallCount()).To(Equal(1))
			Expect(fakeStreamsOpened.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records streams closed after streams opened", func() {
			fakeStreamsClosed.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeStreamsOpened.AddCallCount()).To(Equal(1))
			}

			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeStreamsClosed.AddCallCount()).To(Equal(1))
			Expect(fakeStreamsClosed.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("validates the channel header with the binding inspector", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeInspector.InspectCallCount()).To(Equal(1))
			ctx, header := fakeInspector.InspectArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(proto.Equal(header, channelHeader)).To(BeTrue())
		})

		Context("when channel header validation fails", func() {
			BeforeEach(func() {
				fakeInspector.InspectReturns(errors.New("bad-header-thingy"))
			})

			It("sends a bad request message", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		It("gets the chain from the chain manager", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeChainManager.GetChainCallCount()).To(Equal(1))
			chid := fakeChainManager.GetChainArgsForCall(0)
			Expect(chid).To(Equal("chain-id"))
		})

		It("receives messages until io.EOF is returned", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeReceiver.RecvCallCount()).To(Equal(2))
		})

		It("evaluates access control", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(BeNumerically(">=", 1))
			e, cid := fakePolicyChecker.CheckPolicyArgsForCall(0)
			Expect(proto.Equal(e, envelope)).To(BeTrue())
			Expect(cid).To(Equal("chain-id"))
		})

		It("gets a block iterator from the starting block", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeBlockReader.IteratorCallCount()).To(Equal(1))
			startPosition := fakeBlockReader.IteratorArgsForCall(0)
			Expect(proto.Equal(startPosition, seekInfo.Start)).To(BeTrue())
		})

		Context("when multiple blocks are requested", func() {
			BeforeEach(func() {
				fakeBlockIterator.NextStub = func() (*cb.Block, cb.Status) {
					blk := &cb.Block{
						Header: &cb.BlockHeader{Number: 994 + uint64(fakeBlockIterator.NextCallCount())},
					}
					return blk, cb.Status_SUCCESS
				}
				seekInfo = &ab.SeekInfo{
					Start: &ab.SeekPosition{
						Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 995}},
					},
					Stop: seekNewest,
				}
			})

			It("sends all requested blocks", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(5))
				for i := 0; i < 5; i++ {
					b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(i)
					Expect(b).To(Equal(&cb.Block{
						Header: &cb.BlockHeader{Number: 995 + uint64(i)},
					}))
				}
			})

			It("records requests received, blocks sent, and requests completed", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
				labelValues := fakeRequestsReceived.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block",
				}))

				Expect(fakeBlocksSent.AddCallCount()).To(Equal(5))
				Expect(fakeBlocksSent.WithCallCount()).To(Equal(5))
				for i := 0; i < 5; i++ {
					Expect(fakeBlocksSent.AddArgsForCall(i)).To(BeNumerically("~", 1.0))
					labelValues := fakeBlocksSent.WithArgsForCall(i)
					Expect(labelValues).To(Equal([]string{
						"channel", "chain-id",
						"filtered", "false",
						"data_type", "block",
					}))
				}

				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsCompleted.WithCallCount()).To(Equal(1))
				labelValues = fakeRequestsCompleted.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block",
					"success", "true",
				}))
			})
		})

		Context("when seek info is configured to stop at the oldest block", func() {
			BeforeEach(func() {
				seekInfo = &ab.SeekInfo{Start: &ab.SeekPosition{}, Stop: seekOldest}
			})

			It("sends only the first block returned by the iterator", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeBlockReader.IteratorCallCount()).To(Equal(1))
				start := fakeBlockReader.IteratorArgsForCall(0)
				Expect(start).To(Equal(&ab.SeekPosition{}))
				Expect(fakeBlockIterator.NextCallCount()).To(Equal(1))

				Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(1))
				b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(0)
				Expect(b).To(Equal(&cb.Block{
					Header: &cb.BlockHeader{Number: 100},
				}))
			})
		})

		Context("when seek info is configured to stop at the newest block", func() {
			BeforeEach(func() {
				seekInfo = &ab.SeekInfo{Start: &ab.SeekPosition{}, Stop: seekNewest}

				fakeBlockReader.HeightReturns(3)
				fakeBlockIterator.NextStub = func() (*cb.Block, cb.Status) {
					blk := &cb.Block{
						Header: &cb.BlockHeader{Number: uint64(fakeBlockIterator.NextCallCount())},
					}
					return blk, cb.Status_SUCCESS
				}
			})

			It("sends blocks until the iterator reaches the reader height", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeBlockReader.IteratorCallCount()).To(Equal(1))
				start := fakeBlockReader.IteratorArgsForCall(0)
				Expect(start).To(Equal(&ab.SeekPosition{}))

				Expect(fakeBlockIterator.NextCallCount()).To(Equal(2))
				Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(2))
				for i := 0; i < fakeResponseSender.SendBlockResponseCallCount(); i++ {
					b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(i)
					Expect(b).To(Equal(&cb.Block{
						Header: &cb.BlockHeader{Number: uint64(i + 1)},
					}))
				}
			})
		})

		Context("when seek info is configured to send just the newest block and a new block is committed to the ledger after the iterator is acquired", func() {
			BeforeEach(func() {
				seekInfo = &ab.SeekInfo{Start: seekNewest, Stop: seekNewest}

				fakeBlockReader.IteratorReturns(fakeBlockIterator, 0)
				fakeBlockReader.HeightReturns(2)
				fakeChain.ReaderReturns(fakeBlockReader)
				fakeBlockIterator.NextStub = func() (*cb.Block, cb.Status) {
					blk := &cb.Block{
						Header: &cb.BlockHeader{Number: uint64(fakeBlockIterator.NextCallCount() - 1)},
					}
					return blk, cb.Status_SUCCESS
				}
			})

			It("sends only the newest block at the time the iterator was acquired", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeBlockReader.IteratorCallCount()).To(Equal(1))
				Expect(fakeBlockIterator.NextCallCount()).To(Equal(1))
				Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(1))
				for i := 0; i < fakeResponseSender.SendBlockResponseCallCount(); i++ {
					b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(i)
					Expect(b).To(Equal(&cb.Block{
						Header: &cb.BlockHeader{Number: uint64(i)},
					}))
				}
			})
		})

		Context("when filtered blocks are requested", func() {
			var fakeResponseSender *mock.FilteredResponseSender

			BeforeEach(func() {
				fakeResponseSender = &mock.FilteredResponseSender{}
				fakeResponseSender.IsFilteredReturns(true)
				fakeResponseSender.DataTypeReturns("filtered_block")
				server.ResponseSender = fakeResponseSender
			})

			It("checks if the response sender is filtered", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.IsFilteredCallCount()).To(Equal(1))
			})

			Context("when the response sender indicates it is not filtered", func() {
				BeforeEach(func() {
					fakeResponseSender.IsFilteredReturns(false)
				})

				It("labels the metric with filtered=false", func() {
					err := handler.Handle(context.Background(), server)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
					labelValues := fakeRequestsReceived.WithArgsForCall(0)
					Expect(labelValues).To(Equal([]string{
						"channel", "chain-id",
						"filtered", "false",
						"data_type", "filtered_block",
					}))
				})
			})

			It("records requests received, blocks sent, and requests completed with the filtered label set to true", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
				labelValues := fakeRequestsReceived.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "true",
					"data_type", "filtered_block",
				}))

				Expect(fakeBlocksSent.AddCallCount()).To(Equal(1))
				Expect(fakeBlocksSent.WithCallCount()).To(Equal(1))
				Expect(fakeBlocksSent.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				labelValues = fakeBlocksSent.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "true",
					"data_type", "filtered_block",
				}))

				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsCompleted.WithCallCount()).To(Equal(1))
				labelValues = fakeRequestsCompleted.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "true",
					"data_type", "filtered_block",
					"success", "true",
				}))
			})
		})

		Context("when blocks with private data are requested", func() {
			var fakeResponseSender *mock.PrivateDataResponseSender

			BeforeEach(func() {
				fakeResponseSender = &mock.PrivateDataResponseSender{}
				fakeResponseSender.DataTypeReturns("block_and_pvtdata")
				server.ResponseSender = fakeResponseSender
			})

			It("handles the request and returns private data for all collections", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeResponseSender.DataTypeCallCount()).To(Equal(1))
				Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(1))
				b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(0)
				Expect(b).To(Equal(&cb.Block{
					Header: &cb.BlockHeader{Number: 100},
				}))
			})

			It("records requests received, blocks sent, and requests completed with the privatedata label set to true", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
				labelValues := fakeRequestsReceived.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block_and_pvtdata",
				}))

				Expect(fakeBlocksSent.AddCallCount()).To(Equal(1))
				Expect(fakeBlocksSent.WithCallCount()).To(Equal(1))
				Expect(fakeBlocksSent.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				labelValues = fakeBlocksSent.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block_and_pvtdata",
				}))

				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsCompleted.WithCallCount()).To(Equal(1))
				labelValues = fakeRequestsCompleted.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block_and_pvtdata",
					"success", "true",
				}))
			})
		})

		Context("when sending the block fails", func() {
			BeforeEach(func() {
				fakeResponseSender.SendBlockResponseReturns(errors.New("send-fails"))
			})

			It("returns the error", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).To(MatchError("send-fails"))
			})
		})

		It("sends a success response", func() {
			err := handler.Handle(context.Background(), server)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
			resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
			Expect(resp).To(Equal(cb.Status_SUCCESS))
		})

		It("HandleAttestation sends requested block", func() {
			err := handler.HandleAttestation(context.Background(), server, envelope)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(1))
			b, _, _, _ := fakeResponseSender.SendBlockResponseArgsForCall(0)
			Expect(b).To(Equal(&cb.Block{
				Header: &cb.BlockHeader{Number: 100},
			}))
		})

		Context("when sending the success status fails", func() {
			BeforeEach(func() {
				fakeResponseSender.SendStatusResponseReturns(errors.New("send-success-fails"))
			})

			It("returns the error", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).To(MatchError("send-success-fails"))
			})

			It("HandleAttestation returns the error", func() {
				err := handler.HandleAttestation(context.Background(), server, envelope)
				Expect(err).To(MatchError("send-success-fails"))
			})
		})

		Context("when receive fails", func() {
			BeforeEach(func() {
				fakeReceiver.RecvReturns(nil, errors.New("oh bother"))
			})

			It("returns the error", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).To(MatchError("oh bother"))
			})
		})

		Context("when unmarshalling the payload fails", func() {
			BeforeEach(func() {
				envelope.Payload = []byte("completely-bogus-data")
			})

			It("sends a bad request message", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when the payload header is nil", func() {
			BeforeEach(func() {
				envelope.Payload = protoutil.MarshalOrPanic(&cb.Payload{
					Header: nil,
				})
			})

			It("sends a bad request message", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})

			It("sends a bad envelope to HandleAttestation", func() {
				err := handler.HandleAttestation(context.Background(), server, envelope)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when unmarshalling the channel header fails", func() {
			BeforeEach(func() {
				channelHeaderPayload = []byte("complete-nonsense")
			})

			It("sends a bad request message", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when the channel header timestamp is nil", func() {
			BeforeEach(func() {
				channelHeaderPayload = protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Timestamp: nil,
				})
			})

			It("sends a bad request message", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when the channel header timestamp is out of the time window", func() {
			BeforeEach(func() {
				channelHeaderPayload = protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Timestamp: &timestamp.Timestamp{},
				})
			})

			It("sends status bad request", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when the channel is not found", func() {
			BeforeEach(func() {
				fakeChainManager.GetChainReturns(nil)
			})

			It("sends status not found", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_NOT_FOUND))
			})
		})

		Context("when the client disconnects before reading from the chain", func() {
			var (
				ctx    context.Context
				cancel func()
				done   chan struct{}
			)

			BeforeEach(func() {
				done = make(chan struct{})
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
				fakeBlockIterator.NextStub = func() (*cb.Block, cb.Status) {
					<-done
					return nil, cb.Status_BAD_REQUEST
				}
			})

			AfterEach(func() {
				close(done)
			})

			It("aborts the deliver stream", func() {
				err := handler.Handle(ctx, server)
				Expect(err).To(MatchError("context finished before block retrieved: context canceled"))
			})
		})

		Context("when the chain errors before reading from the chain", func() {
			BeforeEach(func() {
				close(errCh)
			})

			It("sends status service unavailable", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeChain.ReaderCallCount()).To(Equal(0))
				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_SERVICE_UNAVAILABLE))
			})

			Context("when the seek info requests a best effort error response", func() {
				BeforeEach(func() {
					seekInfo.ErrorResponse = ab.SeekInfo_BEST_EFFORT
				})

				It("replies with the desired blocks", func() {
					err := handler.Handle(context.Background(), server)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeResponseSender.SendBlockResponseCallCount()).To(Equal(1))
					Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
					resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
					Expect(resp).To(Equal(cb.Status_SUCCESS))
				})
			})
		})

		Context("when the chain errors while reading from the chain", func() {
			var doneCh chan struct{}

			BeforeEach(func() {
				doneCh = make(chan struct{})
				fakeBlockIterator.NextStub = func() (*cb.Block, cb.Status) {
					<-doneCh
					return &cb.Block{}, cb.Status_INTERNAL_SERVER_ERROR
				}
				fakeChain.ReaderStub = func() blockledger.Reader {
					close(errCh)
					return fakeBlockReader
				}
			})

			AfterEach(func() {
				close(doneCh)
			})

			It("sends status service unavailable", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeChain.ReaderCallCount()).To(Equal(1))
				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_SERVICE_UNAVAILABLE))
			})
		})

		Context("when the access evaluation fails", func() {
			BeforeEach(func() {
				fakePolicyChecker.CheckPolicyReturns(errors.New("no-access-for-you"))
			})

			It("sends status not found", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_FORBIDDEN))
			})

			It("records requests received, (unsuccessful) requests completed, and (zero) blocks sent", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
				labelValues := fakeRequestsReceived.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block",
				}))

				Expect(fakeBlocksSent.AddCallCount()).To(Equal(0))
				Expect(fakeBlocksSent.WithCallCount()).To(Equal(0))

				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(1))
				Expect(fakeRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
				Expect(fakeRequestsCompleted.WithCallCount()).To(Equal(1))
				labelValues = fakeRequestsCompleted.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"channel", "chain-id",
					"filtered", "false",
					"data_type", "block",
					"success", "false",
				}))
			})
		})

		Context("when the access expires", func() {
			BeforeEach(func() {
				fakeChain.SequenceStub = func() uint64 {
					return uint64(fakeChain.SequenceCallCount())
				}
				fakePolicyChecker.CheckPolicyReturnsOnCall(1, errors.New("no-access-for-you"))
			})

			It("sends status not found", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_FORBIDDEN))

				Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(2))
			})
		})

		Context("when unmarshalling seek info fails", func() {
			BeforeEach(func() {
				seekInfoPayload = []byte("complete-nonsense")
			})

			It("sends status bad request", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when seek start and stop are nil", func() {
			BeforeEach(func() {
				seekInfo = &ab.SeekInfo{Start: nil, Stop: nil}
			})

			It("sends status bad request", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when seek info start number is greater than stop number", func() {
			BeforeEach(func() {
				seekInfo = &ab.SeekInfo{
					Start: seekNewest,
					Stop: &ab.SeekPosition{
						Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 99}},
					},
				}
			})

			It("sends status bad request", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_BAD_REQUEST))
			})
		})

		Context("when fail if not ready is set and the next block is unavailable", func() {
			BeforeEach(func() {
				fakeBlockReader.HeightReturns(1000)
				fakeBlockReader.IteratorReturns(fakeBlockIterator, 1000)

				seekInfo = &ab.SeekInfo{
					Behavior: ab.SeekInfo_FAIL_IF_NOT_READY,
					Start: &ab.SeekPosition{
						Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1002}},
					},
					Stop: &ab.SeekPosition{
						Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1003}},
					},
				}
			})

			It("sends status not found", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeBlockIterator.NextCallCount()).To(Equal(0))
				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_NOT_FOUND))
			})
		})

		Context("when next block status does not indicate success", func() {
			BeforeEach(func() {
				fakeBlockIterator.NextReturns(nil, cb.Status_UNKNOWN)
			})

			It("forwards the status response", func() {
				err := handler.Handle(context.Background(), server)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeResponseSender.SendStatusResponseCallCount()).To(Equal(1))
				resp := fakeResponseSender.SendStatusResponseArgsForCall(0)
				Expect(resp).To(Equal(cb.Status_UNKNOWN))
			})
		})
	})
})
