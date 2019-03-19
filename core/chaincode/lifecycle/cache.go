/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/ledger"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"

	p "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/pkg/errors"
)

type LocalChaincodeInfo struct {
	Definition  *ChaincodeDefinition
	Approved    bool
	InstallInfo *ChaincodeInstallInfo
}

type ChaincodeInstallInfo struct {
	PackageID p.PackageID
	Type      string
	Path      string
	Label     string
}

type CachedChaincodeDefinition struct {
	Definition  *ChaincodeDefinition
	Approved    bool
	InstallInfo *ChaincodeInstallInfo

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
	Resources         *Resources
	MyOrgMSPID        string

	// mutex serializes lifecycle operations globally for the peer.  It will cause a lifecycle update
	// in one channel to potentially affect the throughput of another.  However, relative to standard
	// transactions, lifecycle updates should be quite rare, and this is a RW lock so in general, there
	// should not be contention in the normal case.  Because chaincode package installation is a peer global
	// event, by synchronizing at a peer global level, we drastically simplify accounting for which
	// chaincodes are installed and which channels that installed chaincode is currently in use on.
	mutex sync.RWMutex

	// localChaincodes is a map from the hash of the locally installed chaincode's proto
	// encoded hash (yes, the hash of the hash), to a set of channels, to a set of chaincode
	// definitions which reference this local installed chaincode hash.
	localChaincodes map[string]*LocalChaincode
}

type LocalChaincode struct {
	Info       *ChaincodeInstallInfo
	References map[string]map[string]*CachedChaincodeDefinition
}

func NewCache(resources *Resources, myOrgMSPID string) *Cache {
	return &Cache{
		definedChaincodes: map[string]*ChannelCache{},
		localChaincodes:   map[string]*LocalChaincode{},
		Resources:         resources,
		MyOrgMSPID:        myOrgMSPID,
	}
}

// InitializeLocalChaincodes should be called once after cache creation (timing doesn't matter,
// though already installed chaincodes will not be invokable until it it completes).  Ideally,
// this would be part of the constructor, but, we cannot rely on the chaincode store being created
// before the cache is created.
func (c *Cache) InitializeLocalChaincodes() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ccPackages, err := c.Resources.ChaincodeStore.ListInstalledChaincodes()
	if err != nil {
		return errors.WithMessage(err, "could not list installed chaincodes")
	}

	for _, ccPackage := range ccPackages {
		ccPackageBytes, err := c.Resources.ChaincodeStore.Load(ccPackage.PackageID)
		if err != nil {
			return errors.WithMessagef(err, "could not load chaincode with pakcage ID '%s'", ccPackage.PackageID.String())
		}
		parsedCCPackage, err := c.Resources.PackageParser.Parse(ccPackageBytes)
		if err != nil {
			return errors.WithMessagef(err, "could not parse chaincode with pakcage ID '%s'", ccPackage.PackageID.String())
		}
		c.handleChaincodeInstalledWhileLocked(parsedCCPackage.Metadata, ccPackage.PackageID)
	}

	logger.Infof("Initialized lifecycle cache with %d already installed chaincodes", len(c.localChaincodes))
	for channelID, chaincodeCache := range c.definedChaincodes {
		approved, installed, runnable := 0, 0, 0
		for _, cachedChaincode := range chaincodeCache.Chaincodes {
			if cachedChaincode.Approved {
				approved++
			}
			if cachedChaincode.InstallInfo != nil {
				installed++
			}
			if cachedChaincode.Approved && cachedChaincode.InstallInfo != nil {
				runnable++
			}
		}

		logger.Infof("Initialized lifecycle cache for channel '%s' with %d chaincodes runnable (%d approved, %d installed)", channelID, runnable, approved, installed)
	}

	return nil
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

	metadatas, err := c.Resources.Serializer.DeserializeAllMetadata(NamespacesName, publicState)
	if err != nil {
		return errors.WithMessage(err, "could not query namespace metadata")
	}

	dirtyChaincodes := map[string]struct{}{}

	for namespace, metadata := range metadatas {
		switch metadata.Datatype {
		case ChaincodeDefinitionType:
			dirtyChaincodes[namespace] = struct{}{}
		default:
			// non-chaincode
		}
	}

	return c.update(channelID, dirtyChaincodes, qe)
}

// HandleChaincodeInstalled should be invoked whenever a new chaincode is installed
func (c *Cache) HandleChaincodeInstalled(md *persistence.ChaincodePackageMetadata, packageID p.PackageID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.handleChaincodeInstalledWhileLocked(md, packageID)
}

