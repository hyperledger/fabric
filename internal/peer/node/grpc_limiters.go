/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"strings"

	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func initGrpcSemaphores(config *peer.Config) map[string]semaphore.Semaphore {
	semaphores := make(map[string]semaphore.Semaphore)
	endorserConcurrency := config.LimitsConcurrencyEndorserService
	deliverConcurrency := config.LimitsConcurrencyDeliverService

	// Currently concurrency limit is applied to endorser service and deliver service.
	// These services are defined in fabric-protos and fabric-protos-go (generated from fabric-protos).
	// Below service names must match their definitions.
	if endorserConcurrency != 0 {
		logger.Infof("concurrency limit for endorser service is %d", endorserConcurrency)
		semaphores["/protos.Endorser"] = semaphore.New(endorserConcurrency)
	}
	if deliverConcurrency != 0 {
		logger.Infof("concurrency limit for deliver service is %d", deliverConcurrency)
		semaphores["/protos.Deliver"] = semaphore.New(deliverConcurrency)
	}

	return semaphores
}

func unaryGrpcLimiter(semaphores map[string]semaphore.Semaphore) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		serviceName := getServiceName(info.FullMethod)
		sema, ok := semaphores[serviceName]
		if !ok {
			return handler(ctx, req)
		}
		if !sema.TryAcquire() {
			logger.Errorf("Too many requests for %s, exceeding concurrency limit (%d)", serviceName, cap(sema))
			return nil, errors.Errorf("too many requests for %s, exceeding concurrency limit (%d)", serviceName, cap(sema))
		}
		defer sema.Release()
		return handler(ctx, req)
	}
}

func streamGrpcLimiter(semaphores map[string]semaphore.Semaphore) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		serviceName := getServiceName(info.FullMethod)
		sema, ok := semaphores[serviceName]
		if !ok {
			return handler(srv, ss)
		}
		if !sema.TryAcquire() {
			logger.Errorf("Too many requests for %s, exceeding concurrency limit (%d)", serviceName, cap(sema))
			return errors.Errorf("too many requests for %s, exceeding concurrency limit (%d)", serviceName, cap(sema))
		}
		defer sema.Release()
		return handler(srv, ss)
	}
}

func getServiceName(methodName string) string {
	index := strings.LastIndex(methodName, "/")
	return methodName[:index]
}
