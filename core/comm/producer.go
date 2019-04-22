/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("ConnProducer")

// ConnectionFactory creates a connection to a certain endpoint
type ConnectionFactory func(endpoint string) (*grpc.ClientConn, error)

// EndpointCriteria defines an endpoint, and a list of trusted
// organizations it corresponds to.
type EndpointCriteria struct {
	Endpoint      string
	Organizations []string
}

// Equals returns whether this EndpointCriteria is equivalent to the given other EndpointCriteria
func (ec EndpointCriteria) Equals(other EndpointCriteria) bool {
	ss1 := stringSet(ec.Organizations)
	ss2 := stringSet(other.Organizations)
	return ec.Endpoint == other.Endpoint && ss1.equals(ss2)
}

// stringSet defines a collection of strings without
// any significance to their order or number of occurrences.
type stringSet []string

func (ss stringSet) equals(ss2 stringSet) bool {
	// If the sets are of different sizes, they are different.
	if len(ss) != len(ss2) {
		return false
	}
	return reflect.DeepEqual(ss.toMap(), ss2.toMap())
}

func (ss stringSet) toMap() map[string]struct{} {
	m := make(map[string]struct{})
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

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
	// GetEndpoints return ordering service endpoints
	GetEndpoints() []string
}

type connProducer struct {
	sync.RWMutex
	endpoints         []string
	connect           ConnectionFactory
	nextEndpointIndex int
}

// NewConnectionProducer creates a new ConnectionProducer with given endpoints and connection factory.
// It returns nil, if the given endpoints slice is empty.
func NewConnectionProducer(factory ConnectionFactory, endpoints []string) ConnectionProducer {
	if len(endpoints) == 0 {
		return nil
	}
	return &connProducer{endpoints: shuffle(endpoints), connect: factory}
}

// NewConnection creates a new connection.
// Returns the connection, the endpoint selected, nil on success.
// Returns nil, "", error on failure
func (cp *connProducer) NewConnection() (*grpc.ClientConn, string, error) {
	cp.Lock()
	defer cp.Unlock()

	logger.Debugf("Creating a new connection")

	for i := 0; i < len(cp.endpoints); i++ {
		currentEndpoint := cp.endpoints[cp.nextEndpointIndex]
		conn, err := cp.connect(currentEndpoint)
		cp.nextEndpointIndex = (cp.nextEndpointIndex + 1) % len(cp.endpoints)
		if err != nil {
			logger.Error("Failed connecting to", currentEndpoint, ", error:", err)
			continue
		}
		logger.Debugf("Connected to %s", currentEndpoint)
		return conn, currentEndpoint, nil
	}

	logger.Errorf("Could not connect to any of the endpoints: %v", cp.endpoints)

	return nil, "", fmt.Errorf("could not connect to any of the endpoints: %v", cp.endpoints)
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

	cp.nextEndpointIndex = 0
	cp.endpoints = endpoints
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
