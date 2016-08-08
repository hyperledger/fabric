/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package discovery

import (
	"math/rand"
	"sync"
	"time"
)

// Discovery is the interface that consolidates bootstrap peer membership
// selection and validating peer selection for non-validating peers
type Discovery interface {
	AddNode(string) bool           // Add an address to the discovery list
	RemoveNode(string) bool        // Remove an address from the discovery list
	GetAllNodes() []string         // Return all addresses this peer maintains
	GetRandomNodes(n int) []string // Return n random addresses for this peer to connect to
	FindNode(string) bool          // Find a node in the discovery list
}

// DiscoveryImpl is an implementation of Discovery
type DiscoveryImpl struct {
	sync.RWMutex
	nodes  map[string]bool
	seq    []string
	random *rand.Rand
}

// NewDiscoveryImpl is a constructor of a Discovery implementation
func NewDiscoveryImpl() *DiscoveryImpl {
	di := DiscoveryImpl{}
	di.nodes = make(map[string]bool)
	di.random = rand.New(rand.NewSource(time.Now().Unix()))
	return &di
}

// AddNode adds an address to the discovery list
func (di *DiscoveryImpl) AddNode(address string) bool {
	di.Lock()
	defer di.Unlock()
	if _, ok := di.nodes[address]; !ok {
		di.seq = append(di.seq, address)
		di.nodes[address] = true
	}
	return di.nodes[address]
}

// RemoveNode removes an address from the discovery list
func (di *DiscoveryImpl) RemoveNode(address string) bool {
	di.Lock()
	defer di.Unlock()
	if _, ok := di.nodes[address]; ok {
		di.nodes[address] = false
		return true
	}
	return false
}

// GetAllNodes returns an array of all addresses saved in the discovery list
func (di *DiscoveryImpl) GetAllNodes() []string {
	di.RLock()
	defer di.RUnlock()
	var addresses []string
	for address, valid := range di.nodes {
		if valid {
			addresses = append(addresses, address) // TODO Expensive, don't quite like it
		}
	}
	return addresses
}

// GetRandomNodes returns n random nodes
func (di *DiscoveryImpl) GetRandomNodes(n int) []string {
	var pick string
	randomNodes := make([]string, n)
	di.RLock()
	defer di.RUnlock()
	for i := 0; i < n; i++ {
		for {
			pick = di.seq[di.random.Intn(len(di.nodes))]
			if di.nodes[pick] && !inArray(pick, randomNodes) {
				break
			}
		}
		randomNodes[i] = pick
	}
	return randomNodes
}

// FindNode returns true if its address is stored in the discovery list
func (di *DiscoveryImpl) FindNode(address string) bool {
	di.RLock()
	defer di.RUnlock()
	_, ok := di.nodes[address]
	return ok
}

func inArray(element string, array []string) bool {
	for _, val := range array {
		if val == element {
			return true
		}
	}
	return false
}
