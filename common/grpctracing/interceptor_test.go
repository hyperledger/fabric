/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpctracing_test

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/hyperledger/fabric/common/grpctracing"
	"github.com/hyperledger/fabric/common/grpctracing/fakes"
	"github.com/hyperledger/fabric/common/grpctracing/testpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TracesCollector struct {
	spansCollected []sdktrace.ReadOnlySpan
}

func (t *TracesCollector) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	t.spansCollected = append(t.spansCollected, spans...)
	return nil
}

func (t *TracesCollector) Shutdown(ctx context.Context) error {
	return nil
}

var _ = Describe("Interceptor", func() {
	var (
		tracesCollector   *TracesCollector
		tracerProvider    *sdktrace.TracerProvider
		fakeEchoService   *fakes.EchoServiceServer
		echoServiceClient testpb.EchoServiceClient

		listener        net.Listener
		serveCompleteCh chan error
		server          *grpc.Server
	)

	BeforeEach(func() {
		tracesCollector = &TracesCollector{}
		bsp := sdktrace.NewBatchSpanProcessor(tracesCollector)
		tracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(bsp), sdktrace.WithSampler(sdktrace.AlwaysSample()))
		otel.SetTracerProvider(tracerProvider)

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

		server = grpc.NewServer(
			grpc.StreamInterceptor(grpctracing.StreamServerInterceptor()),
			grpc.UnaryInterceptor(grpctracing.UnaryServerInterceptor()),
		)

		testpb.RegisterEchoServiceServer(server, fakeEchoService)
		serveCompleteCh = make(chan error, 1)
		go func() { serveCompleteCh <- server.Serve(listener) }()

		cc, err := grpc.Dial(listener.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
		Expect(err).NotTo(HaveOccurred())
		echoServiceClient = testpb.NewEchoServiceClient(cc)
	})

	AfterEach(func() {
		err := listener.Close()
		Expect(err).NotTo(HaveOccurred())
		Eventually(serveCompleteCh).Should(Receive())
	})

	Describe("Unary Traces", func() {
		It("records traces", func() {
			resp, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "yo"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message).To(Equal("yo"))
			Expect(resp.Sequence).To(Equal(int32(1)))
			tracerProvider.ForceFlush(context.Background())
			Expect(len(tracesCollector.spansCollected)).To(Equal(1))
		})
	})

	Describe("Stream Traces", func() {
		It("records traces", func() {
			streamClient, err := echoServiceClient.EchoStream(context.Background())
			Expect(err).NotTo(HaveOccurred())
			streamMessages(streamClient)
			tracerProvider.ForceFlush(context.Background())
			Expect(len(tracesCollector.spansCollected)).To(Equal(1))
		})

		Context("when stream recv returns an error", func() {
			var errCh chan error

			BeforeEach(func() {
				errCh = make(chan error)
				fakeEchoService.EchoStreamStub = func(svs testpb.EchoService_EchoStreamServer) error {
					return <-errCh
				}
			})

			It("records a trace with an error", func() {
				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())

				errCh <- errors.New("oh bother")
				_, err = streamClient.Recv()
				Expect(err).To(MatchError(status.Errorf(codes.Unknown, "oh bother")))

				err = streamClient.CloseSend()
				Expect(err).NotTo(HaveOccurred())

				_, err = streamClient.Recv()
				Expect(err).To(MatchError(status.Errorf(codes.Unknown, "oh bother")))

				tracerProvider.ForceFlush(context.Background())
				Expect(len(tracesCollector.spansCollected)).To(Equal(1))
				trace := tracesCollector.spansCollected[0]
				Expect(trace.Status().Code).To(Equal(otelcodes.Error))
			})
		})
	})
})

func streamMessages(streamClient testpb.EchoService_EchoStreamClient) {
	err := streamClient.Send(&testpb.Message{Message: "hello"})
	Expect(err).NotTo(HaveOccurred())
	err = streamClient.Send(&testpb.Message{Message: "hello", Sequence: 2})
	Expect(err).NotTo(HaveOccurred())

	_, err = streamClient.Recv()
	Expect(err).NotTo(HaveOccurred())
	_, err = streamClient.Recv()
	Expect(err).NotTo(HaveOccurred())

	err = streamClient.CloseSend()
	Expect(err).NotTo(HaveOccurred())

	msg, err := streamClient.Recv()
	Expect(err).To(Equal(io.EOF))
	Expect(msg).To(BeNil())
}
