/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"

	"github.com/pkg/errors"
)

type CachedChaincodeDefinition struct {
	Definition *ChaincodeDefinition
	Approved   bool

	// Hashes is the list of hashed keys in the implicit collection referring to this definition.
	// These hashes are determined by the current sequence number of chaincode definition.  When dirty,
	// these hashes will be empty, and when not, they will be populated.
	Hashes []string
}

type ChannelCache struct {
	Chaincodes map[string]*CachedChaincodeDefinition

	// InterestingHashes is a map of hashed key names to the chaincode name which they affect.
	// These are to be used for the state listener, to mark chaincode definitions dirty when
	// a write is made into the implicit collection for this org.  Interesting hashes are
	// added when marking a definition clean, and deleted when marking it dirty.
	InterestingHashes map[string]string
}

type Cache struct {
	definedChaincodes map[string]*ChannelCache
	Lifecycle         *Lifecycle
	MyOrgMSPID        string

	// mutex serializes lifecycle operations globally for the peer.  It will cause a lifecycle update
	// in one channel to potentially affect the throughput of another.  However, relative to standard
	// transactions, lifecycle updates should be quite rare, and this is a RW lock so in general, there
	// should not be contention in the normal case.  Because chaincode package installation is a peer global
	// event, by synchronizing at a peer global level, we drastically simplify accounting for which
	// chaincodes are installed and which channels that installed chaincode is currently in use on.
	mutex sync.RWMutex
}

func NewCache(lifecycle *Lifecycle, myOrgMSPID string) *Cache {
	return &Cache{
		definedChaincodes: map[string]*ChannelCache{},
		Lifecycle:         lifecycle,
		MyOrgMSPID:        myOrgMSPID,
	}
}

func (c *Cache) ChaincodeDefinition(channelID, name string) (*ChaincodeDefinition, bool, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	channelChaincodes, ok := c.definedChaincodes[channelID]
	if !ok {
		return nil, false, errors.Errorf("unknown channel '%s'", channelID)
	}

	cachedChaincode, ok := channelChaincodes.Chaincodes[name]
	if !ok {
		return nil, false, errors.Errorf("unknown chaincode '%s' for channel '%s'", name, channelID)
	}

	return cachedChaincode.Definition, cachedChaincode.Approved, nil
}

func (c *Cache) Update(channelID string, dirtyChaincodes map[string]struct{}, qe ledger.SimpleQueryExecutor) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	channelCache, ok := c.definedChaincodes[channelID]
	if !ok {
		channelCache = &ChannelCache{
			Chaincodes:        map[string]*CachedChaincodeDefinition{},
			InterestingHashes: map[string]string{},
		}
		c.definedChaincodes[channelID] = channelCache
	}

	publicState := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}

	orgState := &PrivateQueryExecutorShim{
		Namespace:  LifecycleNamespace,
		Collection: ImplicitCollectionNameForOrg(c.MyOrgMSPID),
		State:      qe,
	}

	for name := range dirtyChaincodes {
		cachedChaincode, ok := channelCache.Chaincodes[name]
		if !ok {
			cachedChaincode = &CachedChaincodeDefinition{}
			channelCache.Chaincodes[name] = cachedChaincode
		}

		for _, hash := range cachedChaincode.Hashes {
			delete(channelCache.InterestingHashes, hash)
		}

		exists, chaincodeDefinition, err := c.Lifecycle.ChaincodeDefinitionIfDefined(name, publicState)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not get chaincode definition for '%s' on channel '%s'", name, channelID))
		}

		if !exists {
			// the chaincode definition was deleted, this is currently not
			// possible, but there should be no problems with that.
			delete(channelCache.Chaincodes, name)
			continue
		}

		cachedChaincode.Definition = chaincodeDefinition
		cachedChaincode.Approved = false

		privateName := fmt.Sprintf("%s#%d", name, chaincodeDefinition.Sequence)

		cachedChaincode.Hashes = []string{
			string(util.ComputeSHA256([]byte(MetadataKey(NamespacesName, privateName)))),
			string(util.ComputeSHA256([]byte(FieldKey(NamespacesName, privateName, "EndorsementInfo")))),
			string(util.ComputeSHA256([]byte(FieldKey(NamespacesName, privateName, "ValidationInfo")))),
			string(util.ComputeSHA256([]byte(FieldKey(NamespacesName, privateName, "Collections")))),
		}

		for _, hash := range cachedChaincode.Hashes {
			channelCache.InterestingHashes[hash] = name
		}

		ok, err = c.Lifecycle.Serializer.IsSerialized(NamespacesName, privateName, chaincodeDefinition.Parameters(), orgState)

		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not check opaque org state for '%s' on channel '%s'", name, channelID))
		}
		if ok {
			cachedChaincode.Approved = true
		}
	}

	return nil
}
