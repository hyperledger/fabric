/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/core/ledger"

	"github.com/pkg/errors"
)

type CachedChaincodeDefinition struct {
	Definition *ChaincodeDefinition
}

type Cache struct {
	definedChaincodes map[string]map[string]*CachedChaincodeDefinition
	Lifecycle         *Lifecycle

	// mutex serializes lifecycle operations globally for the peer.  It will cause a lifecycle update
	// in one channel to potentially affect the throughput of another.  However, relative to standard
	// transactions, lifecycle updates should be quite rare, and this is a RW lock so in general, there
	// should not be contention in the normal case.  Because chaincode package installation is a peer global
	// event, by synchronizing at a peer global level, we drastically simplify accounting for which
	// chaincodes are installed and which channels that installed chaincode is currently in use on.
	mutex sync.RWMutex
}

func NewCache(lifecycle *Lifecycle) *Cache {
	return &Cache{
		definedChaincodes: map[string]map[string]*CachedChaincodeDefinition{},
		Lifecycle:         lifecycle,
	}
}

func (c *Cache) ChaincodeDefinition(channelID, name string) (*ChaincodeDefinition, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	channelChaincodes, ok := c.definedChaincodes[channelID]
	if !ok {
		return nil, errors.Errorf("unknown channel '%s'", channelID)
	}

	cachedChaincode, ok := channelChaincodes[name]
	if !ok {
		return nil, errors.Errorf("unknown chaincode '%s' for channel '%s'", name, channelID)
	}

	return cachedChaincode.Definition, nil
}

func (c *Cache) Update(channelID string, dirtyChaincodes map[string]struct{}, qe ledger.SimpleQueryExecutor) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	channelChaincodes, ok := c.definedChaincodes[channelID]
	if !ok {
		channelChaincodes = map[string]*CachedChaincodeDefinition{}
		c.definedChaincodes[channelID] = channelChaincodes
	}

	publicState := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}

	for name := range dirtyChaincodes {
		cachedChaincode, ok := channelChaincodes[name]
		if !ok {
			cachedChaincode = &CachedChaincodeDefinition{}
			channelChaincodes[name] = cachedChaincode
		}

		exists, chaincodeDefinition, err := c.Lifecycle.ChaincodeDefinitionIfDefined(name, publicState)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not get chaincode definition for '%s' on channel '%s'", name, channelID))
		}

		if !exists {
			// the chaincode definition was deleted, this is currently not
			// possible, but there should be no problems with that.
			delete(channelChaincodes, name)
			continue
		}

		cachedChaincode.Definition = chaincodeDefinition
	}

	return nil
}
