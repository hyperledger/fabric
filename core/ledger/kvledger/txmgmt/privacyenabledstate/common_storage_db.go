/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"encoding/base64"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

const (
	nsJoiner       = "/"
	pvtDataPrefix  = "p"
	hashDataPrefix = "h"
)

// CommonStorageDBProvider implements interface DBProvider
type CommonStorageDBProvider struct {
	statedb.VersionedDBProvider
}

// NewCommonStorageDBProvider constructs an instance of DBProvider
func NewCommonStorageDBProvider() (DBProvider, error) {
	var vdbProvider statedb.VersionedDBProvider
	var err error
	if ledgerconfig.IsCouchDBEnabled() {
		if vdbProvider, err = statecouchdb.NewVersionedDBProvider(); err != nil {
			return nil, err
		}
	} else {
		vdbProvider = stateleveldb.NewVersionedDBProvider()
	}
	return &CommonStorageDBProvider{vdbProvider}, nil
}

// GetDBHandle implements function from interface DBProvider
func (p *CommonStorageDBProvider) GetDBHandle(id string) (DB, error) {
	vdb, err := p.VersionedDBProvider.GetDBHandle(id)
	if err != nil {
		return nil, err
	}
	return NewCommonStorageDB(vdb, id)
}

// Close implements function from interface DBProvider
func (p *CommonStorageDBProvider) Close() {
	p.VersionedDBProvider.Close()
}

// CommonStorageDB implements interface DB. This implementation uses a single database to maintain
// both the public and private data
type CommonStorageDB struct {
	statedb.VersionedDB
}

// NewCommonStorageDB wraps a VersionedDB instance. The public data is managed directly by the wrapped versionedDB.
// For managing the hashed data and private data, this implementation creates separate namespaces in the wrapped db
func NewCommonStorageDB(vdb statedb.VersionedDB, ledgerid string) (DB, error) {
	return &CommonStorageDB{VersionedDB: vdb}, nil
}

// IsBulkOptimizable implements corresponding function in interface DB
func (s *CommonStorageDB) IsBulkOptimizable() bool {
	if _, ok := s.VersionedDB.(statedb.BulkOptimizable); ok {
		return true
	}
	return false
}

// LoadCommittedVersionsOfPubAndHashedKeys implements corresponding function in interface DB
func (s *CommonStorageDB) LoadCommittedVersionsOfPubAndHashedKeys(pubKeys []*statedb.CompositeKey,
	hashedKeys []*HashedCompositeKey) {

	bulkOptimizable, ok := s.VersionedDB.(statedb.BulkOptimizable)
	if !ok {
		return
	}

	// Here, hashedKeys are merged into pubKeys to get a combined set of keys for combined loading
	for _, key := range hashedKeys {
		ns := derivePvtDataNs(key.Namespace, key.CollectionName)
		// No need to check for duplicates as hashedKeys are in separate namespace
		var keyHashStr string
		if !s.BytesKeySuppoted() {
			keyHashStr = base64.StdEncoding.EncodeToString([]byte(key.KeyHash))
		} else {
			keyHashStr = key.KeyHash
		}
		pubKeys = append(pubKeys, &statedb.CompositeKey{
			Namespace: ns,
			Key:       keyHashStr,
		})
	}

	bulkOptimizable.LoadCommittedVersions(pubKeys)
}

// ClearCommittedVersions implements corresponding function in interface DB
func (s *CommonStorageDB) ClearCommittedVersions() {
	bulkOptimizable, ok := s.VersionedDB.(statedb.BulkOptimizable)
	if ok {
		bulkOptimizable.ClearCachedVersions()
	}
}

// GetPrivateData implements corresponding function in interface DB
func (s *CommonStorageDB) GetPrivateData(namespace, collection, key string) (*statedb.VersionedValue, error) {
	return s.GetState(derivePvtDataNs(namespace, collection), key)
}

// GetValueHash implements corresponding function in interface DB
func (s *CommonStorageDB) GetValueHash(namespace, collection string, keyHash []byte) (*statedb.VersionedValue, error) {
	keyHashStr := string(keyHash)
	if !s.BytesKeySuppoted() {
		keyHashStr = base64.StdEncoding.EncodeToString(keyHash)
	}
	return s.GetState(deriveHashedDataNs(namespace, collection), keyHashStr)
}

// GetHashedDataNsAndKeyHashStr implements corresponding function in interface DB
func (s *CommonStorageDB) GetKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, error) {
	keyHashStr := string(keyHash)
	if !s.BytesKeySuppoted() {
		keyHashStr = base64.StdEncoding.EncodeToString(keyHash)
	}
	return s.GetVersion(deriveHashedDataNs(namespace, collection), keyHashStr)
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

// ApplyUpdates overrides the funciton in statedb.VersionedDB and throws appropriate error message
// Otherwise, somewhere in the code, usage of this function could lead to updating only public data.
func (s *CommonStorageDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {
	return fmt.Errorf("This function should not be invoked on this type. Please invoke function 'ApplyPrivacyAwareUpdates'")
}

// ApplyPrivacyAwareUpdates implements corresponding function in interface DB
func (s *CommonStorageDB) ApplyPrivacyAwareUpdates(updates *UpdateBatch, height *version.Height) error {
	addPvtUpdates(updates.PubUpdates, updates.PvtUpdates)
	addHashedUpdates(updates.PubUpdates, updates.HashUpdates, !s.BytesKeySuppoted())
	return s.VersionedDB.ApplyUpdates(updates.PubUpdates.UpdateBatch, height)
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
