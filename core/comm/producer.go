/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
}

type connProducer struct {
	sync.RWMutex
	endpoints []string
	connect   ConnectionFactory
}

// NewConnectionProducer creates a new ConnectionProducer with given endpoints and connection factory.
// It returns nil, if the given endpoints slice is empty.
func NewConnectionProducer(factory ConnectionFactory, endpoints []string) ConnectionProducer {
	if len(endpoints) == 0 {
		return nil
	}
	return &connProducer{endpoints: endpoints, connect: factory}
}

// NewConnection creates a new connection.
// Returns the connection, the endpoint selected, nil on success.
// Returns nil, "", error on failure
func (cp *connProducer) NewConnection() (*grpc.ClientConn, string, error) {
	cp.RLock()
	defer cp.RUnlock()

	endpoints := shuffle(cp.endpoints)
	for _, endpoint := range endpoints {
		conn, err := cp.connect(endpoint)
		if err != nil {
			logger.Error("Failed connecting to", endpoint, ", error:", err)
			continue
		}
		return conn, endpoint, nil
	}
	return nil, "", fmt.Errorf("Could not connect to any of the endpoints: %v", endpoints)
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
