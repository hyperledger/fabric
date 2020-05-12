/*
Copyright Hitachi America, Ltd.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/hyperledger/fabric/core/peer"
)

func TestInitGrpcSemaphores(t *testing.T) {
	config := peer.Config{
		LimitsConcurrencyEndorserService: 5,
		LimitsConcurrencyDeliverService:  5,
	}
	semaphores := initGrpcSemaphores(&config)
	require.Equal(t, 2, len(semaphores))
}

func TestInitGrpcNoSemaphores(t *testing.T) {
	config := peer.Config{
		LimitsConcurrencyEndorserService: 0,
		LimitsConcurrencyDeliverService:  0,
	}
	semaphores := initGrpcSemaphores(&config)
	require.Equal(t, 0, len(semaphores))
}

func TestInitGrpcSemaphoresPanic(t *testing.T) {
	config := peer.Config{
		LimitsConcurrencyEndorserService: -1,
	}
	require.PanicsWithValue(t, "permits must be greater than 0", func() { initGrpcSemaphores(&config) })
}

func TestUnaryGrpcLimiterExceedLimit(t *testing.T) {
	limit := 10
	fullMethod := "/protos.Endorser/ProcessProposal"
	semaphores := initGrpcSemaphores(&peer.Config{LimitsConcurrencyEndorserService: limit})
	// acquire all permits so that semaphore buffer is full, expect error when calling interceptor
	for i := 0; i < limit; i++ {
		semaphores["/protos.Endorser"].TryAcquire()
	}
	interceptor := unaryGrpcLimiter(semaphores)
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: fullMethod}, nil)
	require.EqualError(t, err, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", getServiceName(fullMethod), limit))
}

func TestStreamGrpcLimiterExceedLimit(t *testing.T) {
	limit := 5
	fullMethods := []string{"/protos.Deliver/Deliver", "/protos.Deliver/DeliverFiltered", "/protos.Deliver/DeliverWithPrivateData"}
	semaphores := initGrpcSemaphores(&peer.Config{LimitsConcurrencyDeliverService: limit})
	// acquire all permits so that semaphore buffer is full, expect error when calling interceptor
	for i := 0; i < limit; i++ {
		semaphores["/protos.Deliver"].TryAcquire()
	}
	interceptor := streamGrpcLimiter(semaphores)
	for _, method := range fullMethods {
		err := interceptor(nil, nil, &grpc.StreamServerInfo{FullMethod: method}, nil)
		require.EqualError(t, err, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", getServiceName(method), limit))
	}
}

func TestGetServiceName(t *testing.T) {
	require.Equal(t, "/protos.Endorser", getServiceName("/protos.Endorser/ProcessProposal"))
	require.Equal(t, "/protos.Deliver", getServiceName("/protos.Deliver/Deliver"))
	require.Equal(t, "/protos.Deliver", getServiceName("/protos.Deliver/DeliverFiltered"))
	require.Equal(t, "/protos.Deliver", getServiceName("/protos.Deliver/DeliverWithPrivateData"))
}
