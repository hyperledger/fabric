/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/hyperledger/fabric/common/grpclogging/fakes"
	"github.com/hyperledger/fabric/common/grpclogging/testpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const TimeThreshold = 100 * time.Millisecond

var _ = Describe("Server", func() {
	var (
		fakeEchoService   *fakes.EchoServiceServer
		echoServiceClient testpb.EchoServiceClient

		caCertPool        *x509.CertPool
		clientCertWithKey tls.Certificate

		listener        net.Listener
		serveCompleteCh chan error
		server          *grpc.Server
		clientConn      *grpc.ClientConn

		core     zapcore.Core
		observed *observer.ObservedLogs
		logger   *zap.Logger
	)

	BeforeEach(func() {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())

		core, observed = observer.New(zap.LevelEnablerFunc(func(zapcore.Level) bool { return true }))
		logger = zap.New(core, zap.AddCaller()).Named("test-logger")

		caCert, caKey := generateCA("test-ca", "127.0.0.1")
		clientCert, clientKey := issueCertificate(caCert, caKey, "client", "127.0.0.1")
		clientCertWithKey, err = tls.X509KeyPair(clientCert, clientKey)
		Expect(err).NotTo(HaveOccurred())
		serverCert, serverKey := issueCertificate(caCert, caKey, "server", "127.0.0.1")
		serverCertWithKey, err := tls.X509KeyPair(serverCert, serverKey)
		Expect(err).NotTo(HaveOccurred())

		caCertPool = x509.NewCertPool()
		added := caCertPool.AppendCertsFromPEM(caCert)
		Expect(added).To(BeTrue())

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
				return stream.Send(msg)
			}
		}

		serverTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{serverCertWithKey},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ClientCAs:    caCertPool,
			RootCAs:      caCertPool,
		}
		serverTLSConfig.BuildNameToCertificate()
		server = grpc.NewServer(
			grpc.Creds(credentials.NewTLS(serverTLSConfig)),
			grpc.StreamInterceptor(grpclogging.StreamServerInterceptor(logger)),
			grpc.UnaryInterceptor(grpclogging.UnaryServerInterceptor(logger)),
		)

		testpb.RegisterEchoServiceServer(server, fakeEchoService)
		serveCompleteCh = make(chan error, 1)
		go func() { serveCompleteCh <- server.Serve(listener) }()

		clientTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCertWithKey},
			RootCAs:      caCertPool,
		}
		clientTLSConfig.BuildNameToCertificate()
		dialOpts := []grpc.DialOption{
			grpc.WithTransportCredentials(credentials.NewTLS(clientTLSConfig)),
		}
		clientConn, err = grpc.Dial(listener.Addr().String(), dialOpts...)
		Expect(err).NotTo(HaveOccurred())

		echoServiceClient = testpb.NewEchoServiceClient(clientConn)
	})

	AfterEach(func() {
		clientConn.Close()

		err := listener.Close()
		Expect(err).NotTo(HaveOccurred())
		Eventually(serveCompleteCh).Should(Receive())
	})

	Describe("UnaryServerInterceptor", func() {
		It("logs request data", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()

			resp, err := echoServiceClient.Echo(ctx, &testpb.Message{Message: "hi"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&testpb.Message{Message: "hi", Sequence: 1}))

			var logMessages []string
			for _, entry := range observed.AllUntimed() {
				logMessages = append(logMessages, entry.Message)
			}
			Expect(logMessages).To(ConsistOf(
				"received unary request", // received payload
				"sending unary response", // sending payload
				"unary call completed",
			))

			for _, entry := range observed.AllUntimed() {
				keyNames := map[string]struct{}{}
				for _, field := range entry.Context {
					keyNames[field.Key] = struct{}{}
				}

				switch entry.LoggerName {
				case "test-logger":
					Expect(entry.Level).To(Equal(zapcore.InfoLevel))
					Expect(entry.Context).To(HaveLen(8))
					Expect(keyNames).To(HaveLen(8))
				case "test-logger.payload":
					Expect(entry.Level).To(Equal(zapcore.DebugLevel - 1))
					Expect(entry.Context).To(HaveLen(6))
					Expect(keyNames).To(HaveLen(6))
				default:
					Fail("unexpected logger name: " + entry.LoggerName)
				}
				Expect(entry.Caller.String()).To(ContainSubstring("grpclogging/server.go"))

				for _, field := range entry.Context {
					switch field.Key {
					case "grpc.code":
						Expect(field.Type).To(Equal(zapcore.StringerType))
						Expect(field.Interface).To(Equal(codes.OK))
					case "grpc.call_duration":
						Expect(field.Type).To(Equal(zapcore.DurationType))
						Expect(field.Integer).NotTo(BeZero())
					case "grpc.service":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(Equal("testpb.EchoService"))
					case "grpc.method":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(Equal("Echo"))
					case "grpc.request_deadline":
						deadline, ok := ctx.Deadline()
						Expect(ok).To(BeTrue())
						Expect(field.Type).To(Equal(zapcore.TimeType))
						Expect(field.Integer).NotTo(BeZero())
						Expect(time.Unix(0, field.Integer)).To(BeTemporally("~", deadline, TimeThreshold))
					case "grpc.peer_address":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(HavePrefix("127.0.0.1"))
					case "grpc.peer_subject":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(HavePrefix("CN=client"))
					case "message":
						Expect(field.Type).To(Equal(zapcore.ReflectType))
					case "error":
						Expect(field.Type).To(Equal(zapcore.ErrorType))
					case "":
						Expect(field.Type).To(Equal(zapcore.SkipType))
					default:
						Fail("unexpected context field: " + field.Key)
					}
				}
			}
		})

		It("provides a decorated context", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()
			_, err := echoServiceClient.Echo(ctx, &testpb.Message{Message: "hi"})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeEchoService.EchoCallCount()).To(Equal(1))
			echoContext, _ := fakeEchoService.EchoArgsForCall(0)
			zapFields := grpclogging.ZapFields(echoContext)

			keyNames := []string{}
			for _, field := range zapFields {
				keyNames = append(keyNames, field.Key)
			}
			Expect(keyNames).To(ConsistOf(
				"grpc.service",
				"grpc.method",
				"grpc.request_deadline",
				"grpc.peer_address",
				"grpc.peer_subject",
			))
		})

		Context("when the request ends with an unknown error", func() {
			var expectedErr error

			BeforeEach(func() {
				expectedErr = errors.New("gah!")
				fakeEchoService.EchoReturns(nil, expectedErr)

				_, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "hi"})
				Expect(err).To(HaveOccurred())
			})

			It("logs the unknown code", func() {
				entries := observed.FilterMessage("unary call completed").FilterField(zap.Stringer("grpc.code", codes.Unknown)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})

			It("logs the error", func() {
				entries := observed.FilterMessage("unary call completed").FilterField(grpclogging.Error(expectedErr)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})
		})

		Context("when the request ends with a grpc status error", func() {
			var expectedErr error

			BeforeEach(func() {
				expectedErr = &statusError{Status: status.New(codes.Aborted, "aborted")}
				fakeEchoService.EchoReturns(nil, expectedErr)

				_, err := echoServiceClient.Echo(context.Background(), &testpb.Message{Message: "hi"})
				Expect(err).To(HaveOccurred())
			})

			It("logs the corect code", func() {
				entries := observed.FilterMessage("unary call completed").FilterField(zap.Stringer("grpc.code", codes.Aborted)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})

			It("logs the error", func() {
				entries := observed.FilterMessage("unary call completed").FilterField(grpclogging.Error(expectedErr)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})
		})

		Context("when options are used", func() {
			var (
				listener        net.Listener
				serveCompleteCh chan error
				server          *grpc.Server
				clientConn      *grpc.ClientConn

				leveler        *fakes.Leveler
				payloadLeveler *fakes.Leveler
			)

			BeforeEach(func() {
				var err error
				listener, err = net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())

				leveler = &fakes.Leveler{}
				leveler.Returns(zapcore.ErrorLevel)
				payloadLeveler = &fakes.Leveler{}
				payloadLeveler.Returns(zapcore.WarnLevel)

				server = grpc.NewServer(
					grpc.UnaryInterceptor(grpclogging.UnaryServerInterceptor(
						logger,
						grpclogging.WithLeveler(grpclogging.LevelerFunc(leveler.Spy)),
						grpclogging.WithPayloadLeveler(grpclogging.LevelerFunc(payloadLeveler.Spy)),
					)),
				)

				testpb.RegisterEchoServiceServer(server, fakeEchoService)
				serveCompleteCh = make(chan error, 1)
				go func() { serveCompleteCh <- server.Serve(listener) }()

				clientConn, err = grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
				Expect(err).NotTo(HaveOccurred())
				echoServiceClient = testpb.NewEchoServiceClient(clientConn)

				ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
				defer cancel()

				_, err = echoServiceClient.Echo(ctx, &testpb.Message{Message: "hi"})
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				clientConn.Close()

				err := listener.Close()
				Expect(err).NotTo(HaveOccurred())
				Eventually(serveCompleteCh).Should(Receive())
			})

			It("uses the levels returned by the levelers", func() {
				Expect(leveler.CallCount()).To(Equal(1))
				Expect(observed.FilterMessage("unary call completed").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("unary call completed").AllUntimed()[0].Level).To(Equal(zapcore.ErrorLevel))

				Expect(payloadLeveler.CallCount()).To(Equal(1))
				Expect(observed.FilterMessage("received unary request").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("received unary request").AllUntimed()[0].Level).To(Equal(zapcore.WarnLevel))
				Expect(observed.FilterMessage("sending unary response").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("sending unary response").AllUntimed()[0].Level).To(Equal(zapcore.WarnLevel))
			})

			It("provides the decorated context and full method name to the levelers", func() {
				Expect(leveler.CallCount()).To(Equal(1))
				ctx, fullMethod := leveler.ArgsForCall(0)
				Expect(grpclogging.ZapFields(ctx)).NotTo(BeEmpty())
				Expect(fullMethod).To(Equal("/testpb.EchoService/Echo"))

				Expect(payloadLeveler.CallCount()).To(Equal(1))
				ctx, fullMethod = payloadLeveler.ArgsForCall(0)
				Expect(grpclogging.ZapFields(ctx)).NotTo(BeEmpty())
				Expect(fullMethod).To(Equal("/testpb.EchoService/Echo"))
			})
		})
	})

	Describe("StreamServerInterceptor", func() {
		It("logs stream data", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()
			streamClient, err := echoServiceClient.EchoStream(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = streamClient.Send(&testpb.Message{Message: "hello"})
			Expect(err).NotTo(HaveOccurred())

			msg, err := streamClient.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 1}))

			err = streamClient.CloseSend()
			Expect(err).NotTo(HaveOccurred())

			Eventually(observed.AllUntimed).Should(HaveLen(3))
			var logMessages []string
			for _, entry := range observed.AllUntimed() {
				logMessages = append(logMessages, entry.Message)
			}
			Expect(logMessages).To(ConsistOf(
				"received stream message", // received payload
				"sending stream message",  // sending payload
				"streaming call completed",
			))

			for _, entry := range observed.AllUntimed() {
				keyNames := map[string]struct{}{}
				for _, field := range entry.Context {
					keyNames[field.Key] = struct{}{}
				}

				switch entry.LoggerName {
				case "test-logger":
					Expect(entry.Level).To(Equal(zapcore.InfoLevel))
					Expect(entry.Context).To(HaveLen(8))
					Expect(keyNames).To(HaveLen(8))
				case "test-logger.payload":
					Expect(entry.Level).To(Equal(zapcore.DebugLevel - 1))
					Expect(entry.Context).To(HaveLen(6))
					Expect(keyNames).To(HaveLen(6))
				default:
					Fail("unexpected logger name: " + entry.LoggerName)
				}
				Expect(entry.Caller.String()).To(ContainSubstring("grpclogging/server.go"))

				for _, field := range entry.Context {
					switch field.Key {
					case "grpc.code":
						Expect(field.Type).To(Equal(zapcore.StringerType))
						Expect(field.Interface).To(Equal(codes.OK))
					case "grpc.call_duration":
						Expect(field.Type).To(Equal(zapcore.DurationType))
						Expect(field.Integer).NotTo(BeZero())
					case "grpc.service":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(Equal("testpb.EchoService"))
					case "grpc.method":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(Equal("EchoStream"))
					case "grpc.request_deadline":
						deadline, ok := ctx.Deadline()
						Expect(ok).To(BeTrue())
						Expect(field.Type).To(Equal(zapcore.TimeType))
						Expect(field.Integer).NotTo(BeZero())
						Expect(time.Unix(0, field.Integer)).To(BeTemporally("~", deadline, TimeThreshold))
					case "grpc.peer_address":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(HavePrefix("127.0.0.1"))
					case "grpc.peer_subject":
						Expect(field.Type).To(Equal(zapcore.StringType))
						Expect(field.String).To(HavePrefix("CN=client"))
					case "message":
						Expect(field.Type).To(Equal(zapcore.ReflectType))
					case "error":
						Expect(field.Type).To(Equal(zapcore.ErrorType))
					case "":
						Expect(field.Type).To(Equal(zapcore.SkipType))
					default:
						Fail("unexpected context field: " + field.Key)
					}
				}
			}
		})

		It("provides a decorated context", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()
			streamClient, err := echoServiceClient.EchoStream(ctx)
			Expect(err).NotTo(HaveOccurred())

			err = streamClient.Send(&testpb.Message{Message: "hello"})
			Expect(err).NotTo(HaveOccurred())

			msg, err := streamClient.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 1}))

			err = streamClient.CloseSend()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeEchoService.EchoStreamCallCount()).To(Equal(1))
			echoStream := fakeEchoService.EchoStreamArgsForCall(0)
			zapFields := grpclogging.ZapFields(echoStream.Context())

			keyNames := []string{}
			for _, field := range zapFields {
				keyNames = append(keyNames, field.Key)
			}
			Expect(keyNames).To(ConsistOf(
				"grpc.service",
				"grpc.method",
				"grpc.request_deadline",
				"grpc.peer_address",
				"grpc.peer_subject",
			))
		})

		Context("when tls client auth is missing", func() {
			var clientConn *grpc.ClientConn

			BeforeEach(func() {
				dialOpts := []grpc.DialOption{
					grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(caCertPool, "")),
				}
				var err error
				clientConn, err = grpc.Dial(listener.Addr().String(), dialOpts...)
				Expect(err).NotTo(HaveOccurred())

				echoServiceClient = testpb.NewEchoServiceClient(clientConn)
			})

			AfterEach(func() {
				clientConn.Close()
			})

			It("omits grpc.peer_subject", func() {
				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())

				msg, err := streamClient.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 1}))

				err = streamClient.CloseSend()
				Expect(err).NotTo(HaveOccurred())

				for _, entry := range observed.AllUntimed() {
					keyNames := map[string]struct{}{}
					for _, field := range entry.Context {
						keyNames[field.Key] = struct{}{}
					}
					Expect(keyNames).NotTo(HaveKey("grpc.peer_subject"))
				}
			})
		})

		Context("when the stream ends with an unknown error", func() {
			var expectedErr error

			BeforeEach(func() {
				expectedErr = errors.New("gah!")
				fakeEchoService.EchoStreamReturns(expectedErr)

				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())
				_, err = streamClient.Recv()
				Expect(err).To(HaveOccurred())
			})

			It("logs the unknown code", func() {
				entries := observed.FilterMessage("streaming call completed").FilterField(zap.Stringer("grpc.code", codes.Unknown)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})

			It("logs the error", func() {
				entries := observed.FilterMessage("streaming call completed").FilterField(grpclogging.Error(expectedErr)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})
		})

		Context("when the stream ends with a grpc status error", func() {
			var expectedErr error

			BeforeEach(func() {
				errCh := make(chan error)
				fakeEchoService.EchoStreamStub = func(svr testpb.EchoService_EchoStreamServer) error { return <-errCh }

				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())

				expectedErr = &statusError{Status: status.New(codes.Aborted, "aborted")}
				errCh <- expectedErr

				_, err = streamClient.Recv()
				Expect(err).To(HaveOccurred())
			})

			It("logs the corect code", func() {
				entries := observed.FilterMessage("streaming call completed").FilterField(zap.Stringer("grpc.code", codes.Aborted)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})

			It("logs the error", func() {
				entries := observed.FilterMessage("streaming call completed").FilterField(grpclogging.Error(expectedErr)).AllUntimed()
				Expect(entries).To(HaveLen(1))
			})
		})

		Context("when options are used", func() {
			var (
				listener        net.Listener
				serveCompleteCh chan error
				server          *grpc.Server
				clientConn      *grpc.ClientConn

				leveler        *fakes.Leveler
				payloadLeveler *fakes.Leveler
			)

			BeforeEach(func() {
				var err error
				listener, err = net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())

				leveler = &fakes.Leveler{}
				leveler.Returns(zapcore.ErrorLevel)
				payloadLeveler = &fakes.Leveler{}
				payloadLeveler.Returns(zapcore.WarnLevel)

				server = grpc.NewServer(
					grpc.StreamInterceptor(grpclogging.StreamServerInterceptor(
						logger,
						grpclogging.WithLeveler(grpclogging.LevelerFunc(leveler.Spy)),
						grpclogging.WithPayloadLeveler(grpclogging.LevelerFunc(payloadLeveler.Spy)),
					)),
				)

				testpb.RegisterEchoServiceServer(server, fakeEchoService)
				serveCompleteCh = make(chan error, 1)
				go func() { serveCompleteCh <- server.Serve(listener) }()

				clientConn, err = grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
				Expect(err).NotTo(HaveOccurred())
				echoServiceClient = testpb.NewEchoServiceClient(clientConn)

				streamClient, err := echoServiceClient.EchoStream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				err = streamClient.Send(&testpb.Message{Message: "hello"})
				Expect(err).NotTo(HaveOccurred())
				msg, err := streamClient.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).To(Equal(&testpb.Message{Message: "hello", Sequence: 1}))

				err = streamClient.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				clientConn.Close()

				err := listener.Close()
				Expect(err).NotTo(HaveOccurred())
				Eventually(serveCompleteCh).Should(Receive())
			})

			It("uses the levels returned by the levelers", func() {
				Expect(leveler.CallCount()).To(Equal(1))
				Expect(observed.FilterMessage("streaming call completed").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("streaming call completed").AllUntimed()[0].Level).To(Equal(zapcore.ErrorLevel))

				Expect(payloadLeveler.CallCount()).To(Equal(1))
				Expect(observed.FilterMessage("received stream message").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("received stream message").AllUntimed()[0].Level).To(Equal(zapcore.WarnLevel))
				Expect(observed.FilterMessage("sending stream message").AllUntimed()).To(HaveLen(1))
				Expect(observed.FilterMessage("sending stream message").AllUntimed()[0].Level).To(Equal(zapcore.WarnLevel))
			})

			It("provides the decorated context and full method name to the levelers", func() {
				Expect(leveler.CallCount()).To(Equal(1))
				ctx, fullMethod := leveler.ArgsForCall(0)
				Expect(grpclogging.ZapFields(ctx)).NotTo(BeEmpty())
				Expect(fullMethod).To(Equal("/testpb.EchoService/EchoStream"))

				Expect(payloadLeveler.CallCount()).To(Equal(1))
				ctx, fullMethod = payloadLeveler.ArgsForCall(0)
				Expect(grpclogging.ZapFields(ctx)).NotTo(BeEmpty())
				Expect(fullMethod).To(Equal("/testpb.EchoService/EchoStream"))
			})
		})
	})

	It("uses flogging.PayloadLevel as DefaultPayloadLevel", func() {
		Expect(grpclogging.DefaultPayloadLevel).To(Equal(flogging.PayloadLevel))
	})
})

type statusError struct{ *status.Status }

func (s *statusError) GRPCStatus() *status.Status { return s.Status }

func (s *statusError) Error() string {
	return fmt.Sprintf("🎶 I'm a little error, short and sweet. Here is my message: %s. Here is my code: %d.🎶", s.Status.Message(), s.Status.Code())
}
