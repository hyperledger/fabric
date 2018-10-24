/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// Levelers will be required and should be provided with the full method info

func UnaryServerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		logger := logger
		startTime := time.Now()

		fields := getFields(ctx, startTime, info.FullMethod)
		logger = logger.With(fields...)
		ctx = WithFields(ctx, fields)

		payloadLogger := logger.Named("payload")
		payloadLogger.Debug("received unary request", ProtoMessage("message", req))

		resp, err := handler(ctx, req)

		if err == nil {
			payloadLogger.Debug("sending unary response", ProtoMessage("message", resp))
		}
		logger.Info("unary call completed",
			zap.Error(err),
			zap.Stringer("grpc.code", grpc.Code(err)),
			zap.Duration("grpc.call_duration", time.Since(startTime)),
		)

		return resp, err
	}
}

func StreamServerInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(service interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		logger := logger
		ctx := stream.Context()
		startTime := time.Now()

		fields := getFields(ctx, startTime, info.FullMethod)
		logger = logger.With(fields...)

		wrappedStream := &serverStream{
			ServerStream:  stream,
			context:       WithFields(ctx, fields),
			payloadLogger: logger.Named("payload"),
		}

		err := handler(service, wrappedStream)
		logger.Info("streaming call completed",
			zap.Error(err),
			zap.Stringer("grpc.code", grpc.Code(err)),
			zap.Duration("grpc.call_duration", time.Since(startTime)),
		)
		return err
	}
}

func getFields(ctx context.Context, startTime time.Time, method string) []zapcore.Field {
	fields := []zap.Field{zap.Time("grpc.start_time", startTime)}
	if parts := strings.Split(method, "/"); len(parts) == 3 {
		fields = append(fields, zap.String("grpc.service", parts[1]), zap.String("grpc.method", parts[2]))
	}
	if deadline, ok := ctx.Deadline(); ok {
		fields = append(fields, zap.Time("grpc.request_deadline", deadline))
	}
	if p, ok := peer.FromContext(ctx); ok {
		fields = append(fields, zap.String("grpc.peer_address", p.Addr.String()))
		if ti, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			if len(ti.State.PeerCertificates) > 0 {
				cert := ti.State.PeerCertificates[0]
				fields = append(fields, zap.String("grpc.peer_subject", cert.Subject.String()))
			}
		}
	}
	return fields
}

type serverStream struct {
	grpc.ServerStream
	context       context.Context
	payloadLogger *zap.Logger
}

func (ss *serverStream) Context() context.Context {
	return ss.context
}

func (ss *serverStream) SendMsg(msg interface{}) error {
	ss.payloadLogger.Debug("sending stream message", ProtoMessage("message", msg))
	return ss.ServerStream.SendMsg(msg)
}

func (ss *serverStream) RecvMsg(msg interface{}) error {
	err := ss.ServerStream.RecvMsg(msg)
	ss.payloadLogger.Debug("received stream message", ProtoMessage("message", msg))
	return err
}
