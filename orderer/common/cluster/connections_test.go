/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"sync"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

func TestConcurrentConnections(t *testing.T) {
	t.Parallel()
	// Scenario: Have 100 goroutines try to create a connection together at the same time,
	// wait until one of them succeeds, and then wait until they all return,
	// and also ensure they all return the same connection reference
	n := 100
	var wg sync.WaitGroup
	wg.Add(n)
	dialer := &mocks.SecureDialer{}
	conn := &grpc.ClientConn{}
	dialer.On("Dial", mock.Anything, mock.Anything).Return(conn, nil)
	connStore := cluster.NewConnectionStore(dialer)
	connect := func() {
		defer wg.Done()
		conn2, err := connStore.Connection("", nil)
		assert.NoError(t, err)
		assert.True(t, conn2 == conn)
	}
	for i := 0; i < n; i++ {
		go connect()
	}
	wg.Wait()
	dialer.AssertNumberOfCalls(t, "Dial", 1)
}

type connectionMapperSpy struct {
	lookupDelay   chan struct{}
	lookupInvoked chan struct{}
	cluster.ConnectionMapper
}

func (cms *connectionMapperSpy) Lookup(cert []byte) (*grpc.ClientConn, bool) {
	// Signal that Lookup() has been invoked
	cms.lookupInvoked <- struct{}{}
	// Wait for the main test to signal to advance.
	// This is needed because we need to ensure that all instances
	// of the connectionMapperSpy invoked Lookup()
	<-cms.lookupDelay
	return cms.ConnectionMapper.Lookup(cert)
}

func TestConcurrentLookupMiss(t *testing.T) {
	t.Parallel()
	// Scenario: 2 concurrent connection attempts are made,
	// and the first 2 Lookup operations are delayed,
	// which makes the connection store attempt to connect
	// at the same time twice.
	// A single connection should be created regardless.

	dialer := &mocks.SecureDialer{}
	conn := &grpc.ClientConn{}
	dialer.On("Dial", mock.Anything, mock.Anything).Return(conn, nil)

	connStore := cluster.NewConnectionStore(dialer)
	// Wrap the connection mapping with a spy that intercepts Lookup() invocations
	spy := &connectionMapperSpy{
		ConnectionMapper: connStore.Connections,
		lookupDelay:      make(chan struct{}, 2),
		lookupInvoked:    make(chan struct{}, 2),
	}
	connStore.Connections = spy

	var goroutinesExited sync.WaitGroup
	goroutinesExited.Add(2)

	for i := 0; i < 2; i++ {
		go func() {
			defer goroutinesExited.Done()
			conn2, err := connStore.Connection("", nil)
			assert.NoError(t, err)
			// Ensure all calls for Connection() return the same reference
			// of the gRPC connection.
			assert.True(t, conn2 == conn)
		}()
	}
	// Wait for the Lookup() to be invoked by both
	// goroutines
	<-spy.lookupInvoked
	<-spy.lookupInvoked
	// Signal the goroutines to finish the Lookup() invocations
	spy.lookupDelay <- struct{}{}
	spy.lookupDelay <- struct{}{}
	// Close the channel so that subsequent Lookup() operations won't be blocked
	close(spy.lookupDelay)
	// Wait for all goroutines to exit
	goroutinesExited.Wait()
}
