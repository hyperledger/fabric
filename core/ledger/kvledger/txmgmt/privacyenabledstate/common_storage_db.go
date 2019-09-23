/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"encoding/base64"
	"strings"

	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("privacyenabledstate")

const (
	nsJoiner       = "$$"
	pvtDataPrefix  = "p"
	hashDataPrefix = "h"
)

// CommonStorageDBProvider implements interface DBProvider
type CommonStorageDBProvider struct {
	statedb.VersionedDBProvider
	HealthCheckRegistry ledger.HealthCheckRegistry
	bookkeepingProvider bookkeeping.Provider
}

// NewCommonStorageDBProvider constructs an instance of DBProvider
func NewCommonStorageDBProvider(bookkeeperProvider bookkeeping.Provider, metricsProvider metrics.Provider, healthCheckRegistry ledger.HealthCheckRegistry) (DBProvider, error) {
	var vdbProvider statedb.VersionedDBProvider
	var err error
	if ledgerconfig.IsCouchDBEnabled() {
		if vdbProvider, err = statecouchdb.NewVersionedDBProvider(metricsProvider); err != nil {
			return nil, err
		}
	} else {
		vdbProvider = stateleveldb.NewVersionedDBProvider()
	}

	dbProvider := &CommonStorageDBProvider{vdbProvider, healthCheckRegistry, bookkeeperProvider}

	err = dbProvider.RegisterHealthChecker()
	if err != nil {
		return nil, err
	}

	return dbProvider, nil
}

func (p *CommonStorageDBProvider) RegisterHealthChecker() error {
	if healthChecker, ok := p.VersionedDBProvider.(healthz.HealthChecker); ok {
		return p.HealthCheckRegistry.RegisterChecker("couchdb", healthChecker)
	}
	return nil
}

// GetDBHandle implements function from interface DBProvider
func (p *CommonStorageDBProvider) GetDBHandle(id string) (DB, error) {
	vdb, err := p.VersionedDBProvider.GetDBHandle(id)
	if err != nil {
		return nil, err
	}
	bookkeeper := p.bookkeepingProvider.GetDBHandle(id, bookkeeping.MetadataPresenceIndicator)
	metadataHint := newMetadataHint(bookkeeper)
	return NewCommonStorageDB(vdb, id, metadataHint)
}

// Close implements function from interface DBProvider
func (p *CommonStorageDBProvider) Close() {
	p.VersionedDBProvider.Close()
}

// CommonStorageDB implements interface DB. This implementation uses a single database to maintain
// both the public and private data
type CommonStorageDB struct {
	statedb.VersionedDB
	metadataHint *metadataHint
}

// NewCommonStorageDB wraps a VersionedDB instance. The public data is managed directly by the wrapped versionedDB.
// For managing the hashed data and private data, this implementation creates separate namespaces in the wrapped db
func NewCommonStorageDB(vdb statedb.VersionedDB, ledgerid string, metadataHint *metadataHint) (DB, error) {
	return &CommonStorageDB{vdb, metadataHint}, nil
}

// IsBulkOptimizable implements corresponding function in interface DB
func (s *CommonStorageDB) IsBulkOptimizable() bool {
	_, ok := s.VersionedDB.(statedb.BulkOptimizable)
	return ok
}

// LoadCommittedVersionsOfPubAndHashedKeys implements corresponding function in interface DB
func (s *CommonStorageDB) LoadCommittedVersionsOfPubAndHashedKeys(pubKeys []*statedb.CompositeKey,
	hashedKeys []*HashedCompositeKey) error {

	bulkOptimizable, ok := s.VersionedDB.(statedb.BulkOptimizable)
	if !ok {
		return nil
	}
	// Here, hashedKeys are merged into pubKeys to get a combined set of keys for combined loading
	for _, key := range hashedKeys {
		ns := deriveHashedDataNs(key.Namespace, key.CollectionName)
		// No need to check for duplicates as hashedKeys are in separate namespace
		var keyHashStr string
		if !s.BytesKeySupported() {
			keyHashStr = base64.StdEncoding.EncodeToString([]byte(key.KeyHash))
		} else {
			keyHashStr = key.KeyHash
		}
		pubKeys = append(pubKeys, &statedb.CompositeKey{
			Namespace: ns,
			Key:       keyHashStr,
		})
	}

	err := bulkOptimizable.LoadCommittedVersions(pubKeys)
	if err != nil {
		return err
	}

	return nil
}

// ClearCachedVersions implements corresponding function in interface DB
func (s *CommonStorageDB) ClearCachedVersions() {
	bulkOptimizable, ok := s.VersionedDB.(statedb.BulkOptimizable)
	if ok {
		bulkOptimizable.ClearCachedVersions()
	}
}

