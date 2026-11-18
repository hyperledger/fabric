/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// initGrpcSemaphores returns a semaphore per gRPC method that has a
// concurrency limit configured. The deliver semaphore is keyed by full
// method name because Broadcast shares the AtomicBroadcast service and
// must not be limited by it.
func initGrpcSemaphores(conf *localconfig.TopLevel) map[string]semaphore.Semaphore {
	semaphores := make(map[string]semaphore.Semaphore)

	if deliverConcurrency := conf.General.Limits.Concurrency.DeliverService; deliverConcurrency != 0 {
		logger.Infof("concurrency limit for deliver service is %d", deliverConcurrency)
		semaphores[ab.AtomicBroadcast_Deliver_FullMethodName] = semaphore.New(deliverConcurrency)
	}

	return semaphores
}

// streamGrpcLimiter rejects streams of limited methods once all permits are
// taken, instead of admitting them and letting per-stream state accumulate.
func streamGrpcLimiter(semaphores map[string]semaphore.Semaphore) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		sema, ok := semaphores[info.FullMethod]
		if !ok {
			return handler(srv, ss)
		}
		if !sema.TryAcquire() {
			logger.Errorf("Too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(sema))
			return errors.Errorf("too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(sema))
		}
		defer sema.Release()
		return handler(srv, ss)
	}
}
