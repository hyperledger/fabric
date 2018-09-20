/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"fmt"

	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

// DBProvider provides handle to a PvtVersionedDB
type DBProvider interface {
	// GetDBHandle returns a handle to a PvtVersionedDB
	GetDBHandle(id string) (DB, error)
	// Close closes all the PvtVersionedDB instances and releases any resources held by VersionedDBProvider
	Close()
}

// DB extends VersionedDB interface. This interface provides additional functions for managing private data state
type DB interface {
	statedb.VersionedDB
	IsBulkOptimizable() bool
	LoadCommittedVersionsOfPubAndHashedKeys(pubKeys []*statedb.CompositeKey, hashedKeys []*HashedCompositeKey) error
	GetCachedKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, bool)
	ClearCachedVersions()
	GetChaincodeEventListener() cceventmgmt.ChaincodeLifecycleEventListener
	GetPrivateData(namespace, collection, key string) (*statedb.VersionedValue, error)
	GetValueHash(namespace, collection string, keyHash []byte) (*statedb.VersionedValue, error)
	GetKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, error)
	GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([]*statedb.VersionedValue, error)
	GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (statedb.ResultsIterator, error)
	GetStateMetadata(namespace, key string) ([]byte, error)
	GetPrivateDataMetadataByHash(namespace, collection string, keyHash []byte) ([]byte, error)
	ExecuteQueryOnPrivateData(namespace, collection, query string) (statedb.ResultsIterator, error)
	ApplyPrivacyAwareUpdates(updates *UpdateBatch, height *version.Height) error
}

// PvtdataCompositeKey encloses Namespace, CollectionName and Key components
type PvtdataCompositeKey struct {
	Namespace      string
	CollectionName string
	Key            string
}

// HashedCompositeKey encloses Namespace, CollectionName and KeyHash components
type HashedCompositeKey struct {
	Namespace      string
	CollectionName string
	KeyHash        string
}

// UpdateBatch encapsulates the updates to Public, Private, and Hashed data.
// This is expected to contain a consistent set of updates
type UpdateBatch struct {
	PubUpdates  *PubUpdateBatch
	HashUpdates *HashedUpdateBatch
	PvtUpdates  *PvtUpdateBatch
}

// PubUpdateBatch contains update for the public data
type PubUpdateBatch struct {
	*statedb.UpdateBatch
}

// HashedUpdateBatch contains updates for the hashes of the private data
type HashedUpdateBatch struct {
	UpdateMap
}

// PvtUpdateBatch contains updates for the private data
type PvtUpdateBatch struct {
	UpdateMap
}

// UpdateMap maintains entries of tuple <Namespace, UpdatesForNamespace>
type UpdateMap map[string]nsBatch

// nsBatch contains updates related to one namespace
type nsBatch struct {
	*statedb.UpdateBatch
}

// NewUpdateBatch creates and empty UpdateBatch
func NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{NewPubUpdateBatch(), NewHashedUpdateBatch(), NewPvtUpdateBatch()}
}

// NewPubUpdateBatch creates an empty PubUpdateBatch
func NewPubUpdateBatch() *PubUpdateBatch {
	return &PubUpdateBatch{statedb.NewUpdateBatch()}
}

// NewHashedUpdateBatch creates an empty HashedUpdateBatch
func NewHashedUpdateBatch() *HashedUpdateBatch {
	return &HashedUpdateBatch{make(map[string]nsBatch)}
}

// NewPvtUpdateBatch creates an empty PvtUpdateBatch
func NewPvtUpdateBatch() *PvtUpdateBatch {
	return &PvtUpdateBatch{make(map[string]nsBatch)}
}

// IsEmpty returns true if there exists any updates
func (b UpdateMap) IsEmpty() bool {
	return len(b) == 0
}

// Put sets the value in the batch for a given combination of namespace and collection name
func (b UpdateMap) Put(ns, coll, key string, value []byte, version *version.Height) {
	b.PutValAndMetadata(ns, coll, key, value, nil, version)
}

