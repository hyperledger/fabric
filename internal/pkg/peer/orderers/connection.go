/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orderers

import (
	"bytes"
	"crypto/sha256"
	"math/rand"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"

	"github.com/pkg/errors"
)

type ConnectionSource struct {
	mutex              sync.RWMutex
	allEndpoints       []*Endpoint
	orgToEndpointsHash map[string][]byte
	logger             *flogging.FabricLogger
	overrides          map[string]*Endpoint
}

type Endpoint struct {
	Address   string
	RootCerts [][]byte
	Refreshed chan struct{}
}

type OrdererOrg struct {
	Addresses []string
	RootCerts [][]byte
}

func NewConnectionSource(logger *flogging.FabricLogger, overrides map[string]*Endpoint) *ConnectionSource {
	return &ConnectionSource{
		orgToEndpointsHash: map[string][]byte{},
		logger:             logger,
		overrides:          overrides,
	}
}

func (cs *ConnectionSource) RandomEndpoint() (*Endpoint, error) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	if len(cs.allEndpoints) == 0 {
		return nil, errors.Errorf("no endpoints currently defined")
	}
	return cs.allEndpoints[rand.Intn(len(cs.allEndpoints))], nil
}

func (cs *ConnectionSource) Update(globalAddrs []string, orgs map[string]OrdererOrg) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	cs.logger.Debug("Processing updates for orderer endpoints")

	newOrgToEndpointsHash := map[string][]byte{}

	anyChange := false
	hasOrgEndpoints := false
	for orgName, org := range orgs {
		hasher := sha256.New()
		for _, cert := range org.RootCerts {
			hasher.Write(cert)
		}
		for _, address := range org.Addresses {
			hasOrgEndpoints = true
			hasher.Write([]byte(address))
		}
		hash := hasher.Sum(nil)

		newOrgToEndpointsHash[orgName] = hash

		lastHash, ok := cs.orgToEndpointsHash[orgName]
		if ok && bytes.Equal(hash, lastHash) {
			continue
		}

		cs.logger.Debugf("Found orderer org '%s' has updates", orgName)
		anyChange = true
	}

	for orgName := range cs.orgToEndpointsHash {
		if _, ok := orgs[orgName]; !ok {
			// An org that used to exist has been removed
			cs.logger.Debugf("Found orderer org '%s' has been removed", orgName)
			anyChange = true
		}
	}

	cs.orgToEndpointsHash = newOrgToEndpointsHash

	if hasOrgEndpoints && len(globalAddrs) > 0 {
		cs.logger.Warning("Config defines both orderer org specific endpoints and global endpoints, global endpoints will be ignored")
	}

	if !hasOrgEndpoints && len(globalAddrs) != len(cs.allEndpoints) {
		cs.logger.Debugf("There are no org endpoints, but the global addresses have changed")
		anyChange = true
	}

	if !hasOrgEndpoints && !anyChange && len(globalAddrs) == len(cs.allEndpoints) {
		// There are no org endpoints, there were no org endpoints, and the number
		// of the update's  global endpoints is the same as the number of existing endpoints.
		// So, we check if any of the endpoints addresses differ.

		newAddresses := map[string]struct{}{}
		for _, address := range globalAddrs {
			newAddresses[address] = struct{}{}
		}

		for _, endpoint := range cs.allEndpoints {
			delete(newAddresses, endpoint.Address)
		}

		// Set anyChange true if some new address was not
		// in the set of old endpoints.
		anyChange = len(newAddresses) != 0
		if anyChange {
			cs.logger.Debugf("There are no org endpoints, but some of the global addresses have changed")
		}
	}

	if !anyChange {
		cs.logger.Debugf("No orderer endpoint addresses or TLS certs were changed")
		// No TLS certs changed, no org specified endpoints changed,
		// and if we are using global endpoints, they are the same
		// as our last set.  No need to update anything.
		return
	}

	for _, endpoint := range cs.allEndpoints {
		// Alert any existing consumers that have a reference to the old endpoints
		// that their reference is now stale and they should get a new one.
		// This is done even for endpoints which have the same TLS certs and address
		// but this is desirable to help load balance.  For instance if only
		// one orderer were defined, and the config is updated to include 4 more, we
		// want the peers to disconnect from that original orderer and reconnect
		// evenly across the now five.
		close(endpoint.Refreshed)
	}

	cs.allEndpoints = nil

	var globalRootCerts [][]byte

	for _, org := range orgs {
		var rootCerts [][]byte
		for _, rootCert := range org.RootCerts {
			if hasOrgEndpoints {
				rootCerts = append(rootCerts, rootCert)
			} else {
				globalRootCerts = append(globalRootCerts, rootCert)
			}
		}

		// Note, if !hasOrgEndpoints, this for loop is a no-op
		for _, address := range org.Addresses {
			overrideEndpoint, ok := cs.overrides[address]
			if ok {
				cs.allEndpoints = append(cs.allEndpoints, &Endpoint{
					Address:   overrideEndpoint.Address,
					RootCerts: overrideEndpoint.RootCerts,
					Refreshed: make(chan struct{}),
				})
				continue
			}

			cs.allEndpoints = append(cs.allEndpoints, &Endpoint{
				Address:   address,
				RootCerts: rootCerts,
				Refreshed: make(chan struct{}),
			})
		}
	}

	if len(cs.allEndpoints) != 0 {
		cs.logger.Debugf("Returning an orderer connection pool source with org specific endpoints only")
		// There are some org specific endpoints, so we do not
		// add any of the global endpoints to our pool.
		return
	}

	for _, address := range globalAddrs {
		overrideEndpoint, ok := cs.overrides[address]
		if ok {
			cs.allEndpoints = append(cs.allEndpoints, &Endpoint{
				Address:   overrideEndpoint.Address,
				RootCerts: overrideEndpoint.RootCerts,
				Refreshed: make(chan struct{}),
			})
			continue
		}

		cs.allEndpoints = append(cs.allEndpoints, &Endpoint{
			Address:   address,
			RootCerts: globalRootCerts,
			Refreshed: make(chan struct{}),
		})
	}

	cs.logger.Debugf("Returning an orderer connection pool source with global endpoints only")
}
