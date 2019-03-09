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

// Initialize will populate the set of currently committed chaincode definitions
// for a channel into the cache.  Note, it this looks like a bit of a DRY violation
// with respect to 'Update', but, the error handling is quite different and attempting
// to factor out the common pieces results in a net total of more code.
func (c *Cache) Initialize(channelID string, qe ledger.SimpleQueryExecutor) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	publicState := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}

	namespaces, err := c.Lifecycle.QueryNamespaceDefinitions(publicState)
	if err != nil {
		return errors.WithMessage(err, "could not query namespace definitions")
	}

	dirtyChaincodes := map[string]struct{}{}

	for namespace, namespaceType := range namespaces {
		if namespaceType != FriendlyChaincodeDefinitionType {
			continue
		}
		dirtyChaincodes[namespace] = struct{}{}
	}

	return c.update(channelID, dirtyChaincodes, qe)
}

// HandleStateUpdates is required to implement the ledger state listener interface.  It applies
// any state updates to the cache.
func (c *Cache) HandleStateUpdates(trigger *ledger.StateUpdateTrigger) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	channelID := trigger.LedgerID
	updates, ok := trigger.StateUpdates[LifecycleNamespace]
	if !ok {
		return errors.Errorf("no state updates for promised namespace _lifecycle")
	}

	dirtyChaincodes := map[string]struct{}{}

	for _, publicUpdate := range updates.PublicUpdates {
		matches := SequenceMatcher.FindStringSubmatch(publicUpdate.Key)
		if len(matches) != 2 {
			continue
		}

		dirtyChaincodes[matches[1]] = struct{}{}
	}

	channelCache, ok := c.definedChaincodes[channelID]

	// if the channel cache does not yet exist, there are no interesting hashes, so skip
	if ok {
		for collection, privateUpdates := range updates.CollHashUpdates {
			matches := ImplicitCollectionMatcher.FindStringSubmatch(collection)
			if len(matches) != 2 {
				// This is not an implicit collection
				continue
			}

			if matches[1] != c.MyOrgMSPID {
				// This is not our implicit collection
				continue
			}

			for _, privateUpdate := range privateUpdates {
				chaincodeName, ok := channelCache.InterestingHashes[string(privateUpdate.KeyHash)]
				if ok {
					dirtyChaincodes[chaincodeName] = struct{}{}
				}
			}
		}
	}

	err := c.update(channelID, dirtyChaincodes, trigger.PostCommitQueryExecutor)
	if err != nil {
		return errors.WithMessage(err, "error updating cache")
	}

	return nil
}

// InterestedInNamespaces is required to implement the ledger state listener interface
func (c *Cache) InterestedInNamespaces() []string {
	return []string{LifecycleNamespace}
}

// StateCommitDone is required to implement the ledger state listener interface
func (c *Cache) StateCommitDone(channelName string) {
	// NOTE: It's extremely tempting to acquire the write lock in HandleStateUpdate
	// and release it here, however, this is asking for a deadlock.  In particular,
	// because the 'write lock' on the state is only held for a short period
	// between HandleStateUpdate and StateCommitDone, it's possible (in fact likely)
	// that a chaincode invocation will acquire a read-lock on the world state, then attempt
	// to get chaincode info from the cache, resulting in a deadlock.  So, we choose
	// potential inconsistenty between the cache and the world state which the callers
	// must detect and cope with as necessary.  Note, the cache will always be _at least_
	// as current as the committed state.
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

// update should only be called with the write lock already held
func (c *Cache) update(channelID string, dirtyChaincodes map[string]struct{}, qe ledger.SimpleQueryExecutor) error {
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
