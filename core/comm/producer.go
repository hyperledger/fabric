/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("ConnProducer")

var EndpointDisableInterval = time.Second * 10

// ConnectionFactory creates a connection to a certain endpoint
type ConnectionFactory func(endpoint string) (*grpc.ClientConn, error)

// ConnectionProducer produces connections out of a set of predefined
// endpoints
type ConnectionProducer interface {
	// NewConnection creates a new connection.
	// Returns the connection, the endpoint selected, nil on success.
	// Returns nil, "", error on failure
	NewConnection() (*grpc.ClientConn, string, error)
	// UpdateEndpoints updates the endpoints of the ConnectionProducer
	// to be the given endpoints
	UpdateEndpoints(endpoints []string)
	// DisableEndpoint remove endpoint from endpoint for some time
	DisableEndpoint(endpoint string)
	// GetEndpoints return ordering service endpoints
	GetEndpoints() []string
}

type connProducer struct {
	sync.RWMutex
	endpoints         []string
	disabledEndpoints map[string]time.Time
	connect           ConnectionFactory
}

// NewConnectionProducer creates a new ConnectionProducer with given endpoints and connection factory.
// It returns nil, if the given endpoints slice is empty.
func NewConnectionProducer(factory ConnectionFactory, endpoints []string) ConnectionProducer {
	if len(endpoints) == 0 {
		return nil
	}
	return &connProducer{endpoints: endpoints, connect: factory, disabledEndpoints: make(map[string]time.Time)}
}

// NewConnection creates a new connection.
// Returns the connection, the endpoint selected, nil on success.
// Returns nil, "", error on failure
func (cp *connProducer) NewConnection() (*grpc.ClientConn, string, error) {
	cp.Lock()
	defer cp.Unlock()

	for endpoint, timeout := range cp.disabledEndpoints {
		if time.Since(timeout) >= EndpointDisableInterval {
			delete(cp.disabledEndpoints, endpoint)
		}
	}

	endpoints := shuffle(cp.endpoints)
	checkedEndpoints := make([]string, 0)
	for _, endpoint := range endpoints {
		if _, ok := cp.disabledEndpoints[endpoint]; !ok {
			checkedEndpoints = append(checkedEndpoints, endpoint)
			conn, err := cp.connect(endpoint)
			if err != nil {
				logger.Error("Failed connecting to", endpoint, ", error:", err)
				continue
			}
			return conn, endpoint, nil
		}
	}
	return nil, "", fmt.Errorf("Could not connect to any of the endpoints: %v", checkedEndpoints)
}

// UpdateEndpoints updates the endpoints of the ConnectionProducer
// to be the given endpoints
func (cp *connProducer) UpdateEndpoints(endpoints []string) {
	if len(endpoints) == 0 {
		// Ignore updates with empty endpoints
		return
	}
	cp.Lock()
	defer cp.Unlock()

	newDisabled := make(map[string]time.Time)
	for i := range endpoints {
		if startTime, ok := cp.disabledEndpoints[endpoints[i]]; ok {
			newDisabled[endpoints[i]] = startTime
		}
	}
	cp.endpoints = endpoints
	cp.disabledEndpoints = newDisabled
}

func (cp *connProducer) DisableEndpoint(endpoint string) {
	cp.Lock()
	defer cp.Unlock()

	if len(cp.endpoints)-len(cp.disabledEndpoints) == 1 {
		logger.Warning("Only 1 endpoint remained, will not black-list it")
		return
	}

	for _, currEndpoint := range cp.endpoints {
		if currEndpoint == endpoint {
			cp.disabledEndpoints[endpoint] = time.Now()
			break
		}
	}
}

func shuffle(a []string) []string {
	n := len(a)
	returnedSlice := make([]string, n)
	rand.Seed(time.Now().UnixNano())
	indices := rand.Perm(n)
	for i, idx := range indices {
		returnedSlice[i] = a[idx]
	}
	return returnedSlice
}

// GetEndpoints returns configured endpoints for ordering service
func (cp *connProducer) GetEndpoints() []string {
	cp.RLock()
	defer cp.RUnlock()
	return cp.endpoints
}
