/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"

	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// initGrpcSemaphores returns a semaphore per gRPC method that has a
// concurrency limit configured. The broadcast semaphore is keyed by full
// method name because Deliver shares the AtomicBroadcast service and must
// not be limited by it.
func initGrpcSemaphores(conf *localconfig.TopLevel) map[string]semaphore.Semaphore {
	semaphores := make(map[string]semaphore.Semaphore)

	if broadcastConcurrency := conf.General.Limits.Concurrency.BroadcastService; broadcastConcurrency != 0 {
		logger.Infof("concurrency limit for broadcast service is %d", broadcastConcurrency)
		semaphores[ab.AtomicBroadcast_Broadcast_FullMethodName] = semaphore.New(broadcastConcurrency)
	}

	return semaphores
}

// streamGrpcLimiter rejects streams of limited methods once all permits are
// taken, instead of admitting them and letting per-stream state accumulate.
// For Broadcast this also bounds the messages decoded before any
// authentication check runs.
func streamGrpcLimiter(semaphores map[string]semaphore.Semaphore) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		sema, ok := semaphores[info.FullMethod]
		if !ok {
			return handler(srv, ss)
		}
		if !sema.TryAcquire() {
			logger.Errorf("Too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(sema))
			return status.Error(codes.ResourceExhausted, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(sema)))
		}
		defer sema.Release()
		return handler(srv, ss)
	}
}