func (c *Cache) handleChaincodeInstalledWhileLocked(md *persistence.ChaincodePackageMetadata, packageID p.PackageID) {
	// it would be nice to get this value from the serialization package, but it was not obvious
	// how to expose this in a nice way, so we manually compute it.
	encodedCCHash := protoutil.MarshalOrPanic(&lb.StateData{
		Type: &lb.StateData_String_{String_: packageID.String()},
	})
	hashOfCCHash := string(util.ComputeSHA256(encodedCCHash))
	localChaincode, ok := c.localChaincodes[hashOfCCHash]
	if !ok {
		localChaincode = &LocalChaincode{
			References: map[string]map[string]*CachedChaincodeDefinition{},
		}
		c.localChaincodes[hashOfCCHash] = localChaincode
	}
	localChaincode.Info = &ChaincodeInstallInfo{
		PackageID: packageID,
		Type:      md.Type,
		Path:      md.Path,
		Label:     md.Label,
	}
	for channelID, channelCache := range localChaincode.References {
		for chaincodeName, cachedChaincode := range channelCache {
			cachedChaincode.InstallInfo = localChaincode.Info
			logger.Infof("Installed chaincode with package ID '%s' now available on channel %s for chaincode definition %s:%s", packageID, channelID, chaincodeName, cachedChaincode.Definition.EndorsementInfo.Version)
		}
	}
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

// ChaincodeInfo returns the chaincode definition and its install info.
func (c *Cache) ChaincodeInfo(channelID, name string) (*LocalChaincodeInfo, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	channelChaincodes, ok := c.definedChaincodes[channelID]
	if !ok {
		return nil, errors.Errorf("unknown channel '%s'", channelID)
	}

	cachedChaincode, ok := channelChaincodes.Chaincodes[name]
	if !ok {
		return nil, errors.Errorf("unknown chaincode '%s' for channel '%s'", name, channelID)
	}

	return &LocalChaincodeInfo{
		Definition:  cachedChaincode.Definition,
		InstallInfo: cachedChaincode.InstallInfo,
		Approved:    cachedChaincode.Approved,
	}, nil
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
		logger.Infof("Updating cached definition for chaincode '%s' on channel '%s'", name, channelID)
		cachedChaincode, ok := channelCache.Chaincodes[name]
		if !ok {
			cachedChaincode = &CachedChaincodeDefinition{}
			channelCache.Chaincodes[name] = cachedChaincode
		}

		for _, hash := range cachedChaincode.Hashes {
			delete(channelCache.InterestingHashes, hash)
		}

		exists, chaincodeDefinition, err := c.Resources.ChaincodeDefinitionIfDefined(name, publicState)
		if err != nil {
			return errors.WithMessagef(err, "could not get chaincode definition for '%s' on channel '%s'", name, channelID)
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

		ok, err = c.Resources.Serializer.IsSerialized(NamespacesName, privateName, chaincodeDefinition.Parameters(), orgState)

		if err != nil {
			return errors.WithMessagef(err, "could not check opaque org state for '%s' on channel '%s'", name, channelID)
		}
		if !ok {
			logger.Debugf("Channel %s for chaincode definition %s:%s does not have our org's approval", channelID, name, chaincodeDefinition.EndorsementInfo.Version)
			continue
		}

		cachedChaincode.Approved = true

		isLocalPackage, err := c.Resources.Serializer.IsMetadataSerialized(ChaincodeSourcesName, privateName, &ChaincodeLocalPackage{}, orgState)
		if err != nil {
			return errors.WithMessagef(err, "could not check opaque org state for chaincode source for '%s' on channel '%s'", name, channelID)
		}

		if !isLocalPackage {
			logger.Debugf("Channel %s for chaincode definition %s:%s does not have a chaincode source defined", channelID, name, chaincodeDefinition.EndorsementInfo.Version)
			continue
		}

		hashKey := FieldKey(ChaincodeSourcesName, privateName, "PackageID")
		hashOfCCHash, err := orgState.GetStateHash(hashKey)
		if err != nil {
			return errors.WithMessagef(err, "could not check opaque org state for chaincode source hash for '%s' on channel '%s'", name, channelID)
		}

		localChaincode, ok := c.localChaincodes[string(hashOfCCHash)]
		if !ok {
			localChaincode = &LocalChaincode{
				References: map[string]map[string]*CachedChaincodeDefinition{},
			}
			c.localChaincodes[string(hashOfCCHash)] = localChaincode
		}

		cachedChaincode.InstallInfo = localChaincode.Info
		if localChaincode.Info != nil {
			logger.Infof("Chaincode with package ID '%s' now available on channel %s for chaincode definition %s:%s", localChaincode.Info.PackageID, channelID, name, cachedChaincode.Definition.EndorsementInfo.Version)
		} else {
			logger.Debugf("Chaincode definition for chaincode '%s' on channel '%s' is approved, but not installed", name, channelID)
		}

		channelReferences, ok := localChaincode.References[channelID]
		if !ok {
			channelReferences = map[string]*CachedChaincodeDefinition{}
			localChaincode.References[channelID] = channelReferences
		}

		channelReferences[name] = cachedChaincode
	}

	return nil
}