// PutValAndMetadata adds a key with value and metadata
func (b UpdateMap) PutValAndMetadata(ns, coll, key string, value []byte, metadata []byte, version *version.Height) {
	b.getOrCreateNsBatch(ns).PutValAndMetadata(coll, key, value, metadata, version)
}

// Delete adds a delete marker in the batch for a given combination of namespace and collection name
func (b UpdateMap) Delete(ns, coll, key string, version *version.Height) {
	b.getOrCreateNsBatch(ns).Delete(coll, key, version)
}

// Get retrieves the value from the batch for a given combination of namespace and collection name
func (b UpdateMap) Get(ns, coll, key string) *statedb.VersionedValue {
	nsPvtBatch, ok := b[ns]
	if !ok {
		return nil
	}
	return nsPvtBatch.Get(coll, key)
}

// Contains returns true if the given <ns,coll,key> tuple is present in the batch
func (b UpdateMap) Contains(ns, coll, key string) bool {
	nsBatch, ok := b[ns]
	if !ok {
		return false
	}
	return nsBatch.Exists(coll, key)
}

func (nsb nsBatch) GetCollectionNames() []string {
	return nsb.GetUpdatedNamespaces()
}

func (b UpdateMap) getOrCreateNsBatch(ns string) nsBatch {
	batch, ok := b[ns]
	if !ok {
		batch = nsBatch{statedb.NewUpdateBatch()}
		b[ns] = batch
	}
	return batch
}

// Contains returns true if the given <ns,coll,keyHash> tuple is present in the batch
func (h HashedUpdateBatch) Contains(ns, coll string, keyHash []byte) bool {
	return h.UpdateMap.Contains(ns, coll, string(keyHash))
}

// Put overrides the function in UpdateMap for allowing the key to be a []byte instead of a string
func (h HashedUpdateBatch) Put(ns, coll string, key []byte, value []byte, version *version.Height) {
	h.PutValHashAndMetadata(ns, coll, key, value, nil, version)
}

// PutValHashAndMetadata adds a key with value and metadata
// TODO introducing a new function to limit the refactoring. Later in a separate CR, the 'Put' function above should be removed
func (h HashedUpdateBatch) PutValHashAndMetadata(ns, coll string, key []byte, value []byte, metadata []byte, version *version.Height) {
	h.UpdateMap.PutValAndMetadata(ns, coll, string(key), value, metadata, version)
}

// Delete overrides the function in UpdateMap for allowing the key to be a []byte instead of a string
func (h HashedUpdateBatch) Delete(ns, coll string, key []byte, version *version.Height) {
	h.UpdateMap.Delete(ns, coll, string(key), version)
}

// ToCompositeKeyMap rearranges the update batch data in the form of a single map
func (h HashedUpdateBatch) ToCompositeKeyMap() map[HashedCompositeKey]*statedb.VersionedValue {
	m := make(map[HashedCompositeKey]*statedb.VersionedValue)
	for ns, nsBatch := range h.UpdateMap {
		for _, coll := range nsBatch.GetCollectionNames() {
			for key, vv := range nsBatch.GetUpdates(coll) {
				m[HashedCompositeKey{ns, coll, key}] = vv
			}
		}
	}
	return m
}

// ToCompositeKeyMap rearranges the update batch data in the form of a single map
func (p PvtUpdateBatch) ToCompositeKeyMap() map[PvtdataCompositeKey]*statedb.VersionedValue {
	m := make(map[PvtdataCompositeKey]*statedb.VersionedValue)
	for ns, nsBatch := range p.UpdateMap {
		for _, coll := range nsBatch.GetCollectionNames() {
			for key, vv := range nsBatch.GetUpdates(coll) {
				m[PvtdataCompositeKey{ns, coll, key}] = vv
			}
		}
	}
	return m
}

// String returns a print friendly form of HashedCompositeKey
func (hck *HashedCompositeKey) String() string {
	return fmt.Sprintf("ns=%s, collection=%s, keyHash=%x", hck.Namespace, hck.CollectionName, hck.KeyHash)
}