// GetChaincodeEventListener implements corresponding function in interface DB
func (s *CommonStorageDB) GetChaincodeEventListener() cceventmgmt.ChaincodeLifecycleEventListener {
	_, ok := s.VersionedDB.(statedb.IndexCapable)
	if ok {
		return s
	}
	return nil
}

// GetPrivateData implements corresponding function in interface DB
func (s *CommonStorageDB) GetPrivateData(namespace, collection, key string) (*statedb.VersionedValue, error) {
	return s.GetState(derivePvtDataNs(namespace, collection), key)
}

// GetValueHash implements corresponding function in interface DB
func (s *CommonStorageDB) GetValueHash(namespace, collection string, keyHash []byte) (*statedb.VersionedValue, error) {
	keyHashStr := string(keyHash)
	if !s.BytesKeySupported() {
		keyHashStr = base64.StdEncoding.EncodeToString(keyHash)
	}
	return s.GetState(deriveHashedDataNs(namespace, collection), keyHashStr)
}

// GetKeyHashVersion implements corresponding function in interface DB
func (s *CommonStorageDB) GetKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, error) {
	keyHashStr := string(keyHash)
	if !s.BytesKeySupported() {
		keyHashStr = base64.StdEncoding.EncodeToString(keyHash)
	}
	return s.GetVersion(deriveHashedDataNs(namespace, collection), keyHashStr)
}

// GetCachedKeyHashVersion retrieves the keyhash version from cache
func (s *CommonStorageDB) GetCachedKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, bool) {
	bulkOptimizable, ok := s.VersionedDB.(statedb.BulkOptimizable)
	if !ok {
		return nil, false
	}

	keyHashStr := string(keyHash)
	if !s.BytesKeySupported() {
		keyHashStr = base64.StdEncoding.EncodeToString(keyHash)
	}
	return bulkOptimizable.GetCachedVersion(deriveHashedDataNs(namespace, collection), keyHashStr)
}

// GetPrivateDataMultipleKeys implements corresponding function in interface DB
func (s *CommonStorageDB) GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([]*statedb.VersionedValue, error) {
	return s.GetStateMultipleKeys(derivePvtDataNs(namespace, collection), keys)
}

// GetPrivateDataRangeScanIterator implements corresponding function in interface DB
func (s *CommonStorageDB) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (statedb.ResultsIterator, error) {
	return s.GetStateRangeScanIterator(derivePvtDataNs(namespace, collection), startKey, endKey)
}

// ExecuteQueryOnPrivateData implements corresponding function in interface DB
func (s CommonStorageDB) ExecuteQueryOnPrivateData(namespace, collection, query string) (statedb.ResultsIterator, error) {
	return s.ExecuteQuery(derivePvtDataNs(namespace, collection), query)
}

