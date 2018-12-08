/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpcmetrics_test

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/hyperledger/fabric/common/grpcmetrics"
	"github.com/hyperledger/fabric/common/grpcmetrics/fakes"
	"github.com/hyperledger/fabric/common/grpcmetrics/testpb"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var _ = Describe("Interceptor", func() {
	var (
		fakeEchoService   *fakes.EchoServiceServer
		echoServiceClient testpb.EchoServiceClient

		fakeRequestDuration   *metricsfakes.Histogram
		fakeRequestsReceived  *metricsfakes.Counter
		fakeRequestsCompleted *metricsfakes.Counter
		fakeMessagesSent      *metricsfakes.Counter
		fakeMessagesReceived  *metricsfakes.Counter

		unaryMetrics  *grpcmetrics.UnaryMetrics
		streamMetrics *grpcmetrics.StreamMetrics

		listener        net.Listener
		serveCompleteCh chan error
		server          *grpc.Server
	)

	BeforeEach(func() {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		fakeEchoService = &fakes.EchoServiceServer{}
		fakeEchoService.EchoStub = func(ctx context.Context, msg *testpb.Message) (*testpb.Message, error) {
			msg.Sequence++
			return msg, nil
		}
		fakeEchoService.EchoStreamStub = func(stream testpb.EchoService_EchoStreamServer) error {
			for {
				msg, err := stream.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}

				msg.Sequence++
				err = stream.Send(msg)
				if err != nil {
					return err
				}
			}
		}

		fakeRequestDuration = &metricsfakes.Histogram{}
		fakeRequestDuration.WithReturns(fakeRequestDuration)
		fakeRequestsReceived = &metricsfakes.Counter{}
		fakeRequestsReceived.WithReturns(fakeRequestsReceived)
		fakeRequestsCompleted = &metricsfakes.Counter{}
		fakeRequestsCompleted.WithReturns(fakeRequestsCompleted)
		fakeMessagesSent = &metricsfakes.Counter{}
		fakeMessagesSent.WithReturns(fakeMessagesSent)
		fakeMessagesReceived = &metricsfakes.Counter{}
		fakeMessagesReceived.WithReturns(fakeMessagesReceived)

		unaryMetrics = &grpcmetrics.UnaryMetrics{
			RequestDuration:   fakeRequestDuration,
			RequestsReceived:  fakeRequestsReceived,
			RequestsCompleted: fakeRequestsCompleted,
		}

		streamMetrics = &grpcmetrics.StreamMetrics{
			RequestDuration:   fakeRequestDuration,
			RequestsReceived:  fakeRequestsReceived,
			RequestsCompleted: fakeRequestsCompleted,
			MessagesSent:      fakeMessagesSent,
			MessagesReceived:  fakeMessagesReceived,
		}

		server = grpc.NewServer(
			grpc.StreamInterceptor(grpcmetrics.StreamServerInterceptor(streamMetrics)),
			grpc.UnaryInterceptor(grpcmetrics.UnaryServerInterceptor(unaryMetrics)),
		)

		testpb.RegisterEchoServiceServer(server, fakeEchoService)
		serveCompleteCh = make(chan error, 1)
		go func() { serveCompleteCh <- server.Serve(listener) }()

		cc, err := grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
		Expect(err).NotTo(HaveOccurred())
		echoServiceClient = testpb.NewEchoServiceClient(cc)
	})

	AfterEach(func() {
		err := listener.Close()
		Expect(err).NotTo(HaveOccurred())
		Eventually(serveCompleteCh).Should(Receive())
	})

	Describe("Unary Metrics", func() {
		It("records request duration", func() {
			resp, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "yo"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&testpb.Message{Message: "yo", Sequence: 1}))

			Expect(fakeRequestDuration.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "Echo",
				"code", "OK",
			}))
			Expect(fakeRequestDuration.ObserveCallCount()).To(Equal(1))
			Expect(fakeRequestDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			Expect(fakeRequestDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})

		It("records requests received before requests completed", func() {
			fakeRequestsReceived.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(0))
			}

			resp, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "yo"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&testpb.Message{Message: "yo", Sequence: 1}))

			Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestsReceived.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "Echo",
			}))
			Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
			Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records requests completed after requests received", func() {
			fakeRequestsCompleted.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
			}

			resp, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "yo"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&testpb.Message{Message: "yo", Sequence: 1}))

			Expect(fakeRequestsCompleted.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestsCompleted.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "Echo",
				"code", "OK",
			}))
			Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(1))
			Expect(fakeRequestsCompleted.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})
	})

	Describe("Stream Metrics", func() {
		It("records request duration", func() {
			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)

			Expect(fakeRequestDuration.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "EchoStream",
				"code", "OK",
			}))
			Expect(fakeRequestDuration.ObserveCallCount()).To(Equal(1))
			Expect(fakeRequestDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			Expect(fakeRequestDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})

		It("records requests received before requests completed", func() {
			fakeRequestsReceived.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeRequestsCompleted.AddCallCount()).To(Equal(0))
			}

			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)

			Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "EchoStream",
				"code", "OK",
			}))
			Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
			Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records requests completed after requests received", func() {
			fakeRequestsCompleted.AddStub = func(delta float64) {
				defer GinkgoRecover()
				Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
			}

			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)

			Expect(fakeRequestsReceived.WithCallCount()).To(Equal(1))
			labelValues := fakeRequestDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "EchoStream",
				"code", "OK",
			}))
			Expect(fakeRequestsReceived.AddCallCount()).To(Equal(1))
			Expect(fakeRequestsReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("records messages sent", func() {
			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)

			Expect(fakeMessagesSent.WithCallCount()).To(Equal(1))
			labelValues := fakeMessagesSent.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "EchoStream",
			}))

			Expect(fakeMessagesSent.AddCallCount()).To(Equal(2))
			for i := 0; i < fakeMessagesSent.AddCallCount(); i++ {
				Expect(fakeMessagesSent.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
			}
		})

		It("records messages received", func() {
			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)

			Expect(fakeMessagesReceived.WithCallCount()).To(Equal(1))
			labelValues := fakeMessagesReceived.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"service", "testpb_EchoService",
				"method", "EchoStream",
			}))

			Expect(fakeMessagesReceived.AddCallCount()).To(Equal(2))
			for i := 0; i < fakeMessagesReceived.AddCallCount(); i++ {
				Expect(fakeMessagesReceived.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
			}
		})

		Context("when stream recv returns an error", func() {
			var errCh chan error

			BeforeEach(func() {
				errCh = make(chan error)
				fakeEchoService.EchoStreamStub = func(svs testpb.EchoService_EchoStreamServer) error { return <-errCh }
			})

			It("does not increment the update count", func() {
				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())

				errCh <- errors.New("oh bother")
				_, err = streamClient.Recv()
				Expect(err).To(MatchError(grpc.Errorf(codes.Unknown, "oh bother")))

				err = streamClient.CloseSend()
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeMessagesReceived.AddCallCount()).To(Equal(0))
			})
		})
	})
})

func streamMessages(streamClient testpb.EchoService_EchoStreamClient) {
	err := streamClient.Send(&testpb.Message{Message: "hello"})
	Expect(err).NotTo(HaveOccurred())
	err = streamClient.Send(&testpb.Message{Message: "hello", Sequence: 2})
	Expect(err).NotTo(HaveOccurred())

	msg, err := streamClient.Recv()
	Expect(err).NotTo(HaveOccurred())
	Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 1}))
	msg, err = streamClient.Recv()
	Expect(err).NotTo(HaveOccurred())
	Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 3}))

	err = streamClient.CloseSend()
	Expect(err).NotTo(HaveOccurred())

	msg, err = streamClient.Recv()
	Expect(err).To(Equal(io.EOF))
}
