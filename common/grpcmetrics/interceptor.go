/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpcmetrics

import (
	"context"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type UnaryMetrics struct {
	RequestDuration   metrics.Histogram
	RequestsReceived  metrics.Counter
	RequestsCompleted metrics.Counter
}

func UnaryServerInterceptor(um *UnaryMetrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		service, method := serviceMethod(info.FullMethod)
		um.RequestsReceived.With("service", service, "method", method).Add(1)

		startTime := time.Now()
		resp, err := handler(ctx, req)
		st, _ := status.FromError(err)
		duration := time.Since(startTime)

		um.RequestDuration.With(
			"service", service, "method", method, "code", st.Code().String(),
		).Observe(duration.Seconds())
		um.RequestsCompleted.With("service", service, "method", method, "code", st.Code().String()).Add(1)

		return resp, err
	}
}

type StreamMetrics struct {
	RequestDuration   metrics.Histogram
	RequestsReceived  metrics.Counter
	RequestsCompleted metrics.Counter
	MessagesSent      metrics.Counter
	MessagesReceived  metrics.Counter
}

func StreamServerInterceptor(sm *StreamMetrics) grpc.StreamServerInterceptor {
	return func(svc interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		sm := sm
		service, method := serviceMethod(info.FullMethod)
		sm.RequestsReceived.With("service", service, "method", method).Add(1)

		wrappedStream := &serverStream{
			ServerStream:     stream,
			messagesSent:     sm.MessagesSent.With("service", service, "method", method),
			messagesReceived: sm.MessagesReceived.With("service", service, "method", method),
		}

		startTime := time.Now()
		err := handler(svc, wrappedStream)
		st, _ := status.FromError(err)
		duration := time.Since(startTime)

		sm.RequestDuration.With(
			"service", service, "method", method, "code", st.Code().String(),
		).Observe(duration.Seconds())
		sm.RequestsCompleted.With("service", service, "method", method, "code", st.Code().String()).Add(1)

		return err
	}
}

func serviceMethod(fullMethod string) (service, method string) {
	normalizedMethod := strings.Replace(fullMethod, ".", "_", -1)
	parts := strings.SplitN(normalizedMethod, "/", -1)
	if len(parts) != 3 {
		return "unknown", "unknown"
	}
	return parts[1], parts[2]
}

type serverStream struct {
	grpc.ServerStream
	messagesSent     metrics.Counter
	messagesReceived metrics.Counter
}

func (ss *serverStream) SendMsg(msg interface{}) error {
	ss.messagesSent.Add(1)
	return ss.ServerStream.SendMsg(msg)
}

func (ss *serverStream) RecvMsg(msg interface{}) error {
	err := ss.ServerStream.RecvMsg(msg)
	if err == nil {
		ss.messagesReceived.Add(1)
	}
	return err
}