// ApplyUpdates overrides the function in statedb.VersionedDB and throws appropriate error message
// Otherwise, somewhere in the code, usage of this function could lead to updating only public data.
func (s *CommonStorageDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {
	return errors.New("this function should not be invoked on this type. Please invoke function ApplyPrivacyAwareUpdates")
}

// ApplyPrivacyAwareUpdates implements corresponding function in interface DB
func (s *CommonStorageDB) ApplyPrivacyAwareUpdates(updates *UpdateBatch, height *version.Height) error {
	// combinedUpdates includes both updates to public db and private db, which are partitioned by a separate namespace
	combinedUpdates := updates.PubUpdates
	addPvtUpdates(combinedUpdates, updates.PvtUpdates)
	addHashedUpdates(combinedUpdates, updates.HashUpdates, !s.BytesKeySupported())
	s.metadataHint.setMetadataUsedFlag(updates)
	return s.VersionedDB.ApplyUpdates(combinedUpdates.UpdateBatch, height)
}

// GetStateMetadata implements corresponding function in interface DB. This implementation provides
// an optimization such that it keeps track if a namespaces has never stored metadata for any of
// its items, the value 'nil' is returned without going to the db. This is intended to be invoked
// in the validation and commit path. This saves the chaincodes from paying unnecessary performance
// penalty if they do not use features that leverage metadata (such as key-level endorsement),
func (s *CommonStorageDB) GetStateMetadata(namespace, key string) ([]byte, error) {
	if !s.metadataHint.metadataEverUsedFor(namespace) {
		return nil, nil
	}
	vv, err := s.GetState(namespace, key)
	if err != nil || vv == nil {
		return nil, err
	}
	return vv.Metadata, nil
}

// GetPrivateDataMetadataByHash implements corresponding function in interface DB. For additional details, see
// description of the similar function 'GetStateMetadata'
func (s *CommonStorageDB) GetPrivateDataMetadataByHash(namespace, collection string, keyHash []byte) ([]byte, error) {
	if !s.metadataHint.metadataEverUsedFor(namespace) {
		return nil, nil
	}
	vv, err := s.GetValueHash(namespace, collection, keyHash)
	if err != nil || vv == nil {
		return nil, err
	}
	return vv.Metadata, nil
}

// HandleChaincodeDeploy initializes database artifacts for the database associated with the namespace
// This function deliberately suppresses the errors that occur during the creation of the indexes on couchdb.
// This is because, in the present code, we do not differentiate between the errors because of couchdb interaction
// and the errors because of bad index files - the later being unfixable by the admin. Note that the error suppression
// is acceptable since peer can continue in the committing role without the indexes. However, executing chaincode queries
// may be affected, until a new chaincode with fixed indexes is installed and instantiated
func (s *CommonStorageDB) HandleChaincodeDeploy(chaincodeDefinition *cceventmgmt.ChaincodeDefinition, dbArtifactsTar []byte) error {
	//Check to see if the interface for IndexCapable is implemented
	indexCapable, ok := s.VersionedDB.(statedb.IndexCapable)
	if !ok {
		return nil
	}
	if chaincodeDefinition == nil {
		return errors.New("chaincode definition not found while creating couchdb index")
	}
	dbArtifacts, err := ccprovider.ExtractFileEntries(dbArtifactsTar, indexCapable.GetDBType())
	if err != nil {
		logger.Errorf("Index creation: error extracting db artifacts from tar for chaincode [%s]: %s", chaincodeDefinition.Name, err)
		return nil
	}

	collectionConfigMap, err := extractCollectionNames(chaincodeDefinition)
	if err != nil {
		logger.Errorf("Error while retrieving collection config for chaincode=[%s]: %s",
			chaincodeDefinition.Name, err)
		return nil
	}

	for directoryPath, archiveDirectoryEntries := range dbArtifacts {
		// split the directory name
		directoryPathArray := strings.Split(directoryPath, "/")
		// process the indexes for the chain
		if directoryPathArray[3] == "indexes" {
			err := indexCapable.ProcessIndexesForChaincodeDeploy(chaincodeDefinition.Name, archiveDirectoryEntries)
			if err != nil {
				logger.Errorf("Error processing index for chaincode [%s]: %s", chaincodeDefinition.Name, err)
			}
			continue
		}
		// check for the indexes directory for the collection
		if directoryPathArray[3] == "collections" && directoryPathArray[5] == "indexes" {
			collectionName := directoryPathArray[4]
			_, ok := collectionConfigMap[collectionName]
			if !ok {
				logger.Errorf("Error processing index for chaincode [%s]: cannot create an index for an undefined collection=[%s]", chaincodeDefinition.Name, collectionName)
			} else {
				err := indexCapable.ProcessIndexesForChaincodeDeploy(derivePvtDataNs(chaincodeDefinition.Name, collectionName),
					archiveDirectoryEntries)
				if err != nil {
					logger.Errorf("Error processing collection index for chaincode [%s]: %s", chaincodeDefinition.Name, err)
				}
			}
		}
	}
	return nil
}

// ChaincodeDeployDone is a noop for couchdb state impl
func (s *CommonStorageDB) ChaincodeDeployDone(succeeded bool) {
	// NOOP
}

func derivePvtDataNs(namespace, collection string) string {
	return namespace + nsJoiner + pvtDataPrefix + collection
}

func deriveHashedDataNs(namespace, collection string) string {
	return namespace + nsJoiner + hashDataPrefix + collection
}

func addPvtUpdates(pubUpdateBatch *PubUpdateBatch, pvtUpdateBatch *PvtUpdateBatch) {
	for ns, nsBatch := range pvtUpdateBatch.UpdateMap {
		for _, coll := range nsBatch.GetCollectionNames() {
			for key, vv := range nsBatch.GetUpdates(coll) {
				pubUpdateBatch.Update(derivePvtDataNs(ns, coll), key, vv)
			}
		}
	}
}

func addHashedUpdates(pubUpdateBatch *PubUpdateBatch, hashedUpdateBatch *HashedUpdateBatch, base64Key bool) {
	for ns, nsBatch := range hashedUpdateBatch.UpdateMap {
		for _, coll := range nsBatch.GetCollectionNames() {
			for key, vv := range nsBatch.GetUpdates(coll) {
				if base64Key {
					key = base64.StdEncoding.EncodeToString([]byte(key))
				}
				pubUpdateBatch.Update(deriveHashedDataNs(ns, coll), key, vv)
			}
		}
	}
}

func extractCollectionNames(chaincodeDefinition *cceventmgmt.ChaincodeDefinition) (map[string]bool, error) {
	collectionConfigs := chaincodeDefinition.CollectionConfigs
	collectionConfigsMap := make(map[string]bool)
	if collectionConfigs != nil {
		for _, config := range collectionConfigs.Config {
			sConfig := config.GetStaticCollectionConfig()
			if sConfig == nil {
				continue
			}
			collectionConfigsMap[sConfig.Name] = true
		}
	}
	return collectionConfigsMap, nil
}
