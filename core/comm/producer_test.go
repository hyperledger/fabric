/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestEmptyEndpoints(t *testing.T) {
	t.Parallel()
	noopFactory := func(endpoint string) (*grpc.ClientConn, error) {
		return nil, nil
	}
	assert.Nil(t, NewConnectionProducer(noopFactory, []string{}))
}

func TestConnFailures(t *testing.T) {
	t.Parallel()
	conn2Endpoint := make(map[string]string)
	shouldConnFail := map[string]bool{
		"a": true,
		"b": false,
		"c": false,
	}
	connFactory := func(endpoint string) (*grpc.ClientConn, error) {
		conn := &grpc.ClientConn{}
		conn2Endpoint[fmt.Sprintf("%p", conn)] = endpoint
		if !shouldConnFail[endpoint] {
			return conn, nil
		}
		return nil, fmt.Errorf("Failed connecting to %s", endpoint)
	}
	// Create a producer with some endpoints, and have the first one fail and all others not fail
	producer := NewConnectionProducer(connFactory, []string{"a", "b", "c"})
	conn, _, err := producer.NewConnection()
	assert.NoError(t, err)
	// We should not return 'a' because connecting to 'a' fails
	assert.NotEqual(t, "a", conn2Endpoint[fmt.Sprintf("%p", conn)])
	// Now, revive 'a'
	shouldConnFail["a"] = false
	// Try obtaining a connection 1000 times in order to ensure selection is shuffled
	selected := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		conn, _, err := producer.NewConnection()
		assert.NoError(t, err)
		selected[conn2Endpoint[fmt.Sprintf("%p", conn)]] = struct{}{}
	}
	// The probability of a, b or c not to be selected is really small
	_, isAselected := selected["a"]
	_, isBselected := selected["b"]
	_, isCselected := selected["c"]
	assert.True(t, isBselected)
	assert.True(t, isCselected)
	assert.True(t, isAselected)

	// Now, make every host fail
	shouldConnFail["a"] = true
	shouldConnFail["b"] = true
	shouldConnFail["c"] = true
	conn, _, err = producer.NewConnection()
	assert.Nil(t, conn)
	assert.Error(t, err)
}

func TestUpdateEndpoints(t *testing.T) {
	t.Parallel()
	conn2Endpoint := make(map[string]string)
	connFactory := func(endpoint string) (*grpc.ClientConn, error) {
		conn := &grpc.ClientConn{}
		conn2Endpoint[fmt.Sprintf("%p", conn)] = endpoint
		return conn, nil
	}
	// Create a producer with a single endpoint
	producer := NewConnectionProducer(connFactory, []string{"a"})
	conn, a, err := producer.NewConnection()
	assert.NoError(t, err)
	assert.Equal(t, "a", conn2Endpoint[fmt.Sprintf("%p", conn)])
	assert.Equal(t, "a", a)
	// Now update the endpoint and check that when we create a new connection,
	// we don't connect to the previous endpoint
	producer.UpdateEndpoints([]string{"b"})
	conn, b, err := producer.NewConnection()
	assert.NoError(t, err)
	assert.NotEqual(t, "a", conn2Endpoint[fmt.Sprintf("%p", conn)])
	assert.Equal(t, "b", conn2Endpoint[fmt.Sprintf("%p", conn)])
	assert.Equal(t, "b", b)
	// Next, ensure an empty update is ignored
	producer.UpdateEndpoints([]string{})
	conn, _, err = producer.NewConnection()
	assert.Equal(t, "b", conn2Endpoint[fmt.Sprintf("%p", conn)])
}

func TestDisableEndpoint(t *testing.T) {
	orgEndpointDisableInterval := EndpointDisableInterval
	EndpointDisableInterval = time.Millisecond * 100
	defer func() { EndpointDisableInterval = orgEndpointDisableInterval }()

	conn2Endpoint := make(map[string]string)
	connFactory := func(endpoint string) (*grpc.ClientConn, error) {
		conn := &grpc.ClientConn{}
		conn2Endpoint[fmt.Sprintf("%p", conn)] = endpoint
		return conn, nil
	}
	// Create producer with single endpoint
	producer := NewConnectionProducer(connFactory, []string{"a"})
	conn, a, err := producer.NewConnection()
	assert.NoError(t, err)
	assert.Equal(t, "a", conn2Endpoint[fmt.Sprintf("%p", conn)])
	assert.Equal(t, "a", a)
	// Now disable endpoint for 100 milliseconds
	producer.DisableEndpoint("a")
	_, _, err = producer.NewConnection()
	// Make sure if only 1 endpoint remains, we don't black-list it
	assert.NoError(t, err)
	// Update endpoints - add endpoint 'b'
	producer.UpdateEndpoints([]string{"a", "b"})
	// Disable again
	producer.DisableEndpoint("a")
	conn, a, err = producer.NewConnection()
	assert.NoError(t, err)
	// Ensure that only b is returned because 'a' is disabled
	assert.Equal(t, "b", conn2Endpoint[fmt.Sprintf("%p", conn)])
	assert.Equal(t, "b", a)

}
